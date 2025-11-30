package plugin

import (
	"github.com/sflowg/sflowg/runtime"
)

// Initializer is a type alias to runtime.Initializer.
// This allows plugins to import only the plugin package while the interface is defined in runtime.
//
// Plugins that implement this interface will have Initialize() called at container startup
// before any tasks are executed.
//
// # Optional Interface Pattern
//
// This interface is completely optional. Plugins only need to implement it if they
// require startup initialization. Simple plugins without state don't need it.
//
// # When to Implement
//
// Implement Initializer when your plugin needs to:
//   - Establish database connections
//   - Initialize HTTP clients with connection pools
//   - Validate external service availability
//   - Setup internal caches or state
//   - Load external resources (files, configurations)
//
// # When NOT to Implement
//
// Don't implement if your plugin:
//   - Is purely stateless (e.g., math operations, string formatting)
//   - Has no external dependencies
//   - Requires no setup beyond struct initialization
//
// # Implementation Example
//
//	type DatabasePlugin struct {
//	    config Config
//	    db     *sql.DB
//	}
//
//	func (p *DatabasePlugin) Initialize(exec *Execution) error {
//	    // Config is already validated by framework before this is called
//	    // exec embeds context.Context, so you can use it directly
//	    db, err := sql.Open("postgres", p.config.DSN)
//	    if err != nil {
//	        return fmt.Errorf("failed to connect: %w", err)
//	    }
//
//	    // Test connection - exec implements context.Context
//	    if err := db.PingContext(exec, p.config.DSN); err != nil {
//	        return fmt.Errorf("connection test failed: %w", err)
//	    }
//
//	    // Can also access other plugins via exec.Container
//	    // logger, _ := exec.Container.GetTask("logger.log")
//
//	    p.db = db
//	    return nil
//	}
//
// # Initialization Order
//
// The framework ensures proper initialization order:
//  1. Config validation (framework automatic)
//  2. Plugin struct initialization
//  3. Initialize() called (if implemented)
//  4. Tasks become available
//
// # Error Handling
//
// If Initialize() returns an error, the application will fail to start.
// This is intentional - fail-fast on startup is better than runtime failures.
//
// # Context Usage
//
// The context parameter allows for timeouts and cancellation during
// initialization. Use it when making network calls:
//
//	func (p *MyPlugin) Initialize(exec *Execution) error {
//	    // exec implements context.Context, use it for context propagation
//	    req, err := http.NewRequestWithContext(exec, "GET", p.config.HealthURL, nil)
//	    resp, err := http.DefaultClient.Do(req)
//	    // ...
//	}
type Initializer = runtime.Initializer

// Shutdowner is a type alias to runtime.Shutdowner.
// This allows plugins to import only the plugin package while the interface is defined in runtime.
//
// Plugins that implement this interface will have Shutdown() called during graceful shutdown.
//
// # Optional Interface Pattern
//
// This interface is completely optional. Plugins only need to implement it if they
// require cleanup during shutdown. Many plugins don't need explicit cleanup.
//
// # When to Implement
//
// Implement Shutdowner when your plugin needs to:
//   - Close database connections
//   - Flush buffers or pending writes
//   - Cancel background goroutines
//   - Release file handles or network sockets
//   - Cleanup temporary files or resources
//
// # When NOT to Implement
//
// Don't implement if your plugin:
//   - Has no resources that need explicit cleanup
//   - Only uses memory that will be garbage collected
//   - Has no background processes
//
// # Implementation Example
//
//	type CachePlugin struct {
//	    config Config
//	    cache  *redis.Client
//	}
//
//	func (p *CachePlugin) Shutdown(exec *Execution) error {
//	    if p.cache != nil {
//	        // exec implements context.Context for timeout control
//	        return p.cache.Close()
//	    }
//	    return nil
//	}
//
// # Shutdown Order
//
// Shutdown is called in reverse order of initialization to properly
// handle dependencies between plugins.
//
// # Error Handling
//
// If Shutdown() returns an error, it's logged but doesn't prevent shutdown.
// The application will continue shutting down other plugins.
//
// # Context Usage
//
// The context parameter allows for shutdown timeouts. Use it to ensure
// cleanup doesn't block indefinitely:
//
//	func (p *MyPlugin) Shutdown(exec *Execution) error {
//	    // Flush with timeout - exec implements context.Context
//	    if err := p.buffer.FlushContext(exec); err != nil {
//	        return fmt.Errorf("failed to flush buffer: %w", err)
//	    }
//	    return nil
//	}
type Shutdowner = runtime.Shutdowner
