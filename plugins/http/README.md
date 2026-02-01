# HTTP Plugin

A powerful HTTP client plugin for SFlowG that provides reliable HTTP request execution with automatic retries, timeouts, and comprehensive configuration options.

## Features

- ✅ **All HTTP Methods** - GET, POST, PUT, PATCH, DELETE, etc.
- ✅ **Automatic Retries** - Configurable retry logic with exponential backoff
- ✅ **Timeout Management** - Per-request timeout configuration
- ✅ **Headers & Query Parameters** - Full support with expression evaluation
- ✅ **JSON & Form-Encoded Bodies** - JSON (default) or `application/x-www-form-urlencoded`
- ✅ **Debug Mode** - Detailed request/response logging
- ✅ **Tag-Based Configuration** - Zero-boilerplate setup

## Installation

```yaml
# flow-config.yaml
plugins:
  - source: github.com/sflowg/sflowg/plugins/http
    config:
      timeout: 30s
      max_retries: 3
      debug: false
      retry_wait_ms: 100
```

## Configuration

The HTTP plugin uses declarative configuration tags. All fields have sensible defaults and validation rules.

### Configuration Options

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `timeout` | duration | `30s` | `>= 1s` | HTTP request timeout |
| `max_retries` | int | `3` | `0-10` | Maximum number of retry attempts |
| `debug` | bool | `false` | - | Enable debug logging for requests |
| `retry_wait_ms` | int | `100` | `0-10000` | Wait time between retries (milliseconds) |

### Configuration Examples

**Minimal (use all defaults):**
```yaml
plugins:
  - source: github.com/sflowg/sflowg/plugins/http
```

**Custom timeouts:**
```yaml
plugins:
  - source: github.com/sflowg/sflowg/plugins/http
    config:
      timeout: 60s
      max_retries: 5
      retry_wait_ms: 500
```

**Debug mode:**
```yaml
plugins:
  - source: github.com/sflowg/sflowg/plugins/http
    config:
      debug: true
      timeout: 10s
```

**Environment variable overrides:**
```yaml
plugins:
  - source: github.com/sflowg/sflowg/plugins/http
    config:
      timeout: ${HTTP_TIMEOUT:-30s}
      max_retries: ${HTTP_MAX_RETRIES:-3}
      debug: ${HTTP_DEBUG:-false}
```

## Tasks

### `http.request`

Executes an HTTP request with full control over method, headers, query parameters, and body.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `url` | string | ✅ | Target URL for the request |
| `method` | string | ✅ | HTTP method (GET, POST, PUT, PATCH, DELETE, etc.) |
| `headers` | map | ❌ | Request headers (supports expressions) |
| `query_parameters` | map | ❌ | URL query parameters (supports expressions) |
| `body` | map | ❌ | Request body (supports expressions) |
| `content_type` | string | ❌ | Body encoding: `json` (default) or `form` |

#### Returns

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | HTTP status text (e.g., "200 OK") |
| `status_code` | int | HTTP status code (e.g., 200) |
| `is_error` | bool | Whether the request resulted in an error |
| `headers` | map | Response headers (e.g., `headers.Content-Type`) |
| `body` | map | Response body fields |

## Usage Examples

### Basic GET Request

```yaml
steps:
  - id: fetch_user
    type: http.request
    args:
      url: '"https://api.example.com/users/123"'
      method: GET
```

### POST with JSON Body

```yaml
steps:
  - id: create_user
    type: http.request
    args:
      url: '"https://api.example.com/users"'
      method: POST
      body:
        name: '"John Doe"'
        email: '"john@example.com"'
        age: 30
```

### POST with Form-Encoded Body

For APIs that require `application/x-www-form-urlencoded` (like Stripe):

```yaml
steps:
  - id: create_payment_intent
    type: http.request
    args:
      url: '"https://api.stripe.com/v1/payment_intents"'
      method: POST
      content_type: form
      headers:
        Authorization: '"Bearer " + properties.stripe_secret_key'
      body:
        amount: 1099
        currency: '"usd"'
        payment_method_types:
          - card
        metadata:
          order_id: extract_data.order_id
```

Nested objects use bracket notation: `metadata[order_id]=value`

### Using Expressions

```yaml
steps:
  - id: api_call
    type: http.request
    args:
      url: '"https://api.example.com/users/" + request.body.user_id'
      method: GET
      headers:
        Authorization: '"Bearer " + properties.api_token'
        X-Request-ID: execution.id
```

### With Query Parameters

```yaml
steps:
  - id: search_users
    type: http.request
    args:
      url: '"https://api.example.com/users"'
      method: GET
      query_parameters:
        page: '"1"'
        limit: '"10"'
        search: request.query.q
        sort: '"created_at"'
```

### Complete Example with All Features

