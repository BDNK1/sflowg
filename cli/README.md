# SFlowG CLI

Build tool that compiles YAML flows into Go binaries.

**User docs**: [docs/CLI.md](../docs/CLI.md)

## Build Pipeline

```
flow-config.yaml → config → detector → analyzer → graph → generator → builder → binary
```

1. **config** - Load flow-config.yaml, validate, apply defaults
2. **detector** - Classify each plugin as core/local/remote
3. **analyzer** - Parse plugin Go source via AST, extract metadata
4. **graph** - Build dependency graph, compute initialization order
5. **generator** - Generate go.mod and main.go in temp workspace
6. **builder** - Run `go mod tidy` + `go build`, copy binary to project

## Packages

```
cli/
├── cmd/                 # Cobra CLI commands
└── internal/
    ├── config/          # Parse flow-config.yaml
    ├── detector/        # Classify plugin type (core/local/remote)
    ├── analyzer/        # AST analysis - extract plugin metadata
    ├── graph/           # Dependency graph, topological sort
    ├── generator/       # Generate go.mod and main.go
    ├── builder/         # Run go mod tidy and go build
    ├── workspace/       # Temp directory management
    └── security/        # Path traversal prevention
```

## Key Logic

### Plugin Detection (`detector/`)

```
"http"                    → Core   → github.com/BDNK1/sflowg/plugins/http
"./plugins/custom"        → Local  → synthetic import path + replace directive
"github.com/user/plugin"  → Remote → as-is
```

### AST Analysis (`analyzer/`)

Parses Go source without execution. Discovers by convention:
- Plugin struct: exported name ending with `Plugin`
- Config: lowercase `config` field of type `Config`
- Dependencies: pointer fields to other `*XxxPlugin` types
- Tasks: public methods matching `func(*Execution, Input) (Output, error)`

### Dependency Graph (`graph/`)

- Builds DAG from plugin dependencies
- Detects cycles using DFS
- Topological sort via Kahn's algorithm for initialization order

### Code Generation (`generator/`)

**go.mod**: Module declaration, runtime dependency, plugin dependencies, `replace` directives for local modules.

**main.go** (templates in `templates.go`): For each plugin in topological order:
1. Initialize config struct with env vars from `${VAR:default}` syntax
2. Call `runtime.InitializeConfig()` for defaults + validation
3. Create plugin instance with dependencies injected
4. Register with container

Then start app with `runtime.NewApp(container).Start()`.