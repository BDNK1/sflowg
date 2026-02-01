# Flow Syntax Reference

Complete reference for writing SFlowG flow YAML files.

## Flow Structure

```yaml
id: flow_name                    # Required: unique flow identifier

entrypoint: # Required: how flow is triggered
  type: http
  config: { ... }

properties: # Optional: flow-level variables
  key: value

steps: # Required: processing steps
  - id: step_name
    type: step_type
    args: { ... }

return: # Required: response configuration
  type: json
  args: { ... }
```

## Entrypoint

Defines how the flow is triggered. Currently supports HTTP entrypoints.

### HTTP Entrypoint

```yaml
entrypoint:
  type: http
  config:
    method: get|post|put|delete|patch
    path: /api/resource/:param
    headers:
      - Header-Name
    pathVariables:
      - param
    queryParameters:
      - limit
      - offset
    body:
      type: json
```

**Fields:**

| Field             | Description                                 | Required |
|-------------------|---------------------------------------------|----------|
| `method`          | HTTP method (get, post, put, delete, patch) | Yes      |
| `path`            | URL path with optional `:param` variables   | Yes      |
| `headers`         | List of headers to extract                  | No       |
| `pathVariables`   | List of path parameters to extract          | No       |
| `queryParameters` | List of query parameters to extract         | No       |
| `body.type`       | Request body type (`json`)                  | No       |

**Accessing request data in steps:**

```yaml
# Path variables
request.pathVariables.orderId
request.params.orderId          # Alias

# Query parameters
request.queryParameters.limit
request.query.limit             # Alias

# Headers
request.headers.Authorization
request.headers.X-Request-ID

# Body (for POST/PUT/PATCH)
request.body.field
request.body.nested.field

# Raw body (for webhook signature verification)
request.rawBody                 # Exact string as received
```

## Properties

Flow-level variables accessible in all steps.

```yaml
properties:
  # Literals
  maxRetries: 3
  taxRate: 0.15

  # From global properties (flow-config.yaml)
  apiUrl: properties.serviceUrl

  # Can use expressions
  timeout: 30
```

Access in steps: `properties.maxRetries`

Properties are merged with global properties from `flow-config.yaml`. Flow properties override global properties.

## Steps

Steps are executed sequentially. Each step has:

```yaml
- id: step_name              # Required: unique identifier
  type: step_type            # Required: assign, switch, or plugin.task
  condition: expression      # Optional: skip if false
  args: # Step-specific arguments
    key: value
  retry: # Optional: retry configuration
    maxRetries: 3
    delay: 1000
    backoff: true
    condition: expression
```

### Step Types

#### assign

Assigns values to variables.

```yaml
- id: extract_data
  type: assign
  args:
    # Simple assignment
    orderId: request.pathVariables.orderId

    # Expressions
    total: price * quantity

    # Ternary
    status: 'amount > 100 ? "premium" : "standard"'

    # String concatenation
    message: '"Hello, " + name + "!"'

    # Nested structures
    response:
      success: true
      data:
        id: orderId
        total: total
```

Access results: `extract_data.orderId`, `extract_data.response.data.id`

#### switch

Conditional branching based on expressions.

```yaml
- id: route
  type: switch
  args:
    # Branch name: condition expression
    process_premium: amount > 1000
    process_standard: amount <= 1000
    handle_invalid: amount < 0
```

When a condition evaluates to `true`, execution jumps to a step with matching ID.

```yaml
steps:
  - id: route
    type: switch
    args:
      premium_flow: amount > 1000
      standard_flow: amount <= 1000

  - id: premium_flow
    type: assign
    args:
      discount: 0.1

  - id: standard_flow
    type: assign
    args:
      discount: 0
```

#### Plugin Tasks

Call plugin methods using `plugin.task` syntax.

```yaml
# HTTP request
- id: get_user
  type: http.request
  args:
    method: GET
    url: '"https://api.example.com/users/" + userId'

# Custom plugin task
- id: process_payment
  type: payment.processPayment
  args:
    amount: total
    currency: '"USD"'
```

Access results: `get_user.result`, `process_payment.result`

### Conditional Execution

Skip steps based on conditions:

```yaml
- id: send_notification
  type: http.request
  condition: shouldNotify == true && status == "success"
  args:
    url: notificationUrl
    method: POST
```

### Retry Configuration

Retry failed steps:

```yaml
- id: call_api
  type: http.request
  args:
    url: apiUrl
    method: POST
  retry:
    maxRetries: 3          # Maximum retry attempts
    delay: 1000            # Initial delay in milliseconds
    backoff: true          # Exponential backoff (delay doubles each retry)
    condition: call_api.result.statusCode >= 500  # Retry only if condition is true
```

## Return

Defines the response sent back to the client.

### JSON Response

```yaml
return:
  type: json
  args:
    field: expression
    nested:
      data: step.result
```

### HTTP Response (Full Control)

```yaml
return:
  type: http.response
  args:
    status: 200                    # HTTP status code (can be expression)
    headers:
      X-Request-ID: requestId
      Content-Type: '"application/json"'
    body:
      success: true
      data: result
```

**Dynamic status codes:**

```yaml
return:
  type: http.response
  args:
    status: 'success ? 200 : 400'
    body:
      success: success
      message: message
```

## Expression Syntax

SFlowG uses [expr-lang](https://expr-lang.org/) for expressions.

### Data Types

```yaml
# Strings (must be quoted in expressions)
message: '"Hello, World!"'
greeting: '"Hello, " + name + "!"'

# Numbers
count: 42
price: 19.99
total: price * quantity

# Booleans
enabled: true
isValid: amount > 0

# Null checks
hasValue: value != null
```

### Operators

**Arithmetic:**

```yaml
sum: a + b
diff: a - b
product: a * b
quotient: a / b
remainder: a % b
```

**Comparison:**

```yaml
equal: a == b
notEqual: a != b
greater: a > b
less: a < b
greaterOrEqual: a >= b
lessOrEqual: a <= b
```

**Logical:**

```yaml
and: a && b
or: a || b
not: '!a'
```

**Ternary:**

```yaml
result: 'condition ? valueIfTrue : valueIfFalse'

# Nested ternary
status: >
  code == 200 ? "success" :
  code == 400 ? "bad_request" :
  code == 500 ? "server_error" :
  "unknown"
```

### Accessing Data

```yaml
# Step results
  stepId.fieldName
    stepId.nested.field
    stepId.result.data

  # Request data
    request.body.field
    request.pathVariables.id
    request.queryParameters.page
    request.headers.Authorization

  # Properties
    properties.apiUrl
    properties.maxRetries

  # Null-safe access
    'value != null ? value.field : "default"'
```

### Built-in Functions

```yaml
# Current timestamp
timestamp: now()

# String functions
upper: upper(name)
lower: lower(name)
trim: trim(input)
len: len(array)

# Type conversion
str: string(number)
num: int(stringValue)
float: float(value)

# Base64 encoding/decoding
encoded: base64_encode(value)
decoded: base64_decode(encoded_value)
```

## Complete Example

```yaml
id: create_order

entrypoint:
  type: http
  config:
    method: post
    path: /api/orders
    headers:
      - Authorization
    body:
      type: json

properties:
  taxRate: 0.15
  minAmount: 10.00

steps:
  # Extract and validate
  - id: extract
    type: assign
    args:
      items: request.body.items
      customerId: request.body.customerId

  - id: calculate
    type: assign
    args:
      subtotal: request.body.amount
      tax: request.body.amount * properties.taxRate
      total: request.body.amount * (1 + properties.taxRate)

  # Validate amount
  - id: validate
    type: switch
    args:
      invalid_amount: calculate.total < properties.minAmount
      process_order: calculate.total >= properties.minAmount

  - id: invalid_amount
    type: assign
    args:
      success: false
      error: '"Order amount below minimum"'

  # Process valid order
  - id: process_order
    type: http.request
    args:
      method: POST
      url: '"https://api.payment.com/charges"'
      headers:
        Authorization: request.headers.Authorization
      body:
        amount: calculate.total
        customer: extract.customerId

  - id: build_response
    type: assign
    args:
      success: process_order.result.statusCode == 200
      orderId: process_order.result.body.id

return:
  type: http.response
  args:
    status: 'build_response.success ? 201 : 400'
    body:
      success: build_response.success
      orderId: build_response.orderId
      total: calculate.total
```

## Related Documentation

- [Getting Started](./GETTING_STARTED.md) - Build your first flow
- [Configuration](./FLOW_CONFIG.md) - Project configuration
- [CLI Reference](./CLI.md) - Build and run flows
- [Plugin Development](./PLUGIN_DEVELOPMENT.md) - Create custom tasks