# Getting Started

Build your first SFlowG service in 5 minutes.

## Prerequisites

- Go 1.21 or later installed
- Basic familiarity with YAML

## Step 1: Install the CLI

```bash
go install github.com/sflowg/sflowg/cli@latest
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
plugins:
  - source: http
```

This configures your project to use the built-in HTTP plugin.

## Step 4: Create Your First Flow

Create `flows/hello.yaml`:

```yaml
id: hello_flow
entrypoint:
  type: http
  config:
    method: get
    path: /hello/:name

steps:
  - id: build_greeting
    type: assign
    args:
      greeting: '"Hello, " + request.params.name + "!"'
      timestamp: 'now()'

return:
  type: json
  args:
    message: build_greeting.greeting
    time: build_greeting.timestamp
```

**What this does:**
- Creates a GET endpoint at `/hello/:name`
- Extracts the `name` path parameter
- Builds a greeting message using an expression
- Returns JSON response

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
  "message": "Hello, World!",
  "time": "2024-01-15T10:30:00Z"
}
```

## Adding More Features

### Multiple Steps

Create `flows/calc.yaml`:

```yaml
id: calculate
entrypoint:
  type: http
  config:
    method: post
    path: /calculate

steps:
  - id: parse_input
    type: assign
    args:
      a: request.body.a
      b: request.body.b
      operation: request.body.operation

  - id: compute
    type: switch
    args:
      - condition: 'parse_input.operation == "add"'
        steps:
          - id: add
            type: assign
            args:
              result: parse_input.a + parse_input.b
      - condition: 'parse_input.operation == "multiply"'
        steps:
          - id: multiply
            type: assign
            args:
              result: parse_input.a * parse_input.b

return:
  type: json
  args:
    result: compute.result
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

```yaml
id: fetch_user
entrypoint:
  type: http
  config:
    method: get
    path: /user/:id

steps:
  - id: get_user
    type: http.request
    args:
      method: GET
      url: '"https://api.example.com/users/" + request.params.id'

return:
  type: json
  args:
    user: get_user.response.body
```

### Environment Variables

Use environment variables in `flow-config.yaml`:

```yaml
name: my-api

properties:
  apiBaseUrl: ${API_BASE_URL:http://localhost:9000}
  apiKey: ${API_KEY}

plugins:
  - source: http
    config:
      timeout: ${HTTP_TIMEOUT:30s}
```

Access properties in flows:
```yaml
steps:
  - id: call_api
    type: http.request
    args:
      url: 'properties.apiBaseUrl + "/data"'
      headers:
        Authorization: '"Bearer " + properties.apiKey'
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