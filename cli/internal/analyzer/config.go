package analyzer

import (
	"fmt"
	"go/ast"
	"reflect"
	"strings"
)

// analyzeConfigType analyzes a Config struct type and extracts field metadata
func analyzeConfigType(structType *ast.StructType) *ConfigMetadata {
	if structType == nil || structType.Fields == nil {
		return nil
	}

	metadata := &ConfigMetadata{
		TypeName: "Config",
		Fields:   []ConfigField{},
	}

	// Iterate through struct fields
	for _, field := range structType.Fields.List {
		// Skip fields without names (embedded fields)
		if len(field.Names) == 0 {
			continue
		}

		for _, fieldName := range field.Names {
			configField := ConfigField{
				Name: fieldName.Name,
				Type: typeToString(field.Type),
			}

			// Extract struct tags
			if field.Tag != nil {
				tagValue := field.Tag.Value
				// Remove surrounding backticks
				if len(tagValue) >= 2 && tagValue[0] == '`' && tagValue[len(tagValue)-1] == '`' {
					tagValue = tagValue[1 : len(tagValue)-1]
				}

				configField.YAMLTag = extractTag(tagValue, "yaml")
				configField.DefaultTag = extractTag(tagValue, "default")
				configField.ValidateTag = extractTag(tagValue, "validate")
			}

			metadata.Fields = append(metadata.Fields, configField)
		}
	}

	return metadata
}

// typeToString converts an ast.Expr type to a string representation
func typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		// Simple type: string, int, bool, etc.
		return t.Name

	case *ast.StarExpr:
		// Pointer type: *SomeType
		return "*" + typeToString(t.X)

	case *ast.ArrayType:
		// Array/slice type: []string, [5]int
		if t.Len == nil {
			return "[]" + typeToString(t.Elt)
		}
		return "[...]" + typeToString(t.Elt)

	case *ast.MapType:
		// Map type: map[string]int
		return "map[" + typeToString(t.Key) + "]" + typeToString(t.Value)

	case *ast.SelectorExpr:
		// Qualified type: time.Duration, url.URL
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name + "." + t.Sel.Name
		}
		return typeToString(t.X) + "." + t.Sel.Name

	case *ast.StructType:
		// Inline struct
		return "struct{...}"

	case *ast.InterfaceType:
		// Interface type
		return "interface{}"

	default:
		// Fallback for unknown types
		return fmt.Sprintf("%T", expr)
	}
}

// extractTag extracts a specific tag value from a struct tag string
// Example: extractTag(`yaml:"addr" default:"localhost"`, "yaml") returns "addr"
func extractTag(tagString, tagName string) string {
	// Use reflect.StructTag for robust tag parsing
	tag := reflect.StructTag(tagString)
	value := tag.Get(tagName)

	// Handle comma-separated options in tags (e.g., yaml:"field,omitempty")
	// We only want the field name part
	if tagName == "yaml" || tagName == "json" {
		// Split on comma and take first part
		parts := strings.Split(value, ",")
		if len(parts) > 0 {
			return parts[0]
		}
	}

	return value
}

// isConfigField checks if a struct field is the config field
func isConfigField(field *ast.Field) bool {
	if len(field.Names) == 0 {
		return false
	}

	fieldName := field.Names[0].Name

	// Check if field name is "config" or "Config"
	if fieldName == "config" || fieldName == "Config" {
		// Verify it's of type Config (not pointer)
		if ident, ok := field.Type.(*ast.Ident); ok {
			return ident.Name == "Config"
		}
	}

	return false
}
