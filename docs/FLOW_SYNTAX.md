# Flow Syntax Reference

SFlowG flow files use the `.flow` DSL. A flow declares one entrypoint, optional
properties, zero or more steps, optional `on_error`, and usually a final
`return response.*(...)`. Cron flows are response-less and do not need a
`return`.

Step bodies are Risor code. Entrypoint blocks and property blocks use SFlowG's
simple map syntax.

## Flow Shape

```sflowg
entrypoint.http {
    method: POST
    path: /api/orders
    body: {
        type: json
        schema: {
            customer_email: { type: string, required: true, format: email }
            amount_cents: { type: integer, required: true, minimum: 1 }
        }
    }
}

properties {
    default_currency: "usd"
}

step create_order {
    let result = postgres.get({
        query: `
            INSERT INTO orders (customer_email, amount_cents, currency)
            VALUES ($1, $2, $3)
            RETURNING id
        `,
        params: [request.body.customer_email, request.body.amount_cents, properties.default_currency]
    })

    result
}

return response.json({
    status: 201,
    body: { order: create_order.row }
})
```

Top-level blocks:

- `entrypoint.http`, `entrypoint.kafka`, `entrypoint.flow`, or `entrypoint.cron`
- `properties`
- `step`
- `on_error`
- `return`

Line comments start with `//`.

## Entrypoints

Entrypoints define how a flow is triggered and which root object is available
inside steps. See [Entrypoints](./ENTRYPOINTS.md) for field-level contracts,
runtime packaging, and examples.

| Entrypoint | Trigger | Root object | Response contract |
|---|---|---|---|
| `entrypoint.http` | HTTP request | `request` | `response.json(map)`, `response.text(map)`, `response.redirect(map)` |
| `entrypoint.kafka` | Kafka message | `message` | `response.ack()`, `response.nack()` |
| `entrypoint.flow` | `flow.call` from another flow | `input` | `response.value(map)`, `response.error(map)` |
| `entrypoint.cron` | In-process schedule | `trigger` | none |

