package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sflowg/sflowg/cli/internal/analyzer"
	"github.com/sflowg/sflowg/cli/internal/builder"
	"github.com/sflowg/sflowg/cli/internal/config"
	"github.com/sflowg/sflowg/cli/internal/detector"
	"github.com/sflowg/sflowg/cli/internal/generator"
	"github.com/sflowg/sflowg/cli/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	runtimePath     string
	corePluginsPath string
	embedFlows      bool
)

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
	type detectedPlugin struct {
		config.PluginConfig
		Type       config.PluginType
		ModulePath string
	}

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

	// 4. Analyze plugin packages to extract metadata (config, dependencies, tasks)
	fmt.Println("\nAnalyzing plugin packages...")

	type analyzedPlugin struct {
		detectedPlugin
		Metadata  *analyzer.PluginMetadata
		ConfigGen *generator.ConfigGenData
	}

	var analyzedPlugins []analyzedPlugin

	for _, plugin := range plugins {
		// Determine source path for analysis
		var sourcePath string

		if plugin.Type == config.TypeLocalModule {
			// Local module: use source path directly
			sourcePath = plugin.Source
			if !filepath.IsAbs(sourcePath) {
				sourcePath = filepath.Join(projectDir, plugin.Source)
			}
		} else if plugin.Type == config.TypeCorePlugin && corePluginsPath != "" {
			// Core plugin in development mode
			absPluginsPath, _ := filepath.Abs(corePluginsPath)
			sourcePath = filepath.Join(absPluginsPath, plugin.Name)
		} else {
			// Remote plugin or core plugin without local path - skip analysis for now
			// Will be analyzed after go mod download
			analyzedPlugins = append(analyzedPlugins, analyzedPlugin{
				detectedPlugin: plugin,
				Metadata:       nil,
				ConfigGen:      nil,
			})
			fmt.Printf("  [%s] %s - skipping analysis (will download first)\n", plugin.Type, plugin.Name)
			continue
		}

		// Analyze the plugin package
		metadata, err := analyzer.AnalyzePlugin(plugin.ModulePath, plugin.Name, sourcePath)
		if err != nil {
			fmt.Printf("  [%s] %s - analysis failed: %v\n", plugin.Type, plugin.Name, err)
			// Continue with nil metadata - plugin may not follow conventions
			analyzedPlugins = append(analyzedPlugins, analyzedPlugin{
				detectedPlugin: plugin,
				Metadata:       nil,
				ConfigGen:      nil,
			})
			continue
		}

		// Generate config initialization data if plugin has config
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

	// 5. Copy flows to workspace (only if embedding)
	if embedFlows {
		fmt.Println("\nCopying flows to workspace for embedding...")
		if err := ws.CopyFlows(); err != nil {
			return fmt.Errorf("failed to copy flows: %w", err)
		}
	} else {
		fmt.Println("\nSkipping flow copy (development mode - flows loaded at runtime)")
	}

	// 6. Prepare runtime path for development mode
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

	// 6.5 Validate core plugins path if provided
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

	// 7. Generate go.mod
	fmt.Println("\nGenerating go.mod...")
	goModGen := generator.NewGoModGenerator(ws.UUID, "latest", absRuntimePath)

	for _, plugin := range analyzedPlugins {
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

		// Add metadata from analysis
		if plugin.Metadata != nil {
			pluginInfo.TypeName = plugin.Metadata.TypeName
			pluginInfo.PackageName = plugin.Metadata.PackageName
			pluginInfo.HasConfig = plugin.Metadata.HasConfig
		}

		goModGen.AddPlugin(pluginInfo)
	}

	if err := goModGen.WriteToFile(ws.Path); err != nil {
		return fmt.Errorf("failed to generate go.mod: %w", err)
	}

	fmt.Printf("  ✓ go.mod created\n")

	// 8. Generate main.go
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

	// 9. Build binary
	fmt.Println("\nBuilding binary...")

	// Binary will be output to project root
	outputDir := projectDir

	binaryName := cfg.Name
	bldr := builder.NewBuilder(ws.Path, outputDir, binaryName)

	// Download dependencies
	fmt.Println("  → Downloading dependencies...")
	if err := bldr.DownloadDependencies(); err != nil {
		return fmt.Errorf("failed to download dependencies: %w", err)
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

	outputPath := filepath.Join(outputDir, binaryName)
	fmt.Printf("\n✅ Build successful!\n")
	fmt.Printf("Binary: %s\n", outputPath)
	fmt.Printf("\nRun with: %s\n", outputPath)

	return nil
}
