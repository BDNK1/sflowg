package runtime

import (
	"github.com/gin-gonic/gin"
)

// ResponseHandler defines the contract for all response handlers
type ResponseHandler interface {
	Handle(c *gin.Context, exec *Execution, args map[string]any) error
}

// ResponseHandlerRegistry manages all registered response handlers
type ResponseHandlerRegistry struct {
	handlers map[string]ResponseHandler
}

// NewResponseHandlerRegistry creates a new response handler registry with built-in handlers
func NewResponseHandlerRegistry() *ResponseHandlerRegistry {
	registry := &ResponseHandlerRegistry{
		handlers: make(map[string]ResponseHandler),
	}

	// Register built-in handlers
	registry.Register("http.json", &JSONResponseHandler{})
	registry.Register("http.html", &HTMLResponseHandler{})
	registry.Register("http.redirect", &RedirectResponseHandler{})

	return registry
}

// Register adds a response handler to the registry
func (r *ResponseHandlerRegistry) Register(handlerType string, handler ResponseHandler) {
	r.handlers[handlerType] = handler
}

// Get retrieves a response handler by type
func (r *ResponseHandlerRegistry) Get(handlerType string) (ResponseHandler, bool) {
	handler, exists := r.handlers[handlerType]
	return handler, exists
}

// All returns the full map of registered response handlers.
// Used by the DSL plugin bridge to create Risor-callable response functions.
func (r *ResponseHandlerRegistry) All() map[string]ResponseHandler {
	return r.handlers
}