HTTP, Kafka, and Cron are external transport modules. Flow entrypoints are an
internal core concept for subflows. Boundary input schema details live in
[Entrypoints](./ENTRYPOINTS.md#input-schemas).

## Properties

Flow properties are available under `properties`.

```sflowg
properties {
    default_currency: "usd"
    retry_delay_ms: 500
    api_key: env("API_KEY")
    api_url: env("API_URL", "http://localhost:3000")
}
```

`env("NAME")` and `env("NAME", "default")` are resolved at startup. Flow
properties are merged with global properties from `flow-config.yaml`; flow
properties win.

Access:

```sflowg
properties.default_currency
properties.api_url
```

## Steps

Steps run sequentially until a step sets a response or all steps complete.

```sflowg
step fetch_order {
    postgres.get({
        query: `
            SELECT id, status
            FROM orders
            WHERE id = $1
        `,
        params: [request.pathVariables.id]
    })
}
```

The last expression in a step becomes the step result and is stored under the
step ID:

```sflowg
step extract_request {
    {
        amount: request.body.amount,
        currency: request.body.currency ?? properties.default_currency,
    }
}

step use_extracted {
    log.info("currency", extract_request.currency)
}
```

Plugin tasks are normal function calls:

```sflowg
postgres.get({ query: "...", params: [...] })
postgres.exec({ query: "...", params: [...] })
http.request({ method: "POST", url: properties.url, body: request.body })
```

### Multi-Line Strings and Query Parameters

Use backtick template strings for long strings such as SQL. Backtick strings may
span multiple lines and support `${expr}` interpolation:

```sflowg
step insert_payment as postgres.get {
    query: `
        INSERT INTO payments
            (amount, currency, description, customer_email, customer_name,
             metadata_order_id, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
        RETURNING id, created_at
    `
    params: [
        input.amount, input.currency, input.description,
        input.customer_email, input.customer_name, input.order_id,
        "pending"
    ]
}
```

Single-line quoted strings still work; backticks are recommended when the
string benefits from line breaks or interpolation.

Do not use `${expr}` interpolation for user-controllable SQL values. Risor
expands interpolation before the plugin sees the query, so it bypasses database
parameter binding:

```sflowg
query: `SELECT * FROM orders WHERE id = ${input.id}` // SQL injection risk

query: `SELECT * FROM orders WHERE id = $1`
params: [input.id]
```

Use interpolation only for known-safe values such as constants or trusted
configuration. Use `$N` placeholders with `params` for request data, message
data, flow input, and any other user-controllable value.

Query-like plugins may also document named placeholders as a plugin-specific
convention:

```sflowg
step insert_payment as sql.query {
    query: `
        INSERT INTO payments (email, amount, currency, status)
        VALUES (:email, :amount, :currency, :status)
    `
    params: {
        email: request.body.customer_email,
        amount: request.body.amount,
        currency: request.body.currency,
        status: "pending"
    }
}
```

Named parameters are not DSL syntax. They only work when the target plugin
explicitly supports them.

### Header Call Sugar

For a step that only calls one plugin method or subflow, put the call target in
the step header:

```sflowg
step fetch_order as postgres.get {
    query: `
        SELECT *
        FROM orders
        WHERE id = $1
    `
    params: [request.pathVariables.id]
}

step resolve_currency as subflow.resolve_order_currency {
    requested_currency: request.body.currency
    default_currency: properties.default_currency
}
```

This is equivalent to writing:

```sflowg
step fetch_order {
    postgres.get({
        query: `
            SELECT *
            FROM orders
            WHERE id = $1
        `,
        params: [request.pathVariables.id]
    })
}

step resolve_currency {
    flow.call("resolve_order_currency", {
        requested_currency: request.body.currency,
        default_currency: properties.default_currency
    })
}
```

The block is lowered as Risor map body source; top-level entries may be separated
by newlines as shown above. Use normal `step { ... }` syntax for multi-statement
logic, local variables, conditionals, loops, or manual `flow.call(...)` calls.

Step options still go before `as`:

```sflowg
step call_api(condition: request.body.enabled, retry: { max_attempts: 3 }) as http.request {
    method: "POST"
    url: properties.api_url
    body: request.body
}
```

### Conditions

Use `condition` in step options to skip a step when false:

```sflowg
step not_found(condition: fetch_order.found == false) {
    response.json({
        status: 404,
        body: { error: "order not found" }
    })
}
```

### Retry

Retry options are part of the step options:

```sflowg
step create_payment(retry: {
    max_attempts: 3,
    delay: 500,
    backoff: "exponential",
    max_delay: 5000,
    jitter: true,
    when: error.type == "transient",
    non_retryable: ["INVALID_INPUT"]
}) {
    let result = http.request({
        method: "POST",
        url: properties.payment_url,
        body: request.body
    })
    if (result.status_code >= 500) {
        raise("transient", "PAYMENT_SERVICE_UNAVAILABLE", "payment service temporarily unavailable")
    }
    result
}
```

Retry fields:

| Field | Description |
|---|---|
| `max_attempts` | Maximum attempts including the first attempt |
| `delay` | Base delay in milliseconds |
| `backoff` | `none`, `linear`, or `exponential` |
| `max_delay` | Maximum delay in milliseconds |
| `jitter` | Add small random delay variation |
| `when` | Expression evaluated with `error` in scope |
| `non_retryable` | Error codes that must not retry |

### Fallback

A fallback runs if the primary step exhausts retries or fails without retrying.

```sflowg
step create_payment(retry: { max_attempts: 3, delay: 500, backoff: "exponential" }) {
    let result = http.request({
        url: properties.payment_url,
        method: "POST",
        body: request.body
    })
    if (result.status_code >= 500) {
        raise("transient", "PAYMENT_SERVICE_UNAVAILABLE", "payment service temporarily unavailable")
    }
    result
} fallback {
    response.json({
        status: 202,
        body: { queued: true }
    })
}
```

The fallback body can inspect the failure as `error`.

### Compensation

`compensate` bodies run in reverse order when a later step fails and the flow
enters failure handling.

```sflowg
step insert_payment {
    postgres.get({
        query: `
            INSERT INTO payments (...)
            VALUES (...)
            RETURNING id
        `,
        params: [...]
    })
} compensate {
    postgres.exec({
        query: `
            DELETE FROM payments
            WHERE id = $1
        `,
        params: [insert_payment.row.id]
    })
}
```

Compensation receives:

- `compensation.step`
- `compensation.path`

## Error Handling

Use `raise` to fail the current step with a flow error:

```sflowg
raise("PAYMENT_NOT_FOUND", "payment record was not found")
raise("transient", "STRIPE_SERVICE_ERROR", "Stripe API temporarily unavailable")
```

Forms:

- `raise(code, message)` defaults to permanent error type
- `raise(type, code, message)` sets an explicit error type

`on_error` is a flow-level recovery block:

```sflowg
on_error {
    let error_response = match error.type {
        "timeout" => {
            status: 504,
            code: "TIMEOUT",
            message: "request timed out"
        }
        "transient" => {
            status: 503,
            code: error.code,
            message: error.message
        }
        _ => {
            status: 500,
            code: error.code,
            message: error.message
        }
    }

    response.json({
        status: error_response.status,
        body: {
            error: {
                code: error_response.code,
                message: error_response.message
            }
        }
    })
}
```

`error` contains fields such as:

- `error.type`
- `error.code`
- `error.message`
- `error.step`
- `error.retries`
- `error.meta`

## Responses

Responses are entrypoint-specific. Calls are validated at build/startup and at
runtime.

HTTP, Kafka, and Flow entrypoints require a valid response when the flow
completes. Missing responses fail with `RUNTIME_ERROR` and
`error.meta.reason == "missing_response"`; invalid response subtypes fail with
`RUNTIME_ERROR` and `error.meta.reason == "invalid_response"`. These errors can
run `on_error` once. For response-required entrypoints, `on_error` must also
complete with a valid response. Cron is response-less and may complete without
any response.

### HTTP Responses

HTTP supports:

- `response.json(map)`
- `response.text(map)`
- `response.redirect(map)`

Each call takes exactly one map argument.
HTTP flows must have a top-level terminal `return response.*(...)`.

```sflowg
return response.json({
    status: 200,
    headers: {
        "X-Request-ID": request.headers["X-Request-ID"]
    },
    body: {
        ok: true
    }
})
```

```sflowg
return response.text({
    status: 200,
    body: rendered.html
})
```

```sflowg
return response.redirect({
    status: 302,
    location: checkout_url
})
```

### Flow Responses

Subflows support:

- `response.value(map)`
- `response.error(map)`

Subflows must have a top-level terminal `return response.*(...)`.

```sflowg
return response.value({
    currency: input.requested_currency || input.default_currency
})
```

### Kafka Responses

Kafka supports:

- `response.ack()`
- `response.nack()`

These take no arguments. A Kafka flow must have a top-level return of one of
those calls.

```sflowg
on_error {
    log.error("kafka order failed", error)
    response.nack()
}

return response.ack()
```

### Cron Responses

Cron is response-less. A cron flow may complete without a `return`, and
`response.*(...)` calls are invalid in cron steps, fallbacks, returns, and
`on_error`.

```sflowg
entrypoint.cron {
    schedule: "*/5 * * * *"
    timezone: UTC
}

step cleanup {
    log.info("scheduled cleanup", {
        scheduled_at: trigger.scheduled_at,
        fire_count: trigger.fire_count
    })
}

on_error {
    log.error("scheduled cleanup failed", {
        code: error.code,
        message: error.message
    })
}
```

The cron schedule uses exactly five fields: minute, hour, day-of-month, month,
and day-of-week. Seconds are not supported in cron v1. `trigger.scheduled_at`
is the scheduled instant, not the wall-clock instant when execution happened.

## Built-Ins

### Logging

```sflowg
log.debug("message", {key: "value"})
log.info("message", {key: "value"})
log.warn("message", {key: "value"})
log.error("message", {key: "value"})
```

User logs include execution and flow context.

### Metrics

Dynamic metric API:

```sflowg
metric.counter("orders_created", 1, {"currency": request.body.currency})
metric.updowncounter("queue_depth", -1, {"queue": "payments"})
metric.histogram("payment_duration_ms", duration_ms, {"provider": "stripe"})
metric.gauge("active_workers", 4)
```

Predeclared metric handles from `flow-config.yaml`:

```sflowg
metric.payment_attempts.inc(1, {"provider": "stripe", "outcome": "success"})
metric.payment_duration_ms.observe(duration_ms, {"provider": "stripe"})
metric.queue_depth.add(-1, {"queue": "payments"})
metric.active_workers.set(4)
```

Metric failures are logged and do not fail the step.

### Flow Calls

```sflowg
flow.call("subflow_name", {
    arg_name: value
})
```

### Formatting and Encoding

```sflowg
sprintf("order-%v", request.pathVariables.id)
base64_encode(request.rawBody)
```

## Risor Notes

Step bodies use Risor syntax. Common patterns from the examples:

```sflowg
let value = request.body.currency ?? "usd"

if (result.status_code >= 500) {
    raise("transient", "UPSTREAM_UNAVAILABLE", "temporary failure")
}

let status = match request.body.event_type {
    "payment_intent.succeeded" => "paid"
    "payment_intent.payment_failed" => "payment_failed"
    _ => "unknown"
}

let email = request.body.customer_email?.to_lower()?.trim_space()
```

Missing map attributes resolve to `nil`, so nil checks and optional chaining are
safe for request, message, step result, and property objects.

## Examples

See:

- [Ecommerce example](./examples/ecom/README.md)
- [Stripe integration example](./examples/stripe-integration/README.md)
- [Kafka consumer example](./examples/kafka-consumer/README.md)

## Related Documentation

- [Entrypoints](./ENTRYPOINTS.md)
- [Getting Started](./GETTING_STARTED.md)
- [Flow Config](./FLOW_CONFIG.md)
- [CLI Reference](./CLI.md)
- [Plugin Development](./PLUGIN_DEVELOPMENT.md)
