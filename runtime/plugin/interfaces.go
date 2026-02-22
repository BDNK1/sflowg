package plugin

import (
	"github.com/BDNK1/sflowg/runtime"
)

// Initializer is a type alias to runtime.Initializer.
// Plugins implementing this interface will have Initialize() called at container startup.
//
// # When to Implement
//
// Implement Initializer when your plugin needs to:
//   - Establish database connections
//   - Initialize HTTP clients with connection pools
//   - Validate external service availability
//   - Setup internal caches or state
//
// # Implementation Example
//
//	type DatabasePlugin struct {
//	    Config Config
//	    db     *sql.DB
//	}
//
//	func (p *DatabasePlugin) Initialize() error {
//	    db, err := sql.Open("postgres", p.Config.DSN)
//	    if err != nil {
//	        return fmt.Errorf("failed to connect: %w", err)
//	    }
//
//	    ctx, cancel := context.WithTimeout(context.Background(), p.Config.ConnectTimeout)
//	    defer cancel()
//
//	    if err := db.PingContext(ctx); err != nil {
//	        return fmt.Errorf("connection test failed: %w", err)
//	    }
//
//	    p.db = db
//	    return nil
//	}
//
// # Error Handling
//
// If Initialize() returns an error, the application will fail to start.
// This is intentional - fail-fast on startup is better than runtime failures.
type Initializer = runtime.Initializer

// Shutdowner is a type alias to runtime.Shutdowner.
// Plugins implementing this interface will have Shutdown() called during graceful shutdown.
//
// # When to Implement
//
// Implement Shutdowner when your plugin needs to:
//   - Close database connections
//   - Flush buffers or pending writes
//   - Cancel background goroutines
//   - Release file handles or network sockets
//
// # Implementation Example
//
//	func (p *CachePlugin) Shutdown() error {
//	    if p.cache != nil {
//	        return p.cache.Close()
//	    }
//	    return nil
//	}
//
// # Shutdown Order
//
// Shutdown is called in reverse order of initialization to properly
// handle dependencies between plugins.
type Shutdowner = runtime.Shutdowner
