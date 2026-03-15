# flow-config.yaml Configuration

## Overview

The `flow-config.yaml` file is the main configuration file for SFlowG projects. It defines project metadata, runtime settings, global properties, and plugin configurations.

## File Structure

```yaml
name: project-name
version: "1.0.0"

runtime:
  port: "8080"

observability:
  logging:
    level: info
    max_payload_bytes: 10240
  tracing:
    enabled: false
  metrics:
    enabled: false

properties:
  key: value

plugins:
  - source: plugin-source
    config:
      key: value
```

## Sections

### Project Metadata

```yaml
name: payment-system          # Optional: defaults to directory name
version: "1.0.0"              # Optional: defaults to "latest"
```

**Fields:**
- `name` - Project name (used for generated binary name)
- `version` - Project version

### Runtime Configuration

```yaml
runtime:
  port: "8080"                # HTTP server port
```

**Fields:**
- `port` - HTTP server port (default: "8080")

### Observability Configuration

```yaml
observability:
  logging:
    level: info                  # debug | info | warn | error
    format: json                 # currently only json is supported
    max_payload_bytes: 10240     # default: 10 KB
    attributes:
      service: payment-service
      environment: prod
    sources:
      framework: info
      plugin: info
      user: debug
    masking:
      fields: [authorization, password, token]
      placeholder: "***"
    export:
      enabled: true
      mode: [stdout, otlp]       # stdout | otlp | [stdout, otlp]
      endpoint: localhost:4317   # required when otlp mode is enabled
      insecure: true
      attributes:
        service.name: payment-api
        deployment.environment: staging
  tracing:
    enabled: true
    endpoint: localhost:4317     # OTLP/gRPC collector
    insecure: true               # disable TLS for local collectors
    sampler: parent_based        # always_on | always_off | trace_id_ratio | parent_based
    sample_rate: 0.5             # used by trace_id_ratio and parent_based
    attributes:
      service.name: payment-api
      deployment.environment: staging
  metrics:
    enabled: true
    endpoint: localhost:4317     # OTLP/gRPC collector
    insecure: true               # disable TLS for local collectors
    export_interval_ms: 10000    # 1s - 60s, default: 10s
    attributes:
      service.name: payment-api
      deployment.environment: staging
    histogram_buckets:
      http_request_ms: [5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000]
      flow_ms: [10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000]
      step_ms: [5, 10, 25, 50, 100, 250, 500, 1000, 2500]
      plugin_ms: [5, 10, 25, 50, 100, 250, 500, 1000, 2500]
```

**Logging Fields:**
- `level` - Default log level
- `format` - Log output format (currently `json`)
- `max_payload_bytes` - Max size for string-like payload fields before truncation
- `attributes` - Static attributes attached to all log entries
- `sources.framework` - Optional override for framework log level
- `sources.plugin` - Optional override for plugin log level
- `sources.user` - Optional override for DSL user log level
- `masking.fields` - Optional list of field names to mask
- `masking.placeholder` - Replacement string for masked values
- `export.enabled` - Enables OTLP log export when `mode` includes `otlp`
- `export.mode` - Output target(s); accepts a single value (`stdout` or `otlp`) or a list such as `[stdout, otlp]`
- `export.endpoint` - OTLP/gRPC collector endpoint in `host:port` form; required when `mode` includes `otlp`
- `export.insecure` - Disable TLS for OTLP/gRPC log export, useful for local collectors
- `export.attributes` - Static resource attributes attached to exported OTLP logs

**Tracing Fields:**
- `enabled` - Enable OpenTelemetry tracing (default: `false`)
- `endpoint` - OTLP/gRPC collector endpoint in `host:port` form, required when tracing is enabled
- `insecure` - Disable TLS for OTLP/gRPC export, useful for local collectors
- `sampler` - Sampling strategy: `always_on`, `always_off`, `trace_id_ratio`, or `parent_based`
- `sample_rate` - Sampling ratio from `0.0` to `1.0`; used by `trace_id_ratio` and `parent_based`
- `attributes` - Static resource attributes attached to emitted traces

