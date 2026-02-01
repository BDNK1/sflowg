# flow-config.yaml Configuration

## Overview

The `flow-config.yaml` file is the main configuration file for SFlowG projects. It defines project metadata, runtime settings, global properties, and plugin configurations.

## File Structure

```yaml
name: project-name
version: "1.0.0"

runtime:
  port: "8080"

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
- [PLUGIN_DEVELOPMENT.md](./PLUGIN_DEVELOPMENT.md) - Creating custom plugins
