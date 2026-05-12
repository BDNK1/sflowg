# SFlowG

**Service Flow Generator** - Build HTTP services from declarative `.flow` definitions.

SFlowG compiles declarative flow files into standalone Go binaries. Define your API endpoints, business logic, and integrations in the SFlowG DSL - get a production-ready executable.

## Why SFlowG?

- **Declarative**: Define workflows in `.flow` files, not application code
- **Compiled**: Single binary deployment, no runtime dependencies
- **Extensible**: Custom plugins for any integration
- **Fast**: Native Go performance with zero cold start
- **Observable**: Structured logs plus optional OpenTelemetry tracing

## Quick Example

**flow-config.yaml:**
```yaml
name: my-api
runtime:
  engine: dsl
plugins:
  - source: http
```

**flows/user.flow:**
```sflowg
entrypoint.http {
    method: GET
    path: /user/:id
    pathVariables: {
        id: { type: string, required: true }
    }
}

step fetch_user {
    http.request({
        method: "GET",
        url: "https://jsonplaceholder.typicode.com/users/" + request.pathVariables.id
    })
}

step transform {
    {
        id: fetch_user.body.id,
        name: fetch_user.body.name,
        email: fetch_user.body.email
    }
}

return response.json({
    status: 200,
    body: {
        user: transform
    }
})
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
| [Flow Syntax](docs/FLOW_SYNTAX.md) | Complete `.flow` DSL reference |
| [Configuration](docs/FLOW_CONFIG.md) | Project configuration (flow-config.yaml) |
| [CLI Reference](docs/CLI.md) | Build tool commands and options |
| [Plugin Development](docs/PLUGIN_DEVELOPMENT.md) | Create custom plugins |

## Project Structure

```
my-project/
├── flow-config.yaml    # Project configuration
├── flows/              # Flow definitions
│   └── *.flow
├── plugins/            # Local plugins (optional)
│   └── custom/
└── my-project          # Generated binary
```

## Core Concepts

### Flows
`.flow` files that define complete request-response cycles. Each flow can be an HTTP endpoint, Kafka consumer, cron job, or subflow.

### Entrypoint
Defines how a flow is triggered, such as HTTP method and path, Kafka topic, cron schedule, or subflow input.

### Steps
Sequential processing units within a flow. Each step has an ID and a Risor body; the last expression becomes the step result.

### Step Bodies
- Use Risor expressions and statements for data transformation and control flow
- Call plugin tasks as functions, such as `http.request(...)`
- Call subflows with `flow.call(...)`
- Run work in the background with `async step`

### Return
Defines the response sent back to the caller, such as `response.json(...)`, `response.ack()`, or `response.value(...)`.

### Plugins
Extend SFlowG with custom tasks. Use core plugins (`http`), local plugins (`./plugins/custom`), or remote plugins (`github.com/user/plugin`).

## Examples

See [docs/examples](docs/examples/) for complete working projects:
- [E-commerce API](docs/examples/ecom/) - Orders, payments, subflows, async work, and Postgres
- [Stripe Integration](docs/examples/stripe-integration/) - Webhooks, checkout flows, and custom plugins
- [Kafka Consumer](docs/examples/kafka-consumer/) - Kafka entrypoint example
- [Foreach](docs/examples/foreach/) - Sequential and parallel foreach over request arrays
