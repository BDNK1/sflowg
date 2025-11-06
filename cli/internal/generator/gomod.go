package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sflowg/sflowg/cli/internal/config"
	"github.com/sflowg/sflowg/cli/internal/constants"
)

// GoVersion is the minimum required Go version for generated applications
const GoVersion = "1.25.3"

// GoModGenerator generates go.mod files for builds
type GoModGenerator struct {
	ModuleName     string
	RuntimeVersion string
	RuntimePath    string // Absolute path to runtime module
	Plugins        []PluginInfo
}

// PluginInfo contains information about a plugin for go.mod generation
type PluginInfo struct {
	Name       string
	ModulePath string
	Version    string
	Type       config.PluginType
	LocalPath  string // For local modules, path to module directory
}

// NewGoModGenerator creates a new go.mod generator
func NewGoModGenerator(buildUUID string, runtimeVersion string, runtimePath string) *GoModGenerator {
	return &GoModGenerator{
		ModuleName:     fmt.Sprintf("sflowg-build-%s", buildUUID),
		RuntimeVersion: runtimeVersion,
		RuntimePath:    runtimePath,
		Plugins:        []PluginInfo{},
	}
}

// AddPlugin adds a plugin to the go.mod dependencies
func (g *GoModGenerator) AddPlugin(info PluginInfo) {
	g.Plugins = append(g.Plugins, info)
}

// Generate creates the go.mod content
func (g *GoModGenerator) Generate() string {
	var sb strings.Builder

	// Module declaration
	sb.WriteString(fmt.Sprintf("module %s\n\n", g.ModuleName))

	// Go version
	sb.WriteString(fmt.Sprintf("go %s\n\n", GoVersion))

	// Require section
	sb.WriteString("require (\n")

	// Runtime dependency
	runtimeVersion := g.RuntimeVersion
	if runtimeVersion == "" || runtimeVersion == "latest" {
		runtimeVersion = "v0.0.0" // Will be replaced by replace directive
	}
	sb.WriteString(fmt.Sprintf("\t%s %s\n", constants.RuntimeModulePath, runtimeVersion))

	// Plugin dependencies (all types)
	for _, plugin := range g.Plugins {
		version := plugin.Version
		if version == "" || version == "latest" {
			version = "v0.0.0" // Will be replaced by replace directive or go mod tidy
		}
		sb.WriteString(fmt.Sprintf("\t%s %s\n", plugin.ModulePath, version))
	}

	sb.WriteString(")\n\n")

	// Replace directives for local modules (development mode only)
	hasReplace := false

	// Add plugin replace directives (only for plugins with LocalPath set)
	for _, plugin := range g.Plugins {
		if plugin.LocalPath != "" {
			if !hasReplace {
				sb.WriteString("replace (\n")
				hasReplace = true
			}
			sb.WriteString(fmt.Sprintf("\t%s => %s\n", plugin.ModulePath, plugin.LocalPath))
		}
	}

	// Add runtime replace directive (only if RuntimePath is set - development mode)
	if g.RuntimePath != "" {
		if !hasReplace {
			sb.WriteString("replace (\n")
			hasReplace = true
		}
		sb.WriteString(fmt.Sprintf("\t%s => %s\n", constants.RuntimeModulePath, g.RuntimePath))
	}

	if hasReplace {
		sb.WriteString(")\n")
	}

	return sb.String()
}

// WriteToFile writes the generated go.mod to the workspace
func (g *GoModGenerator) WriteToFile(workspacePath string) error {
	content := g.Generate()
	goModPath := filepath.Join(workspacePath, "go.mod")

	if err := os.WriteFile(goModPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	return nil
}
