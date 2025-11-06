package runtime

import "context"

// Lifecycle interface allows plugins to manage connections and resources
// Plugins implementing this interface will have Initialize called at startup
// and Shutdown called during graceful shutdown
type Lifecycle interface {
	// Initialize is called once when the container starts up
	// Use this to establish connections, initialize clients, etc.
	Initialize(ctx context.Context) error

	// Shutdown is called during graceful shutdown
	// Use this to close connections, cleanup resources, etc.
	Shutdown(ctx context.Context) error
}
