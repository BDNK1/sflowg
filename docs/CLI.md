# SFlowG CLI

The SFlowG CLI (`sflowg`) compiles YAML-based workflow definitions into standalone Go executables.

## Installation

```bash
# Install from source
go install github.com/sflowg/sflowg/cli@latest

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
│   ├── auth.yaml
│   └── payment.yaml
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

Create `flows/hello.yaml`:
```yaml
id: hello_flow
entrypoint:
  type: http
  config:
    method: get
    path: /hello
steps:
  - id: respond
    type: assign
    args:
      message: "Hello, World!"
return:
  type: json
  args:
    message: respond.message
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

## Common Workflows

### Multi-Environment Deployment

Use environment variables for environment-specific configuration (see [FLOW_CONFIG.md](./FLOW_CONFIG.md#environment-variable-syntax)).

```bash
# Development (uses defaults)
./my-app

# Staging
API_BASE_URL=https://staging-api.example.com ./my-app

# Production
API_BASE_URL=https://api.example.com ./my-app
```

### CI/CD Pipeline

```bash
#!/bin/bash
set -e

# Install CLI
go install github.com/sflowg/sflowg/cli@latest

# Build production binary
sflowg build . --embed-flows

# Smoke test
./my-app --port 8080 &
sleep 2
curl http://localhost:8080/health || exit 1
kill $!

# Deploy
./deploy.sh ./my-app
```

## Troubleshooting

### "plugin not found"

**Cause:** Plugin source is not accessible.

**Solutions:**
- Core plugins: Verify plugin name exists (`http`)
- Local plugins: Check path is correct and relative (`./plugins/custom`)
- Remote plugins: Verify import path and network access

### "required environment variable not set"

**Cause:** Environment variable without default value is missing.

**Solution:**
```bash
export VAR_NAME=value
sflowg build .
```

See [FLOW_CONFIG.md](./FLOW_CONFIG.md#environment-variable-syntax) for syntax.

### "flows not found" at runtime

**Cause:** Binary can't locate flow files.

**Solutions:**
- Development mode: Provide `--flows` flag
  ```bash
  ./my-app --flows ./flows
  ```
- Production mode: Rebuild with `--embed-flows`
  ```bash
  sflowg build . --embed-flows
  ```

### Build is slow

**Cause:** First build downloads Go dependencies.

**Solutions:**
- Subsequent builds use cached dependencies
- Use local development mode:
  ```bash
  sflowg build . --runtime-path ../runtime --core-plugins-path ../plugins
  ```

## Best Practices

1. **Lock plugin versions in production**
   ```yaml
   plugins:
     - source: github.com/user/plugin
       version: v1.2.3
   ```

2. **Use `--embed-flows` for production** - Single binary, immutable deployment

3. **Use environment variables for secrets** - Never commit secrets to flow-config.yaml

4. **Test locally before deploying**
   ```bash
   sflowg build .
   ./my-app --flows ./flows
   # verify it works
   sflowg build . --embed-flows
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
- [FLOW_SYNTAX.md](./FLOW_SYNTAX.md) - Flow YAML syntax reference
- [PLUGIN_DEVELOPMENT.md](./PLUGIN_DEVELOPMENT.md) - Creating custom plugins
- [CLI Technical Docs](../cli/README.md) - CLI internals for contributors