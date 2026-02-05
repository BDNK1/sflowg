package detector

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BDNK1/sflowg/cli/internal/config"
	"github.com/BDNK1/sflowg/cli/internal/constants"
)

// DetectPluginType determines the type of plugin based on its source
func DetectPluginType(source string) config.PluginType {
	// Rule 1: ./ or ../ or / prefix → Local path
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || strings.HasPrefix(source, "/") {
		return config.TypeLocalModule
	}

	// Rule 2: Contains / → RemoteModule (e.g., "github.com/user/repo")
	if strings.Contains(source, "/") {
		return config.TypeRemoteModule
	}

	// Rule 3: Single word (no path separators) → CorePlugin
	// Examples: "http", "database", "queue", "my-plugin"
	// This allows any single-word identifier as a core plugin
	return config.TypeCorePlugin
}

// hasGoMod checks if a directory contains a go.mod file
func hasGoMod(dirPath string) bool {
	// First, verify the path is actually a directory
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return false
	}
	if !dirInfo.IsDir() {
		return false
	}

	// Check if go.mod exists in the directory
	goModPath := filepath.Join(dirPath, "go.mod")
	_, err = os.Stat(goModPath)
	return err == nil
}

// InferPluginName derives a plugin name from its source
func InferPluginName(source string, pluginType config.PluginType) string {
	switch pluginType {
	case config.TypeCorePlugin:
		return source

	case config.TypeLocalModule, config.TypeRemoteModule:
		// Extract last component of module path
		parts := strings.Split(source, "/")
		return parts[len(parts)-1]

	default:
		return "unknown"
	}
}

// ExpandCorePlugin converts core plugin shorthand to full module path
func ExpandCorePlugin(name string) string {
	return constants.PluginsBasePath + "/" + name
}

// ResolveVersion determines the version to use for a plugin
func ResolveVersion(version string) string {
	// Phase 1: Always use latest
	// Phase 2: Will implement proper version resolution
	if version == "" || version == "latest" {
		return "latest"
	}
	return version
}