**Metrics Fields:**
- `enabled` - Enable OpenTelemetry metrics export (default: `false`)
- `endpoint` - OTLP/gRPC collector endpoint in `host:port` form, required when metrics are enabled
- `insecure` - Disable TLS for OTLP/gRPC export, useful for local collectors
- `export_interval_ms` - Periodic export interval in milliseconds (default: `10000`)
- `attributes` - Static resource attributes attached to emitted metrics
- `histogram_buckets.http_request_ms` - Optional custom buckets for `sflowg.http.server.duration_ms`
- `histogram_buckets.flow_ms` - Optional custom buckets for `sflowg.flow.duration_ms`
- `histogram_buckets.step_ms` - Optional custom buckets for `sflowg.step.duration_ms`
- `histogram_buckets.plugin_ms` - Optional custom buckets for `sflowg.plugin.duration_ms`

**Notes:**
- Masking is opt-in. There is no default masking list.
- Payload truncation is enforced centrally for framework, plugin, and DSL logs.
- Flow logs automatically include `execution_id` and `flow_id`.
- Step-scoped logs also include `step_id`.
- When tracing is enabled, logs emitted inside an active span also include `trace_id` and `span_id`.
- If `logging.export.mode` is omitted, the effective default is `stdout`.
- Use `logging.export.mode: [stdout, otlp]` to keep local terminal logs while also exporting to OTLP.
- Incoming HTTP `traceparent` headers are continued automatically when present.
- Each traced execution produces a flow span with child spans for executed steps and plugin calls.
- When tracing is disabled, the runtime uses a noop tracer and flow code does not need to change.
- When metrics are enabled, the runtime emits OTLP metrics for flows, steps, retries, plugin calls, and HTTP requests.
- Metric attributes are runtime-owned and bounded. Raw `execution_id`, raw request paths, and arbitrary error text are not exported as metric labels.
- `sample_rate` only matters for `trace_id_ratio` and `parent_based`.

### Global Properties

```yaml
properties:
  # Service URLs
  paymentServiceURL: ${PAYMENT_URL:http://localhost:9002}
  inventoryServiceURL: ${INVENTORY_URL:http://localhost:9001}

  # Feature flags
  enableNotifications: ${ENABLE_NOTIFICATIONS:true}
  debugMode: ${DEBUG:false}

  # Literal values
  maxRetries: 3
  timeout: 30s
```

**Capabilities:**
- Define properties shared across all flows
- Support environment variable substitution: `${VAR:default}`
- Flow-level properties override global properties
- Accessible in flows as `properties.{name}`

### Plugins

```yaml
plugins:
  # Core plugin (built-in)
  - source: http
    config:
      timeout: ${HTTP_TIMEOUT:30s}
      max_retries: 3

  # Local plugin (relative path)
  - source: ./plugins/payment
    config:
      api_key: ${PAYMENT_API_KEY}
      timeout: 30s

  # Remote plugin (git URL)
  - source: github.com/user/plugin
    version: v1.2.3
    config:
      enabled: true
```

**Plugin Fields:**
- `source` - Plugin location (core name, local path, or git URL)
- `name` - Optional: plugin identifier (auto-detected from source)
- `version` - Optional: for remote plugins (default: "latest")
- `config` - Optional: plugin-specific configuration

**Plugin Types:**
- **Core** - Built-in plugins (e.g., `http`)
- **Local** - Local directory plugins (e.g., `./plugins/payment`)
- **Remote** - Git repository plugins (e.g., `github.com/user/plugin`)

## Environment Variable Syntax

Both `properties` and plugin `config` support environment variable substitution:

**Syntax:**
- `${VAR}` - Required environment variable (fails if not set)
- `${VAR:default}` - Optional environment variable with default value
- `literal` - Plain literal value (no substitution)

**Examples:**
```yaml
properties:
  # Required (must be set)
  apiKey: ${API_KEY}

  # Optional with default
  serviceURL: ${SERVICE_URL:http://localhost:8080}
  timeout: ${TIMEOUT:30s}
  enabled: ${ENABLED:true}
  maxRetries: ${MAX_RETRIES:3}

  # Literal values
  taxRate: 0.15
  shippingCost: 9.99
```

**Variable Name Rules:**
- Must start with uppercase letter (A-Z) or underscore (_)
- Can contain uppercase letters, numbers, and underscores
- Examples: `API_KEY`, `SERVICE_URL`, `MAX_RETRIES`, `_DEBUG`

