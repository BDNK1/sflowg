# SFlowG Plugin Development Guide

This package provides the minimal interface for developing SFlowG plugins. Plugin authors should import **only** this package and never import the parent `runtime` package directly.

## Quick Start

### 1. Import the Plugin Package

```go
import "github.com/sflowg/sflowg/runtime/plugin"
```

**✅ Correct:** Import `runtime/plugin`
**❌ Wrong:** Import `runtime` (too much access to internals)

### 2. Minimal Plugin

A minimal plugin requires:
1. A plugin struct (can be empty)
2. At least one public task method

```go
package hello

import "github.com/sflowg/sflowg/runtime/plugin"

type HelloPlugin struct{}

// Task: hello.greet
func (p *HelloPlugin) Greet(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    name := args["name"].(string)
    return plugin.Output{
        "message": "Hello, " + name,
    }, nil
}
```

That's it! The framework handles task discovery, registration, and execution.

## Configuration

### Declarative Config with Tags

Plugins can define configuration using struct tags. The framework handles defaults, validation, and environment variable overrides automatically.

```go
package redis

import (
    "time"
    "github.com/sflowg/sflowg/runtime/plugin"
)

type Config struct {
    Addr     string        `yaml:"addr" default:"localhost:6379" validate:"required,hostname_port"`
    Password string        `yaml:"password"`
    DB       int           `yaml:"db" default:"0" validate:"gte=0,lte=15"`
    PoolSize int           `yaml:"pool_size" default:"10" validate:"gte=1,lte=1000"`
    Timeout  time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s"`
}

type RedisPlugin struct {
    config Config
    client *redis.Client
}
```

**Tag Reference:**
- `yaml:"field_name"` - YAML field name for user configuration
- `default:"value"` - Default value if not specified
- `validate:"rules"` - Validation rules (required, gte, lte, etc.)

**No boilerplate needed:**
- ❌ No `DefaultConfig()` function required
- ❌ No `Validate()` method required
- ✅ Framework handles everything via tags

### Common Validation Rules

```go
type Config struct {
    // Required fields
    APIKey string `validate:"required"`

    // Numeric ranges
    Port int `validate:"gte=1,lte=65535"`

    // String length
    Name string `validate:"min=3,max=50"`

    // Format validation
    Email string `validate:"email"`
    URL   string `validate:"url"`

    // Custom validators (provided by framework)
    Addr  string `validate:"hostname_port"`  // "host:port" format
    DSN   string `validate:"dsn"`            // Database DSN

    // Enums
    Level string `validate:"oneof=debug info warn error"`
}
```

## Lifecycle Management

Plugins can optionally implement lifecycle interfaces for initialization and cleanup.

### Initializer Interface (Optional)

Implement `plugin.Initializer` if your plugin needs startup initialization:

```go
func (p *RedisPlugin) Initialize(ctx context.Context) error {
    // Config is already validated by framework at this point
    p.client = redis.NewClient(&redis.Options{
        Addr:     p.config.Addr,
        Password: p.config.Password,
        DB:       p.config.DB,
        PoolSize: p.config.PoolSize,
    })

    // Test connection
    if err := p.client.Ping(ctx).Err(); err != nil {
        return fmt.Errorf("redis connection failed: %w", err)
    }

    return nil
}
```

**When to implement:**
- Establish database connections
- Initialize HTTP clients with connection pools
- Validate external service availability
- Setup internal caches or state

**When NOT to implement:**
- Plugin is purely stateless (e.g., math operations)
- Has no external dependencies
- Requires no setup beyond struct initialization

### Shutdowner Interface (Optional)

Implement `plugin.Shutdowner` if your plugin needs cleanup during shutdown:

```go
func (p *RedisPlugin) Shutdown(ctx context.Context) error {
    if p.client != nil {
        return p.client.Close()
    }
    return nil
}
```

**When to implement:**
- Close database connections
- Flush buffers or pending writes
- Cancel background goroutines
- Release file handles or network sockets

**When NOT to implement:**
- Has no resources that need explicit cleanup
- Only uses memory that will be garbage collected
- Has no background processes

**Note:** These interfaces are completely independent - implement one, both, or neither based on your plugin's needs.

## Task Methods

Task methods are automatically discovered by the framework.

### Task Method Signature

```go
func (p *PluginType) MethodName(exec *plugin.Execution, args plugin.Input) (plugin.Output, error)
```

### Task Naming

- Plugin: `RegisterPlugin("payment", paymentPlugin)`
- Method: `func (p *PaymentPlugin) Charge(...)`
- **Task name:** `payment.charge` (lowercase)

### Accessing Arguments

```go
func (p *PaymentPlugin) Charge(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    // Extract typed arguments
    amount := args["amount"].(float64)
    currency := args["currency"].(string)

    // Safe extraction with check
    if description, ok := args["description"].(string); ok {
        // Use optional description
    }

    // Business logic
    chargeID, err := p.processCharge(amount, currency)
    if err != nil {
        return nil, fmt.Errorf("charge failed: %w", err)
    }

    return plugin.Output{
        "charge_id": chargeID,
        "status": "succeeded",
    }, nil
}
```

### Accessing Execution State

```go
func (p *MyPlugin) Task(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    // Access flow properties
    apiKey := exec.Values["properties_apiKey"].(string)

    // Access previous step results
    // Note: step-id becomes step_id in Values (dashes → underscores)
    userID := exec.Values["fetch_user_result_user_id"].(int)

    // Add values to execution state (optional - framework does this automatically)
    exec.AddValue("my_custom_value", "data")

    return plugin.Output{"result": "ok"}, nil
}
```

### Using Execution as Context

```go
func (p *HTTPPlugin) Request(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    url := args["url"].(string)

    // Execution implements context.Context
    req, err := http.NewRequestWithContext(exec, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    resp, err := http.DefaultClient.Do(req)
    // ...
}
```

## Health Checks

Plugins can optionally implement `HealthChecker` for monitoring:

```go
func (p *RedisPlugin) HealthCheck(ctx context.Context) error {
    // Quick health check (< 1 second)
    if err := p.client.Ping(ctx).Err(); err != nil {
        return fmt.Errorf("redis unhealthy: %w", err)
    }
    return nil
}
```

**HealthChecker is optional** - only implement if your plugin manages external resources.

## Complete Example

```go
package email

import (
    "context"
    "fmt"
    "net/smtp"
    "github.com/sflowg/sflowg/runtime/plugin"
)

// Configuration with tags
type Config struct {
    SMTPHost string `yaml:"smtp_host" default:"smtp.gmail.com" validate:"required"`
    SMTPPort int    `yaml:"smtp_port" default:"587" validate:"gte=1,lte=65535"`
    Username string `yaml:"username" validate:"required"`
    Password string `yaml:"password" validate:"required"`
    From     string `yaml:"from" validate:"required,email"`
}

// Plugin struct
type EmailPlugin struct {
    config Config
    client *smtp.Client
}

// Optional: Lifecycle implementation
func (p *EmailPlugin) Initialize(ctx context.Context) error {
    addr := fmt.Sprintf("%s:%d", p.config.SMTPHost, p.config.SMTPPort)
    client, err := smtp.Dial(addr)
    if err != nil {
        return fmt.Errorf("failed to connect to SMTP: %w", err)
    }
    p.client = client
    return nil
}

func (p *EmailPlugin) Shutdown(ctx context.Context) error {
    if p.client != nil {
        return p.client.Close()
    }
    return nil
}

// Optional: Health check
func (p *EmailPlugin) HealthCheck(ctx context.Context) error {
    if p.client == nil {
        return fmt.Errorf("SMTP client not initialized")
    }
    // Could add more checks here
    return nil
}

// Task: email.send
func (p *EmailPlugin) Send(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    to := args["to"].(string)
    subject := args["subject"].(string)
    body := args["body"].(string)

    // Email sending logic
    message := fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body)
    err := smtp.SendMail(
        fmt.Sprintf("%s:%d", p.config.SMTPHost, p.config.SMTPPort),
        smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.SMTPHost),
        p.config.From,
        []string{to},
        []byte(message),
    )

    if err != nil {
        return nil, fmt.Errorf("failed to send email: %w", err)
    }

    return plugin.Output{
        "sent": true,
        "to": to,
    }, nil
}

