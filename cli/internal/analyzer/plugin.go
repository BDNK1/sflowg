package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// AnalyzePlugin analyzes a plugin package via AST to extract metadata
// including dependencies, config structure, and task methods
func AnalyzePlugin(importPath, pluginName, sourcePath string) (*PluginMetadata, error) {
	// Parse the plugin package
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, sourcePath, nil, parser.ParseComments)
	if err != nil {
		return nil, &AnalysisError{
			PluginName: pluginName,
			ImportPath: importPath,
			Message:    "failed to parse plugin package",
			Cause:      err,
		}
	}

	// Find the package (should be only one non-test package)
	var pkg *ast.Package
	for name, p := range pkgs {
		if !strings.HasSuffix(name, "_test") {
			pkg = p
			break
		}
	}

	if pkg == nil {
		return nil, &AnalysisError{
			PluginName: pluginName,
			ImportPath: importPath,
			Message:    "no non-test package found",
		}
	}

	metadata := &PluginMetadata{
		Name:         pluginName,
		ImportPath:   importPath,
		PackageName:  pkg.Name,
		Dependencies: []Dependency{},
		Tasks:        []TaskMetadata{},
	}

	// Analyze all files in the package
	for _, file := range pkg.Files {
		analyzeFile(file, metadata)
	}

	// Validate that we found a plugin struct
	if metadata.TypeName == "" {
		return nil, &AnalysisError{
			PluginName: pluginName,
			ImportPath: importPath,
			Message:    "no plugin struct found (looking for exported struct with 'Plugin' suffix)",
		}
	}

	return metadata, nil
}

// analyzeFile analyzes a single Go source file
func analyzeFile(file *ast.File, metadata *PluginMetadata) {
	// First pass: Find Config struct type definition
	var configStructType *ast.StructType
	ast.Inspect(file, func(n ast.Node) bool {
		if typeSpec, ok := n.(*ast.TypeSpec); ok {
			if typeSpec.Name.Name == "Config" {
				if structType, ok := typeSpec.Type.(*ast.StructType); ok {
					configStructType = structType
				}
			}
		}
		return true
	})

	// Second pass: Analyze plugin struct and methods
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.TypeSpec:
			// Look for plugin struct type
			if structType, ok := node.Type.(*ast.StructType); ok {
				if isPluginStruct(node.Name.Name) {
					metadata.TypeName = node.Name.Name
					analyzeDependencies(structType, metadata)
					checkConfigField(structType, metadata, configStructType)
				}
			}

		case *ast.FuncDecl:
			// Look for task methods on plugin type
			if node.Recv != nil && len(node.Recv.List) > 0 {
				analyzeTaskMethod(node, metadata)
			}
		}

		return true
	})
}

// isPluginStruct checks if a type name looks like a plugin struct
// Convention: Exported struct ending with "Plugin"
func isPluginStruct(typeName string) bool {
	return len(typeName) > 0 &&
		typeName[0] >= 'A' && typeName[0] <= 'Z' && // Exported
		strings.HasSuffix(typeName, "Plugin")
}

// analyzeDependencies extracts dependency fields from plugin struct
func analyzeDependencies(structType *ast.StructType, metadata *PluginMetadata) {
	for _, field := range structType.Fields.List {
		// Skip fields without names (embedded fields)
		if len(field.Names) == 0 {
			continue
		}

		fieldName := field.Names[0].Name

		// Skip Config field (handled separately)
		if fieldName == "config" || fieldName == "Config" {
			continue
		}

		// Check if field is a pointer type (dependencies must be pointers)
		starExpr, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}

		// Check if it's a selector (e.g., http.HTTPPlugin)
		var pluginType string
		var pluginPkg string

		switch typeExpr := starExpr.X.(type) {
		case *ast.SelectorExpr:
			// Qualified type: package.Type
			if ident, ok := typeExpr.X.(*ast.Ident); ok {
				pluginPkg = ident.Name
				pluginType = typeExpr.Sel.Name
			}
		case *ast.Ident:
			// Unqualified type: Type (same package)
			pluginType = typeExpr.Name
			pluginPkg = metadata.PackageName
		default:
			continue
		}

		// Check if type name suggests it's a plugin (ends with "Plugin")
		if !strings.HasSuffix(pluginType, "Plugin") {
			continue
		}

		// Extract inject tag if present
		injectTag := extractInjectTag(field.Tag)

		// Determine plugin name from field name or inject tag
		pluginName := injectTag
		if pluginName == "" {
			// Convert field name to plugin name (lowercase)
			pluginName = strings.ToLower(fieldName)
		}

		dep := Dependency{
			FieldName:  fieldName,
			PluginType: fmt.Sprintf("*%s.%s", pluginPkg, pluginType),
			PluginName: pluginName,
			InjectTag:  injectTag,
			IsExported: isExported(fieldName),
		}

		metadata.Dependencies = append(metadata.Dependencies, dep)
	}
}

