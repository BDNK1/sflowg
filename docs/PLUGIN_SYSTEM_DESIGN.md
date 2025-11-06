# SFlowG Plugin System - Comprehensive Design Document

**Version:** 2.0
**Date:** 2025-11-05
**Status:** Design Specification

---

## Table of Contents

1. [Overview](#overview)
2. [Core Principles](#core-principles)
3. [Architecture](#architecture)
4. [Domain Model: Plugins & Tasks](#domain-model-plugins--tasks)
5. [Dependency Injection](#dependency-injection)
6. [Plugin Types & Distribution](#plugin-types--distribution)
7. [Core Plugins Structure](#core-plugins-structure)
8. [CLI Build System](#cli-build-system)
9. [Configuration](#configuration)
10. [Container & Lifecycle](#container--lifecycle)
11. [Validation & Health Checks](#validation--health-checks)
12. [Binary Size Optimization](#binary-size-optimization)
13. [Typed Task Support](#typed-task-support)
14. [Implementation Phases](#implementation-phases)
15. [Examples](#examples)

---

## Overview

### What Problem Does This Solve?

SFlowG currently has hardcoded infrastructure (HTTP client) and no way for users to extend functionality. The plugin system enables:

1. **Extensibility**: Users can create custom business logic and infrastructure
2. **Reusability**: Share tasks across projects and with community
3. **Composability**: Tasks can depend on and use other tasks
4. **Modularity**: Clean separation between engine and extensions

### Design Goals

- **Simple Mental Model**: Clear hierarchy: Plugin → Tasks
- **Type-Safe**: Compile-time linking, typed inputs/outputs optional
- **Flexible**: Support lightweight scripts and complex modules
- **Composable**: Tasks can depend on other tasks through DI
- **Production-Ready**: Single binary output, no runtime dependencies
- **Zero Bloat**: Only compiled plugins included in binary

---

## Core Principles

### 1. Plugin = Container of Tasks

**Plugin**: Go struct with multiple public methods
**Task**: Individual public method on the plugin
**Task Name**: `plugin_name.method_name` (e.g., `email.validate`, `email.send`)

```go
// Plugin: EmailPlugin
type EmailPlugin struct {
    smtp *SMTPClient  // Shared state
}

// Task: email.validate
func (e *EmailPlugin) Validate(exec *Execution, input ValidateInput) (ValidateOutput, error) {}

// Task: email.send
func (e *EmailPlugin) Send(exec *Execution, input SendInput) (SendOutput, error) {}

// Task: email.format
func (e *EmailPlugin) Format(exec *Execution, input FormatInput) (FormatOutput, error) {}
```

**Benefits:**
- Shared state across tasks (one plugin instance)
- Natural Go pattern (struct with methods)
- Automatic task discovery (CLI finds all public methods)
- Clear namespacing in flows

### 2. Optional Interface Pattern

Plugins implement only the interfaces they need:

```go
// Optional: Plugin needs lifecycle management
type Lifecycle interface {
    Initialize(ctx context.Context) error
    Shutdown(ctx context.Context) error
}

// Optional: Plugin needs health monitoring
type HealthChecker interface {
    HealthCheck() error
}

// Optional: Plugin declares required environment variables
type EnvVarProvider interface {
    RequiredEnvVars() []string
}
```

**Why this pattern?**
- Simple: Basic plugins need no special methods
- Flexible: Infrastructure plugins add lifecycle as needed
- Discoverable: Engine detects capabilities via type assertions
- Per-plugin: Lifecycle called once per plugin, not per task

### 3. Type-Based Dependency Injection

Plugins declare dependencies via struct fields. Container injects them automatically.

```go
type PaymentPlugin struct {
    http  *HTTPPlugin      // ← Engine detects and injects
    cache *CachePlugin     // ← Engine detects and injects
}
```

**Why type-based?**
- Go-idiomatic: Uses struct composition pattern
- Explicit: Dependencies visible in type definition
- Simple: No magic strings or reflection at runtime
- Compile-safe: Wrong types = compile error

### 4. Automatic Type Detection

CLI automatically detects plugin type - no manual specification:

```yaml
plugins:
  - source: http              # Auto-detected: core plugin
  - source: ./plugins/payment # Auto-detected: local module (must have go.mod)
  - source: github.com/org/email-plugin  # Auto-detected: remote module
```

**Detection rules:**
- Core plugin: shorthand name (http, redis, postgres)
- Local module: path with go.mod (required)
- Remote module: URL pattern (GitHub, etc.)

### 5. Compile-Time Composition

No runtime plugin loading. All plugins compiled into single binary.

**Benefits:**
- Security: No dynamic code execution
- Performance: No runtime overhead
- Reliability: All dependencies resolved at build time
- Deployment: Single binary, no dependencies
- **Size**: Only imported plugins included

---

## Architecture

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                        User Project                          │
│  ┌────────────────┬──────────────────┬────────────────────┐ │
│  │  flow.yaml     │ flow-config.yaml │  plugins/          │ │
│  │  (what to do)  │ (how to build)   │  (custom plugins)  │ │
│  └────────────────┴──────────────────┴────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │   SFlowG CLI     │
                    │                  │
                    │  • Parse configs │
                    │  • Auto-detect   │
                    │  • Generate code │
                    │  • Build binary  │
                    └──────────────────┘
                              │
                              ▼
              ┌──────────────────────────────┐
              │   Temp Build Workspace       │
              │                              │
              │  ┌────────────────────────┐ │
              │  │  go.mod (generated)    │ │
              │  │  - sflowg/runtime      │ │
              │  │  - core plugins        │ │
              │  │  - replace directives  │ │
              │  └────────────────────────┘ │
              │                              │
              │  ┌────────────────────────┐ │
              │  │  main.go (generated)   │ │
              │  │  - Import plugins      │ │
              │  │  - Discover tasks      │ │
              │  │  - Setup DI            │ │
              │  └────────────────────────┘ │
              └──────────────────────────────┘
                              │
                              ▼ go build
                    ┌──────────────────┐
                    │  Single Binary   │
                    │                  │
                    │  • SFlowG runtime│
                    │  • Used plugins  │
                    │  • Flow configs  │
                    └──────────────────┘
```

### Runtime Architecture

```
┌────────────────────────────────────────────────────────┐
│                    Execution                            │
│  • ID, Values (flow state)                             │
│  • Container reference                                  │
│  • Implements context.Context                          │
└─────────────────────┬──────────────────────────────────┘
                      │
                      ▼
        ┌─────────────────────────────┐
        │       Container              │
        │                              │
        │  Plugins: map[string]any     │
        │    "http" → HTTPPlugin       │
        │    "payment" → PaymentPlugin │
        │                              │
        │  Tasks: map[string]Executor  │
        │    "http.request" → executor │
        │    "http.upload" → executor  │
        │    "payment.charge" → exec   │
        │                              │
        │  Manages:                    │
        │  • Plugin registration       │
        │  • Task discovery            │
        │  • Dependency injection      │
        │  • Lifecycle (connect/close) │
        └──────────────────────────────┘
                      │
                      ▼
           ┌────────────────────┐
           │   Plugin Instances  │
           └────────────────────┘
            /        |          \
           /         |           \
   ┌──────▼──┐  ┌──▼───┐  ┌────▼────┐
   │ HTTP    │  │ Redis│  │ Payment │
   │ Plugin  │  │Plugin│  │ Plugin  │
   │ • Request│  │ • Get│  │ • Charge│
   │ • Upload │  │ • Set│  │ • Refund│
   └─────────┘  └──────┘  └─────────┘
```

---

## Domain Model: Plugins & Tasks

### Terminology

| Term | Definition | Example |
|------|------------|---------|
| **Plugin** | Go struct with multiple tasks | `EmailPlugin`, `PaymentPlugin` |
| **Task** | Individual public method on plugin | `Validate()`, `Send()`, `CreateCharge()` |
| **Task Name** | Namespaced identifier in flows | `email.validate`, `payment.charge` |
| **Container** | Registry of plugin instances and task executors | Manages lifecycle |
| **Execution** | Runtime context for task execution | Passed to every task |

### Plugin Structure

```go
// Plugin = struct with shared state and multiple tasks
type EmailPlugin struct {
    // Shared state across all tasks
    smtp    *SMTPClient
    http    *HTTPClient  // Dependencies injected here
}

// Task 1: email.validate
func (e *EmailPlugin) Validate(exec *Execution, input ValidateInput) (ValidateOutput, error) {
    // Use shared state
    return ValidateOutput{Valid: true}, nil
}

// Task 2: email.send
func (e *EmailPlugin) Send(exec *Execution, input SendInput) (SendOutput, error) {
    // Another task using same plugin instance
    return SendOutput{MessageID: "123"}, nil
}

// Task 3: email.format
func (e *EmailPlugin) Format(exec *Execution, input FormatInput) (FormatOutput, error) {
    return FormatOutput{Formatted: "..."}, nil
}

// Optional: Lifecycle management (called once per plugin)
func (e *EmailPlugin) Initialize(ctx context.Context) error {
    e.smtp = NewSMTPClient()
    return e.smtp.Connect()
}

func (e *EmailPlugin) Shutdown(ctx context.Context) error {
    return e.smtp.Close()
}
```

### Task Signatures

CLI auto-discovers methods with valid signatures:

```go
// Signature 1: Untyped (map-based)
func (p *Plugin) TaskName(exec *Execution, args map[string]any) (map[string]any, error)

// Signature 2: Typed (recommended for complex logic)
func (p *Plugin) TaskName(exec *Execution, input TInput) (TOutput, error)
```

### Container Registration

```go
// Container.RegisterPlugin discovers and registers all tasks
func (c *Container) RegisterPlugin(name string, plugin any) error {
    c.plugins[name] = plugin

    // Auto-discover all public methods
    tasks := discoverTasks(plugin)

    for _, task := range tasks {
        taskName := fmt.Sprintf("%s.%s", name, task.MethodName)
        executor := createTaskExecutor(plugin, task)
        c.tasks[taskName] = executor
    }

    // Check optional interfaces (once per plugin)
    if lifecycle, ok := plugin.(Lifecycle); ok {
        c.lifecycleTasks = append(c.lifecycleTasks, lifecycle)
    }

    return nil
}
```

### Usage in Flows

```yaml
# flow.yaml
steps:
  - id: validate
    type: email.validate    # plugin_name.task_name
    args:
      email: ${ request.body.email }

  - id: send
    type: email.send
    args:
      to: ${ request.body.email }
      subject: "Welcome"
```

---

## Dependency Injection

### The Problem

Plugins need to reuse other plugins:

```yaml
# flow.yaml
steps:
  - type: payment.charge
    args:
      amount: 100
```

Internally, `payment.charge` needs:
- HTTP plugin (to call Stripe API)
- Cache plugin (to cache exchange rates)
- Database plugin (to record transaction)

### Solution: Type-Based Field Injection

**Phase 1: Plugins can access core infrastructure**

```go
type PaymentPlugin struct {
    // No dependencies - Phase 1
}

func (p *PaymentPlugin) Charge(exec *Execution, args map[string]any) (map[string]any, error) {
    // Manual lookup via container
    httpPlugin := exec.Container.GetPlugin("http").(*HTTPPlugin)

    // Call task
    result, _ := httpPlugin.Request(exec, map[string]any{
        "url": "https://api.stripe.com/charges",
    })

    return result, nil
}
```

**Phase 2: Convention-Based Dependency Injection**

**Dependency Detection Rules:**

The CLI automatically detects dependencies using a convention-based approach:

1. **Field must be exported** (starts with capital letter)
2. **Field type must be pointer to registered plugin** (e.g., `*HTTPPlugin`)
3. **Field name matches plugin instance name** (case-insensitive)

**Example:**

```go
type PaymentPlugin struct {
    http  *HTTPPlugin   // ← Auto-injected (matches plugin "http")
    redis *RedisPlugin  // ← Auto-injected (matches plugin "redis")

    // Non-plugin fields (ignored by DI)
    apiKey string
    client *http.Client
    mu     sync.Mutex
}
```

**Matching Logic:**

1. CLI analyzes `PaymentPlugin` struct fields via go/ast
2. For field `http *HTTPPlugin`:
   - Is `*HTTPPlugin` a registered plugin type? ✅ Yes
   - Field name "http" → look for plugin instance named "http" ✅ Found
   - Inject `httpPlugin` instance
3. For field `apiKey string`:
   - Is `string` a plugin type? ❌ No
   - Ignore field

**Multiple Instances:**

When multiple instances of same plugin type exist, use explicit field names:

```yaml
# flow-config.yaml
plugins:
  - source: redis
    name: redis_cache
  - source: redis
    name: redis_sessions
```

```go
type MyPlugin struct {
    redis_cache    *RedisPlugin  // Matches "redis_cache"
    redis_sessions *RedisPlugin  // Matches "redis_sessions"
}
```

**When Tag Is Needed (Optional):**

Use `inject` tag only when field name doesn't match plugin instance name:

```go
type MyPlugin struct {
    cache *RedisPlugin `inject:"redis_cache"`  // Field name != plugin name
}
```

**Tag Format:**
```go
`inject:"plugin_instance_name"`
```

### Injection Algorithm

**Step 1: Analyze Plugin Types (CLI)**

```go
type PluginMetadata struct {
    Name         string
    ImportPath   string
    TypeName     string
    Tasks        []TaskMetadata
    Dependencies []Dependency
}

type Dependency struct {
    FieldName    string
    PluginType   string  // e.g., "*HTTPPlugin"
    PluginName   string  // e.g., "http"
}
```

**Step 2: Build Dependency Graph (CLI)**

```go
func buildDependencyGraph(plugins []PluginMetadata) (*Graph, error) {
    graph := NewGraph()

    for _, plugin := range plugins {
        graph.AddNode(plugin.Name)

        for _, dep := range plugin.Dependencies {
            graph.AddEdge(plugin.Name, dep.PluginName)
        }
    }

    // Detect cycles
    if graph.HasCycle() {
        return nil, errors.New("circular dependency detected")
    }

    return graph, nil
}
```

**Step 3: Generate Initialization Code (CLI)**

```go
// CLI generates main.go with proper initialization order
func main() {
    container := runtime.NewContainer()

    // Initialize in dependency order (topological sort)
    httpPlugin := &http.HTTPPlugin{}
    container.RegisterPlugin("http", httpPlugin)

    cachePlugin := &cache.CachePlugin{}
    container.RegisterPlugin("cache", cachePlugin)

    // PaymentPlugin depends on http and cache - inject them
    paymentPlugin := &payment.PaymentPlugin{
        http:  httpPlugin,   // ← Inject
        cache: cachePlugin,  // ← Inject
    }
    container.RegisterPlugin("payment", paymentPlugin)

    app := runtime.NewApp(container)
    app.Run(":8080")
}
```

---

## Plugin Types & Distribution

### Auto-Detection Logic

CLI automatically determines plugin type from `source` field:

```go
func detectPluginType(source string) (PluginType, string) {
    // Type 1: Local path (starts with ./ or /)
    if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "/") {
        if fileExists(filepath.Join(source, "go.mod")) {
            return LocalModule, filepath.Base(source)
        }
        return "", fmt.Errorf("local plugin at %s must have go.mod file", source)
    }

    // Type 2: Core plugin shorthand (single word, no slashes)
    if !strings.Contains(source, "/") && isCorePlugin(source) {
        return CorePlugin, source
    }

    // Type 3: Remote module (URL pattern)
    return RemoteModule, inferNameFromPath(source)
}
```

### Type 1: Core Plugin (Shorthand)

**Configuration:**
```yaml
plugins:
  - source: http              # Shorthand
    version: v1.2.0           # Optional

  - source: redis             # Latest version
```

**CLI Expansion:**
```go
// Expands to:
source: github.com/sflowg/sflowg/plugins/http
version: v1.2.0  // or latest compatible
```

**Benefits:**
- ✅ Concise configuration
- ✅ Official, well-maintained
- ✅ Version controlled
- ✅ Zero bloat (selective import)

### Type 2: Remote Module

**Configuration:**
```yaml
plugins:
  - source: github.com/myorg/email-plugin
    version: v2.0.0  # Optional
```

**CLI Handling:**
1. Adds to go.mod: `require github.com/myorg/email-plugin v2.0.0`
2. Runs `go get github.com/myorg/email-plugin@v2.0.0`
3. Generates import: `import email "github.com/myorg/email-plugin"`

**Benefits:**
- ✅ Version controlled
- ✅ Community shareable
- ✅ Can have dependencies
- ✅ Reusable across projects

### Type 3: Local Module

**Configuration:**
```yaml
plugins:
  - source: ./plugins/payment
```

**CLI Handling:**
1. Detects `go.mod` exists in directory (required)
2. Adds replace directive: `replace payment => /absolute/path/to/plugins/payment`
3. Generates import: `import "payment"`

**Benefits:**
- ✅ Project-specific logic
- ✅ Can have dependencies
- ✅ Private, not published
- ✅ Can reference local files
- ✅ Standard Go module structure

**Requirements:**
- ❗ Must have `go.mod` file in plugin directory
- ❗ Use `go mod init plugin_name` to create module

### Simplified Configuration

```yaml
# Minimal - everything auto-detected
plugins:
  - source: http                    # Auto: core plugin
  - source: redis                   # Auto: core plugin
  - source: ./plugins/payment       # Auto: local module (requires go.mod)
  - source: github.com/org/plugin   # Auto: remote module

# With optional overrides
plugins:
  - source: http
    name: my_http                   # Override inferred name
    version: v1.1.0                 # Specific version
```

---

## Core Plugins Structure

### Monorepo Layout

```
github.com/sflowg/sflowg/
├── runtime/                        # Go module: github.com/sflowg/sflowg/runtime
│   ├── go.mod
│   ├── container.go
│   ├── execution.go
│   └── app.go
│
├── plugins/                        # Each plugin is its own module
│   ├── http/
│   │   ├── go.mod                  # module: github.com/sflowg/sflowg/plugins/http
│   │   ├── plugin.go               # HTTPPlugin with Request, Upload, etc.
│   │   └── client.go
│   ├── redis/
│   │   ├── go.mod                  # module: github.com/sflowg/sflowg/plugins/redis
│   │   ├── plugin.go               # RedisPlugin with Get, Set, etc.
│   │   └── pool.go
│   ├── postgres/
│   │   ├── go.mod                  # module: github.com/sflowg/sflowg/plugins/postgres
│   │   ├── plugin.go               # PostgresPlugin with Query, Execute, etc.
│   │   └── connection.go
│   └── kafka/
│       ├── go.mod                  # module: github.com/sflowg/sflowg/plugins/kafka
│       ├── plugin.go               # KafkaPlugin with Publish, Subscribe, etc.
│       └── consumer.go
│
└── cli/                            # Go module: github.com/sflowg/sflowg/cli
    ├── go.mod
    └── main.go
```

### Separate Module Benefits

**Key Benefits:**
- **Clean go.mod**: Each plugin declares only its own dependencies
- **No dependency pollution**: HTTP plugin's dependencies don't affect Redis plugin
- **Independent versioning**: Each plugin can evolve separately
- **Selective compilation**: Go only compiles what you import

```go
// Only import needed plugins
import (
    "github.com/sflowg/sflowg/plugins/http"    // ✅ Only http and its deps
    "github.com/sflowg/sflowg/plugins/redis"   // ✅ Only redis and its deps
    // postgres NOT imported = NOT in binary ✅
)
```

### Core Plugin Expansion

**User config:**
```yaml
plugins:
  - source: http
  - source: redis
```

**CLI generates:**
```go
import (
    "github.com/sflowg/sflowg/plugins/http"
    "github.com/sflowg/sflowg/plugins/redis"
)

func main() {
    httpPlugin := &http.HTTPPlugin{}
    redisPlugin := &redis.RedisPlugin{}
    // postgres and kafka NOT imported = NOT in binary
}
```

---

## CLI Build System

### Complete Build Flow

```
User: sflowg build ./my-project
         │
         ▼
┌──────────────────────┐
│ 1. Parse Configs     │
│  • flow.yaml         │
│  • flow-config.yaml  │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 2. Auto-Detect Types │
│  For each plugin:    │
│   • Core shorthand?  │
│   • .go file?        │
│   • Directory?       │
│   • URL pattern?     │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 3. Expand Core       │
│  http → full path    │
│  redis → full path   │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 4. Create Temp Dir   │
│  /tmp/sflowg-xyz/    │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 5. Init go.mod       │
│  module sflowg-xyz   │
│  require runtime     │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 6. Process Plugins   │
│  Remote: require     │
│  Local: replace      │
│  Core: require       │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 7. Analyze Deps      │
│  • Parse AST         │
│  • Build dep graph   │
│  • Detect cycles     │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 8. Generate main.go  │
│  • Selective imports │
│  • Task discovery    │
│  • DI wiring         │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 9. Download Deps     │
│  go mod download     │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 10. Build Binary     │
│  go build -o binary  │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 11. Move Binary      │
│  cp to user project  │
└──────────────────────┘
         │
         ▼
┌──────────────────────┐
│ 12. Cleanup          │
│  rm -rf /tmp/...     │
└──────────────────────┘
```

### Auto-Detection Implementation

```go
func (cli *CLI) detectAndExpandPlugins(configs []PluginConfig) ([]ExpandedPlugin, error) {
    expanded := make([]ExpandedPlugin, 0)

    for _, cfg := range configs {
        plugin := ExpandedPlugin{
            Source:  cfg.Source,
            Version: cfg.Version,
            Name:    cfg.Name,
        }

        // Auto-detect type
        switch {
        case strings.HasPrefix(cfg.Source, "./") || strings.HasPrefix(cfg.Source, "/"):
            // Local path - must have go.mod
            if !fileExists(filepath.Join(cfg.Source, "go.mod")) {
                return nil, fmt.Errorf("local plugin at %s must have go.mod file", cfg.Source)
            }
            plugin.Type = LocalModule
            if plugin.Name == "" {
                plugin.Name = filepath.Base(cfg.Source)
            }

        case !strings.Contains(cfg.Source, "/"):
            // Single word - check if core plugin
            if isCorePlugin(cfg.Source) {
                plugin.Type = CorePlugin
                plugin.Source = fmt.Sprintf("github.com/sflowg/sflowg/plugins/%s", cfg.Source)
                if plugin.Version == "" {
                    plugin.Version = getLatestCompatibleVersion(cfg.Source)
                }
                if plugin.Name == "" {
                    plugin.Name = cfg.Source
                }
            } else {
                return nil, fmt.Errorf("unknown plugin: %s", cfg.Source)
            }

        default:
            // URL pattern
            plugin.Type = RemoteModule
            if plugin.Name == "" {
                plugin.Name = inferNameFromPath(cfg.Source)
            }
        }

        expanded = append(expanded, plugin)
    }

    return expanded, nil
}

var corePlugins = []string{"http", "redis", "postgres", "kafka", "mongodb"}

func isCorePlugin(name string) bool {
    for _, cp := range corePlugins {
        if name == cp {
            return true
        }
    }
    return false
}
```

### Generated Code Example

**User's flow-config.yaml:**
```yaml
plugins:
  - source: http
  - source: redis
  - source: ./plugins/payment
```

**Generated go.mod:**
```go
module sflowg-build-abc123

go 1.24

require (
    github.com/sflowg/sflowg/runtime v0.1.0
    github.com/sflowg/sflowg/plugins v0.1.0  // One module for all core
)

replace payment => /Users/you/my-project/plugins/payment
```

**Generated main.go:**
```go
package main

import (
    "log"
    "github.com/sflowg/sflowg/runtime"

    // Core plugins - selective import
    "github.com/sflowg/sflowg/plugins/http"
    "github.com/sflowg/sflowg/plugins/redis"
    // postgres, kafka NOT imported = NOT in binary

    // User plugins
    "payment"
)

func main() {
    container := runtime.NewContainer()

    // Register core plugins
    httpPlugin := &http.HTTPPlugin{}
    container.RegisterPlugin("http", httpPlugin)
    // CLI discovers: http.request, http.upload, http.download

    redisPlugin := &redis.RedisPlugin{}
    container.RegisterPlugin("redis", redisPlugin)
    // CLI discovers: redis.get, redis.set, redis.delete

    // Register user plugins with DI
    paymentPlugin := &payment.PaymentPlugin{
        http:  httpPlugin,   // Injected
        redis: redisPlugin,  // Injected
    }
    container.RegisterPlugin("payment", paymentPlugin)
    // CLI discovers: payment.charge, payment.refund

    app := runtime.NewApp(container)
    if err := app.LoadFlow("./flows/flow.yaml"); err != nil {
        log.Fatal(err)
    }
    app.Run(":8080")
}
```

---

## Configuration

**Full Design**: See [[PLUGIN_CONFIG_FINAL]] for the complete tag-based configuration system with framework utilities.

**Phase 1 (MVP)**: Simple hardcoded environment variables without AST parsing
**Phase 2**: Full tag-based system with defaults, validation, and AST-based code generation

This section describes the **Phase 1 MVP approach** for configuration. The advanced tag-based system is deferred to Phase 2.

---

### Phase 1: Simple Environment Variable Configuration

In the MVP, plugins use Config structs with basic tags (no AST parsing yet, simplified framework utilities):

**Plugin Side:**
```go
// plugins/redis/config.go
package redis

import "time"

// Config defines redis plugin configuration using declarative tags.
// Framework will handle defaults and validation.
type Config struct {
    Addr     string        `yaml:"addr" default:"localhost:6379" validate:"required"`
    Password string        `yaml:"password"`  // Optional
    DB       int           `yaml:"db" default:"0" validate:"gte=0,lte=15"`
    PoolSize int           `yaml:"pool_size" default:"10" validate:"gte=1,lte=1000"`
    Timeout  time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s"`
}

// No DefaultConfig() needed - framework handles defaults via tags
// No Validate() method needed - framework validates via tags
```

**User Configuration:**
```yaml
# flow-config.yaml
plugins:
  - source: redis
    # No config section needed in Phase 1
    # Plugin reads from hardcoded env var names
```

**Generated Code:**
```go
// main.go (generated)
func main() {
    container := runtime.NewContainer()

    // Framework applies defaults and validates
    redisConfig := redis.Config{}
    if err := runtime.PrepareConfig(&redisConfig); err != nil {
        log.Fatalf("Redis config preparation failed: %v", err)
    }

    // Apply env var overrides
    if addr := os.Getenv("REDIS_ADDR"); addr != "" {
        redisConfig.Addr = addr
    }
    if password := os.Getenv("REDIS_PASSWORD"); password != "" {
        redisConfig.Password = password
    }

    // Validate after overrides
    if err := runtime.ValidateConfig(redisConfig); err != nil {
        log.Fatalf("Redis config invalid: %v", err)
    }

    redisPlugin := redis.NewPlugin(redisConfig)
    container.RegisterPlugin("redis", redisPlugin)

    // ... rest of initialization
}
```

**Phase 1 Limitations:**
- ❌ No AST parsing of Config structs (manual code generation)
- ❌ No user-configurable env var mappings (hardcoded names in generated code)
- ❌ Simplified ApplyEnvOverrides (direct field assignment, not reflection-based)
- ✅ Uses tag-based config structs (defaults, validation)
- ✅ Framework utilities work (PrepareConfig, ValidateConfig)
- ✅ Simple to implement
- ✅ Works for MVP use cases

**Phase 2 Enhancement:**
The full tag-based system (described in PLUGIN_CONFIG_FINAL.md) will be added in Phase 2, including:
- Declarative tags (`default`, `validate`, `yaml`)
- Framework utilities (ApplyDefaults, ValidateConfig, ApplyEnvOverrides)
- AST-based Config struct analysis
- User-configurable env var mappings in flow-config.yaml

---

### Type Handling in YAML

#### Approach: Simple YAML Unmarshaling

Plugin configurations are parsed using standard Go YAML unmarshaling with type inference:

```go
// CLI reads flow-config.yaml
type PluginConfig struct {
    Source  string
    Name    string
    Version string
    Config  map[string]any  // YAML → map[string]any
}
```

**Type Conversion:**

YAML parser automatically converts types:
- `addr: localhost:6379` → `string`
- `db: 0` → `int`
- `pool_size: 10` → `int`
- `timeout: 30s` → `string` (parsed by plugin via `time.ParseDuration`)
- `enabled: true` → `bool`
- `brokers: [...]` → `[]any`

**Plugin Responsibility:**

Each plugin's `Config` struct defines types with YAML tags:

```go
type Config struct {
    Addr     string        `yaml:"addr"`      // YAML string → Go string
    DB       int           `yaml:"db"`        // YAML int → Go int
    PoolSize int           `yaml:"pool_size"` // YAML int → Go int
    Timeout  time.Duration `yaml:"timeout"`   // YAML string → parse to Duration
}
```

**CLI Generation:**

CLI generates initialization code that constructs the config struct:

```go
// Generated by CLI
redisConfig := redis.Config{
    Addr:     getEnvOrDefault("REDIS_ADDR", "localhost:6379"),  // string
    DB:       0,                                                  // int
    PoolSize: 10,                                                 // int
    Timeout:  30 * time.Second,                                   // Duration literal
}
```

**Duration Handling:**

For duration fields, CLI detects `time.Duration` type and:
1. If value is string like "30s", generates: `30 * time.Second`
2. If value is int, treats as seconds: `30 * time.Second`

**No Runtime Type Conversion:**

All type handling happens at CLI generation time, not runtime. The generated `main.go` contains properly-typed Go code.

---

### CLI Side: Generate Configuration Code

**Note:** This section describes the **Phase 2 approach** with AST-based analysis. For Phase 1, the CLI generates simpler code using `DefaultConfig()` and hardcoded `os.Getenv()` calls as shown in the Phase 1 section above.

#### Discovery Process (Phase 2)

```bash
$ sflowg build
```

CLI analyzes each plugin:
1. Finds `Config` struct in plugin package using `go/ast`
2. Extracts field names, types, and YAML tags
3. Matches config values from `flow-config.yaml`
4. Generates initialization code with env var substitution

#### Generated main.go

```go
// main.go (GENERATED by CLI)
package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/sflowg/sflowg"
    "github.com/sflowg/sflowg/plugins/redis"
    "github.com/sflowg/sflowg/plugins/postgres"
    "github.com/sflowg/sflowg/plugins/kafka"
    "github.com/sflowg/sflowg/plugins/http"
)

func main() {
    container := sflowg.NewContainer()

    // ===== Redis Plugin Configuration =====
    redisConfig := redis.Config{
        Addr:     getEnvOrDefault("REDIS_ADDR", "localhost:6379"),
        Password: mustGetEnv("REDIS_PASSWORD"),
        DB:       0,
        PoolSize: 10,
        Timeout:  30 * time.Second,
    }
    redisPlugin := redis.NewPlugin(redisConfig)
    container.RegisterPlugin("redis", redisPlugin)

    // ===== Postgres Plugin Configuration =====
    postgresConfig := postgres.Config{
        DSN:             mustGetEnv("DATABASE_URL"),
        MaxOpenConns:    25,
        MaxIdleConns:    5,
        ConnMaxLifetime: 1 * time.Hour,
    }
    postgresPlugin := postgres.NewPlugin(postgresConfig)
    container.RegisterPlugin("postgres", postgresPlugin)

    // ===== Kafka Plugin Configuration =====
    kafkaConfig := kafka.Config{
        Brokers: []string{
            getEnvOrDefault("KAFKA_BROKER_1", "localhost:9092"),
            getEnvOrDefault("KAFKA_BROKER_2", "localhost:9093"),
        },
        GroupID: "payment-processor",
        Security: kafka.SecurityConfig{
            Protocol: "SASL_SSL",
            Username: mustGetEnv("KAFKA_USERNAME"),
            Password: mustGetEnv("KAFKA_PASSWORD"),
        },
        Topics: []string{"payments", "notifications"},
    }
    kafkaPlugin := kafka.NewPlugin(kafkaConfig)
    container.RegisterPlugin("kafka", kafkaPlugin)

    // ===== HTTP Plugin (no config, uses defaults) =====
    httpPlugin := http.NewPlugin(http.DefaultConfig())
    container.RegisterPlugin("http", httpPlugin)

    // Initialize all plugins (fail-fast)
    ctx := context.Background()
    if err := container.Initialize(ctx); err != nil {
        log.Fatal("Plugin initialization failed: ", err)
    }

    // Setup graceful shutdown
    setupGracefulShutdown(container)

    // Start application
    app := sflowg.NewApp(container)
    if err := app.Run(); err != nil {
        log.Fatal("Application failed: ", err)
    }
}

// Helper functions generated by CLI
func mustGetEnv(key string) string {
    val := os.Getenv(key)
    if val == "" {
        log.Fatalf("Required environment variable %s not set", key)
    }
    return val
}

func getEnvOrDefault(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

func setupGracefulShutdown(container *sflowg.Container) {
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    go func() {
        <-sigChan
        log.Println("Shutting down gracefully...")

        shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        if err := container.Shutdown(shutdownCtx); err != nil {
            log.Printf("Shutdown error: %v", err)
        }

        os.Exit(0)
    }()
}
```

---

### Configuration Validation

#### Validation Timing

1. **Compile-time**: CLI validates config structure matches plugin Config struct
2. **Startup-time**: Framework validates config via tags BEFORE calling `Initialize()`
3. **Initialize-time**: Plugin tests connections and performs integration checks
4. **Runtime**: Tasks validate inputs using validator library

#### Plugin Validation Example

**Config Definition with Validation Tags:**
```go
// plugins/postgres/config.go
type Config struct {
    DSN             string        `yaml:"dsn" validate:"required,dsn"`
    MaxOpenConns    int           `yaml:"max_open_conns" default:"25" validate:"gte=1,lte=1000"`
    MaxIdleConns    int           `yaml:"max_idle_conns" default:"5" validate:"gte=1,ltefield=MaxOpenConns"`
    ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" default:"1h" validate:"gte=0"`
}
```

**Plugin Initialize (No Manual Validation):**
```go
func (p *PostgresPlugin) Initialize(ctx context.Context) error {
    // Config already validated by framework before Initialize is called
    // No need to check DSN, MaxOpenConns, MaxIdleConns - framework did it

    // Open database connection
    db, err := sql.Open("postgres", p.config.DSN)
    if err != nil {
        return fmt.Errorf("failed to open database: %w", err)
    }

    // Apply configuration (already validated)
    db.SetMaxOpenConns(p.config.MaxOpenConns)
    db.SetMaxIdleConns(p.config.MaxIdleConns)
    db.SetConnMaxLifetime(p.config.ConnMaxLifetime)

    // Test connection (fail-fast)
    if err := db.PingContext(ctx); err != nil {
        return fmt.Errorf("database connection test failed: %w", err)
    }

    p.db = db
    return nil
}
```

**Error Output at Startup:**
```bash
$ ./sflowg
2024/01/15 10:30:00 Plugin initialization failed: postgres DSN is required
```

---

### Multiple Instances

Same plugin can be instantiated multiple times with different configs:

```yaml
plugins:
  # Cache instance
  - source: redis
    name: redis_cache
    config:
      addr: localhost:6379
      db: 0  # Database 0 for cache

  # Session instance
  - source: redis
    name: redis_sessions
    config:
      addr: localhost:6379
      db: 1  # Database 1 for sessions
```

**Usage in flows:**
```yaml
steps:
  - name: get_from_cache
    task: redis_cache.Get
    args: {key: "user:123"}

  - name: get_session
    task: redis_sessions.Get
    args: {key: "session:abc"}
```

**Generated code:**
```go
// ===== Redis Cache Instance =====
redisCacheConfig := redis.Config{}
runtime.PrepareConfig(&redisCacheConfig)  // Apply defaults from tags
if addr := os.Getenv("REDIS_CACHE_ADDR"); addr != "" {
    redisCacheConfig.Addr = addr
}
redisCacheConfig.DB = 0  // Literal from flow-config.yaml
if err := runtime.ValidateConfig(redisCacheConfig); err != nil {
    log.Fatalf("Redis cache config invalid: %v", err)
}
redisCachePlugin := redis.NewPlugin(redisCacheConfig)
container.RegisterPlugin("redis_cache", redisCachePlugin)

// ===== Redis Sessions Instance =====
redisSessionsConfig := redis.Config{}
runtime.PrepareConfig(&redisSessionsConfig)
if addr := os.Getenv("REDIS_SESSIONS_ADDR"); addr != "" {
    redisSessionsConfig.Addr = addr
}
redisSessionsConfig.DB = 1  // Literal from flow-config.yaml
if err := runtime.ValidateConfig(redisSessionsConfig); err != nil {
    log.Fatalf("Redis sessions config invalid: %v", err)
}
redisSessionsPlugin := redis.NewPlugin(redisSessionsConfig)
container.RegisterPlugin("redis_sessions", redisSessionsPlugin)
```

---

### Field Reference

| Field | Required | Description | Default |
|-------|----------|-------------|---------|
| `source` | ✅ Yes | Plugin location/name | N/A |
| `name` | ❌ No | Plugin instance name | Inferred from source |
| `version` | ❌ No | Plugin version (remote/core) | "latest" |
| `config` | ❌ No | Plugin-specific configuration | Plugin's DefaultConfig() |

---

### Detection Examples

```yaml
# Example 1: Core plugin with config
- source: redis
  config:
    addr: ${REDIS_ADDR:localhost:6379}
    password: ${REDIS_PASSWORD}
# Detected: type=core, name=redis, expands to github.com/sflowg/sflowg/plugins/redis

# Example 2: Core with version and config
- source: postgres
  version: v1.2.0
  config:
    dsn: ${DATABASE_URL}
# Detected: type=core, name=postgres, version=v1.2.0

# Example 3: Local module with config (requires go.mod)
- source: ./plugins/payment
  config:
    api_key: ${PAYMENT_API_KEY}
    base_url: https://api.payment.com
# Detected: type=local-module (must have go.mod), name=payment

# Example 4: Remote module with config
- source: github.com/myorg/email-plugin
  version: v2.0.0
  config:
    smtp_host: smtp.gmail.com
    smtp_port: 587
# Detected: type=remote, name=email-plugin

# Example 5: Override name with config
- source: redis
  name: custom_redis
  config:
    addr: redis.prod.internal:6379
# Detected: type=core, name=custom_redis (overridden)

# Example 6: No config (uses defaults)
- source: http
# Detected: type=core, name=http, uses http.DefaultConfig()
```

---

### Configuration Best Practices

#### Plugin Developer

1. ✅ **Define Config struct** with declarative tags (`yaml`, `default`, `validate`)
2. ✅ **Use framework validation** - let tags handle validation, not Initialize()
3. ✅ **Document config fields** in struct comments
4. ✅ **Use standard types** (time.Duration, not int seconds)
5. ✅ **Let framework handle** defaults, validation, and env var overrides
6. ✅ **Zero boilerplate** - no DefaultConfig(), no Validate() methods needed

#### User

1. ✅ **Use environment variables** for sensitive data
2. ✅ **Provide defaults** for optional values (`${VAR:default}`)
3. ✅ **Test locally** with .env file
4. ✅ **Document required env vars** in README
5. ✅ **Validate early** - let plugins fail at startup, not runtime

---

## Container & Lifecycle

### Enhanced Container Structure

```go
type Container struct {
    // Plugin instances
    plugins map[string]any

    // Task executors (plugin_name.method_name → executor)
    tasks map[string]TaskExecutor

    // Lifecycle management
    lifecycleTasks []Lifecycle

    // Health monitoring
    healthCheckers []HealthChecker

    // Initialization flag
    initialized bool
}

type TaskExecutor interface {
    Execute(exec *Execution, args map[string]any) (map[string]any, error)
}
```

### Plugin Registration

```go
func (c *Container) RegisterPlugin(pluginName string, plugin any) error {
    c.plugins[pluginName] = plugin

    // Discover all public methods
    methods := reflectPublicMethods(plugin)

    for _, method := range methods {
        if !isValidTaskSignature(method) {
            continue
        }

        taskName := fmt.Sprintf("%s.%s", pluginName, toLowerFirst(method.Name))
        executor := wrapMethod(plugin, method)
        c.tasks[taskName] = executor
    }

    // Check optional interfaces (once per plugin)
    if lifecycle, ok := plugin.(Lifecycle); ok {
        c.lifecycleTasks = append(c.lifecycleTasks, lifecycle)
    }

    if healthChecker, ok := plugin.(HealthChecker); ok {
        c.healthCheckers = append(c.healthCheckers, healthChecker)
    }

    return nil
}
```

### Initialization Sequence

```go
func (c *Container) Initialize(ctx context.Context) error {
    if c.initialized {
        return nil
    }

    // Phase 1: Validate environment variables
    for _, plugin := range c.plugins {
        if envProvider, ok := plugin.(EnvVarProvider); ok {
            required := envProvider.RequiredEnvVars()
            if err := validateEnvVars(required); err != nil {
                return fmt.Errorf("env validation failed: %w", err)
            }
        }
    }

    // Phase 2: Initialize lifecycle plugins
    for _, lifecycle := range c.lifecycleTasks {
        if err := lifecycle.Initialize(ctx); err != nil {
            return fmt.Errorf("initialize failed: %w", err)
        }
    }

    // Phase 3: Health checks
    for _, checker := range c.healthCheckers {
        if err := checker.HealthCheck(); err != nil {
            return fmt.Errorf("health check failed: %w", err)
        }
    }

    c.initialized = true
    return nil
}
```

### Shutdown Sequence

```go
func (c *Container) Shutdown(ctx context.Context) error {
    var errors []error

    // Shutdown in reverse order
    for i := len(c.lifecycleTasks) - 1; i >= 0; i-- {
        if err := c.lifecycleTasks[i].Shutdown(ctx); err != nil {
            errors = append(errors, err)
        }
    }

    if len(errors) > 0 {
        return fmt.Errorf("shutdown errors: %v", errors)
    }

    c.initialized = false
    return nil
}
```

### Error Handling Strategy

#### MVP Approach: Fail-Fast with Panic

For the MVP, SFlowG uses a **fail-fast** error handling approach where critical errors cause the application to panic immediately. This ensures clean failure modes and prevents partial or corrupted state.

**Rationale:**
- **Simplicity**: No complex error recovery logic in MVP
- **Safety**: Prevents running with invalid state or configuration
- **Clarity**: Errors are immediately visible, not hidden
- **Reliability**: Avoids subtle bugs from partial initialization

**When to Panic:**

```go
// 1. Plugin initialization failures
func (p *RedisPlugin) Initialize(ctx context.Context) error {
    if p.config.Addr == "" {
        panic("redis addr is required")  // MVP: fail-fast
    }

    client := redis.NewClient(&redis.Options{Addr: p.config.Addr})
    if err := client.Ping(ctx).Err(); err != nil {
        panic(fmt.Sprintf("redis connection failed: %v", err))  // MVP: fail-fast
    }

    p.client = client
    return nil
}

// 2. Missing required environment variables
func mustGetEnv(key string) string {
    val := os.Getenv(key)
    if val == "" {
        panic(fmt.Sprintf("required environment variable %s not set", key))
    }
    return val
}

// 3. Configuration validation errors
func (c *Container) Initialize(ctx context.Context) error {
    for _, plugin := range c.plugins {
        if err := plugin.Initialize(ctx); err != nil {
            panic(fmt.Sprintf("plugin initialization failed: %v", err))
        }
    }
    return nil
}

// 4. Dependency injection errors (circular dependencies, missing plugins)
func (cli *CLI) buildDependencyGraph(plugins []PluginMetadata) *Graph {
    graph := NewGraph()
    // ... build graph
    if graph.HasCycle() {
        panic("circular dependency detected in plugins")
    }
    return graph
}
```

**When NOT to Panic (Return Errors):**

```go
// 1. Task execution errors (business logic)
func (p *PaymentPlugin) Charge(exec *Execution, input ChargeInput) (ChargeOutput, error) {
    if input.Amount <= 0 {
        return ChargeOutput{}, fmt.Errorf("amount must be positive")  // Return error
    }

    result, err := p.http.Request(exec, requestInput)
    if err != nil {
        return ChargeOutput{}, fmt.Errorf("payment API failed: %w", err)  // Return error
    }

    return ChargeOutput{ChargeID: result.ID}, nil
}

// 2. HTTP request failures during flow execution
func (h *HTTPHandler) executeFlow(c *gin.Context) {
    result, err := h.executor.Execute(exec, flow)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})  // Return error to client
        return
    }
    c.JSON(200, result)
}

// 3. Validation errors in task inputs
func (p *EmailPlugin) Send(exec *Execution, input SendInput) (SendOutput, error) {
    if input.To == "" {
        return SendOutput{}, fmt.Errorf("'to' field is required")  // Return error
    }
    // ... send email
}
```

**Error Handling Decision Tree:**

```
Error occurs
├─ Initialization phase? (startup)
│  └─ YES → PANIC (fail-fast)
│      Examples:
│      - Plugin connection failed
│      - Required env var missing
│      - Invalid configuration
│      - Circular dependencies
│
└─ Runtime phase? (during request)
   └─ YES → RETURN ERROR (handle gracefully)
       Examples:
       - Task execution failed
       - Invalid task input
       - API call failed
       - Business logic error
```

**Future Enhancement (Post-MVP):**

In future versions, graceful error recovery can be added:
- Retry logic for transient failures
- Circuit breakers for external services
- Fallback strategies for non-critical failures
- Error telemetry and monitoring

**Example: MVP Error Flow**

```go
// main.go (generated by CLI)
func main() {
    container := runtime.NewContainer()

    // Register plugins
    httpPlugin := &http.HTTPPlugin{}
    container.RegisterPlugin("http", httpPlugin)

    redisPlugin := redis.NewPlugin(redis.Config{
        Addr: mustGetEnv("REDIS_ADDR"),  // Panics if missing
    })
    container.RegisterPlugin("redis", redisPlugin)

    // Initialize (panics on any failure)
    ctx := context.Background()
    if err := container.Initialize(ctx); err != nil {
        panic(fmt.Sprintf("initialization failed: %v", err))
    }

    // Start application (runtime errors are returned, not panicked)
    app := runtime.NewApp(container)
    if err := app.Run(":8080"); err != nil {
        log.Fatal(err)  // Non-panic for server errors
    }
}
```

**Benefits:**
- ✅ Simple to implement and understand
- ✅ Catches configuration errors immediately
- ✅ No partial initialization state
- ✅ Clear separation: startup errors vs runtime errors
- ✅ Easier debugging (stack traces at failure point)

**Trade-offs:**
- ❌ No graceful degradation for startup issues
- ❌ Requires restart for configuration fixes
- ⚠️ Acceptable for MVP, can be improved later

---

## Validation & Health Checks

### Three-Layer Validation

#### 1. Build-Time (CLI)

```go
func (cli *CLI) validateFlow(flow *Flow, plugins []PluginMetadata) error {
    // Check all tasks used in flow exist
    usedTasks := extractTasksFromFlow(flow)
    availableTasks := extractTasksFromPlugins(plugins)

    for _, taskName := range usedTasks {
        if !contains(availableTasks, taskName) {
            return fmt.Errorf("task '%s' not provided by any plugin", taskName)
        }
    }

    // Check for dependency cycles
    depGraph := buildDependencyGraph(plugins)
    if depGraph.HasCycle() {
        return errors.New("circular dependency detected")
    }

    return nil
}
```

#### 2. Startup (Runtime)

```go
func (a *App) Initialize() error {
    exec := NewExecution(a.flow, a.container)

    // Validate environment
    // Initialize plugins (calls Connect)
    // Run health checks
    if err := a.container.Initialize(&exec); err != nil {
        return fmt.Errorf("initialization failed: %w", err)
    }

    return nil
}
```

#### 3. Task Execution (Runtime)

```go
func (p *EmailPlugin) Validate(exec *Execution, args map[string]any) (map[string]any, error) {
    // Validate inputs
    email, ok := args["email"].(string)
    if !ok {
        return nil, fmt.Errorf("'email' must be a string")
    }

    if email == "" {
        return nil, fmt.Errorf("'email' is required")
    }

    // Business logic
    valid := validateEmail(email)
    return map[string]any{"valid": valid}, nil
}
```

### Health Check Implementation

```go
// HTTP endpoint
func (h *HttpHandler) healthHandler(c *gin.Context) {
    results := make(map[string]string)
    allHealthy := true

    for name, plugin := range h.container.plugins {
        if checker, ok := plugin.(HealthChecker); ok {
            if err := checker.HealthCheck(); err != nil {
                results[name] = fmt.Sprintf("unhealthy: %v", err)
                allHealthy = false
            } else {
                results[name] = "healthy"
            }
        }
    }

    status := http.StatusOK
    if !allHealthy {
        status = http.StatusServiceUnavailable
    }

    c.JSON(status, gin.H{
        "status": map[bool]string{true: "healthy", false: "unhealthy"}[allHealthy],
        "plugins": results,
    })
}
```

---

## Binary Size Optimization

### How Selective Import Works

Go's linker only includes code that's actually imported and used.

**Key principle:** Import specific subpackages, not parent packages.

```go
// ✅ Selective - only http compiled
import "github.com/sflowg/sflowg/plugins/http"

// ❌ All plugins - everything compiled
import "github.com/sflowg/sflowg/plugins"
```

### Real-World Size Examples

#### Scenario 1: HTTP Only

**flow-config.yaml:**
```yaml
plugins:
  - source: http
```

**Generated imports:**
```go
import "github.com/sflowg/sflowg/plugins/http"
```

**Binary breakdown:**
- Runtime: ~3MB
- HTTP plugin: ~500KB
- Resty (HTTP client): ~1.5MB
- **Total: ~5MB**

Dependencies NOT included:
- Redis client: 0 bytes ✅
- Postgres driver: 0 bytes ✅
- Kafka client: 0 bytes ✅

---

#### Scenario 2: HTTP + Redis

**flow-config.yaml:**
```yaml
plugins:
  - source: http
  - source: redis
```

**Generated imports:**
```go
import (
    "github.com/sflowg/sflowg/plugins/http"
    "github.com/sflowg/sflowg/plugins/redis"
)
```

**Binary breakdown:**
- Runtime: ~3MB
- HTTP plugin: ~500KB
- Redis plugin: ~800KB
- Resty: ~1.5MB
- go-redis: ~2MB
- **Total: ~7.8MB**

Dependencies NOT included:
- Postgres driver: 0 bytes ✅
- Kafka client: 0 bytes ✅

---

#### Scenario 3: All Core Plugins

**flow-config.yaml:**
```yaml
plugins:
  - source: http
  - source: redis
  - source: postgres
  - source: kafka
```

**Binary breakdown:**
- Runtime: ~3MB
- HTTP: ~500KB
- Redis: ~800KB
- Postgres: ~1.2MB
- Kafka: ~3MB
- All dependencies: ~6MB
- **Total: ~14.5MB**

---

#### Scenario 4: HTTP + User Plugin

**flow-config.yaml:**
```yaml
plugins:
  - source: http
  - source: ./plugins/payment  # Uses http internally
```

**Binary breakdown:**
- Runtime: ~3MB
- HTTP plugin: ~500KB
- Payment plugin: ~200KB (mostly business logic)
- Resty: ~1.5MB
- Stripe SDK: ~1MB (if used by payment)
- **Total: ~6.2MB**

---

### Size Comparison Table

| Plugins Used | Binary Size | Savings vs All |
|-------------|-------------|----------------|
| http only | ~5MB | -65% |
| http + redis | ~7.8MB | -46% |
| http + redis + postgres | ~11MB | -24% |
| All core (4 plugins) | ~14.5MB | baseline |

### Why This Matters

1. **Faster deployments**: Smaller binaries upload quicker
2. **Less memory**: Smaller binaries use less RAM
3. **Faster startup**: Less code to load
4. **Better caching**: Docker layers smaller

### Best Practices

1. **Only import what you need**: Don't add plugins "just in case"
2. **Local modules for all custom logic**: Use standard go.mod structure
3. **Keep plugins focused**: Single responsibility principle
4. **Monitor binary size**: Track size in CI/CD

---

## Typed Task Support

### Status: Deferred to Phase 2

**Decision**: Typed task support is **NOT included in the MVP** (Phase 1) and will be added in Phase 2.

**MVP Approach (Phase 1)**: All tasks use map-based signatures:

```go
func (p *Plugin) TaskName(exec *Execution, args map[string]any) (map[string]any, error)
```

**Why Defer to Phase 2:**

1. **Simplicity First**: MVP focuses on core plugin system without type conversion complexity
2. **Faster Time-to-Market**: Get plugin system working quickly with simpler approach
3. **Validated Need**: Assess if typed tasks are truly needed based on MVP usage
4. **Complexity**: Type conversion adds runtime overhead and CLI generation complexity
5. **Flexibility**: Users can manually add typed wrappers if needed for specific tasks

**Phase 1 User Experience:**

```go
type PaymentPlugin struct {
    http *HTTPPlugin  // Dependencies still work
}

// All tasks use map-based signature
func (p *PaymentPlugin) Charge(exec *Execution, args map[string]any) (map[string]any, error) {
    // Manual type assertions (verbose but simple)
    amount, ok := args["amount"].(float64)
    if !ok {
        return nil, fmt.Errorf("amount must be a number")
    }

    currency, ok := args["currency"].(string)
    if !ok {
        return nil, fmt.Errorf("currency must be a string")
    }

    // Business logic
    if amount <= 0 {
        return nil, fmt.Errorf("amount must be positive")
    }

    // Use dependencies
    result, err := p.http.Request(exec, map[string]any{
        "url":    "https://api.stripe.com/charges",
        "method": "POST",
        "body": map[string]any{
            "amount":   int(amount * 100),
            "currency": currency,
        },
    })

    if err != nil {
        return nil, fmt.Errorf("payment API failed: %w", err)
    }

    return map[string]any{
        "charge_id": result["body"].(map[string]any)["id"],
        "status":    "succeeded",
    }, nil
}
```

**Manual Validation Pattern (Phase 1):**

```go
func (p *EmailPlugin) Send(exec *Execution, args map[string]any) (map[string]any, error) {
    // Extract and validate required fields
    to, ok := args["to"].(string)
    if !ok || to == "" {
        return nil, fmt.Errorf("'to' field is required and must be a string")
    }

    subject, ok := args["subject"].(string)
    if !ok {
        return nil, fmt.Errorf("'subject' field is required and must be a string")
    }

    body, ok := args["body"].(string)
    if !ok {
        return nil, fmt.Errorf("'body' field is required and must be a string")
    }

    // Business logic
    err := p.smtp.Send(to, subject, body)
    if err != nil {
        return nil, fmt.Errorf("failed to send email: %w", err)
    }

    return map[string]any{
        "message_id": "msg_123",
        "sent":       true,
    }, nil
}
```

**Optional: User-Provided Typed Wrapper (Phase 1):**

Advanced users can create their own typed wrappers if needed:

```go
// Define typed structs
type ChargeInput struct {
    Amount   float64 `json:"amount"`
    Currency string  `json:"currency"`
}

type ChargeOutput struct {
    ChargeID string `json:"charge_id"`
    Status   string `json:"status"`
}

// Internal typed method
func (p *PaymentPlugin) chargeTyped(exec *Execution, input ChargeInput) (ChargeOutput, error) {
    if input.Amount <= 0 {
        return ChargeOutput{}, fmt.Errorf("amount must be positive")
    }

    result, err := p.http.Request(exec, map[string]any{
        "url": "https://api.stripe.com/charges",
        "body": map[string]any{"amount": input.Amount * 100},
    })

    return ChargeOutput{
        ChargeID: result["id"].(string),
        Status:   "succeeded",
    }, nil
}

// Public task - map-based wrapper
func (p *PaymentPlugin) Charge(exec *Execution, args map[string]any) (map[string]any, error) {
    // Manual conversion to typed input
    input := ChargeInput{
        Amount:   args["amount"].(float64),
        Currency: args["currency"].(string),
    }

    output, err := p.chargeTyped(exec, input)
    if err != nil {
        return nil, err
    }

    // Manual conversion to map output
    return map[string]any{
        "charge_id": output.ChargeID,
        "status":    output.Status,
    }, nil
}
```

**Phase 2 Plan: Automatic Typed Task Support**

When added in Phase 2, the CLI will automatically:
1. Detect typed task signatures via AST analysis
2. Generate conversion wrappers at compile-time
3. Support both map-based and typed signatures
4. Provide validation helpers for typed inputs

**Benefits of Deferring:**
- ✅ Simpler MVP implementation
- ✅ Faster development timeline
- ✅ Less runtime overhead in Phase 1
- ✅ Validate need before adding complexity
- ✅ Users can opt-in to manual typed wrappers if needed

**Trade-offs:**
- ❌ More verbose task implementations in Phase 1
- ❌ No compile-time type checking for task args
- ❌ Manual type assertions required
- ⚠️ Acceptable for MVP, can be improved in Phase 2

---

## Implementation Phases

### Phase 1: Foundation (MVP)

**Goal:** Get plugin system working with basic functionality and map-based tasks

**Core Features:**
- ✅ Three plugin types (core, remote, local)
- ✅ Auto-detection of plugin types
- ✅ CLI build process with code generation
- ✅ Plugin = struct with multiple tasks
- ✅ Auto-discovery of public methods
- ✅ Container with plugin registration
- ✅ Lifecycle (Initialize/Shutdown), HealthChecker, EnvVarProvider interfaces
- ✅ Startup validation (fail-fast with panic)
- ✅ Graceful shutdown (context-based)
- ✅ Core plugin shorthand expansion
- ✅ Selective imports (no binary bloat)
- ✅ Separate go.mod per core plugin

**Phase 1 Limitations:**
- ❌ No DI between plugins (manual lookup only via `exec.Container.GetPlugin()`)
- ❌ No typed task support (map-based signatures only)
- ❌ No runtime type conversion helpers
- ❌ No AST parsing of Config structs (manual code generation for each config field)
- ❌ No user-configurable env var mappings (hardcoded env var names)
- ✅ Tag-based config structs (defaults, validation via framework)
- ✅ Framework utilities work (PrepareConfig, ValidateConfig)

**User Experience:**
```go
// plugins/payment/config.go
type Config struct {
    StripeAPIKey string        `yaml:"stripe_api_key" validate:"required"`
    BaseURL      string        `yaml:"base_url" default:"https://api.stripe.com/v1"`
    Timeout      time.Duration `yaml:"timeout" default:"30s"`
}

// plugins/payment/plugin.go
type PaymentPlugin struct {
    config Config  // Validated by framework before Initialize
}

func NewPlugin(config Config) *PaymentPlugin {
    return &PaymentPlugin{config: config}
}

func (p *PaymentPlugin) Initialize(ctx context.Context) error {
    // Config already validated - no need to check
    return nil
}

func (p *PaymentPlugin) Shutdown(ctx context.Context) error {
    return nil  // Cleanup if needed
}

// Map-based task signature (Phase 1)
func (p *PaymentPlugin) Charge(exec *Execution, args map[string]any) (map[string]any, error) {
    // Manual type assertions
    amount := args["amount"].(float64)
    currency := args["currency"].(string)

    // Manual plugin lookup (Phase 1 - no DI yet)
    httpPlugin := exec.Container.GetPlugin("http").(*HTTPPlugin)

    result, err := httpPlugin.Request(exec, map[string]any{
        "url": fmt.Sprintf("%s/charges", p.config.BaseURL),
        "headers": map[string]string{
            "Authorization": fmt.Sprintf("Bearer %s", p.config.StripeAPIKey),
        },
        "body": map[string]any{
            "amount":   int(amount * 100),
            "currency": currency,
        },
    })

    if err != nil {
        return nil, fmt.Errorf("payment failed: %w", err)
    }

    return map[string]any{
        "charge_id": result["body"].(map[string]any)["id"],
        "status":    "succeeded",
    }, nil
}
```

**Generated main.go (Phase 1):**
```go
func main() {
    container := runtime.NewContainer()

    // ===== HTTP Plugin =====
    httpConfig := http.Config{}
    runtime.PrepareConfig(&httpConfig)  // Apply defaults
    runtime.ValidateConfig(httpConfig)
    httpPlugin := http.NewPlugin(httpConfig)
    container.RegisterPlugin("http", httpPlugin)

    // ===== Payment Plugin =====
    paymentConfig := payment.Config{}
    runtime.PrepareConfig(&paymentConfig)
    // Apply env overrides (hardcoded in Phase 1)
    if apiKey := os.Getenv("STRIPE_API_KEY"); apiKey != "" {
        paymentConfig.StripeAPIKey = apiKey
    }
    runtime.ValidateConfig(paymentConfig)
    paymentPlugin := payment.NewPlugin(paymentConfig)
    container.RegisterPlugin("payment", paymentPlugin)

    // Initialize all plugins (fail-fast)
    ctx := context.Background()
    if err := container.Initialize(ctx); err != nil {
        panic(fmt.Sprintf("initialization failed: %v", err))
    }

    // Start app
    app := runtime.NewApp(container)
    app.Run(":8080")
}
```

### Phase 2: Dependency Injection & Typed Tasks

**Goal:** Enable plugin-to-plugin dependencies and optional typed task support

**New Features:**
- ✅ Convention-based dependency injection
- ✅ CLI analyzes plugin struct fields using `go/ast`
- ✅ Dependency graph building and cycle detection
- ✅ Topological sort for initialization order
- ✅ Automatic field injection based on field names
- ✅ Optional `inject` tags for custom naming
- ✅ Single instance sharing
- ✅ Typed task support (optional)
- ✅ Automatic type conversion wrappers
- ✅ Support both map-based and typed signatures
- ✅ Tag-based configuration system (see PLUGIN_CONFIG_FINAL.md)
- ✅ AST-based Config struct analysis for auto-configuration
- ✅ Framework utilities (ApplyDefaults, ValidateConfig, ApplyEnvOverrides)
- ✅ Declarative config tags (`default`, `validate`, `yaml`)
- ✅ User-configurable env var mappings in flow-config.yaml

**User Experience:**
```go
type PaymentPlugin struct {
    http  *HTTPPlugin     // ← Auto-injected (matches "http")
    cache *CachePlugin    // ← Auto-injected (matches "cache")

    stripeKey string
}

func (p *PaymentPlugin) Initialize(ctx context.Context) error {
    p.stripeKey = mustGetEnv("STRIPE_API_KEY")
    // Dependencies already injected by CLI
    return nil
}

// Map-based task (still supported)
func (p *PaymentPlugin) Charge(exec *Execution, args map[string]any) (map[string]any, error) {
    // Dependencies already available
    rate, _ := p.cache.GetRate(exec, map[string]any{"key": "usd_rate"})
    result, _ := p.http.Request(exec, map[string]any{"url": "..."})
    return result, nil
}

// NEW: Typed task (optional in Phase 2)
type RefundInput struct {
    ChargeID string `json:"charge_id"`
    Amount   float64 `json:"amount"`
}

type RefundOutput struct {
    RefundID string `json:"refund_id"`
    Status   string `json:"status"`
}

func (p *PaymentPlugin) Refund(exec *Execution, input RefundInput) (RefundOutput, error) {
    // Fully typed! IDE autocomplete works!
    result, _ := p.http.Request(exec, map[string]any{
        "url": fmt.Sprintf("https://api.stripe.com/refunds/%s", input.ChargeID),
        "body": map[string]any{"amount": input.Amount},
    })

    return RefundOutput{
        RefundID: result["id"].(string),
        Status:   "succeeded",
    }, nil
}
```

**Generated main.go (Phase 2):**
```go
func main() {
    container := runtime.NewContainer()

    // Core plugins
    httpPlugin := &http.HTTPPlugin{}
    container.RegisterPlugin("http", httpPlugin)

    cachePlugin := &cache.CachePlugin{}
    container.RegisterPlugin("cache", cachePlugin)

    // User plugin with DI
    paymentPlugin := &payment.PaymentPlugin{
        http:  httpPlugin,   // ← Injected by CLI
        cache: cachePlugin,  // ← Injected by CLI
    }
    container.RegisterPlugin("payment", paymentPlugin)

    // Initialize all plugins
    ctx := context.Background()
    if err := container.Initialize(ctx); err != nil {
        panic(fmt.Sprintf("initialization failed: %v", err))
    }

    app := runtime.NewApp(container)
    app.Run(":8080")
}
```

### Phase 3: Advanced Features (Future)

**Production Enhancements:**
- Schema-based validation (OpenAPI/JSON Schema)
- Plugin versioning and compatibility checks
- Security checksums and signature verification
- Plugin marketplace and discovery
- Observability integration (metrics, tracing, logging)
- Rate limiting and cost tracking per plugin
- Hot-reloading (dev mode only)
- Plugin sandboxing and resource limits
- Distributed plugin execution
- Plugin testing framework

**Developer Experience:**
- Visual plugin builder
- Plugin template generator
- Interactive debugging tools
- Plugin performance profiler
- Dependency visualization

---

## Examples

### Example 1: Multi-Task Email Plugin

**File:** `plugins/email/config.go`

```go
package email

// Config defines email plugin configuration
type Config struct {
    SMTPHost     string `yaml:"smtp_host" default:"smtp.gmail.com" validate:"required,hostname"`
    SMTPPort     int    `yaml:"smtp_port" default:"587" validate:"gte=1,lte=65535"`
    SMTPUsername string `yaml:"smtp_username" validate:"required"`
    SMTPPassword string `yaml:"smtp_password" validate:"required"`
    FromEmail    string `yaml:"from_email" validate:"required,email"`
}
```

**File:** `plugins/email/plugin.go`

```go
package email

import (
    "context"
    "fmt"
    "strings"
    "net/smtp"
)

type EmailPlugin struct {
    config Config
    smtp   *smtp.Client
    http   *HTTPPlugin  // Dependency (Phase 2)
}

func NewPlugin(config Config) *EmailPlugin {
    return &EmailPlugin{config: config}
}

// Lifecycle
func (e *EmailPlugin) Initialize(ctx context.Context) error {
    // Config already validated by framework
    addr := fmt.Sprintf("%s:%d", e.config.SMTPHost, e.config.SMTPPort)
    client, err := smtp.Dial(addr)
    if err != nil {
        return fmt.Errorf("failed to connect to SMTP server: %w", err)
    }
    e.smtp = client
    return nil
}

func (e *EmailPlugin) Shutdown(ctx context.Context) error {
    if e.smtp != nil {
        return e.smtp.Close()
    }
    return nil
}

// Task 1: email.validate
type ValidateInput struct {
    Email string `json:"email"`
}

type ValidateOutput struct {
    Valid  bool   `json:"valid"`
    Reason string `json:"reason"`
}

func (e *EmailPlugin) Validate(exec *Execution, input ValidateInput) (ValidateOutput, error) {
    if input.Email == "" {
        return ValidateOutput{Valid: false, Reason: "required"}, nil
    }

    valid := strings.Contains(input.Email, "@") && strings.Contains(input.Email, ".")
    return ValidateOutput{
        Valid:  valid,
        Reason: map[bool]string{true: "valid", false: "invalid format"}[valid],
    }, nil
}

// Task 2: email.send
type SendInput struct {
    To      string `json:"to"`
    Subject string `json:"subject"`
    Body    string `json:"body"`
}

type SendOutput struct {
    MessageID string `json:"message_id"`
    Sent      bool   `json:"sent"`
}

func (e *EmailPlugin) Send(exec *Execution, input SendInput) (SendOutput, error) {
    msg := fmt.Sprintf("Subject: %s\r\n\r\n%s", input.Subject, input.Body)

    // Use config values for SMTP settings
    addr := fmt.Sprintf("%s:%d", e.config.SMTPHost, e.config.SMTPPort)
    auth := smtp.PlainAuth("", e.config.SMTPUsername, e.config.SMTPPassword, e.config.SMTPHost)

    err := smtp.SendMail(addr, auth, e.config.FromEmail, []string{input.To}, []byte(msg))

    return SendOutput{
        MessageID: "msg_123",
        Sent:      err == nil,
    }, err
}

// Task 3: email.format (untyped example)
func (e *EmailPlugin) Format(exec *Execution, args map[string]any) (map[string]any, error) {
    template := args["template"].(string)
    data := args["data"].(map[string]any)

    formatted := strings.ReplaceAll(template, "{{name}}", data["name"].(string))

    return map[string]any{"formatted": formatted}, nil
}
```

**flow-config.yaml:**
```yaml
plugins:
  - source: ./plugins/email
    config:
      smtp_host: smtp.gmail.com
      smtp_port: 587
      smtp_username: ${SMTP_USERNAME}
      smtp_password: ${SMTP_PASSWORD}
      from_email: ${FROM_EMAIL}
```

**flow.yaml:**
```yaml
steps:
  - id: validate
    type: email.validate
    args:
      email: ${ request.body.email }

  - id: send
    type: email.send
    args:
      to: ${ request.body.email }
      subject: "Welcome"
      body: "Thanks for signing up!"

  - id: format
    type: email.format
    args:
      template: "Hello {{name}}"
      data:
        name: ${ request.body.name }
```

### Example 2: Payment Plugin with Dependencies

**File:** `plugins/payment/config.go`

```go
package payment

import "time"

// Config defines payment plugin configuration
type Config struct {
    StripeAPIKey string        `yaml:"stripe_api_key" validate:"required"`
    BaseURL      string        `yaml:"base_url" default:"https://api.stripe.com/v1" validate:"url"`
    Timeout      time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s,lte=2m"`
}
```

**File:** `plugins/payment/plugin.go`

```go
package payment

type PaymentPlugin struct {
    http  *HTTPPlugin     // ← Phase 2: Auto-injected
    cache *CachePlugin    // ← Phase 2: Auto-injected

    config Config  // Validated config from framework
}

func NewPlugin(config Config) *PaymentPlugin {
    return &PaymentPlugin{config: config}
}

func (p *PaymentPlugin) Initialize(ctx context.Context) error {
    // Config already validated by framework
    // p.config.StripeAPIKey is guaranteed to be non-empty
    return nil
}

func (p *PaymentPlugin) Shutdown(ctx context.Context) error {
    return nil  // No cleanup needed for this plugin
}

// Task 1: payment.charge
type ChargeInput struct {
    Amount   float64 `json:"amount"`
    Currency string  `json:"currency"`
}

type ChargeOutput struct {
    ChargeID string `json:"charge_id"`
    Status   string `json:"status"`
}

func (p *PaymentPlugin) Charge(exec *Execution, input ChargeInput) (ChargeOutput, error) {
    // Use cache dependency
    rate, _ := p.cache.GetExchangeRate(exec, CacheInput{
        Key: fmt.Sprintf("rate:%s", input.Currency),
    })

    convertedAmount := input.Amount * rate.Value.(float64)

    // Use HTTP dependency with config values
    result, err := p.http.Request(exec, HTTPInput{
        URL:    fmt.Sprintf("%s/charges", p.config.BaseURL),
        Method: "POST",
        Headers: map[string]string{
            "Authorization": fmt.Sprintf("Bearer %s", p.config.StripeAPIKey),
        },
        Body: map[string]any{
            "amount":   int(convertedAmount * 100),
            "currency": "usd",
        },
    })

    if err != nil {
        return ChargeOutput{}, err
    }

    return ChargeOutput{
        ChargeID: result.Body["id"].(string),
        Status:   result.Body["status"].(string),
    }, nil
}

// Task 2: payment.refund
type RefundInput struct {
    ChargeID string `json:"charge_id"`
}

type RefundOutput struct {
    RefundID string `json:"refund_id"`
    Status   string `json:"status"`
}

func (p *PaymentPlugin) Refund(exec *Execution, input RefundInput) (RefundOutput, error) {
    result, err := p.http.Request(exec, HTTPInput{
        URL:    fmt.Sprintf("%s/refunds", p.config.BaseURL),
        Method: "POST",
        Headers: map[string]string{
            "Authorization": fmt.Sprintf("Bearer %s", p.config.StripeAPIKey),
        },
        Body: map[string]any{
            "charge": input.ChargeID,
        },
    })

    if err != nil {
        return RefundOutput{}, err
    }

    return RefundOutput{
        RefundID: result.Body["id"].(string),
        Status:   result.Body["status"].(string),
    }, nil
}
```

**flow-config.yaml:**
```yaml
plugins:
  - source: http
  - source: redis
  - source: ./plugins/cache   # Wraps redis
  - source: ./plugins/payment # Uses http + cache
    config:
      stripe_api_key: ${STRIPE_API_KEY}
      base_url: https://api.stripe.com/v1
      timeout: 30s
```

**Generated main.go (Phase 2):**
```go
func main() {
    container := runtime.NewContainer()

    // ===== HTTP Plugin =====
    httpConfig := http.Config{}
    runtime.PrepareConfig(&httpConfig)
    httpPlugin := http.NewPlugin(httpConfig)
    container.RegisterPlugin("http", httpPlugin)

    // ===== Redis Plugin =====
    redisConfig := redis.Config{}
    runtime.PrepareConfig(&redisConfig)
    if addr := os.Getenv("REDIS_ADDR"); addr != "" {
        redisConfig.Addr = addr
    }
    runtime.ValidateConfig(redisConfig)
    redisPlugin := redis.NewPlugin(redisConfig)
    container.RegisterPlugin("redis", redisPlugin)

    // ===== Cache Plugin (with DI) =====
    cacheConfig := cache.Config{}
    runtime.PrepareConfig(&cacheConfig)
    cachePlugin := &cache.CachePlugin{
        redis: redisPlugin,  // ← Injected
        config: cacheConfig,
    }
    container.RegisterPlugin("cache", cachePlugin)

    // ===== Payment Plugin (with DI and config) =====
    paymentConfig := payment.Config{}
    runtime.PrepareConfig(&paymentConfig)
    if apiKey := os.Getenv("STRIPE_API_KEY"); apiKey != "" {
        paymentConfig.StripeAPIKey = apiKey
    }
    runtime.ValidateConfig(paymentConfig)
    paymentPlugin := &payment.PaymentPlugin{
        http:   httpPlugin,   // ← Injected
        cache:  cachePlugin,  // ← Injected
        config: paymentConfig,
    }
    container.RegisterPlugin("payment", paymentPlugin)

    // Initialize all plugins
    ctx := context.Background()
    if err := container.Initialize(ctx); err != nil {
        log.Fatal(err)
    }

    app := runtime.NewApp(container)
    app.Run(":8080")
}
```

---

## Summary

### Key Design Decisions

1. **Plugin = Struct with Tasks**: Natural Go pattern, shared state
2. **Auto-Detection**: No manual type specification needed
3. **Core Plugin Shorthand**: `source: http` → full path expansion
4. **Selective Imports**: Only used plugins in binary (no bloat)
5. **Optional Interfaces**: Plugins implement only what they need
6. **Type-Based DI**: Dependencies via struct fields
7. **Typed Tasks**: Optional type-safe inputs/outputs
8. **Compile-Time**: All plugins compiled into single binary

### Benefits

- **Simple**: Clear Plugin → Tasks hierarchy
- **Standard Go**: All plugins use standard go.mod structure
- **Type-Safe**: Compile-time errors, optional typed tasks
- **Performant**: No runtime overhead, optimized binary
- **Zero Bloat**: Only used plugins compiled
- **Composable**: Plugins reuse other plugins through DI
- **Production-Ready**: Single binary deployment

### Trade-offs

- **No Hot-Reloading**: Must rebuild for changes (production feature)
- **Tight Coupling**: Plugins depend on concrete types (performance)
- **Build Complexity**: CLI analyzes and generates code (one-time)
- **No Runtime Plugins**: Security and reliability over flexibility

---

**End of Design Document**