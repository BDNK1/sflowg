# Getting Started

Build your first SFlowG service in 5 minutes.

## Prerequisites

- Go 1.21 or later installed
- Basic familiarity with YAML and Risor-style expressions

## Step 1: Install the CLI

```bash
go install github.com/BDNK1/sflowg/cli@latest
```

Verify installation:
```bash
sflowg --help
```

## Step 2: Create Project Structure

```bash
mkdir my-api
cd my-api
mkdir flows
```

## Step 3: Create Configuration

Create `flow-config.yaml`:

```yaml
name: my-api

runtime:
  engine: dsl
  port: 8080

plugins:
  - source: http
```

This configures your project to load `.flow` files with the DSL engine and use
the built-in HTTP plugin.

## Step 4: Create Your First Flow

Create `flows/hello.flow`:

```sflowg
entrypoint.http {
    method: GET
    path: /hello/:name
    pathVariables: {
        name: { type: string, required: true }
    }
}

step build_greeting {
    {
        message: "Hello, " + request.pathVariables.name + "!"
    }
}

return response.json({
    status: 200,
    body: {
        message: build_greeting.message
    }
})
```

**What this does:**
- Creates a GET endpoint at `/hello/:name`
- Extracts the `name` path variable into `request.pathVariables.name`
- Runs the `build_greeting` step and stores its result under that step name
- Returns an HTTP JSON response

## Step 5: Build

```bash
sflowg build .
```

This generates a `my-api` binary in your project directory.

## Step 6: Run

```bash
./my-api --flows ./flows
```

You should see:
```
Starting server on :8080
Loaded 1 flow(s)
```

## Step 7: Test

```bash
curl http://localhost:8080/hello/World
```

Response:
```json
{
  "message": "Hello, World!"
}
```

## Adding More Features

### Multiple Steps

Create `flows/calc.flow`:

```sflowg
entrypoint.http {
    method: POST
    path: /calculate
    body: {
        type: json
        schema: {
            a: { type: integer, required: true }
            b: { type: integer, required: true }
            operation: { type: string, required: true, enum: [add, multiply] }
        }
    }
}

step parse_input {
    {
        a: request.body.a,
        b: request.body.b,
        operation: request.body.operation
    }
}

step compute {
    match parse_input.operation {
        "add" => parse_input.a + parse_input.b
        "multiply" => parse_input.a * parse_input.b
        _ => raise("INVALID_OPERATION", "unsupported operation")
    }
}

return response.json({
    status: 200,
    body: {
        result: compute
    }
})
```

Rebuild and test:
```bash
sflowg build .
./my-api --flows ./flows

curl -X POST http://localhost:8080/calculate \
  -H "Content-Type: application/json" \
  -d '{"a": 5, "b": 3, "operation": "add"}'
# {"result": 8}
```

### HTTP Requests

Call external APIs using the HTTP plugin:

```sflowg
entrypoint.http {
    method: GET
    path: /user/:id
    pathVariables: {
        id: { type: string, required: true }
    }
}

step get_user {
    http.request({
        method: "GET",
        url: "https://api.example.com/users/" + request.pathVariables.id
    })
}

return response.json({
    status: 200,
    body: {
        user: get_user.body
    }
})
```

### Environment Variables

Use environment variables in `flow-config.yaml`:

```yaml
name: my-api

runtime:
  engine: dsl

properties:
  api_base_url: ${API_BASE_URL:http://localhost:9000}
  api_key: ${API_KEY}

plugins:
  - source: http
    config:
      timeout: ${HTTP_TIMEOUT:30s}
```

Access properties in flows:
```sflowg
step call_api {
    http.request({
        method: "GET",
        url: properties.api_base_url + "/data",
        headers: {
            Authorization: "Bearer " + properties.api_key
        }
    })
}
```

## Production Deployment

Build with embedded flows for production:

```bash
sflowg build . --embed-flows
./my-api  # No --flows flag needed
```

## Next Steps

- [Flow Syntax Reference](./FLOW_SYNTAX.md) - All step types and expressions
- [Configuration Reference](./FLOW_CONFIG.md) - Project configuration options
- [CLI Reference](./CLI.md) - Build commands and options
- [Plugin Development](./PLUGIN_DEVELOPMENT.md) - Create custom plugins
- [Examples](./examples/) - Complete working projects
