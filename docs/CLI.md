# SFlowG CLI

The SFlowG CLI (`sflowg`) compiles flow definitions into standalone Go executables.

## Installation

```bash
# Install from source
go install github.com/BDNK1/sflowg/cli@latest

# Or build locally
cd cli
go build -o sflowg .
```

## Commands

### `sflowg build [project-dir]`

Compiles flows into a standalone executable binary.

```bash
sflowg build [project-dir] [flags]
```

**Arguments:**
- `project-dir` - Directory containing `flow-config.yaml` (default: current directory)

**Flags:**
| Flag | Description |
|------|-------------|
| `--runtime-path <path>` | Use local runtime module (development) |
| `--core-plugins-path <path>` | Use local core plugins directory (development) |
| `--embed-flows` | Embed flow files into binary (production) |
| `--help` | Show help |

**Examples:**

```bash
# Build current directory
sflowg build

# Build specific project
sflowg build ./my-project

# Development mode with local runtime
sflowg build . \
  --runtime-path ../runtime \
  --core-plugins-path ../plugins

# Production build with embedded flows
sflowg build . --embed-flows
```

## Build Modes

### Development Mode (Default)

Flows are loaded at runtime from the file system. Allows rapid iteration without rebuilding.

```bash
sflowg build .
./my-app --flows ./flows
```

**Use for:** Local development, testing flow changes, debugging.

### Production Mode

Flows are embedded directly into the binary using Go's `//go:embed`.

```bash
sflowg build . --embed-flows
./my-app
```

**Use for:** Production deployments, single-file distribution, immutable deployments.

## Project Structure

```
my-project/
├── flow-config.yaml     # Project configuration (see FLOW_CONFIG.md)
├── flows/               # Flow definitions (see FLOW_SYNTAX.md)
│   ├── auth.flow
│   └── payment.flow
├── plugins/             # Local plugins (optional)
│   └── custom/
│       ├── go.mod
│       └── plugin.go
└── my-project           # Generated binary (after build)
```

For configuration details, see [FLOW_CONFIG.md](./FLOW_CONFIG.md).

## Build Output

### Binary Location

The generated binary is placed in the project directory:

```bash
sflowg build ./payment-system
# Creates: ./payment-system/payment-system
```

### Binary Naming

Binary name follows this precedence:
1. `name` field in `flow-config.yaml`
2. Project directory name

### Generated Binary Flags

```bash
./my-app [flags]

Flags:
  --flows <path>   Path to flows directory (dev mode only)
  --port <port>    Override HTTP server port (default: 8080)
```

**Examples:**
```bash
./my-app --flows ./flows        # Specify flows directory
./my-app --port 3000            # Custom port
./my-app --flows ./flows --port 3000
```

### Flow File Resolution

In development mode, the generated binary resolves flow files in this order:
1. `--flows <path>`
2. `FLOWS_PATH`
3. `flows/` next to the binary

Notes:
- Runtime flow loading currently reads `*.flow` files from the resolved directory.
- The binary does not scan its own directory as a fallback.
- On startup, the binary logs the resolved flows directory and its source (`flag`, `env`, `adjacent_flows_dir`, or `embedded`).

Examples:
```bash
./my-app --flows ./flows
FLOWS_PATH=/srv/my-app/flows ./my-app
./my-app                     # uses ./flows next to the binary if present
```

## Development Workflow

### 1. Create Project

```bash
mkdir my-project && cd my-project
mkdir flows
```

Create `flow-config.yaml`:
```yaml
name: my-project
plugins:
  - source: http
```

Create `flows/hello.flow`:
```sflowg
entrypoint.http {
    method: GET
    path: /hello
}

return response.json({
    status: 200,
    body: {
        message: "Hello, World!"
    }
})
```

### 2. Build

```bash
# With local SFlowG source
sflowg build . \
  --runtime-path /path/to/sflowg/runtime \
  --core-plugins-path /path/to/sflowg/plugins

# Or with published modules
sflowg build .
```

### 3. Run

```bash
./my-project --flows ./flows
```

### 4. Test

```bash
curl http://localhost:8080/hello
# {"message":"Hello, World!"}
```

## Quick Reference

```bash
# Build
sflowg build                    # Current directory
sflowg build ./project          # Specific project
sflowg build . --embed-flows    # Production mode

# Development flags
sflowg build . --runtime-path ../runtime
sflowg build . --core-plugins-path ../plugins

# Run binary
./my-app                        # Production (embedded)
./my-app --flows ./flows        # Development
./my-app --port 3000            # Custom port

# Help
sflowg --help
sflowg build --help
```

## Related Documentation

- [FLOW_CONFIG.md](./FLOW_CONFIG.md) - Project configuration reference
- [FLOW_SYNTAX.md](./FLOW_SYNTAX.md) - Flow DSL syntax reference
- [PLUGIN_DEVELOPMENT.md](./PLUGIN_DEVELOPMENT.md) - Creating custom plugins
- [CLI Technical Docs](../cli/README.md) - CLI internals for contributors
