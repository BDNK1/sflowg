package generator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/sflowg/sflowg/cli/internal/constants"
)

// MainGoGenerator generates main.go files for builds
type MainGoGenerator struct {
	ModuleName        string
	RuntimeModulePath string
	Port              string
	EmbedFlows        bool
	GlobalProperties  map[string]interface{} // Global properties from flow-config.yaml
	Plugins           []PluginInfo
}

// NewMainGoGenerator creates a new main.go generator
func NewMainGoGenerator(moduleName string, port string, embedFlows bool, globalProperties map[string]interface{}) *MainGoGenerator {
	if port == "" {
		port = constants.DefaultPort
	}
	return &MainGoGenerator{
		ModuleName:        moduleName,
		RuntimeModulePath: constants.RuntimeModulePath,
		Port:              port,
		EmbedFlows:        embedFlows,
		GlobalProperties:  globalProperties,
		Plugins:           []PluginInfo{},
	}
}

// AddPlugin adds a plugin to the main.go generation
func (g *MainGoGenerator) AddPlugin(info PluginInfo) {
	g.Plugins = append(g.Plugins, info)
}

// Generate creates the main.go content using template
func (g *MainGoGenerator) Generate() (string, error) {
	tmpl, err := template.New("main").Funcs(template.FuncMap{
		"capitalize": capitalize,
		"sanitize":   sanitizeGoIdentifier,
	}).Parse(mainGoTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse main.go template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, g); err != nil {
		return "", fmt.Errorf("failed to execute main.go template: %w", err)
	}

	return buf.String(), nil
}

// WriteToFile writes the generated main.go to the workspace
func (g *MainGoGenerator) WriteToFile(workspacePath string) error {
	content, err := g.Generate()
	if err != nil {
		return err
	}

	mainGoPath := filepath.Join(workspacePath, "main.go")

	if err := os.WriteFile(mainGoPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write main.go: %w", err)
	}

	return nil
}

// capitalize returns the string uppercased (for constructor names)
// Phase 1: simple uppercase for common cases like http -> HTTP
// Phase 2: could add special handling for mixed case
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s)
}

// sanitizeGoIdentifier converts a string to a valid Go identifier
// Replaces hyphens and other invalid characters with underscores
func sanitizeGoIdentifier(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}
