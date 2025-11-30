# SFlowG CLI Documentation

The SFlowG CLI transforms YAML-based workflow definitions into standalone, deployable Go executables with automatic plugin discovery, dependency resolution, and configuration management.

## Table of Contents

- [Quick Start](#quick-start)
- [Commands](#commands)
- [Build Workflow](#build-workflow)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [Plugin System](#plugin-system)
- [Development Guide](#development-guide)

---

## Quick Start

### Build a Flow Application

```bash
# Build from flow-config.yaml
sflowg build

# Build specific project
sflowg build /path/to/project

# Development mode with local runtime
sflowg build --runtime-path ../runtime --plugins-path ../plugins

# Production build with embedded flows
sflowg build --embed-flows

# Custom port
sflowg build --port 3000
```

### Project Structure

```
myproject/
├── flow-config.yaml    # Plugin configuration
├── flows/              # YAML flow definitions
│   ├── auth.yaml
│   └── payment.yaml
└── myproject           # Generated binary (after build)
```

---

## Commands

### `sflowg build [project-dir]`

Compiles YAML flows into a standalone executable.

**Flags:**
- `--runtime-path <path>`: Local runtime module path (development)
- `--plugins-path <path>`: Local plugins directory path (development)
- `--port <port>`: HTTP server port (default: 8080)
- `--embed-flows`: Embed flow files into binary (production)

**Arguments:**
- `project-dir`: Project directory containing `flow-config.yaml` (default: current directory)

**Output:**
- Executable binary in project directory
- Binary name matches project directory or `flow-config.yaml` name

---

## Build Workflow

The CLI performs these steps automatically:

1. **Load Configuration** - Parse `flow-config.yaml`
2. **Create Workspace** - Temporary build directory (`/tmp/sflowg-build-<uuid>`)
3. **Detect Plugins** - Classify plugins (core/local/remote)
4. **Analyze Code** - AST-based analysis of plugin packages
   - Discover plugin structs, dependencies, config, tasks
5. **Copy Flows** - (If `--embed-flows`) Copy YAML files to workspace
6. **Generate Code**
   - `go.mod`: Module definition with dependencies
   - `main.go`: Application with plugin initialization
7. **Compile Binary** - `go mod tidy` + `go build`
8. **Deliver** - Copy executable to project directory
9. **Cleanup** - Remove temporary workspace

---

## Architecture

### Package Structure

```
cli/
├── cmd/                    # CLI commands (Cobra)
│   ├── root.go            # Root command
│   └── build.go           # Build command (main workflow)
└── internal/
    ├── analyzer/          # AST-based plugin analysis
    │   ├── plugin.go      # Plugin metadata extraction
    │   ├── config.go      # Config struct analysis
    │   └── types.go       # Metadata definitions
    ├── builder/           # Go binary compilation
    ├── config/            # flow-config.yaml parsing
    ├── detector/          # Plugin type detection
    ├── generator/         # Code generation (go.mod, main.go)
    ├── graph/             # Dependency graph & topological sort
    ├── security/          # Path traversal prevention
    └── workspace/         # Temporary build directory management
```

### Key Components

**Analyzer** - Static analysis via Go AST:
- Plugin struct detection
- Dependency extraction from struct fields
- Config field analysis with tags
- Task method discovery

**Generator** - Template-based code generation:
- `go.mod` with dependencies and replace directives
- `main.go` with initialization in dependency order
- Config code with env vars, literals, and defaults

**Graph** - Dependency resolution:
- Build directed dependency graph
- Topological sort for initialization order
- Cycle detection to prevent circular dependencies

**Security** - Path validation:
- Prevents path traversal attacks
- Validates all file operations stay within boundaries

---

## Configuration

### flow-config.yaml

```yaml
name: my-app             # Optional: defaults to directory name
version: latest          # Optional: defaults to "latest"
plugins:
  # Core plugin (single word)
  - source: http

  # Local module (relative path)
  - source: ./plugins/custom
    name: custom         # Optional: auto-detected

  # Remote module (full import path)
  - source: github.com/user/plugin
    version: v1.2.3     # Optional: defaults to "latest"
    name: myplugin      # Optional: auto-detected
    config:             # Plugin-specific configuration
      host: ${HOST:localhost}
      port: 8080
      timeout: 30s
      debug: true
```

### Plugin Configuration

**Three-layer configuration system:**

1. **Struct Tag Defaults**:
   ```go
   type Config struct {
       Host string `yaml:"host" default:"localhost"`
       Port int    `yaml:"port" default:"8080"`
   }
   ```

2. **flow-config.yaml Overrides**:
   ```yaml
   config:
     host: ${HTTP_HOST:api.example.com}
     port: 3000
   ```

3. **Runtime Environment Variables**:
   ```bash
   HTTP_HOST=production.com ./myapp
   ```

**Resolution Order**: Defaults → Environment Variables → Literal Values → Validation

### Environment Variable Syntax

- `${VAR}` - Required environment variable (build fails if not set)
- `${VAR:default}` - Optional with default value
- `literal` - Plain literal value (no variable substitution)

**Examples:**
```yaml
config:
  host: ${API_HOST}              # Required
  port: ${API_PORT:8080}         # Optional with default
  apiKey: ${SECRET_KEY}          # Required secret
  timeout: 30s                   # Literal value
  debug: ${DEBUG:false}          # Boolean with default
```

---

## Plugin System

### Plugin Detection

**Automatic Type Detection:**
- **Core Plugin**: Single word → `github.com/sflowg/sflowg/plugins/<name>`
- **Local Module**: Starts with `./`, `../`, `/` → Relative path
- **Remote Module**: Contains `/` → Full import path

**Plugin Name Inference:**
- Core: Use source name (`http` → `http`)
- Local: Last path segment (`./plugins/mylib` → `mylib`)
- Remote: Last path segment (`github.com/user/awesome` → `awesome`)

### Plugin Structure

**Convention-Based Requirements:**

```go
// Plugin struct must:
// 1. Be exported (capitalized)
// 2. End with "Plugin" suffix
type HTTPPlugin struct {
    config Config              // Optional: plugin configuration
    cache  *cache.CachePlugin  // Optional: dependencies (injected)
}

// Config struct (optional)
type Config struct {
    Host    string        `yaml:"host" default:"localhost" validate:"required"`
    Port    int           `yaml:"port" default:"8080" validate:"gte=1,lte=65535"`
    Timeout time.Duration `yaml:"timeout" default:"30s"`
}

// Task methods
func (p *HTTPPlugin) Request(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    // Implementation
}
```

### Dependency Injection

**Automatic Detection:**

```go
type APIPlugin struct {
    config Config
    http   *http.HTTPPlugin    // Injected by field name
    cache  *cache.CachePlugin `inject:"redis"` // Injected by tag
}
```

**Injection Rules:**
1. Analyze struct fields of plugin struct
2. Detect pointer fields to other plugin types
3. Extract plugin names from:
   - `inject:"name"` struct tag (explicit)
   - Field name in lowercase (implicit)
4. Build dependency graph
5. Validate no circular dependencies
6. Initialize plugins in dependency order

### Task Discovery

**Valid Task Signatures:**

```go
// Map-based (Phase 2.1)
func (p *Plugin) TaskName(exec *plugin.Execution, args map[string]any) (map[string]any, error)

// Typed (Phase 2.4)
func (p *Plugin) TaskName(exec *plugin.Execution, input TInput) (TOutput, error)
```

**Task Naming:**
- Plugin `http` with method `Request` → Task name: `http.request`
- Plugin `payment` with method `Charge` → Task name: `payment.charge`

---

## Development Guide

### Local Development Workflow

**1. Development with Local Runtime:**

```bash
sflowg build \
  --runtime-path /path/to/runtime \
  --plugins-path /path/to/plugins
```

**Generated go.mod includes:**
```go
replace (
    github.com/sflowg/sflowg/runtime => /path/to/runtime
    github.com/sflowg/sflowg/plugins/http => /path/to/plugins/http
)
```

**2. Development with Remote Runtime:**

```bash
sflowg build --plugins-path /path/to/plugins
```

Only local plugins use replace directives.

**3. Production Build:**

```bash
sflowg build --embed-flows
```

All dependencies from published modules.

### Generated main.go Structure

```go
package main

import (
    "context"
    "flag"
    "github.com/gin-gonic/gin"
    "github.com/sflowg/sflowg/runtime"
    httpplugin "github.com/sflowg/sflowg/plugins/http"
)

func main() {
    // Parse flags
    flowsPath := flag.String("flows", "", "Path to flows directory")
    port := flag.String("port", "8080", "Server port")
    flag.Parse()

    // Create container
    container := runtime.NewContainer()

    // Initialize plugins (in dependency order)
    httpConfig := httpplugin.Config{
        Host:    "localhost",  // Default
        Port:    8080,         // Literal from YAML
        Timeout: 30 * time.Second,
    }
    // Override with environment variables
    if host := os.Getenv("HTTP_HOST"); host != "" {
        httpConfig.Host = host
    }

    // Validate configuration
    if err := runtime.PrepareConfig(&httpConfig); err != nil {
        panic(err)
    }

    httpPlugin := &httpplugin.HTTPPlugin{}
    httpPlugin.SetConfig(httpConfig)
    container.RegisterPlugin("http", httpPlugin)

    // Initialize container
    ctx := context.Background()
    if err := container.Initialize(ctx); err != nil {
        panic(err)
    }

    // Setup graceful shutdown
    defer container.Shutdown(ctx)

    // Load flows
    app := runtime.NewApp(*flowsPath)
    app.Container = container
    flows, err := app.LoadFlows()
    if err != nil {
        panic(err)
    }

    // Start HTTP server
    g := gin.Default()
    for _, flow := range flows {
        handler := runtime.NewHttpHandler(flow, &app)
        g.Handle(flow.Entrypoint.Method, flow.Entrypoint.Path, handler.Handle)
    }

    g.Run(":" + *port)
}
```

### AST-Based Analysis

**Plugin Metadata Extraction:**

```go
// Analyzer discovers:
type PluginMetadata struct {
    Name         string          // "http"
    ImportPath   string          // "github.com/sflowg/sflowg/plugins/http"
    TypeName     string          // "HTTPPlugin"
    PackageName  string          // "http"
    HasConfig    bool            // true
    ConfigType   *ConfigMetadata // Config struct details
    Dependencies []Dependency    // Other plugins needed
    Tasks        []TaskMetadata  // Public task methods
}
```

**Config Field Analysis:**

```go
type ConfigMetadata struct {
    TypeName string         // "Config"
    Fields   []ConfigField  // All struct fields
}

type ConfigField struct {
    Name        string  // "Host"
    Type        string  // "string"
    YAMLTag     string  // "host"
    DefaultTag  string  // "localhost"
    ValidateTag string  // "required"
}
```

### Dependency Graph

**Graph Operations:**

```go
// Build graph
graph := graph.BuildGraph(pluginMetadata)

// Topological sort (Kahn's algorithm)
order, err := graph.TopologicalSort()
// Returns: ["cache", "http", "api"] (dependencies first)

// Cycle detection (DFS)
if cycle := graph.DetectCycle(); cycle != nil {
    // Returns: ["api", "payment", "user", "api"]
}
```

**Error Types:**
- `ErrorMissingDependency`: Required plugin not registered
- `ErrorCircularDependency`: Cycle detected in dependency graph

### Security Validation

**Path Traversal Prevention:**

```go
// Validate single path
err := security.ValidatePathWithinBoundary("/workspace", "/workspace/flows/auth.yaml")
// Returns: nil (valid)

err := security.ValidatePathWithinBoundary("/workspace", "/workspace/../etc/passwd")
// Returns: error (path traversal attempt)

// Validate multiple paths
err := security.ValidatePathsWithinBoundary("/workspace",
    "/workspace/flows/auth.yaml",
    "/workspace/flows/payment.yaml",
)
```

**Used in:**
- Workspace flow copying
- Local plugin path resolution
- All file operations

---

## Testing

### Test Coverage

**6 Test Files:**
1. `internal/security/path_test.go` - Path traversal prevention
2. `internal/analyzer/plugin_test.go` - Plugin AST analysis
3. `internal/analyzer/config_test.go` - Config struct analysis
4. `internal/graph/dependency_test.go` - Dependency graph operations
5. `internal/config/envvar_test.go` - Environment variable parsing
6. `internal/generator/config_test.go` - Config code generation

### Running Tests

```bash
cd cli
go test ./...

# Verbose output
go test -v ./...

# Specific package
go test -v ./internal/analyzer
```

---

## Advanced Features

### Flow Embedding

**Development Mode** (default):
```bash
sflowg build
# Flows loaded at runtime from --flows flag or relative path
./myapp --flows ./flows
```

**Production Mode** (`--embed-flows`):
```bash
sflowg build --embed-flows
# Flows embedded in binary, no external files needed
./myapp  # Flows already inside
```

### Multi-Mode Configuration

**Scenario: Different configs for dev/staging/prod**

```yaml
# flow-config.yaml
plugins:
  - source: http
    config:
      host: ${API_HOST:localhost}
      port: ${API_PORT:8080}
      timeout: ${TIMEOUT:30s}
      debug: ${DEBUG:false}
```

```bash
# Development
API_HOST=localhost ./myapp

# Staging
API_HOST=staging.api.com DEBUG=true ./myapp

# Production
API_HOST=api.example.com TIMEOUT=60s ./myapp
```

### Custom Core Plugins

**Register new core plugins:**

Update `cli/internal/constants/modules.go`:
```go
const PluginsBasePath = "github.com/sflowg/sflowg/plugins"

// Core plugins:
// http    → github.com/sflowg/sflowg/plugins/http
// custom  → github.com/sflowg/sflowg/plugins/custom
```

---

## Troubleshooting

### Build Fails: "plugin not found"

**Cause:** Plugin source not accessible

**Solutions:**
- **Core plugin**: Ensure published in `github.com/sflowg/sflowg/plugins/<name>`
- **Local module**: Verify path exists and is relative
- **Remote module**: Check import path is correct and accessible

### Build Fails: "circular dependency"

**Cause:** Plugins have circular dependencies

**Solution:** Review plugin struct fields:
```go
// BAD: Circular dependency
type PluginA struct {
    b *PluginB
}
type PluginB struct {
    a *PluginA  // Circular!
}

// GOOD: One-way dependency
type PluginA struct {
    b *PluginB
}
type PluginB struct {
    // No reference to PluginA
}
```

### Build Fails: "config validation failed"

**Cause:** Missing required environment variable or invalid config

**Solution:** Check `flow-config.yaml` and environment:
```yaml
config:
  apiKey: ${API_KEY}  # Required - must set API_KEY env var
```

```bash
API_KEY=secret-key sflowg build
```

### Runtime Error: "flows not found"

**Cause:** Flows directory not accessible

**Solutions:**
- **With `--embed-flows`**: Flows should be embedded, no flag needed
- **Without `--embed-flows`**: Provide `--flows` flag at runtime:
  ```bash
  ./myapp --flows ./flows
  ```

---

## Performance

### Build Time Optimization

**Typical build times:**
- Small project (2-3 plugins): ~5-10 seconds
- Medium project (5-10 plugins): ~15-30 seconds
- Large project (15+ plugins): ~30-60 seconds

**Factors affecting build time:**
- Number of plugins
- Plugin dependency depth
- First build (downloads dependencies)
- Subsequent builds (dependencies cached)

### Binary Size

**Typical sizes:**
- Minimal (http plugin only): ~15-20 MB
- Medium (5 plugins): ~25-35 MB
- Large (10+ plugins): ~40-60 MB

**Size reduction:**
- Use `--embed-flows` only for production
- Remove unused plugins from `flow-config.yaml`
- Use `go build -ldflags="-s -w"` for stripped binaries (future feature)

---

## Examples

### Example 1: Simple HTTP API

**flow-config.yaml:**
```yaml
name: api
plugins:
  - source: http
    config:
      timeout: 10s
```

**Build:**
```bash
sflowg build
./api --flows ./flows
```

### Example 2: Multi-Plugin Application

**flow-config.yaml:**
```yaml
name: payment-service
plugins:
  - source: http
    config:
      timeout: 30s

  - source: ./plugins/database
    config:
      dsn: ${DATABASE_URL}

  - source: github.com/myorg/payment-processor
    version: v1.0.0
    config:
      apiKey: ${PAYMENT_API_KEY}
```

**Build:**
```bash
DATABASE_URL=postgres://... PAYMENT_API_KEY=key sflowg build --embed-flows
```

### Example 3: Development with Local Plugins

**flow-config.yaml:**
```yaml
plugins:
  - source: http
  - source: ./mylib
    name: custom
```

**Build:**
```bash
sflowg build --runtime-path ../runtime --plugins-path ../plugins
```

---

## Best Practices

1. **Always specify plugin versions for remote modules in production**
   ```yaml
   - source: github.com/user/plugin
     version: v1.2.3  # Lock version
   ```

2. **Use environment variables for secrets**
   ```yaml
   config:
     apiKey: ${API_KEY}  # Never commit secrets
   ```

3. **Use `--embed-flows` for production builds**
   ```bash
   sflowg build --embed-flows  # Self-contained binary
   ```

4. **Validate configuration locally first**
   ```bash
   # Test with env vars set
   API_KEY=test sflowg build
   ```

5. **Keep plugin dependencies shallow**
   - Avoid deep dependency chains
   - Watch for circular dependencies

6. **Name plugins consistently**
   - Use clear, descriptive names
   - Avoid conflicts with core plugins

7. **Document plugin configuration**
   - Add comments to `flow-config.yaml`
   - Document required environment variables

---

## Architecture Decisions

### Why AST-Based Analysis?

**Benefits:**
- No code execution required (security)
- Fast analysis (no compilation)
- Works with incomplete code
- Discovers metadata without running code

**Trade-offs:**
- Only analyzes struct definitions
- Cannot detect runtime behavior
- Relies on naming conventions

### Why Convention Over Configuration?

**Benefits:**
- Less boilerplate
- Easier to get started
- Self-documenting code
- Reduced configuration complexity

**Trade-offs:**
- Must follow naming conventions
- Less flexibility for edge cases

### Why Code Generation?

**Benefits:**
- Type-safe initialization
- No reflection overhead at runtime
- Clear dependency order
- Easy to debug generated code

**Trade-offs:**
- Build step required
- Generated code can be verbose
- Temporary files during build

---

## Future Enhancements

**Planned features:**
- Build caching for faster rebuilds
- Binary size optimization flags
- Plugin discovery from GitHub
- Hot reload during development
- Build profiles (dev/staging/prod)
- Dependency version resolution
- Plugin marketplace integration

---

## Contributing

The CLI follows standard Go project structure. Key areas for contribution:

1. **Analyzer**: Enhance AST analysis capabilities
2. **Generator**: Improve code generation templates
3. **Graph**: Optimize dependency resolution algorithms
4. **Security**: Additional validation and sanitization
5. **Tests**: Increase coverage and edge cases

---

## License

Part of the SFlowG project. See main repository for license details.
