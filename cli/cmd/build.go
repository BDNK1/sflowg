package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BDNK1/sflowg/cli/internal/analyzer"
	"github.com/BDNK1/sflowg/cli/internal/builder"
	"github.com/BDNK1/sflowg/cli/internal/config"
	"github.com/BDNK1/sflowg/cli/internal/constants"
	"github.com/BDNK1/sflowg/cli/internal/detector"
	"github.com/BDNK1/sflowg/cli/internal/generator"
	"github.com/BDNK1/sflowg/cli/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	runtimePath     string
	corePluginsPath string
	embedFlows      bool
)

type detectedPlugin struct {
	config.PluginConfig
	Type       config.PluginType
	ModulePath string
}

var buildCmd = &cobra.Command{
	Use:   "build [project-dir]",
	Short: "Build a deployable binary from flow configuration",
	Long: `Build command reads flow-config.yaml, resolves plugins, and generates
a single executable binary with all dependencies compiled in.

Example:
  sflowg build .
  sflowg build ./my-project
  sflowg build . --runtime-path ../runtime --core-plugins-path ../plugins
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBuild,
}

func init() {
	buildCmd.Flags().StringVar(&runtimePath, "runtime-path", "", "Path to local runtime module (for development)")
	buildCmd.Flags().StringVar(&corePluginsPath, "core-plugins-path", "", "Path to local core plugins directory (for development)")
	buildCmd.Flags().BoolVar(&embedFlows, "embed-flows", false, "Embed flow files into the binary (production mode)")
}

// validateRuntimePath ensures the runtime path exists and is accessible
// Just a helpful UX check - doesn't enforce structure
func validateRuntimePath(runtimePath string) error {
	info, err := os.Stat(runtimePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", runtimePath)
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", runtimePath)
	}

	return nil
}

// validateCorePluginsPath ensures the core plugins path exists and is accessible
// Just a helpful UX check - doesn't enforce structure
func validateCorePluginsPath(corePluginsPath string) error {
	info, err := os.Stat(corePluginsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", corePluginsPath)
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", corePluginsPath)
	}

	return nil
}

func runBuild(_ *cobra.Command, args []string) error {
	projectDir := "."
	if len(args) > 0 {
		projectDir = args[0]
	}

	// Convert to absolute path to ensure replace directives work correctly
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("failed to resolve project directory: %w", err)
	}
	projectDir = absProjectDir

	fmt.Printf("Building project in: %s\n", projectDir)

	// 1. Parse flow-config.yaml
	cfg, err := config.Load(projectDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Create temp workspace
	ws, err := workspace.Create(projectDir)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}
	defer func() {
		if cleanupErr := ws.Cleanup(); cleanupErr != nil {
			fmt.Printf("Warning: failed to cleanup workspace: %v\n", cleanupErr)
		}
	}()

	fmt.Printf("Workspace: %s\n", ws.Path)

	fmt.Printf("Project: %s (version: %s)\n", cfg.Name, cfg.Version)
	fmt.Printf("Plugins: %d\n\n", len(cfg.Plugins))

	// 3. Auto-detect plugin types and expand core plugins
	var plugins []detectedPlugin

	for _, plugin := range cfg.Plugins {
		// Detect plugin type
		pluginType := detector.DetectPluginType(plugin.Source)

		// Infer name if not provided
		if plugin.Name == "" {
			plugin.Name = detector.InferPluginName(plugin.Source, pluginType)
		}

		// Expand core plugins to full module path
		// For local modules, generate a synthetic module path
		modulePath := plugin.Source
		if pluginType == config.TypeCorePlugin {
			modulePath = detector.ExpandCorePlugin(plugin.Source)
		} else if pluginType == config.TypeLocalModule {
			// Generate synthetic module path for local modules
			// Use format: example.com/local/{plugin-name}
			modulePath = fmt.Sprintf("example.com/local/%s", plugin.Name)
		}

		// Resolve version
		version := detector.ResolveVersion(plugin.Version)

		plugins = append(plugins, detectedPlugin{
			PluginConfig: config.PluginConfig{
				Source:  plugin.Source,
				Name:    plugin.Name,
				Version: version,
				Config:  plugin.Config,
			},
			Type:       pluginType,
			ModulePath: modulePath,
		})

		fmt.Printf("  [%s] %s\n", pluginType, plugin.Name)
		fmt.Printf("    Source: %s\n", plugin.Source)
		if pluginType == config.TypeCorePlugin {
			fmt.Printf("    Module: %s\n", modulePath)
		}
		if pluginType == config.TypeRemoteModule && version != "latest" {
			fmt.Printf("    Version: %s\n", version)
		}
		fmt.Println()
	}

	type analyzedPlugin struct {
		detectedPlugin
		Metadata  *analyzer.PluginMetadata
		ConfigGen *generator.ConfigGenData
	}

	// 4. Copy flows to workspace (only if embedding)
	if embedFlows {
		fmt.Println("\nCopying flows to workspace for embedding...")
		if err := ws.CopyFlows(); err != nil {
			return fmt.Errorf("failed to copy flows: %w", err)
		}
	} else {
		fmt.Println("\nSkipping flow copy (development mode - flows loaded at runtime)")
	}

	// 5. Prepare runtime path for development mode
	var absRuntimePath string
	if runtimePath != "" {
		// Development mode: use local runtime
		absRuntimePath, err = filepath.Abs(runtimePath)
		if err != nil {
			return fmt.Errorf("failed to resolve runtime path: %w", err)
		}

		// Validate runtime path exists and is a valid Go module
		if err := validateRuntimePath(absRuntimePath); err != nil {
			return fmt.Errorf("invalid runtime path: %w", err)
		}

		fmt.Printf("\n✓ Development mode enabled\n")
		fmt.Printf("  Runtime: %s\n", absRuntimePath)
	} else {
		// Production mode: use published module from GitHub
		fmt.Println("\n✓ Production mode")
		fmt.Println("  Runtime will be downloaded from GitHub")
	}

	// 5.5 Validate core plugins path if provided
	if corePluginsPath != "" {
		absPluginsPath, err := filepath.Abs(corePluginsPath)
		if err != nil {
			return fmt.Errorf("failed to resolve core plugins path: %w", err)
		}

		if err := validateCorePluginsPath(absPluginsPath); err != nil {
			return fmt.Errorf("invalid core plugins path: %w", err)
		}

		fmt.Printf("  Core Plugins: %s\n", absPluginsPath)
	}

	// 6. Generate go.mod (phase 1: dependency resolution)
	fmt.Println("\nGenerating go.mod...")
	goModGen := generator.NewGoModGenerator(ws.UUID, cfg.Runtime.Version, absRuntimePath)

	for _, plugin := range plugins {
		pluginInfo := generator.PluginInfo{
			Name:       plugin.Name,
			ModulePath: plugin.ModulePath,
			Version:    plugin.Version,
			Type:       plugin.Type,
		}

		// For local modules and core plugins (in dev), set absolute local paths
		if plugin.Type == config.TypeLocalModule {
			// Make absolute path
			absPath := plugin.Source
			if !filepath.IsAbs(absPath) {
				absPath = filepath.Join(projectDir, plugin.Source)
			}
			pluginInfo.LocalPath = absPath
		} else if plugin.Type == config.TypeCorePlugin && corePluginsPath != "" {
			// Core plugins point to local plugins directory during development
			// Only set local path if --core-plugins-path flag is provided
			absPluginsPath, err := filepath.Abs(corePluginsPath)
			if err != nil {
				return fmt.Errorf("failed to resolve core plugins path: %w", err)
			}
			pluginInfo.LocalPath = filepath.Join(absPluginsPath, plugin.Name)
		}

		goModGen.AddPlugin(pluginInfo)
	}

	if err := goModGen.WriteToFile(ws.Path); err != nil {
		return fmt.Errorf("failed to generate go.mod: %w", err)
	}

	fmt.Printf("  ✓ go.mod created\n")

	// Resolve "latest" versions to concrete module versions before downloading modules.
	if err := pinDynamicModuleVersions(ws.Path, absRuntimePath, cfg.Runtime.Version, corePluginsPath, plugins); err != nil {
		return fmt.Errorf("failed to resolve dynamic module versions: %w", err)
	}

	// 7. Resolve/download dependencies before analysis (phase 1)
	// This makes remote/core plugin source available in module cache so we can analyze
	// real plugin types/config in phase 2.
	binaryName := cfg.Name
	bldr := builder.NewBuilder(ws.Path, projectDir, binaryName)

	fmt.Println("\nResolving dependencies...")
	fmt.Println("  → Downloading modules...")
	if err := downloadModules(ws.Path); err != nil {
		return fmt.Errorf("failed to download modules: %w", err)
	}

	// 8. Analyze plugin packages after deps are resolved (phase 2)
	fmt.Println("\nAnalyzing plugin packages...")
	var analyzedPlugins []analyzedPlugin
	for _, plugin := range plugins {
		var sourcePath string

		switch {
		case plugin.Type == config.TypeLocalModule:
			sourcePath = plugin.Source
			if !filepath.IsAbs(sourcePath) {
				sourcePath = filepath.Join(projectDir, plugin.Source)
			}
		case plugin.Type == config.TypeCorePlugin && corePluginsPath != "":
			absPluginsPath, err := filepath.Abs(corePluginsPath)
			if err != nil {
				return fmt.Errorf("failed to resolve core plugins path: %w", err)
			}
			sourcePath = filepath.Join(absPluginsPath, plugin.Name)
		default:
			sourcePath, err = resolveModuleDir(ws.Path, plugin.ModulePath)
			if err != nil {
				return fmt.Errorf("failed to resolve module directory for plugin '%s' (%s): %w", plugin.Name, plugin.ModulePath, err)
			}
		}

		metadata, err := analyzer.AnalyzePlugin(plugin.ModulePath, plugin.Name, sourcePath)
		if err != nil {
			return fmt.Errorf("failed to analyze plugin '%s' at %s: %w", plugin.Name, sourcePath, err)
		}

		var configGen *generator.ConfigGenData
		if metadata.HasConfig && metadata.ConfigType != nil {
			configGen, err = generator.GenerateConfigInit(metadata.ConfigType, plugin.Config)
			if err != nil {
				return fmt.Errorf("failed to generate config for plugin '%s': %w", plugin.Name, err)
			}
		}

		analyzedPlugins = append(analyzedPlugins, analyzedPlugin{
			detectedPlugin: plugin,
			Metadata:       metadata,
			ConfigGen:      configGen,
		})

		fmt.Printf("  ✓ %s", plugin.Name)
		if metadata.HasConfig {
			fmt.Printf(" (config: %d fields)", len(metadata.ConfigType.Fields))
		}
		if len(metadata.Dependencies) > 0 {
			fmt.Printf(" (deps: %d)", len(metadata.Dependencies))
		}
		if len(metadata.Tasks) > 0 {
			fmt.Printf(" (tasks: %d)", len(metadata.Tasks))
		}
		fmt.Println()
	}

	// 9. Generate main.go
	fmt.Println("\nGenerating main.go...")
	mainGoGen := generator.NewMainGoGenerator(goModGen.ModuleName, cfg.Runtime.Port, embedFlows, cfg.Properties)

	for _, plugin := range analyzedPlugins {
		pluginInfo := generator.PluginInfo{
			Name:       plugin.Name,
			ModulePath: plugin.ModulePath,
			Type:       plugin.Type,
		}

		// Add metadata from analysis
		if plugin.Metadata != nil {
			pluginInfo.TypeName = plugin.Metadata.TypeName
			pluginInfo.PackageName = plugin.Metadata.PackageName
			pluginInfo.HasConfig = plugin.Metadata.HasConfig

			// Add dependencies for Phase 2.2
			for _, dep := range plugin.Metadata.Dependencies {
				pluginInfo.Dependencies = append(pluginInfo.Dependencies, generator.PluginDependency{
					FieldName:  dep.FieldName,
					PluginName: dep.PluginName,
				})
			}
		}

		// Add config generation data for Phase 2.3
		if plugin.ConfigGen != nil {
			pluginInfo.ConfigGen = plugin.ConfigGen
		}

		mainGoGen.AddPlugin(pluginInfo)
	}

	if err := mainGoGen.WriteToFile(ws.Path); err != nil {
		return fmt.Errorf("failed to generate main.go: %w", err)
	}

	fmt.Printf("  ✓ main.go created\n")

	// 10. Build binary
	fmt.Println("\nBuilding binary...")

	// Ensure go.mod/go.sum include all imports from generated main.go and plugins.
	fmt.Println("  → Syncing dependencies...")
	if err := bldr.DownloadDependencies(); err != nil {
		return fmt.Errorf("failed to sync dependencies: %w", err)
	}

	// Compile
	fmt.Println("  → Compiling...")
	if err := bldr.Build(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	// Copy to output directory
	fmt.Println("  → Copying binary...")
	if err := bldr.CopyBinary(); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	outputPath := filepath.Join(projectDir, binaryName)
	fmt.Printf("\n✅ Build successful!\n")
	fmt.Printf("Binary: %s\n", outputPath)
	fmt.Printf("\nRun with: %s\n", outputPath)

	return nil
}

func resolveModuleDir(workspacePath, modulePath string) (string, error) {
	type moduleInfo struct {
		Dir   string `json:"Dir"`
		Error *struct {
			Err string `json:"Err"`
		} `json:"Error"`
		Replace *struct {
			Dir string `json:"Dir"`
		} `json:"Replace"`
	}

	// First try go list -m -json to read resolved module dir.
	cmd := exec.Command("go", "list", "-m", "-json", modulePath)
	cmd.Dir = workspacePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go list failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	var info moduleInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return "", fmt.Errorf("failed to parse go list output for %s: %w", modulePath, err)
	}
	if info.Error != nil && info.Error.Err != "" {
		return "", fmt.Errorf("module resolution error for %s: %s", modulePath, info.Error.Err)
	}
	if strings.TrimSpace(info.Dir) != "" {
		return strings.TrimSpace(info.Dir), nil
	}
	if info.Replace != nil && strings.TrimSpace(info.Replace.Dir) != "" {
		return strings.TrimSpace(info.Replace.Dir), nil
	}

	// Some module states keep Dir empty in go list; force module extraction and retry via download.
	downloadCmd := exec.Command("go", "mod", "download", "-json", modulePath)
	downloadCmd.Dir = workspacePath
	downloadOut, err := downloadCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go mod download failed for %s: %w: %s", modulePath, err, strings.TrimSpace(string(downloadOut)))
	}

	var downloaded moduleInfo
	if err := json.Unmarshal(downloadOut, &downloaded); err != nil {
		return "", fmt.Errorf("failed to parse go mod download output for %s: %w", modulePath, err)
	}
	if strings.TrimSpace(downloaded.Dir) != "" {
		return strings.TrimSpace(downloaded.Dir), nil
	}

	return "", fmt.Errorf("empty module dir for %s", modulePath)
}

func downloadModules(workspacePath string) error {
	cmd := exec.Command("go", "mod", "download")
	cmd.Dir = workspacePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go mod download failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func pinDynamicModuleVersions(
	workspacePath string,
	absRuntimePath string,
	runtimeVersion string,
	corePluginsPath string,
	plugins []detectedPlugin,
) error {
	// Runtime module: resolve latest only when not overridden by local runtime path.
	if absRuntimePath == "" && isLatestVersion(runtimeVersion) {
		resolved, err := resolveLatestVersion(workspacePath, constants.RuntimeModulePath)
		if err != nil {
			return fmt.Errorf("runtime latest resolution failed: %w", err)
		}
		if err := setModuleRequireVersion(workspacePath, constants.RuntimeModulePath, resolved); err != nil {
			return fmt.Errorf("failed to pin runtime version: %w", err)
		}
	}

	for _, plugin := range plugins {
		// Local plugins are replaced to local paths in go.mod; no remote version resolution.
		if plugin.Type == config.TypeLocalModule {
			continue
		}
		// Core plugins with local override path are also local replaces.
		if plugin.Type == config.TypeCorePlugin && corePluginsPath != "" {
			continue
		}
		// Explicit versions are already concrete.
		if !isLatestVersion(plugin.Version) {
			continue
		}

		resolved, err := resolveLatestVersion(workspacePath, plugin.ModulePath)
		if err != nil {
			return fmt.Errorf("plugin '%s' latest resolution failed: %w", plugin.Name, err)
		}
		if err := setModuleRequireVersion(workspacePath, plugin.ModulePath, resolved); err != nil {
			return fmt.Errorf("failed to pin plugin '%s' version: %w", plugin.Name, err)
		}
	}

	return nil
}

func isLatestVersion(version string) bool {
	return strings.TrimSpace(version) == "" || strings.TrimSpace(version) == "latest"
}

func resolveLatestVersion(workspacePath, modulePath string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", modulePath+"@latest")
	cmd.Dir = workspacePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go list latest failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return "", fmt.Errorf("empty latest version for %s", modulePath)
	}
	return version, nil
}

func setModuleRequireVersion(workspacePath, modulePath, version string) error {
	cmd := exec.Command("go", "mod", "edit", "-require", fmt.Sprintf("%s@%s", modulePath, version))
	cmd.Dir = workspacePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go mod edit failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
