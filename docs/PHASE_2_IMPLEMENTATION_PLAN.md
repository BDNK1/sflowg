# SFlowG Plugin System - Phase 2 Implementation Plan

**Version:** 2.0
**Date:** 2024-11-06
**Status:** Implementation Specification

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Package Structure](#package-structure)
4. [Plugin Contract](#plugin-contract)
5. [Interfaces & Types](#interfaces--types)
6. [Dependency Injection](#dependency-injection)
7. [Configuration System](#configuration-system)
8. [Task Discovery & Execution](#task-discovery--execution)
9. [Typed Tasks](#typed-tasks)
10. [CLI Implementation](#cli-implementation)
11. [Runtime Implementation](#runtime-implementation)
12. [Examples](#examples)
13. [Implementation Steps](#implementation-steps)

---

## Overview

### Goals

Phase 2 adds dependency injection and typed tasks to the plugin system:

- ✅ **Convention-based dependency injection** between plugins
- ✅ **Optional typed task signatures** for type safety
- ✅ **Clean separation** via `runtime/plugin` minimal interface package
- ✅ **Zero boilerplate** for plugin authors
- ✅ **Automatic discovery** of dependencies, config, and tasks

### Differences from Phase 1

| Feature         | Phase 1                                        | Phase 2                          |
|-----------------|------------------------------------------------|----------------------------------|
| Dependencies    | Manual lookup via `exec.Container.GetPlugin()` | Auto-injected via struct fields  |
| Task Signatures | Map-based only                                 | Map-based + optional typed       |
| Imports         | `runtime` package                              | `runtime/plugin` minimal package |
| Config          | Constructor required                           | Constructor optional             |
| Initialization  | Constructor                                    | Struct literal preferred         |

---

## Architecture

### High-Level Flow

```
┌─────────────────────────────────────────────────────────┐
│                    Build Time (CLI)                      │
│                                                          │
│  1. Parse flow-config.yaml                              │
│  2. Analyze plugin packages via AST                     │
│     • Detect Config type                                │
│     • Detect dependency fields                          │
│     • Detect task methods                               │
│  3. Build dependency graph                              │
│     • Topological sort                                  │
│     • Detect cycles                                     │
│  4. Generate go.mod                                     │
│  5. Generate main.go                                    │
│     • Initialize in dependency order                    │
│     • Inject dependencies via struct literals           │
│     • Apply config with validation                      │
└─────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────┐
│                  Runtime (Generated Binary)             │
│                                                          │
│  1. Container.RegisterPlugin()                          │
│     • Discover public methods via reflection            │
│     • Create TaskExecutor for each method              │
│     • Detect optional interfaces                       │
│  2. Container.Initialize()                              │
│     • Call Initialize() on Lifecycle plugins           │
│     • Run health checks                                │
│  3. Flow Execution                                      │
│     • Lookup task: "pluginName.taskName"               │
│     • Execute with Execution context                   │
└─────────────────────────────────────────────────────────┘
```

---

## Package Structure

### Directory Layout

```
github.com/sflowg/sflowg/
├── runtime/
│   ├── plugin/                      # Minimal interface (what plugins import)
│   │   ├── execution.go             # Execution interface
│   │   ├── interfaces.go            # Lifecycle, HealthChecker
│   │   ├── config.go                # Config utilities
│   │   └── types.go                 # Common types
│   │
│   ├── container.go                 # Full implementation (internal)
│   ├── executor.go
│   ├── app.go
│   └── execution_impl.go            # Implements plugin.Execution
│
├── plugins/
│   ├── http/
│   │   ├── go.mod                   # module: github.com/sflowg/sflowg/plugins/http
│   │   ├── plugin.go                # Imports runtime/plugin only
│   │   └── client.go
│   └── redis/
│       ├── go.mod                   # module: github.com/sflowg/sflowg/plugins/redis
│       ├── plugin.go                # Imports runtime/plugin only
│       └── pool.go
│
└── cli/
    ├── internal/
    │   ├── analyzer/                # AST analysis
    │   │   ├── plugin.go            # Analyze plugin structs
    │   │   ├── config.go            # Detect Config type
    │   │   └── tasks.go             # Detect task methods
    │   ├── generator/
    │   │   ├── main.go              # Generate main.go
    │   │   └── gomod.go             # Generate go.mod
    │   └── graph/
    │       └── dependency.go        # Dependency graph
    └── main.go
```

### Import Restrictions

```go
// ✅ Plugins import minimal interface
import "github.com/sflowg/sflowg/runtime/plugin"

// ❌ Plugins CANNOT import full runtime
import "github.com/sflowg/sflowg/runtime" // Compilation error!

// ✅ Runtime imports plugin interface
import "github.com/sflowg/sflowg/runtime/plugin"
```

---

## Plugin Contract

### What Plugin Author Provides

#### 1. Plugin Struct (Required)

**Minimal:**

```go
type MyPlugin struct {}
```

**With Config:**

```go
type MyPlugin struct {
config Config
}
```

**With Dependencies:**

```go
type MyPlugin struct {
http   *http.HTTPPlugin // Auto-injected by CLI
redis  *redis.RedisPlugin // Auto-injected by CLI
config Config
}
```

**Field Types:**

- **Dependencies:** Exported pointer to plugin type → Auto-injected
- **Config:** Type named `Config` → Passed to initialization
- **Other fields:** Plugin manages (created in Initialize or elsewhere)

#### 2. Config Struct (Optional)

```go
type Config struct {
Addr     string        `yaml:"addr" default:"localhost:6379" validate:"required,hostname_port"`
Password string        `yaml:"password"`
DB       int           `yaml:"db" default:"0" validate:"gte=0,lte=15"`
Timeout  time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s"`
}
```

**Naming Convention:** Must be named exactly `Config`

**Tags:**

- `yaml:"..."` - YAML field name
- `default:"..."` - Default value (applied by framework)
- `validate:"..."` - Validation constraint (checked by framework)

#### 3. Task Methods (Required - At Least One)

**Map-based (always supported):**

```go
func (p *MyPlugin) TaskName(exec *plugin.Execution, input plugin.Input) (plugin.Output, error)
```

**Typed (Phase 2):**

```go
func (p *MyPlugin) TaskName(exec *plugin.Execution, input InputType) (OutputType, error)
```

**Discovery:**

- Method must be public (capitalized)
- Method name becomes task: `Send()` → `"myplugin.send"`

#### 4. Lifecycle Interface (Optional)

```go
func (p *MyPlugin) Initialize(exec *plugin.Execution) error {
// Setup connections, resources
return nil
}

func (p *MyPlugin) Shutdown(exec *plugin.Execution) error {
// Cleanup
return nil
}
```

#### 5. HealthChecker Interface (Optional)

```go
func (p *MyPlugin) HealthCheck(exec *plugin.Execution) error {
// Check plugin health
return nil
}
```

#### 6. Constructor Function (Optional)

**NOT required in Phase 2.** CLI uses struct literal initialization by default.

**If provided:**

```go
func NewMyPlugin(config Config, dep1 *Dep1Plugin, dep2 *Dep2Plugin) *MyPlugin {
// Custom initialization logic
return &MyPlugin{config: config, dep1: dep1, dep2: dep2}
}
```

**CLI detection:** If `NewPluginName` function exists, CLI will use it. Otherwise, uses struct literal.

---

## Interfaces & Types

### runtime/plugin/execution.go

```go
package plugin

// Execution is the runtime context passed to plugin tasks
// This is the actual struct from runtime package, not an interface
// Plugins receive *Execution pointer
type Execution struct {
	ID        string
	Values    map[string]any
	Flow      *Flow
	Container *Container
}

// Note: Execution is defined in runtime package
// This is a reference showing what plugins receive
// Do NOT modify the Execution struct
```

### runtime/plugin/interfaces.go

```go
package plugin

// Lifecycle for plugins that need initialization/shutdown
type Lifecycle interface {
	Initialize(exec *Execution) error
	Shutdown(exec *Execution) error
}

// HealthChecker for plugins that want health monitoring
type HealthChecker interface {
	HealthCheck(exec *Execution) error
}
```

### runtime/plugin/types.go

```go
package plugin

// Input and Output are type aliases for task method signatures
type Input map[string]any
type Output map[string]any

// TaskExecutor wraps a task method
type TaskExecutor interface {
	Execute(exec *Execution, args Input) (Output, error)
}
```

### runtime/plugin/error.go

```go
package plugin

// TaskError wraps task execution errors with metadata
// Allows plugins to return execution metadata alongside errors
type TaskError struct {
	Err      error          // The underlying error (can be nil for warnings-only)
	Metadata map[string]any // Execution metadata (warnings, retry hints, metrics, etc.)
}

func (e *TaskError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "task completed with metadata"
}

func (e *TaskError) Unwrap() error {
	return e.Err
}

// NewTaskError creates a new task error
func NewTaskError(err error) *TaskError {
	return &TaskError{
		Err:      err,
		Metadata: make(map[string]any),
	}
}

// WithMetadata adds metadata to the error
func (e *TaskError) WithMetadata(metadata map[string]any) *TaskError {
	e.Metadata = metadata
	return e
}
```

**Note:** Config helpers (`ApplyDefaults`, `ValidateConfig`, `PrepareConfig`) are **CLI-internal only**, not exposed in `runtime/plugin` package. Plugin developers don't need them.

---

## Dependency Injection

### Detection Rules

**CLI detects a field as a dependency if:**

1. Field is **exported** (starts with capital letter)
2. Field type is **pointer**: `*SomeType`
3. `SomeType` is a **registered plugin** (from flow-config.yaml)
4. Field name matches plugin instance name (case-insensitive)

**Optional:** Use `inject` tag if field name doesn't match:

```go
cache *RedisPlugin `inject:"redis_cache"`
```

### Examples

#### Example 1: Simple Dependencies

```go
// flow-config.yaml
plugins:
- source: http
- source: redis
- source:./plugins/payment

// payment/plugin.go
type PaymentPlugin struct {
http  *http.HTTPPlugin // ← Matches "http"
redis *redis.RedisPlugin // ← Matches "redis"
}

// CLI generates:
paymentPlugin := &payment.PaymentPlugin{
http:  httpPlugin, // ← Injected
redis: redisPlugin, // ← Injected
}
```

#### Example 2: Multiple Instances

```go
// flow-config.yaml
plugins:
- source: redis
name: redis_cache
- source: redis
name: redis_sessions
- source:./plugins/app

// app/plugin.go
type AppPlugin struct {
cache    *redis.RedisPlugin `inject:"redis_cache"`
sessions *redis.RedisPlugin `inject:"redis_sessions"`
}

// CLI generates:
appPlugin := &app.AppPlugin{
cache:    redisCachePlugin, // ← Injected based on tag
sessions: redisSessionsPlugin, // ← Injected based on tag
}
```

#### Example 3: Non-Dependency Fields

```go
type EmailPlugin struct {
http   *http.HTTPPlugin // ← Dependency (injected)
config Config           // ← Config (passed)

// These are ignored by CLI (plugin manages them)
client  *smtp.Client // ← Not a plugin type
logger  *log.Logger      // ← Not a plugin type
cache   map[string]any   // ← Not a pointer to plugin
counter int              // ← Not a pointer
}

// CLI generates:
emailPlugin := &email.EmailPlugin{
http:   httpPlugin, // ← Only dependency injected
config: emailConfig, // ← Only config passed
// client, logger, cache, counter: not set by CLI
}

// Plugin creates its own state:
func (p *EmailPlugin) Initialize(exec *plugin.Execution) error {
p.client = smtp.Dial(...)
p.logger = log.New(...)
p.cache = make(map[string]any)
return nil
}
```

### Dependency Graph

**CLI builds graph:**

```
http ──┐
       ├──> payment
redis ─┘

cache ─────> payment
```

**Topological sort for initialization order:**

```
1. http (no dependencies)
2. redis (no dependencies)
3. cache (depends on redis)
4. payment (depends on http, cache)
```

**Cycle detection:**

```
A ──> B ──> C ──> A   ❌ Circular dependency error
```

---

## Configuration System

### Phase 2 Approach

**Full tag-based system with AST analysis:**

#### Plugin Side

```go
// plugins/redis/config.go
package redis

import "time"

type Config struct {
	Addr     string        `yaml:"addr" default:"localhost:6379" validate:"required,hostname_port"`
	Password string        `yaml:"password"`
	DB       int           `yaml:"db" default:"0" validate:"gte=0,lte=15"`
	PoolSize int           `yaml:"pool_size" default:"10" validate:"gte=1,lte=1000"`
	Timeout  time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s"`
}
```

**No boilerplate:**

- ❌ No `DefaultConfig()` function
- ❌ No `Validate()` method
- ✅ Framework handles everything

#### User Side

```yaml
# flow-config.yaml
plugins:
  - source: redis
    config:
      addr: ${REDIS_ADDR:localhost:6379}
      password: ${REDIS_PASSWORD}
      pool_size: 20
```

**Environment variable syntax:**

- `${VAR}` - Required, error if missing
- `${VAR:default}` - Optional with default
- Plain value - Literal

**Precedence:**

1. Environment variable (highest)
2. flow-config.yaml literal
3. struct tag default (lowest)

#### CLI Generated Code

```go
// Initialize redis plugin
redisConfig := redis.Config{}

// Apply defaults from tags
if err := plugin.PrepareConfig(&redisConfig); err != nil {
log.Fatal(err)
}

// Apply environment overrides
if addr := os.Getenv("REDIS_ADDR"); addr != "" {
redisConfig.Addr = addr
}
if password := os.Getenv("REDIS_PASSWORD"); password != "" {
redisConfig.Password = password
}

// Apply literal from flow-config.yaml
redisConfig.PoolSize = 20

// Validate after all overrides
if err := plugin.ValidateConfig(redisConfig); err != nil {
log.Fatal(err)
}

// Initialize plugin
redisPlugin := &redis.RedisPlugin{
config: redisConfig,
}
```

### Config Detection Algorithm

```go
// CLI analyzes plugin package
func detectConfig(pkg *ast.Package) *ConfigMetadata {
// 1. Find type named "Config"
configType := findTypeByName(pkg, "Config")
if configType == nil {
return nil // No config
}

// 2. Extract fields and tags
for _, field := range configType.Fields {
meta := FieldMetadata{
Name:         field.Name,
Type:         field.Type,
YAMLTag:      field.Tag.Get("yaml"),
DefaultTag:   field.Tag.Get("default"),
ValidateTag:  field.Tag.Get("validate"),
}
configMeta.Fields = append(configMeta.Fields, meta)
}

return configMeta
}
```

---

## Task Discovery & Execution

### Discovery Algorithm

**Runtime uses reflection:**

```go
func (c *Container) RegisterPlugin(name string, plugin any) error {
c.plugins[name] = plugin

// Discover all public methods
pluginType := reflect.TypeOf(plugin)
for i := 0; i < pluginType.NumMethod(); i++ {
method := pluginType.Method(i)

// Check if valid task signature
if isValidTaskSignature(method) {
taskName := fmt.Sprintf("%s.%s", name, toLowerFirst(method.Name))
executor := createTaskExecutor(plugin, method)
c.tasks[taskName] = executor
}
}

// Detect optional interfaces
if lifecycle, ok := plugin.(plugin.Lifecycle); ok {
c.lifecycles = append(c.lifecycles, lifecycle)
}

if checker, ok := plugin.(plugin.HealthChecker); ok {
c.healthCheckers = append(c.healthCheckers, checker)
}

return nil
}
```

### Valid Task Signatures

**Map-based:**

```go
func (p *Plugin) Method(exec *plugin.Execution, args map[string]any) (map[string]any, error)
```

**Typed:**

```go
func (p *Plugin) Method(exec *plugin.Execution, input TInput) (TOutput, error)
where TInput and TOutput are structs
```

### Task Naming

**Convention:**

- `PluginName.MethodName` → `pluginname.methodname` (lowercase)
- Example: `EmailPlugin.Send()` → `"email.send"`

### Execution Flow

```yaml
# flow.yaml
steps:
  - id: charge
    type: payment.charge
    args:
      amount: 100
      currency: "usd"
```

```go
// Runtime execution
func (e *Executor) executeStep(step Step, exec *ExecutionImpl) error {
// 1. Parse task name
taskName := step.Type // "payment.charge"

// 2. Lookup executor
executor, ok := exec.Container.GetTask(taskName)
if !ok {
return fmt.Errorf("task not found: %s", taskName)
}

// 3. Execute
result, err := executor.Execute(exec, step.Args)
if err != nil {
return err
}

// 4. Store result
exec.Values[step.ID] = result
return nil
}
```

---

## Typed Tasks

### How They Work

#### Plugin Defines Typed Method

```go
type ChargeInput struct {
Amount   float64 `json:"amount" validate:"required,gt=0"`
Currency string  `json:"currency" validate:"required,iso4217"`
}

type ChargeOutput struct {
ChargeID string `json:"charge_id"`
Status   string `json:"status"`
}

func (p *PaymentPlugin) Charge(exec *plugin.Execution, input ChargeInput) (ChargeOutput, error) {
// Fully typed - IDE autocomplete works
if input.Amount < 1.00 {
return ChargeOutput{}, fmt.Errorf("minimum is 1.00")
}

return ChargeOutput{
ChargeID: "ch_123",
Status:   "succeeded",
}, nil
}
```

#### Runtime Generates Wrapper

**At registration time, container wraps typed method:**

```go
func createTaskExecutor(plugin any, method reflect.Method) TaskExecutor {
methodType := method.Type

// Check if typed signature
if isTypedSignature(methodType) {
return createTypedWrapper(plugin, method)
}

// Otherwise, direct map-based
return createMapWrapper(plugin, method)
}

func createTypedWrapper(plugin any, method reflect.Method) TaskExecutor {
inputType := method.Type.In(2)   // Third param
outputType := method.Type.Out(0) // First return

return func (exec plugin.Execution, args map[string]any) (map[string]any, error) {
// Convert map → struct
input := reflect.New(inputType).Interface()
if err := mapToStruct(args, input); err != nil {
return nil, fmt.Errorf("invalid input: %w", err)
}

// Validate
if err := plugin.ValidateConfig(input); err != nil {
return nil, fmt.Errorf("validation failed: %w", err)
}

// Call typed method via reflection
results := method.Func.Call([]reflect.Value{
reflect.ValueOf(plugin),
reflect.ValueOf(exec),
reflect.ValueOf(input).Elem(),
})

// Extract output and error
output := results[0].Interface()
err := results[1].Interface()

if err != nil {
return nil, err.(error)
}

// Convert struct → map
return structToMap(output)
}
}
```

#### Conversion Utilities

```go
import "github.com/mitchellh/mapstructure"
import "encoding/json"

func mapToStruct(m map[string]any, target any) error {
decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
Result:  target,
TagName: "json",
DecodeHook: mapstructure.ComposeDecodeHookFunc(
mapstructure.StringToTimeDurationHookFunc(),
mapstructure.StringToTimeHookFunc(time.RFC3339),
),
})
if err != nil {
return err
}
return decoder.Decode(m)
}

func structToMap(s any) (map[string]any, error) {
var result map[string]any
data, err := json.Marshal(s)
if err != nil {
return nil, err
}
err = json.Unmarshal(data, &result)
return result, err
}
```

### Mixed Signatures

**Both work in same plugin:**

```go
type PaymentPlugin struct {
http *http.HTTPPlugin
}

// Map-based task
func (p *PaymentPlugin) Refund(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
chargeID := args["charge_id"].(string)
return map[string]any{"refund_id": "re_123"}, nil
}

// Typed task
func (p *PaymentPlugin) Charge(exec *plugin.Execution, input ChargeInput) (ChargeOutput, error) {
return ChargeOutput{ChargeID: "ch_123"}, nil
}

// Both auto-discovered and work correctly
```

---

## CLI Implementation

### Step 1: Parse Config

```go
// cli/internal/config/loader.go
func Load(projectDir string) (*FlowConfig, error) {
configPath := filepath.Join(projectDir, "flow-config.yaml")

data, err := os.ReadFile(configPath)
if err != nil {
return nil, err
}

var config FlowConfig
if err := yaml.Unmarshal(data, &config); err != nil {
return nil, err
}

return &config, nil
}
```

### Step 2: Analyze Plugins

```go
// cli/internal/analyzer/plugin.go
type PluginMetadata struct {
Name         string
ImportPath   string
TypeName     string
HasConfig    bool
ConfigType   *ConfigMetadata
Dependencies []Dependency
Tasks        []TaskMetadata
}

type Dependency struct {
FieldName    string
PluginType   string
PluginName   string
InjectTag    string // From inject:"..." tag
}

type ConfigMetadata struct {
Fields []ConfigField
}

type ConfigField struct {
Name        string
Type        string
YAMLTag     string
DefaultTag  string
ValidateTag string
}

func AnalyzePlugin(importPath, pluginName string) (*PluginMetadata, error) {
// 1. Load package with go/packages
cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedSyntax}
pkgs, err := packages.Load(cfg, importPath)
if err != nil {
return nil, err
}

pkg := pkgs[0]

meta := &PluginMetadata{
Name:       pluginName,
ImportPath: importPath,
}

// 2. Find Config type
configType := findTypeByName(pkg, "Config")
if configType != nil {
meta.HasConfig = true
meta.ConfigType = analyzeConfigType(configType)
}

// 3. Find plugin struct
pluginStruct := findTypeByName(pkg, pluginName+"Plugin")
if pluginStruct == nil {
return nil, fmt.Errorf("plugin struct not found")
}

meta.TypeName = pluginStruct.Name()

// 4. Analyze struct fields for dependencies
for _, field := range getStructFields(pluginStruct) {
if !field.Exported() {
continue
}

// Skip config field
if field.Type() == configType {
continue
}

// Check if dependency
if isPointerToPluginType(field.Type()) {
dep := Dependency{
FieldName:  field.Name(),
PluginType: field.Type().String(),
PluginName: inferPluginName(field),
InjectTag:  field.Tag("inject"),
}
meta.Dependencies = append(meta.Dependencies, dep)
}
}

// 5. Find task methods
methods := getPublicMethods(pluginStruct)
for _, method := range methods {
if isValidTaskSignature(method) {
meta.Tasks = append(meta.Tasks, TaskMetadata{
Name:      method.Name(),
Signature: getSignature(method),
})
}
}

return meta, nil
}

func inferPluginName(field Field) string {
// Check inject tag first
if tag := field.Tag("inject"); tag != "" {
return tag
}

// Otherwise use field name (lowercase)
return strings.ToLower(field.Name())
}
```

### Step 3: Build Dependency Graph

```go
// cli/internal/graph/dependency.go
type Graph struct {
nodes map[string]*Node
edges map[string][]string
}

type Node struct {
Name         string
Dependencies []string
}

func BuildGraph(plugins []*PluginMetadata) (*Graph, error) {
graph := NewGraph()

// Add nodes
for _, plugin := range plugins {
graph.AddNode(plugin.Name)
}

// Add edges
for _, plugin := range plugins {
for _, dep := range plugin.Dependencies {
graph.AddEdge(plugin.Name, dep.PluginName)
}
}

// Check for cycles
if graph.HasCycle() {
return nil, fmt.Errorf("circular dependency detected")
}

return graph, nil
}

func (g *Graph) TopologicalSort() ([]string, error) {
// Kahn's algorithm
inDegree := make(map[string]int)
queue := []string{}
result := []string{}

// Calculate in-degrees
for node := range g.nodes {
inDegree[node] = 0
}
for _, edges := range g.edges {
for _, to := range edges {
inDegree[to]++
}
}

// Find nodes with no dependencies
for node, degree := range inDegree {
if degree == 0 {
queue = append(queue, node)
}
}

// Process queue
for len(queue) > 0 {
node := queue[0]
queue = queue[1:]
result = append(result, node)

for _, neighbor := range g.edges[node] {
inDegree[neighbor]--
if inDegree[neighbor] == 0 {
queue = append(queue, neighbor)
}
}
}

if len(result) != len(g.nodes) {
return nil, fmt.Errorf("cycle detected")
}

return result, nil
}
```

### Step 4: Generate Code

```go
// cli/internal/generator/main.go
func GenerateMainGo(plugins []*PluginMetadata, initOrder []string) (string, error) {
tmpl := `package main

import (
    "context"
    "log"
    "os"

    "github.com/sflowg/sflowg/runtime"
    "github.com/sflowg/sflowg/runtime/plugin"
{{ range .Imports }}
    {{ .Alias }} "{{ .Path }}"
{{ end }}
)

func main() {
    container := runtime.NewContainer()

{{ range .Plugins }}
    // ===== {{ .Name }} Plugin =====
{{ if .HasConfig }}
    {{ .Name }}Config := {{ .TypeName }}.Config{}
    if err := plugin.PrepareConfig(&{{ .Name }}Config); err != nil {
        log.Fatal(err)
    }

{{ range .ConfigEnvVars }}
    if val := os.Getenv("{{ .EnvVar }}"); val != "" {
        {{ $.Name }}Config.{{ .Field }} = val
    }
{{ end }}
{{ range .ConfigLiterals }}
    {{ $.Name }}Config.{{ .Field }} = {{ .Value }}
{{ end }}

    if err := plugin.ValidateConfig({{ .Name }}Config); err != nil {
        log.Fatal(err)
    }
{{ end }}

    {{ .Name }}Plugin := &{{ .TypeName }}.{{ .StructName }}{
{{ if .HasConfig }}
        config: {{ .Name }}Config,
{{ end }}
{{ range .Dependencies }}
        {{ .FieldName }}: {{ .PluginName }}Plugin,
{{ end }}
    }
    container.RegisterPlugin("{{ .Name }}", {{ .Name }}Plugin)

{{ end }}

    // Initialize all plugins
    ctx := context.Background()
    if err := container.Initialize(ctx); err != nil {
        log.Fatal(err)
    }

    // Start application
    app := runtime.NewApp(container)
    if err := app.Run(":8080"); err != nil {
        log.Fatal(err)
    }
}
`

data := prepareTemplateData(plugins, initOrder)
return executeTemplate(tmpl, data)
}
```

---

## Runtime Implementation

### Container

```go
// runtime/container.go
package runtime

import (
	"github.com/sflowg/sflowg/runtime/plugin"
)

type Container struct {
	plugins        map[string]any
	tasks          map[string]plugin.TaskExecutor
	lifecycles     []plugin.Lifecycle
	healthCheckers []plugin.HealthChecker
	initialized    bool
}

func NewContainer() *Container {
	return &Container{
		plugins: make(map[string]any),
		tasks:   make(map[string]plugin.TaskExecutor),
	}
}

func (c *Container) RegisterPlugin(name string, pluginInstance any) error {
	c.plugins[name] = pluginInstance

	// Discover tasks via reflection
	tasks := discoverTasks(pluginInstance, name)
	for taskName, executor := range tasks {
		c.tasks[taskName] = executor
	}

	// Detect optional interfaces
	if lifecycle, ok := pluginInstance.(plugin.Lifecycle); ok {
		c.lifecycles = append(c.lifecycles, lifecycle)
	}

	if checker, ok := pluginInstance.(plugin.HealthChecker); ok {
		c.healthCheckers = append(c.healthCheckers, checker)
	}

	return nil
}

func (c *Container) Initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}

	exec := newExecutionImpl(ctx, c)

	// Initialize lifecycle plugins
	for _, lifecycle := range c.lifecycles {
		if err := lifecycle.Initialize(exec); err != nil {
			return fmt.Errorf("initialization failed: %w", err)
		}
	}

	// Run health checks
	for _, checker := range c.healthCheckers {
		if err := checker.HealthCheck(exec); err != nil {
			return fmt.Errorf("health check failed: %w", err)
		}
	}

	c.initialized = true
	return nil
}

func (c *Container) Shutdown(ctx context.Context) error {
	exec := newExecutionImpl(ctx, c)

	// Shutdown in reverse order
	for i := len(c.lifecycles) - 1; i >= 0; i-- {
		if err := c.lifecycles[i].Shutdown(exec); err != nil {
			return err
		}
	}

	c.initialized = false
	return nil
}

func (c *Container) GetTask(name string) (plugin.TaskExecutor, bool) {
	executor, ok := c.tasks[name]
	return executor, ok
}
```

### Task Discovery

```go
// runtime/discovery.go
func discoverTasks(pluginInstance any, pluginName string) map[string]plugin.TaskExecutor {
tasks := make(map[string]plugin.TaskExecutor)

pluginType := reflect.TypeOf(pluginInstance)

for i := 0; i < pluginType.NumMethod(); i++ {
method := pluginType.Method(i)

if !isValidTaskSignature(method) {
continue
}

taskName := fmt.Sprintf("%s.%s", pluginName, toLowerFirst(method.Name))
executor := createTaskExecutor(pluginInstance, method)
tasks[taskName] = executor
}

return tasks
}

func isValidTaskSignature(method reflect.Method) bool {
methodType := method.Type

// Check parameter count: receiver + exec + input
if methodType.NumIn() != 3 {
return false
}

// Check return count: output + error
if methodType.NumOut() != 2 {
return false
}

// Check exec parameter (second param, first is receiver)
execParam := methodType.In(1)
if !implementsExecution(execParam) {
return false
}

// Check error return (second return)
errorReturn := methodType.Out(1)
if !isErrorType(errorReturn) {
return false
}

// Valid signatures:
// 1. Map-based: (exec, map[string]any) (map[string]any, error)
// 2. Typed: (exec, TInput) (TOutput, error)

return true
}

func createTaskExecutor(plugin any, method reflect.Method) plugin.TaskExecutor {
methodType := method.Type
inputType := methodType.In(2)
outputType := methodType.Out(0)

// Check if typed signature
if inputType.Kind() == reflect.Struct && outputType.Kind() == reflect.Struct {
return createTypedWrapper(plugin, method)
}

// Map-based signature
return createMapWrapper(plugin, method)
}

func createMapWrapper(plugin any, method reflect.Method) plugin.TaskExecutor {
return func (exec plugin.Execution, args map[string]any) (map[string]any, error) {
// Direct call via reflection
results := method.Func.Call([]reflect.Value{
reflect.ValueOf(plugin),
reflect.ValueOf(exec),
reflect.ValueOf(args),
})

output := results[0].Interface().(map[string]any)
err := results[1].Interface()

if err != nil {
return nil, err.(error)
}

return output, nil
}
}
```

### Execution Implementation

```go
// runtime/execution_impl.go
type ExecutionImpl struct {
context.Context
container *Container
values    map[string]any
logger    *Logger
request   *Request
}

func newExecutionImpl(ctx context.Context, container *Container) *ExecutionImpl {
return &ExecutionImpl{
Context:   ctx,
container: container,
values:    make(map[string]any),
logger:    newLogger(),
}
}

func (e *ExecutionImpl) Get(key string) any {
return e.values[key]
}

func (e *ExecutionImpl) Logger() plugin.Logger {
return e.logger
}

func (e *ExecutionImpl) Request() plugin.Request {
return e.request
}
```

---

## Examples

### Example 1: Minimal Plugin

```go
// plugins/minimal/plugin.go
package minimal

import "github.com/sflowg/sflowg/runtime/plugin"

type MinimalPlugin struct{}

func (p *MinimalPlugin) Greet(exec plugin.Execution, args map[string]any) (map[string]any, error) {
	name := args["name"].(string)
	return map[string]any{
		"message": "Hello, " + name,
	}, nil
}
```

```yaml
# flow-config.yaml
plugins:
  - source: ./plugins/minimal
```

```go
// Generated main.go
minimalPlugin := &minimal.MinimalPlugin{}
container.RegisterPlugin("minimal", minimalPlugin)
```

### Example 2: Plugin with Config

```go
// plugins/email/config.go
package email

type Config struct {
	SMTPHost string `yaml:"smtp_host" default:"smtp.gmail.com" validate:"required"`
	SMTPPort int    `yaml:"smtp_port" default:"587" validate:"gte=1,lte=65535"`
	Username string `yaml:"username" validate:"required"`
	Password string `yaml:"password" validate:"required"`
}

// plugins/email/plugin.go
type EmailPlugin struct {
	config Config
	client *smtp.Client
}

func (p *EmailPlugin) Initialize(exec plugin.Execution) error {
	addr := fmt.Sprintf("%s:%d", p.config.SMTPHost, p.config.SMTPPort)
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	p.client = client
	return nil
}

func (p *EmailPlugin) Shutdown(exec plugin.Execution) error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

func (p *EmailPlugin) Send(exec plugin.Execution, args map[string]any) (map[string]any, error) {
	to := args["to"].(string)
	subject := args["subject"].(string)
	body := args["body"].(string)

	// Use p.client and p.config

	return map[string]any{"sent": true}, nil
}
```

```yaml
# flow-config.yaml
plugins:
  - source: ./plugins/email
    config:
      smtp_host: smtp.gmail.com
      username: ${SMTP_USERNAME}
      password: ${SMTP_PASSWORD}
```

```go
// Generated main.go
emailConfig := email.Config{}
plugin.PrepareConfig(&emailConfig)

if val := os.Getenv("SMTP_USERNAME"); val != "" {
emailConfig.Username = val
}
if val := os.Getenv("SMTP_PASSWORD"); val != "" {
emailConfig.Password = val
}

plugin.ValidateConfig(emailConfig)

emailPlugin := &email.EmailPlugin{
config: emailConfig,
}
container.RegisterPlugin("email", emailPlugin)
```

### Example 3: Plugin with Dependencies

```go
// plugins/payment/plugin.go
package payment

import (
	"github.com/sflowg/sflowg/runtime/plugin"
	"github.com/sflowg/sflowg/plugins/http"
	"github.com/sflowg/sflowg/plugins/redis"
)

type Config struct {
	StripeAPIKey string `yaml:"stripe_api_key" validate:"required"`
}

type PaymentPlugin struct {
	http   *http.HTTPPlugin
	redis  *redis.RedisPlugin
	config Config
}

func (p *PaymentPlugin) Initialize(exec plugin.Execution) error {
	exec.Logger().Info("Payment plugin initialized")
	return nil
}

func (p *PaymentPlugin) Charge(exec plugin.Execution, args map[string]any) (map[string]any, error) {
	amount := args["amount"].(float64)

	// Use dependencies
	result, err := p.http.Request(exec, map[string]any{
		"url": "https://api.stripe.com/charges",
		"headers": map[string]string{
			"Authorization": "Bearer " + p.config.StripeAPIKey,
		},
	})

	return result, err
}
```

```yaml
# flow-config.yaml
plugins:
  - source: http
  - source: redis
  - source: ./plugins/payment
    config:
      stripe_api_key: ${STRIPE_API_KEY}
```

```go
// Generated main.go (in dependency order)
httpPlugin := &http.HTTPPlugin{}
container.RegisterPlugin("http", httpPlugin)

redisPlugin := &redis.RedisPlugin{}
container.RegisterPlugin("redis", redisPlugin)

paymentConfig := payment.Config{}
plugin.PrepareConfig(&paymentConfig)
if val := os.Getenv("STRIPE_API_KEY"); val != "" {
paymentConfig.StripeAPIKey = val
}
plugin.ValidateConfig(paymentConfig)

paymentPlugin := &payment.PaymentPlugin{
http:   httpPlugin,  // Injected
redis:  redisPlugin, // Injected
config: paymentConfig,
}
container.RegisterPlugin("payment", paymentPlugin)
```

### Example 4: Typed Tasks

```go
// plugins/payment/types.go
package payment

type ChargeInput struct {
	Amount   float64 `json:"amount" validate:"required,gt=0"`
	Currency string  `json:"currency" validate:"required,iso4217"`
}

type ChargeOutput struct {
	ChargeID string `json:"charge_id"`
	Status   string `json:"status"`
}

// plugins/payment/plugin.go
func (p *PaymentPlugin) ChargeTyped(exec plugin.Execution, input ChargeInput) (ChargeOutput, error) {
	// Fully typed - IDE autocomplete works
	if input.Amount < 1.00 {
		return ChargeOutput{}, fmt.Errorf("minimum is 1.00")
	}

	return ChargeOutput{
		ChargeID: "ch_123",
		Status:   "succeeded",
	}, nil
}

// Map-based still works
func (p *PaymentPlugin) Refund(exec plugin.Execution, args map[string]any) (map[string]any, error) {
	return map[string]any{"refund_id": "re_123"}, nil
}
```

---

## Implementation Steps

### Phase 2.1: Core Infrastructure

**Duration:** 1-2 weeks

1. **Create runtime/plugin package**
    - [ ] `execution.go` - Execution interface
    - [ ] `interfaces.go` - Lifecycle, HealthChecker
    - [ ] `config.go` - Config utilities (defaults, validation)
    - [ ] `types.go` - TaskExecutor

2. **Update runtime package**
    - [ ] `execution_impl.go` - Implement plugin.Execution
    - [ ] Update Container to use plugin.Execution
    - [ ] Update all interfaces to use plugin.Execution

3. **Update existing plugins**
    - [ ] Migrate HTTP plugin to use runtime/plugin
    - [ ] Add Config struct with tags
    - [ ] Update method signatures

**Validation:**

- [ ] Existing plugins compile with new structure
- [ ] No breaking changes to Phase 1 functionality

### Phase 2.2: Dependency Injection

**Duration:** 1-2 weeks

1. **CLI Analyzer**
    - [ ] `analyzer/plugin.go` - Analyze plugin structs
    - [ ] Detect dependency fields (pointer to plugin)
    - [ ] Extract inject tags
    - [ ] Detect config field

2. **Dependency Graph**
    - [ ] `graph/dependency.go` - Build dependency graph
    - [ ] Topological sort
    - [ ] Cycle detection

3. **Code Generator**
    - [ ] Update `generator/main.go` for DI
    - [ ] Generate struct literal initialization
    - [ ] Inject dependencies in correct order

**Validation:**

- [ ] Test with 2-3 plugin dependency chain
- [ ] Detect and error on circular dependencies
- [ ] Verify initialization order

### Phase 2.3: Configuration System

**Duration:** 1 week

1. **Config Analysis**
    - [ ] `analyzer/config.go` - Detect Config type
    - [ ] Extract field tags (yaml, default, validate)
    - [ ] Parse environment variable syntax

2. **Config Generation**
    - [ ] Generate PrepareConfig calls
    - [ ] Generate environment variable overrides
    - [ ] Generate literal value assignments
    - [ ] Generate validation calls

3. **Framework Utilities**
    - [ ] Implement `plugin.PrepareConfig()`
    - [ ] Implement `plugin.ValidateConfig()`
    - [ ] Register custom validators

**Validation:**

- [ ] Test defaults from tags
- [ ] Test env var overrides
- [ ] Test literal values
- [ ] Test validation errors

### Phase 2.4: Typed Tasks

**Duration:** 1-2 weeks

1. **Runtime Detection**
    - [ ] Update `isValidTaskSignature()` for typed
    - [ ] Implement `createTypedWrapper()`
    - [ ] Add conversion utilities (mapToStruct, structToMap)

2. **Validation**
    - [ ] Validate typed inputs before calling method
    - [ ] Handle conversion errors gracefully

3. **Testing**
    - [ ] Create test plugin with typed tasks
    - [ ] Test both map and typed signatures
    - [ ] Test validation errors

**Validation:**

- [ ] Typed tasks work correctly
- [ ] Map-based tasks still work
- [ ] Mixed signatures in same plugin work

### Phase 2.5: Testing & Documentation

**Duration:** 1 week

1. **Integration Tests**
    - [ ] End-to-end test with 3+ plugins
    - [ ] Test dependency injection
    - [ ] Test configuration system
    - [ ] Test typed tasks

2. **Documentation**
    - [ ] Update plugin development guide
    - [ ] Add examples for each pattern
    - [ ] Document all conventions

3. **Migration Guide**
    - [ ] Phase 1 → Phase 2 migration steps
    - [ ] Breaking changes list
    - [ ] Code examples

**Validation:**

- [ ] All tests pass
- [ ] Documentation complete
- [ ] Example plugins work

---

## Summary

### Key Design Decisions

1. **No Constructor Required** - Struct literal initialization preferred
2. **Convention-Based DI** - Field names match plugin names
3. **Minimal Imports** - Plugins import `runtime/plugin` only
4. **Optional Typed Tasks** - Support both map and typed signatures
5. **Tag-Based Config** - Zero boilerplate for plugin authors
6. **Execution Context** - Use `plugin.Execution` everywhere (not `context.Context`)

### Plugin Author Experience

**Minimal Plugin:**

```go
type MyPlugin struct {}

func (p *MyPlugin) Task(exec plugin.Execution, args map[string]any) (map[string]any, error) {
return map[string]any{"result": "done"}, nil
}
```

**Full-Featured Plugin:**

```go
type Config struct {
APIKey string `yaml:"api_key" validate:"required"`
}

type MyPlugin struct {
http   *http.HTTPPlugin // Auto-injected
config Config           // Auto-validated
}

func (p *MyPlugin) Initialize(exec plugin.Execution) error {
return nil
}

func (p *MyPlugin) Task(exec plugin.Execution, input Input) (Output, error) {
return Output{}, nil
}
```

**Zero boilerplate. Maximum power.**

---

**End of Implementation Plan**