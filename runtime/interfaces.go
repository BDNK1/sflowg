package runtime

// Initializer interface allows plugins to perform startup initialization.
// Plugins implementing this interface will have Initialize called at container startup.
type Initializer interface {
	// Initialize is called once when the container starts up.
	// Use this to establish connections, initialize clients, etc.
	// Config and dependencies are already set on the plugin struct.
	Initialize() error
}

// Shutdowner interface allows plugins to perform graceful shutdown.
// Plugins implementing this interface will have Shutdown called during graceful shutdown.
type Shutdowner interface {
	// Shutdown is called during graceful shutdown.
	// Use this to close connections, cleanup resources, etc.
	Shutdown() error
}