```yaml
steps:
  - id: create_order
    type: http.request
    args:
      url: '"https://api.example.com/orders"'
      method: POST
      headers:
        Content-Type: '"application/json"'
        Authorization: '"Bearer " + properties.api_key'
        X-Idempotency-Key: execution.id
      query_parameters:
        notify: '"true"'
        async: '"false"'
      body:
        customer_id: fetch_user.result.body.id
        items:
          - product_id: '"123"'
            quantity: 2
          - product_id: '"456"'
            quantity: 1
        total: calculate_total.result.amount
```

### Accessing Response Data

```yaml
steps:
  - id: get_user
    type: http.request
    args:
      url: '"https://api.example.com/users/123"'
      method: GET

  - id: use_response
    type: assign
    args:
      # Access status code
      status_code: get_user.result.status_code

      # Access response headers
      content_type: get_user.result.headers.Content-Type
      request_id: get_user.result.headers.X-Request-Id

      # Access response body fields
      user_id: get_user.result.body.id
      user_name: get_user.result.body.name

      # Check if request was successful
      was_successful: get_user.result.is_error == false
```

## Error Handling

The HTTP plugin handles errors at multiple levels:

### Request Failures

If a request fails (network error, timeout, etc.), the task returns an error that can be handled with retry configuration.

### HTTP Error Status Codes

HTTP error responses (4xx, 5xx) are returned as regular responses with `is_error: true`:

```yaml
steps:
  - id: api_call
    type: http.request
    args:
      url: '"https://api.example.com/users/999"'
      method: GET

  - id: check_status
    type: switch
    args:
      handle_error: api_call.result.is_error == true
      process_user: api_call.result.is_error == false
```

### Automatic Retries

The plugin automatically retries failed requests based on configuration:

- Retries only for network errors and 5xx status codes
- Exponential backoff between retries
- Configurable via `max_retries` and `retry_wait_ms`

## Advanced Usage

### Dynamic URLs

```yaml
properties:
  base_url: "https://api.example.com"

steps:
  - id: dynamic_request
    type: http.request
    args:
      url: properties.base_url + "/users/" + request.params.id
      method: GET
```

### Conditional Headers

```yaml
steps:
  - id: authenticated_request
    type: http.request
    args:
      url: '"https://api.example.com/protected"'
      method: GET
      headers:
        Authorization: 'auth.result.token != "" ? "Bearer " + auth.result.token : ""'
```

### Response Transformation

```yaml
steps:
  - id: fetch_data
    type: http.request
    args:
      url: '"https://api.example.com/data"'
      method: GET

  - id: transform_response
    type: assign
    args:
      formatted_data:
        id: fetch_data.result.body.id
        name: fetch_data.result.body.attributes.name
        status_code: fetch_data.result.status_code
```

## Debugging

Enable debug mode to see detailed request/response information:

```yaml
plugins:
  - source: github.com/sflowg/sflowg/plugins/http
    config:
      debug: true
```

Debug output includes:
- Request URL, method, headers, body
- Response status, headers, body
- Timing information
- Retry attempts

## Best Practices

1. **Use Timeouts Appropriately**
   - Set shorter timeouts for internal APIs
   - Set longer timeouts for external services
   - Consider the operation type (quick lookups vs. long processing)

2. **Configure Retries Based on Operation**
   - Use retries for idempotent operations (GET, PUT, DELETE)
   - Be cautious with retries for POST (may duplicate data)
   - Adjust retry wait time based on expected recovery time

3. **Handle Errors Explicitly**
   - Always check `is_error` in response
   - Use switch or condition to handle error cases
   - Log error responses for debugging

4. **Use Expressions Wisely**
   - Evaluate expressions in headers for dynamic authentication
   - Use expressions in body for dynamic data
   - Avoid complex logic in expressions (move to separate steps)

5. **Leverage Query Parameters**
   - Use query parameters for filtering, pagination, sorting
   - Expressions work in query parameter values
   - Keep URLs clean by using query parameters instead of path parameters

## Performance Considerations

- **Connection Pooling**: The plugin uses connection pooling automatically
- **Retries**: Each retry adds latency; tune `max_retries` appropriately
- **Timeout**: Balance between user experience and reliability
- **Debug Mode**: Disable in production for better performance

## Troubleshooting

### Timeouts

If experiencing frequent timeouts:
```yaml
config:
  timeout: 60s  # Increase timeout
  max_retries: 5  # More retry attempts
  retry_wait_ms: 1000  # Wait longer between retries
```

### SSL/TLS Issues

For self-signed certificates or SSL issues, check your runtime environment's certificate configuration.

### Large Responses

For very large responses, consider:
- Streaming endpoints (if available)
- Pagination with multiple smaller requests
- Increasing timeout values

## License

Part of the SFlowG project.