## Complete Example

```yaml
name: payment-system-integration
version: "1.0.0"

runtime:
  port: "8080"

observability:
  logging:
    level: info
    sources:
      framework: info
      plugin: info
      user: debug
    masking:
      fields: [authorization, password, token]
    export:
      enabled: true
      mode: [stdout, otlp]
      endpoint: ${OTEL_EXPORTER_OTLP_ENDPOINT:localhost:4317}
      insecure: true
      attributes:
        service.name: payment-system-integration
        deployment.environment: dev
  tracing:
    enabled: true
    endpoint: ${OTEL_EXPORTER_OTLP_ENDPOINT:localhost:4317}
    insecure: true
    sampler: parent_based
    sample_rate: 0.25
    attributes:
      service.name: payment-system-integration
      deployment.environment: dev
  metrics:
    enabled: true
    endpoint: ${OTEL_EXPORTER_OTLP_ENDPOINT:localhost:4317}
    insecure: true
    export_interval_ms: 10000
    attributes:
      service.name: payment-system-integration
      deployment.environment: dev
    histogram_buckets:
      http_request_ms: [5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000]
      flow_ms: [10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000]
      step_ms: [5, 10, 25, 50, 100, 250, 500, 1000, 2500]
      plugin_ms: [5, 10, 25, 50, 100, 250, 500, 1000, 2500]

properties:
  # Service URLs (environment-specific)
  paymentProviderURL: ${PAYMENT_PROVIDER_URL:http://localhost:9002/v1/charges}
  notificationServiceURL: ${NOTIFICATION_SERVICE_URL:http://localhost:9000/notify}
  inventoryServiceURL: ${INVENTORY_SERVICE_URL:http://localhost:9001/inventory}

  # Feature flags
  enableNotifications: ${ENABLE_NOTIFICATIONS:true}
  debugMode: ${DEBUG_MODE:false}

  # Business constants (can be overridden by flows)
  taxRate: 0.15
  shippingCost: 9.99
  maxRetries: 3

plugins:
  # Core HTTP plugin
  - source: http
    config:
      timeout: ${HTTP_TIMEOUT:30s}
      max_retries: ${HTTP_MAX_RETRIES:3}
      debug: ${HTTP_DEBUG:false}
      retry_wait_ms: ${HTTP_RETRY_WAIT:100}

  # Local UUID generator plugin (no config)
  - source: ./plugins/uuidgen

  # Local payment plugin with configuration
  - source: ./plugins/payment
    config:
      provider_name: ${PAYMENT_PROVIDER:StripeProvider}
      api_base_url: ${PAYMENT_API_URL:https://api.stripe.com/v1}
      api_key: ${PAYMENT_API_KEY}  # Required, no default
      timeout: ${PAYMENT_TIMEOUT:30s}
      max_retries: ${PAYMENT_MAX_RETRIES:3}
      min_amount: ${PAYMENT_MIN_AMOUNT:0.01}
      max_amount: ${PAYMENT_MAX_AMOUNT:999999.99}
      default_currency: ${PAYMENT_CURRENCY:USD}
      enable_refunds: ${PAYMENT_ENABLE_REFUNDS:true}
      enable_validation: ${PAYMENT_ENABLE_VALIDATION:true}
      debug_mode: ${PAYMENT_DEBUG:false}
```

## Property Precedence

Properties are merged with the following precedence (highest wins):

1. **Environment Variables** (runtime)
2. **Flow Properties** (individual flow YAML files)
3. **Global Properties** (flow-config.yaml)

**Example:**
```yaml
# flow-config.yaml
properties:
  taxRate: 0.10  # Global default

# flows/purchase_flow.yaml
properties:
  taxRate: 0.15  # Overrides global for this flow

# Runtime
$ export TAXRATE=0.20  # Would override if using ${TAXRATE:...} syntax
```

## Related Documentation

- [CLI.md](./CLI.md) - Build commands and CLI usage
- [FLOW_SYNTAX.md](./FLOW_SYNTAX.md) - Flow YAML syntax reference
- [TRACING.md](./TRACING.md) - Tracing configuration and architecture
- [PLUGIN_DEVELOPMENT.md](./PLUGIN_DEVELOPMENT.md) - Creating custom plugins
