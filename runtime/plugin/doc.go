// Package plugin provides the minimal interface for SFlowG plugin development.
//
// This package contains ONLY the types and interfaces that plugin developers
// need to interact with. Plugin authors should import this package and never
// import the parent "runtime" package directly.
//
// # Import Restriction
//
// Plugin developers should ONLY import:
//
//	import "github.com/sflowg/sflowg/runtime/plugin"
//
// NEVER import:
//
//	import "github.com/sflowg/sflowg/runtime"  // ❌ Too much access to internals
//
// # Plugin Structure
//
// A minimal plugin requires:
//  1. A plugin struct (can be empty)
//  2. At least one public task method
//
// Example:
//
//	type MyPlugin struct {}
//
//	func (p *MyPlugin) Greet(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
//	    name := args["name"].(string)
//	    return plugin.Output{"message": "Hello, " + name}, nil
//	}
//
// # Configuration
//
// Plugins can define a Config struct with declarative tags:
//
//	type Config struct {
//	    Timeout time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s"`
//	    Port    int          `yaml:"port" default:"8080" validate:"gte=1,lte=65535"`
//	}
//
//	type MyPlugin struct {
//	    config Config
//	}
//
// The framework handles all config processing (defaults, validation, env vars).
// Plugin developers never call validation or default functions - just write tags.
//
// # Lifecycle Management
//
// Plugins can optionally implement Lifecycle for initialization/cleanup:
//
//	func (p *MyPlugin) Initialize(ctx context.Context) error {
//	    // Setup connections, resources
//	    // Config is already validated at this point
//	    return nil
//	}
//
//	func (p *MyPlugin) Shutdown(ctx context.Context) error {
//	    // Cleanup resources
//	    return nil
//	}
//
// # Task Methods
//
// Task methods are automatically discovered if they have the correct signature:
//
//	func (p *PluginType) MethodName(exec *plugin.Execution, args plugin.Input) (plugin.Output, error)
//
// Task naming: PluginName.MethodName → pluginname.methodname (lowercase)
//
// Example: EmailPlugin.Send() → task name "email.send" in YAML flows
//
// # Execution Context
//
// The Execution parameter provides access to:
//   - Flow state: exec.Values (map of all previous step results)
//   - Container: exec.Container (access to other plugins/tasks if needed)
//   - Context: exec implements context.Context (for cancellation, timeouts)
//   - Request: exec.Request() (HTTP request details if flow is HTTP-triggered)
//
// # What Plugin Developers DON'T Do
//
//   - Never call config validation functions (framework does this)
//   - Never register tasks manually (framework auto-discovers them)
//   - Never manage plugin lifecycle directly (framework handles Initialize/Shutdown)
//   - Never import "runtime" package (use "runtime/plugin" instead)
//
// # Complete Example
//
//	package email
//
//	import (
//	    "context"
//	    "net/smtp"
//	    "github.com/sflowg/sflowg/runtime/plugin"
//	)
//
//	type Config struct {
//	    SMTPHost string `yaml:"smtp_host" default:"smtp.gmail.com" validate:"required"`
//	    SMTPPort int    `yaml:"smtp_port" default:"587" validate:"gte=1,lte=65535"`
//	    Username string `yaml:"username" validate:"required"`
//	    Password string `yaml:"password" validate:"required"`
//	}
//
//	type EmailPlugin struct {
//	    config Config
//	    client *smtp.Client
//	}
//
//	// Optional: Lifecycle implementation
//	func (p *EmailPlugin) Initialize(ctx context.Context) error {
//	    // Config is already validated by framework
//	    addr := fmt.Sprintf("%s:%d", p.config.SMTPHost, p.config.SMTPPort)
//	    client, err := smtp.Dial(addr)
//	    if err != nil {
//	        return err
//	    }
//	    p.client = client
//	    return nil
//	}
//
//	func (p *EmailPlugin) Shutdown(ctx context.Context) error {
//	    if p.client != nil {
//	        return p.client.Close()
//	    }
//	    return nil
//	}
//
//	// Task: email.send
//	func (p *EmailPlugin) Send(exec *plugin.Execution, args plugin.Input) (plugin.Output, error) {
//	    to := args["to"].(string)
//	    subject := args["subject"].(string)
//	    body := args["body"].(string)
//
//	    // Send email using p.client
//	    // ...
//
//	    return plugin.Output{"sent": true, "message_id": "msg_123"}, nil
//	}
package plugin