// Task: email.validate
func (p *EmailPlugin) Validate(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    email := args["email"].(string)

    // Email validation logic
    valid := strings.Contains(email, "@") && strings.Contains(email, ".")

    return plugin.Output{
        "valid": valid,
        "email": email,
    }, nil
}
```

## Usage in Flows

```yaml
# flow-config.yaml
plugins:
  - source: ./plugins/email
    config:
      smtp_host: smtp.gmail.com
      smtp_port: 587
      username: ${SMTP_USERNAME}
      password: ${SMTP_PASSWORD}
      from: noreply@example.com

# flow.yaml
steps:
  - id: validate-email
    type: email.validate
    args:
      email: ${ request.body.email }

  - id: send-welcome
    type: email.send
    args:
      to: ${ request.body.email }
      subject: "Welcome!"
      body: "Thanks for signing up."
```

## What Plugin Developers DON'T Do

❌ **Don't write:**
- `DefaultConfig()` functions (use `default:"..."` tags)
- `Validate()` methods (use `validate:"..."` tags)
- Manual environment variable parsing (framework handles this)
- Task registration code (framework auto-discovers methods)
- Plugin lifecycle management (framework calls Initialize/Shutdown)

✅ **Do write:**
- Plugin struct with Config field
- Config struct with tags
- Task methods with correct signature
- Optional Lifecycle implementation
- Optional HealthChecker implementation

## Framework Responsibilities

The framework automatically handles:
- ✅ Config default value application
- ✅ Config validation
- ✅ Environment variable overrides
- ✅ Task discovery and registration
- ✅ Lifecycle management (Initialize/Shutdown)
- ✅ Health check orchestration
- ✅ Execution context management
- ✅ Error handling and logging

Plugin developers just write business logic!

## Best Practices

1. **Keep tasks focused** - One task = one operation
2. **Use Config tags** - Let framework handle configuration
3. **Implement Lifecycle** - If you manage resources (connections, clients)
4. **Add HealthChecker** - If you connect to external services
5. **Return descriptive errors** - Use `fmt.Errorf("context: %w", err)`
6. **Type assert safely** - Check `ok` for optional arguments
7. **Document Config fields** - Use comments to explain options
8. **Keep Initialize fast** - Fail-fast if dependencies unavailable

## Next Steps

- See `../PLUGIN_SYSTEM_DESIGN.md` for architecture details
- See `../PLUGIN_CONFIG_FINAL.md` for configuration system details
- Check `/plugins/http/` for a complete example
- Check `/examples/` for more plugin examples