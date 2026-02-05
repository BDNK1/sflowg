# Plugin Development

Create custom plugins to extend SFlowG with new tasks.

## When to Create a Plugin

Create a plugin when you need to:
- Call external APIs with custom logic
- Integrate with databases or services
- Implement business-specific operations
- Reuse functionality across flows

## Quick Start

### 1. Create Plugin Directory

```bash
mkdir -p plugins/greeting
cd plugins/greeting
go mod init example.com/plugins/greeting
```

### 2. Create Plugin Code

Create `plugin.go`:

```go
package greeting

import "github.com/BDNK1/sflowg/runtime/plugin"

// Plugin struct (can be empty for simple plugins)
type GreetingPlugin struct{}

// Task: greeting.hello
func (p *GreetingPlugin) Hello(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    name := args["name"].(string)
    return plugin.Output{
        "message": "Hello, " + name + "!",
    }, nil
}
```

### 3. Register Plugin

Add to `flow-config.yaml`:

```yaml
plugins:
  - source: ./plugins/greeting
```

### 4. Use in Flows

```yaml
steps:
  - id: greet_user
    type: greeting.hello
    args:
      name: request.body.name

return:
  type: json
  args:
    greeting: greet_user.result.message
```

## Plugin Structure

```
plugins/myPlugin/
├── go.mod           # Go module file
└── plugin.go        # Plugin implementation
```

### Naming Conventions

| Component | Convention | Example |
|-----------|------------|---------|
| Package | lowercase | `payment` |
| Plugin struct | `XxxPlugin` | `PaymentPlugin` |
| Config struct | `Config` | `Config` |
| Task method | `PascalCase` | `ProcessPayment` |
| Task name | `plugin.method` | `payment.processPayment` |

## Configuration

Add configuration to your plugin:

```go
package payment

import (
    "time"
    "github.com/BDNK1/sflowg/runtime/plugin"
)

// Config with struct tags
type Config struct {
    APIKey     string        `yaml:"api_key" validate:"required"`
    BaseURL    string        `yaml:"base_url" default:"https://api.payment.com"`
    Timeout    time.Duration `yaml:"timeout" default:"30s"`
    MaxRetries int           `yaml:"max_retries" default:"3" validate:"gte=0,lte=10"`
}

type PaymentPlugin struct {
    config Config  // Must be lowercase, named "config"
}

func (p *PaymentPlugin) Charge(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    // Access config
    url := p.config.BaseURL + "/charges"
    // ...
}
```

Configure in `flow-config.yaml`:

```yaml
plugins:
  - source: ./plugins/payment
    config:
      api_key: ${PAYMENT_API_KEY}
      base_url: https://api.stripe.com
      timeout: 60s
      max_retries: 5
```

### Struct Tags

| Tag | Purpose | Example |
|-----|---------|---------|
| `yaml:"name"` | YAML field name | `yaml:"api_key"` |
| `default:"value"` | Default if not set | `default:"30s"` |
| `validate:"rules"` | Validation rules | `validate:"required"` |

### Validation Rules

```go
type Config struct {
    Required  string `validate:"required"`           // Must be set
    Port      int    `validate:"gte=1,lte=65535"`   // Range
    Email     string `validate:"email"`              // Email format
    URL       string `validate:"url"`                // URL format
    Level     string `validate:"oneof=debug info"`   // Enum
}
```

## Lifecycle Hooks

### Initialize (Optional)

Called after configuration is loaded:

```go
import "context"

func (p *PaymentPlugin) Initialize(ctx context.Context) error {
    // Setup connections, clients, etc.
    p.client = NewAPIClient(p.config.BaseURL, p.config.APIKey)
    return nil
}
```

### Shutdown (Optional)

Called during graceful shutdown:

```go
func (p *PaymentPlugin) Shutdown(ctx context.Context) error {
    if p.client != nil {
        return p.client.Close()
    }
    return nil
}
```

## Task Methods

### Signature

```go
func (p *PluginType) TaskName(exec *plugin.Execution, args plugin.Input) (plugin.Output, error)
```

### Accessing Arguments

```go
func (p *MyPlugin) Process(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    // Required argument
    amount := args["amount"].(float64)

    // Optional argument with check
    var description string
    if desc, ok := args["description"].(string); ok {
        description = desc
    }

    // Return result
    return plugin.Output{
        "status": "success",
        "processed_amount": amount,
    }, nil
}
```

### Using Execution Context

```go
func (p *HTTPPlugin) Fetch(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    url := args["url"].(string)

    // exec implements context.Context
    req, _ := http.NewRequestWithContext(exec, "GET", url, nil)
    resp, err := http.DefaultClient.Do(req)
    // ...
}
```

