# SFlowG Runtime

Execution engine that runs generated binaries.

**Plugin development**: [docs/PLUGIN_DEVELOPMENT.md](../docs/PLUGIN_DEVELOPMENT.md)

## Lifecycle

**Startup** (`App.Start`):
1. Initialize container - call `Initialize()` on plugins that implement it
2. Load flows from YAML directory into `map[id]Flow`
3. Setup Gin router - register HTTP endpoints from flow entrypoints
4. Start HTTP server

**Request Handling**:
```
HTTP Request → HttpHandler → NewExecution → Executor.ExecuteSteps → toResponse → HTTP Response
```

1. **HttpHandler** - Match route to flow, extract request data (path params, query, headers, body)
2. **NewExecution** - Create context with UUID, values map, merged properties
3. **ExecuteSteps** - Process each step sequentially (assign/switch/task)
4. **toResponse** - Evaluate return expressions, send JSON response

## Files

```
runtime/
├── app.go           # Application lifecycle (start, shutdown)
├── container.go     # Plugin registry, task discovery via reflection
├── executor.go      # Step execution (assign, switch, tasks)
├── execution.go     # Request context, values storage
├── http_handler.go  # Gin routing, request/response handling
├── components.go    # Flow YAML struct definitions
├── config.go        # Plugin config: defaults, validation
├── expression.go    # expr-lang wrapper
├── converter.go     # Map ↔ struct conversion
├── format.go        # Key normalization (dots/hyphens → underscores)
└── plugin/          # Public SDK types for plugin developers
```

## Key Logic

### Task Discovery (`container.go`)

Uses reflection to find plugin methods matching signature:
```go
func (p *Plugin) TaskName(exec *Execution, args map[string]any) (map[string]any, error)
```
Task name: `pluginname.methodname` (lowercase)

Also supports typed signatures with struct input/output - automatically converts via `mapToStruct`/`structToMap`.

### Step Execution (`executor.go`)

For each step:
1. Check `condition` expression (skip if false)
2. Execute by type:
   - `assign`: evaluate expressions recursively, store in values
   - `switch`: evaluate branch conditions, set next step ID to jump
   - `<plugin.task>`: lookup task in container, execute, store result
3. Handle retry if configured (with backoff)
4. Store results as `{stepID}.{key}` or `{stepID}.result.{key}` for tasks

### Expression Evaluation

All expressions go through `format.go` before `expr-lang` evaluation. Converts dots and hyphens to underscores so `step.result.data` becomes `step_result_data` in the values map.

### Request Data Storage

Values are stored with prefixes for namespacing:
- `request.pathVariables.{name}`
- `request.queryParameters.{name}`
- `request.headers.{name}`
- `request.body.{path}` (JSON flattened)
- `properties.{name}`