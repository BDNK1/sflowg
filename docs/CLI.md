# SFlowG CLI

## Overview

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

**Synopsis:**
```bash
sflowg build [project-dir] [flags]
```

**Arguments:**
- `project-dir` - Directory containing `flow-config.yaml` (default: current directory)

**Flags:**
- `--runtime-path <path>` - Use local runtime module (development)
- `--core-plugins-path <path>` - Use local core plugins directory (development)
- `--embed-flows` - Embed flow files into binary (production)
- `--help` - Show help

**Examples:**

```bash
# Build current directory
sflowg build

# Build specific project
sflowg build ./my-project

# Development mode with local runtime and core plugins
sflowg build . \
  --runtime-path ../runtime \
  --core-plugins-path ../plugins

# Production build with embedded flows
sflowg build . --embed-flows
```

## Build Modes

### Development Mode (Default)

Flows are loaded at runtime from the file system.

```bash
# Build
sflowg build .

# Run with flows directory
./my-app --flows ./flows
```

**When to use:**
- Local development
- Rapid iteration on flows
- Testing flow changes

### Production Mode

Flows are embedded directly into the binary.

```bash
# Build with embedded flows
sflowg build . --embed-flows

# Run (no --flows flag needed)
./my-app
```

**When to use:**
- Production deployments
- Single-file distribution
- Immutable deployments

## Project Structure

```
my-project/
├── flow-config.yaml     # Project configuration
├── flows/               # Flow definitions
│   ├── auth.yaml
│   └── payment.yaml
├── plugins/             # Local plugins (optional)
│   └── custom/
│       ├── go.mod
│       └── plugin.go
└── my-project           # Generated binary (after build)
```

## Configuration

### flow-config.yaml

Defines project metadata, runtime settings, properties, and plugins.

**Minimal Example:**
```yaml
name: my-app
plugins:
  - source: http
```

**Complete Example:**
```yaml
name: payment-system
version: "1.0.0"

runtime:
  port: "8080"

properties:
  serviceURL: ${SERVICE_URL:http://localhost:9000}
  enableNotifications: ${ENABLE_NOTIFICATIONS:true}

plugins:
  - source: http
    config:
      timeout: ${HTTP_TIMEOUT:30s}
      max_retries: 3

  - source: ./plugins/custom
    config:
      api_key: ${API_KEY}
```

See [FLOW_CONFIG.md](./FLOW_CONFIG.md) for complete configuration reference.

## Plugin Sources

### Core Plugins

Built-in plugins maintained by SFlowG.

```yaml
plugins:
  - source: http
```

**Available core plugins:**
- `http` - HTTP client with retry/timeout support

### Local Plugins

Custom plugins in your project.

```yaml
plugins:
  - source: ./plugins/custom
  - source: ../shared/plugins/auth
```

**Requirements:**
- Must have `go.mod` file
- Must follow plugin conventions

### Remote Plugins

Plugins from Git repositories.

```yaml
plugins:
  - source: github.com/user/awesome-plugin
    version: v1.2.3
```

**Version resolution:**
- Specify `version` for stable builds
- Omit for `latest`

## Environment Variables

Both `properties` and plugin `config` support environment variable substitution:

**Syntax:**
- `${VAR}` - Required (fails if not set)
- `${VAR:default}` - Optional with default

**Examples:**
```yaml
properties:
  # Required
  apiKey: ${API_KEY}

  # Optional with defaults
  serviceURL: ${SERVICE_URL:http://localhost:8080}
  timeout: ${TIMEOUT:30s}
  debug: ${DEBUG:false}

  # Literals (no substitution)
  taxRate: 0.15
  maxRetries: 3
```

## Development Workflow

### 1. Create Project Structure

```bash
mkdir my-project
cd my-project
mkdir flows

# Create flow-config.yaml
cat > flow-config.yaml << EOF
name: my-project
plugins:
  - source: http
EOF

# Create a flow
cat > flows/hello.yaml << EOF
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
EOF
```

### 2. Build Application

```bash
# For local development with SFlowG source
sflowg build . \
  --runtime-path /path/to/sflowg/runtime \
  --core-plugins-path /path/to/sflowg/plugins

# Or for production (requires published runtime)
sflowg build .
```

### 3. Run Application

```bash
# Development mode
./my-project --flows ./flows

# Production mode (with --embed-flows)
./my-project
```

### 4. Test

```bash
curl http://localhost:8080/hello
# Response: {"message":"Hello, World!"}
```

## Build Output

### Binary Location

The generated binary is placed in the project directory with the project name:

```bash
sflowg build ./payment-system
# Creates: ./payment-system/payment-system
```

### Binary Naming

Binary name follows this precedence:
1. `name` field in `flow-config.yaml`
2. Project directory name

### Runtime Flags

Generated binaries accept these flags:

```bash
./my-app [flags]

Flags:
  --flows string   Path to flows directory (dev mode only)
  --port string    Override HTTP server port
```

**Examples:**
```bash
# Use different flows directory
./my-app --flows /path/to/flows

# Use different port
./my-app --port 3000

# Both
./my-app --flows ./flows --port 3000
```

## Common Workflows

### Multi-Environment Setup

Use environment variables for environment-specific configuration.

**flow-config.yaml:**
```yaml
plugins:
  - source: http
    config:
      base_url: ${API_BASE_URL:http://localhost:8080}
      timeout: ${API_TIMEOUT:30s}
```

**Development:**
```bash
sflowg build .
./my-app
# Uses defaults
```

**Staging:**
```bash
API_BASE_URL=https://staging-api.example.com \
API_TIMEOUT=60s \
./my-app
```

**Production:**
```bash
API_BASE_URL=https://api.example.com \
API_TIMEOUT=60s \
./my-app
```

### CI/CD Pipeline

```bash
#!/bin/bash
set -e

# Install sflowg
go install github.com/sflowg/sflowg/cli@latest

# Set required environment variables
export API_KEY="${PRODUCTION_API_KEY}"
export SERVICE_URL="${PRODUCTION_SERVICE_URL}"

# Build production binary
sflowg build . --embed-flows

# Test binary
./my-app --port 8080 &
APP_PID=$!
sleep 2
curl http://localhost:8080/health || exit 1
kill $APP_PID

# Deploy binary
./deploy.sh ./my-app
```

### Local Plugin Development

```bash
# Project structure
my-project/
├── flow-config.yaml
├── flows/
└── plugins/
    └── custom/
        ├── go.mod
        ├── plugin.go
        └── plugin_test.go

# Build with local plugin
sflowg build .

# Plugin is automatically included
```

## Troubleshooting

### "plugin not found"

**Problem:** Plugin source is not accessible.

**Solution:**
- For core plugins: Ensure plugin exists in SFlowG repository
- For local plugins: Verify path is correct and relative
- For remote plugins: Check import path and network access

### "required environment variable not set"

**Problem:** Required environment variable (without default) is missing.

**Solution:**
```bash
# Check flow-config.yaml for ${VAR} syntax (no default)
# Set the variable:
export VAR=value
sflowg build .
```

### "flows not found" at runtime

**Problem:** Binary can't locate flow files.

**Solution:**
- **Development mode:** Provide `--flows` flag:
  ```bash
  ./my-app --flows ./flows
  ```
- **Production mode:** Rebuild with `--embed-flows`:
  ```bash
  sflowg build . --embed-flows
  ```

### Build is slow

**Problem:** First build downloads dependencies.

**Solution:**
- Subsequent builds are faster (dependencies cached)
- Use local development mode to avoid re-downloading:
  ```bash
  sflowg build . --runtime-path ../runtime --core-plugins-path ../plugins
  ```

## Best Practices

1. **Lock versions in production**
   ```yaml
   plugins:
     - source: github.com/user/plugin
       version: v1.2.3  # Always specify version
   ```

2. **Never commit secrets**
   ```yaml
   config:
     apiKey: ${API_KEY}  # Use env vars for secrets
   ```

3. **Use embedded flows for production**
   ```bash
   sflowg build . --embed-flows
   ```

4. **Test locally before deploying**
   ```bash
   # Build and test locally
   sflowg build .
   ./my-app --flows ./flows

   # Then build for production
   sflowg build . --embed-flows
   ```

5. **Document required environment variables**
   ```yaml
   # flow-config.yaml
   # Required environment variables:
   # - API_KEY: API authentication key
   # - SERVICE_URL: Backend service URL (default: http://localhost:8080)

   plugins:
     - source: http
       config:
         api_key: ${API_KEY}
         service_url: ${SERVICE_URL:http://localhost:8080}
   ```

## Quick Reference

```bash
# Basic build
sflowg build

# Development with local runtime
sflowg build . \
  --runtime-path ../runtime \
  --core-plugins-path ../plugins

# Production build
sflowg build . --embed-flows

# Run generated binary
./my-app                    # Production (embedded flows)
./my-app --flows ./flows    # Development
./my-app --port 3000        # Custom port

# Build specific project
sflowg build /path/to/project

# Get help
sflowg build --help
sflowg --help
```