### Accessing Flow Data

```go
func (p *MyPlugin) Task(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    // Access properties
    apiKey := exec.Values["properties.apiKey"]

    // Access previous step results
    userID := exec.Values["fetch_user.result.id"]

    // ...
}
```

## Dependencies

Plugins can depend on other plugins:

```go
type APIPlugin struct {
    config Config
    http   *http.HTTPPlugin  // Dependency - pointer to another plugin
}

func (p *APIPlugin) FetchUser(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    // Use HTTP plugin
    result, err := p.http.Request(exec, plugin.Input{
        "url": p.config.BaseURL + "/users/" + args["id"].(string),
        "method": "GET",
    })
    // ...
}
```

Dependencies are automatically injected by the framework.

## Complete Example

```go
package notification

import (
    "context"
    "fmt"
    "net/smtp"

    "github.com/BDNK1/sflowg/runtime/plugin"
)

type Config struct {
    SMTPHost string `yaml:"smtp_host" default:"localhost" validate:"required"`
    SMTPPort int    `yaml:"smtp_port" default:"587" validate:"gte=1,lte=65535"`
    From     string `yaml:"from" validate:"required,email"`
    Username string `yaml:"username"`
    Password string `yaml:"password"`
}

type NotificationPlugin struct {
    config Config
}

func (p *NotificationPlugin) Initialize(ctx context.Context) error {
    // Validate SMTP connection
    addr := fmt.Sprintf("%s:%d", p.config.SMTPHost, p.config.SMTPPort)
    conn, err := smtp.Dial(addr)
    if err != nil {
        return fmt.Errorf("SMTP connection failed: %w", err)
    }
    conn.Close()
    return nil
}

// Task: notification.sendEmail
func (p *NotificationPlugin) SendEmail(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    to := args["to"].(string)
    subject := args["subject"].(string)
    body := args["body"].(string)

    message := fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body)
    addr := fmt.Sprintf("%s:%d", p.config.SMTPHost, p.config.SMTPPort)

    var auth smtp.Auth
    if p.config.Username != "" {
        auth = smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.SMTPHost)
    }

    err := smtp.SendMail(addr, auth, p.config.From, []string{to}, []byte(message))
    if err != nil {
        return nil, fmt.Errorf("send email failed: %w", err)
    }

    return plugin.Output{
        "sent": true,
        "to":   to,
    }, nil
}

// Task: notification.sendSMS
func (p *NotificationPlugin) SendSMS(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
    phone := args["phone"].(string)
    message := args["message"].(string)

    // SMS sending logic here
    // ...

    return plugin.Output{
        "sent":  true,
        "phone": phone,
    }, nil
}
```

**flow-config.yaml:**
```yaml
plugins:
  - source: ./plugins/notification
    config:
      smtp_host: ${SMTP_HOST:smtp.gmail.com}
      smtp_port: ${SMTP_PORT:587}
      from: ${SMTP_FROM}
      username: ${SMTP_USER}
      password: ${SMTP_PASS}
```

**Usage in flow:**
```yaml
steps:
  - id: send_welcome
    type: notification.sendEmail
    args:
      to: request.body.email
      subject: '"Welcome to our service!"'
      body: '"Thank you for signing up."'
```

## Testing Plugins

```go
package notification

import (
    "context"
    "testing"

    "github.com/BDNK1/sflowg/runtime/plugin"
)

func TestSendEmail(t *testing.T) {
    p := &NotificationPlugin{
        config: Config{
            SMTPHost: "localhost",
            SMTPPort: 1025, // MailHog or similar
            From:     "test@example.com",
        },
    }

    exec := &plugin.Execution{
        Values: make(map[string]any),
    }

    result, err := p.SendEmail(exec, plugin.Input{
        "to":      "user@example.com",
        "subject": "Test",
        "body":    "Test message",
    })

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if result["sent"] != true {
        t.Errorf("expected sent=true, got %v", result["sent"])
    }
}
```

## Best Practices

1. **Keep tasks focused** - One task, one responsibility
2. **Use configuration tags** - Let framework handle defaults and validation
3. **Implement lifecycle hooks** - If managing connections or resources
4. **Return descriptive errors** - Wrap errors with context
5. **Type assert safely** - Check `ok` for optional arguments
6. **Document config fields** - Add comments explaining options

## Related Documentation

- [Flow Syntax](./FLOW_SYNTAX.md) - Using plugins in flows
- [Configuration](./FLOW_CONFIG.md) - Plugin configuration in flow-config.yaml
- [HTTP Plugin](../plugins/http/README.md) - Example core plugin