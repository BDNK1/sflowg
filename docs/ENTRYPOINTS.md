# Entrypoints

Entrypoints define how a flow is triggered, what boundary input is validated,
which root object is available in step code, and which response calls are valid.

Supported entrypoints:

| Entrypoint | Trigger | Runtime support |
|---|---|---|
| `entrypoint.http` | HTTP request | External `transports/http` module |
| `entrypoint.kafka` | Kafka message | External `transports/kafka` module |
| `entrypoint.flow` | `flow.call` from another flow | Built into core |
| `entrypoint.cron` | In-process schedule | External `transports/cron` module |

HTTP, Kafka, and Cron transports are registered by generated applications only
when matching flows exist. Subflows are not an external transport.

## HTTP

HTTP flows are served by the external HTTP transport module.

```sflowg
entrypoint.http {
    method: GET
    path: /api/orders/:id
    pathVariables: {
        id: { type: integer, required: true, minimum: 1 }
    }
    queryParameters: {
        include: { type: array, items: { type: string } }
    }
    headers: {
        X-Request-ID: { type: string }
    }
}
```

Fields:

| Field | Required | Description |
|---|---:|---|
| `method` | Yes | `GET` or `POST` |
| `path` | Yes | Route path, including `:name` variables |
| `timeout` | No | Flow timeout in milliseconds |
| `pathVariables` | No | List or schema map for route variables |
| `queryParameters` | No | List or schema map for query values |
| `headers` | No | List or schema map for request headers |
| `body` | No | Body declaration; currently `type: json` |

HTTP does not parse request bodies implicitly. `request.body` and
`request.rawBody` exist only when `body: { type: json }` is declared.

```sflowg
entrypoint.http {
    method: POST
    path: /api/webhooks/stripe
    headers: {
        Stripe-Signature: { type: string, required: true, minLength: 1 }
    }
    body: {
        type: json
        schema: {
            id: { type: string, required: true }
            type: { type: string, required: true }
        }
    }
}
```

HTTP namespaces:

- `request.body`
- `request.rawBody`
- `request.pathVariables`
- `request.queryParameters`
- `request.headers`

Header names with dashes can be read with index syntax:

```sflowg
request.headers["Stripe-Signature"]
request.headers["X-Request-ID"]
```

HTTP response calls:

- `response.json(map)`
- `response.text(map)`
- `response.redirect(map)`

Each HTTP response call takes exactly one map argument.
HTTP flows must have a top-level terminal `return response.*(...)` with one of
the valid HTTP response calls.

```sflowg
return response.json({
    status: 200,
    body: { ok: true }
})
```

## Kafka

Kafka flows are served by the external Kafka transport module. Broker
connection settings live in `flow-config.yaml`; the flow entrypoint declares
which topic and consumer group the flow uses.

```sflowg
entrypoint.kafka {
    broker: default
    topic: orders.events
    group_id: orders-service
    auto_offset_reset: earliest
    value: {
        type: json
        schema: {
            order_id: { type: string, required: true }
            amount_cents: { type: integer, required: true, minimum: 1 }
            currency: { type: string, default: usd, enum: [usd, eur] }
        }
    }
}
```

Fields:

| Field | Required | Description |
|---|---:|---|
| `broker` | No | Named broker config from `flow-config.yaml`; defaults to `default` |
| `topic` | Yes | Kafka topic to subscribe to |
| `group_id` | Yes | Kafka consumer group |
| `auto_offset_reset` | Yes | `earliest` or `latest` |
| `value.type` | Yes | Must be `json` in v1 |
| `value.schema` | No | Optional schema for parsed JSON payload |

Kafka runtime config:

```yaml
runtime:
  kafka:
    brokers:
      default:
        brokers: ["localhost:9092"]
        client_id: "orders-service"
        nack_redelivery_delay_ms: 100
```

Kafka v1 treats keys, values, and headers as UTF-8 text. Invalid UTF-8 or
invalid JSON is a boundary validation error. The original payload text is
available as `message.rawValue`; the parsed JSON payload is available as
`message.value`.

Kafka namespaces:

- `message.key`
- `message.value`
- `message.rawValue`
- `message.headers`
- `message.topic`
- `message.partition`
- `message.offset`
- `message.timestamp`

`message.timestamp` is an ISO-8601 string.

Kafka flows must have a top-level terminal return:

```sflowg
return response.ack()
```

or:

```sflowg
return response.nack()
```

`response.ack()` and `response.nack()` take no arguments. They are also allowed
in `on_error`, where they can override the default error outcome. Normal steps,
fallbacks, and compensation bodies cannot return Kafka ack/nack responses.

Kafka boundary failures and unhandled execution errors default to `nack`.
Duplicate `(broker, topic, group_id)` subscriptions in one app are rejected at
startup.

## Flow

`entrypoint.flow` declares an internal subflow callable with `flow.call`. It is
built into core and is not an external transport.

```sflowg
entrypoint.flow {
    input: {
        requested_currency: { type: string, enum: [usd, eur] }
        default_currency: { type: string, required: true, enum: [usd, eur] }
    }
}

return response.value({
    currency: input.requested_currency || input.default_currency
})
```

Call it from another flow:

```sflowg
step resolve_currency {
    flow.call("resolve_order_currency", {
        requested_currency: request.body.currency,
        default_currency: properties.default_currency
    })
}
```

Subflow input is available under `input`.

Flow response calls:

- `response.value(map)`
- `response.error(map)`

Each flow response call takes exactly one map argument.
Subflows must have a top-level terminal `return response.*(...)` with one of
the valid flow response calls.

## Cron