// checkConfigField checks if plugin has a Config field and analyzes it
func checkConfigField(structType *ast.StructType, metadata *PluginMetadata, configStructType *ast.StructType) {
	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			continue
		}

		fieldName := field.Names[0].Name

		// Look for "Config" field (both uppercase and lowercase)
		if fieldName == "Config" || fieldName == "config" {
			// Check if type is "Config"
			if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "Config" {
				metadata.HasConfig = true

				// If we found the Config struct type definition, analyze it
				if configStructType != nil {
					metadata.ConfigType = analyzeConfigType(configStructType)
				} else {
					// Config field exists but we couldn't find the type definition
					metadata.ConfigType = &ConfigMetadata{
						TypeName: "Config",
						Fields:   []ConfigField{},
					}
				}
			}
		}
	}
}

// analyzeTaskMethod checks if a method is a valid task method
func analyzeTaskMethod(funcDecl *ast.FuncDecl, metadata *PluginMetadata) {
	// Only analyze methods on the plugin type
	if !isMethodOnPluginType(funcDecl, metadata.TypeName) {
		return
	}

	methodName := funcDecl.Name.Name

	// Skip unexported methods
	if !isExported(methodName) {
		return
	}

	// Check task signature:
	// func (p *Plugin) Method(exec *plugin.Execution, args map[string]any) (map[string]any, error)
	hasValidSig := hasValidTaskSignature(funcDecl)

	taskName := fmt.Sprintf("%s.%s", strings.ToLower(metadata.PackageName), toLowerFirst(methodName))

	task := TaskMetadata{
		MethodName:        methodName,
		TaskName:          taskName,
		IsExported:        true,
		HasValidSignature: hasValidSig,
	}

	metadata.Tasks = append(metadata.Tasks, task)
}

// isMethodOnPluginType checks if method is on the plugin struct
func isMethodOnPluginType(funcDecl *ast.FuncDecl, pluginTypeName string) bool {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
		return false
	}

	recv := funcDecl.Recv.List[0]

	// Check for *PluginType receiver
	if starExpr, ok := recv.Type.(*ast.StarExpr); ok {
		if ident, ok := starExpr.X.(*ast.Ident); ok {
			return ident.Name == pluginTypeName
		}
	}

	return false
}

// hasValidTaskSignature checks if method matches task signature
func hasValidTaskSignature(funcDecl *ast.FuncDecl) bool {
	// Should have 2 parameters: exec *plugin.Execution, args map[string]any
	if funcDecl.Type.Params == nil || len(funcDecl.Type.Params.List) != 2 {
		return false
	}

	// Should have 2 return values: map[string]any, error
	if funcDecl.Type.Results == nil || len(funcDecl.Type.Results.List) != 2 {
		return false
	}

	// TODO: Could add more detailed type checking here
	return true
}

// extractInjectTag extracts the value from inject:"name" tag
func extractInjectTag(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}

	// Tag value includes backticks, e.g., `inject:"redis"`
	tagValue := strings.Trim(tag.Value, "`")

	// Parse inject tag
	prefix := `inject:"`
	if idx := strings.Index(tagValue, prefix); idx >= 0 {
		start := idx + len(prefix)
		if end := strings.Index(tagValue[start:], `"`); end >= 0 {
			return tagValue[start : start+end]
		}
	}

	return ""
}

// isExported checks if identifier is exported
func isExported(name string) bool {
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

// toLowerFirst converts first character to lowercase
func toLowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}
