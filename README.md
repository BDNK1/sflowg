# SFlowG

**Service Flow Generator** - Build HTTP services from YAML workflow definitions.

SFlowG compiles declarative YAML flows into standalone Go binaries. Define your API endpoints, business logic, and integrations in YAML - get a production-ready executable.

## Why SFlowG?

- **Declarative**: Define workflows in YAML, not code
- **Compiled**: Single binary deployment, no runtime dependencies
- **Extensible**: Custom plugins for any integration
- **Fast**: Native Go performance with zero cold start

## Quick Example

**flow-config.yaml:**
```yaml
name: my-api
plugins:
  - source: http
```

**flows/user.yaml:**
```yaml
id: get_user
entrypoint:
  type: http
  config:
    method: get
    path: /user/:id

steps:
  - id: fetch
    type: http.request
    args:
      method: GET
      url: '"https://jsonplaceholder.typicode.com/users/" + request.params.id'

  - id: transform
    type: assign
    args:
      user:
        id: fetch.result.body.id
        name: fetch.result.body.name
        email: fetch.result.body.email

return:
  type: json
  args:
    user: transform.user
```

**Build and run:**
```bash
sflowg build .
./my-api --flows ./flows

curl http://localhost:8080/user/1
# {"user":{"id":1,"name":"Leanne Graham","email":"Sincere@april.biz"}}
```

## Installation

```bash
go install github.com/BDNK1/sflowg/cli@latest
```

## Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/GETTING_STARTED.md) | 5-minute tutorial to build your first service |
| [Flow Syntax](docs/FLOW_SYNTAX.md) | Complete YAML flow reference |
| [Configuration](docs/FLOW_CONFIG.md) | Project configuration (flow-config.yaml) |
| [CLI Reference](docs/CLI.md) | Build tool commands and options |
| [Plugin Development](docs/PLUGIN_DEVELOPMENT.md) | Create custom plugins |

## Project Structure

```
my-project/
├── flow-config.yaml    # Project configuration
├── flows/              # Flow definitions
│   └── *.yaml
├── plugins/            # Local plugins (optional)
│   └── custom/
└── my-project          # Generated binary
```

## Core Concepts

### Flows
YAML files that define complete request-response cycles. Each flow is an independent HTTP endpoint.

### Entrypoint
Defines how a flow is triggered - HTTP method, path, headers, and body configuration.

### Steps
Sequential processing units within a flow. Each step has an `id`, `type`, and `args`.

### Step Types
- **assign** - Set variables and transform data
- **switch** - Conditional branching
- **plugin.task** - Call plugin methods (e.g., `http.request`)

### Return
Defines the response sent back to the client - status code, headers, and body.

### Plugins
Extend SFlowG with custom tasks. Use core plugins (`http`), local plugins (`./plugins/custom`), or remote plugins (`github.com/user/plugin`).

## Examples

See [docs/examples](docs/examples/) for complete working projects:
- [Payment System Integration](docs/examples/payment-system-integration/) - Multi-service workflow with retries