Cron flows are served by the external Cron transport module. They run on an
in-process schedule inside the generated application.

```sflowg
entrypoint.cron {
    schedule: "*/5 * * * *"
    timezone: UTC
    timeout: 10000
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

Fields:

| Field | Required | Description |
|---|---:|---|
| `schedule` | Yes | Five-field cron expression: minute, hour, day-of-month, month, day-of-week |
| `timezone` | No | IANA timezone name; defaults to `UTC` |
| `timeout` | No | Flow timeout in milliseconds |

The scheduler does not fire immediately on startup. It computes the next
scheduled instant and waits for that tick. If a previous run of the same flow is
still active when the next tick arrives, the tick is skipped rather than queued.
If the process wakes up after scheduled instants were missed, those missed ticks
are also skipped rather than replayed.

Cron trigger fields:

- `trigger.scheduled_at`: scheduled instant as an RFC3339Nano string in the configured timezone
- `trigger.fire_count`: process-local count of started runs for this flow

Cron is response-less:

- Top-level `return response.*(...)` is not required.
- Any `response.*(...)` call is invalid for `entrypoint.cron`.
- `on_error` may complete without setting a response.

Cron v1 uses five-field cron expressions, so schedules have minute granularity.
Seconds fields such as `*/20 * * * * *` are rejected.

## Response Completion Semantics

Response completion is part of the entrypoint contract:

| Entrypoint | Requires response | Valid responses |
|---|---:|---|
| `entrypoint.http` | Yes | `response.json(map)`, `response.text(map)`, `response.redirect(map)` |
| `entrypoint.kafka` | Yes | `response.ack()`, `response.nack()` |
| `entrypoint.flow` | Yes | `response.value(map)`, `response.error(map)` |
| `entrypoint.cron` | No | none |

For response-required entrypoints, normal completion without a response fails
with `RUNTIME_ERROR` and `error.meta.reason == "missing_response"`. Producing a
response subtype that is not valid for the entrypoint fails with
`RUNTIME_ERROR` and `error.meta.reason == "invalid_response"`.

Missing or invalid response errors can run the flow's `on_error` block once.
If `on_error` completes for HTTP, Kafka, or Flow, it must set a valid response.
If `on_error` itself completes without a required response or with an invalid
response, the flow fails with that runtime response error and does not recurse
into `on_error` again. Cron `on_error` may complete without setting a response.

## Input Schemas

Schemas are supported for HTTP inputs, Kafka `message.value`, and flow inputs.
They are part of the entrypoint boundary: validation runs before normal steps
and normalized values stay in the entrypoint root object.

Schema locations:

| Entrypoint | Schema location | Runtime value |
|---|---|---|
| `entrypoint.http` | `pathVariables`, `queryParameters`, `headers`, `body.schema` | `request.*` |
| `entrypoint.kafka` | `value.schema` | `message.value` |
| `entrypoint.flow` | `input` | `input` |
| `entrypoint.cron` | none | `trigger.*` |

Types:

- `object`
- `array`
- `string`
- `integer`
- `number`
- `boolean`

Constraints:

| Constraint | Applies To | Description |
|---|---|---|
| `required` | all types | Missing value fails validation unless `default` exists |
| `default` | all types | Value used when input is absent |
| `enum` | `string` | Value must match an allowed string |
| `minimum` | `integer`, `number` | Inclusive lower bound |
| `maximum` | `integer`, `number` | Inclusive upper bound |
| `minLength` | `string` | Minimum string length |
| `maxLength` | `string` | Maximum string length |
| `pattern` | `string` | Regular expression match |
| `format` | `string` | `email`, `uuid`, `url`, `date`, or `date-time` |
| `properties` | `object` | Nested object fields |
| `items` | `array` | Array item schema |

Object schemas can use either explicit `properties`:

```sflowg
metadata: {
    type: object
    properties: {
        order_id: { type: string, required: true }
    }
}
```

or the shorthand used by examples:

```sflowg
schema: {
    customer_email: { type: string, required: true, format: email }
    amount_cents: { type: integer, required: true, minimum: 1 }
}
```

HTTP list forms extract string values without schema validation:

```sflowg
entrypoint.http {
    method: GET
    path: /api/orders/:id
    pathVariables: [id]
    queryParameters: [limit]
    headers: [Authorization]
}
```

HTTP object forms validate and normalize values before steps run:

```sflowg
entrypoint.http {
    method: GET
    path: /api/orders/:id
    pathVariables: {
        id: { type: integer, required: true, minimum: 1 }
    }
    queryParameters: {
        limit: { type: integer, default: 50, minimum: 1 }
        include: { type: array, items: { type: string } }
    }
}
```

Path variables, query parameters, and headers start as strings and can be
coerced to `integer`, `number`, `boolean`, or validated strings. Repeated query
parameters and multi-value headers can be validated as arrays.

If validation fails, normal steps do not run and the runtime raises a boundary
`SCHEMA_VIOLATION` error. `on_error` can handle it.

```sflowg
on_error {
    if (error.code == "SCHEMA_VIOLATION") {
        response.json({
            status: 400,
            body: {
                error: "invalid_request",
                fields: error.meta.fields,
            }
        })
    } else {
        response.json({
            status: 500,
            body: { error: error.code }
        })
    }
}
```

If an HTTP flow does not handle a schema violation, the transport returns
`400 application/problem+json`.

Kafka schema and boundary failures default to `nack` unless `on_error` returns
`response.ack()` or `response.nack()`.

## Validation

Response calls and input schemas are entrypoint-specific and are validated at
build/startup and at runtime. The CLI can validate HTTP, Kafka, Flow, and Cron
response semantics without importing external transport modules.
