package runtime

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// JSONResponseHandler handles JSON responses
type JSONResponseHandler struct{}

func (h *JSONResponseHandler) Handle(c *gin.Context, exec *Execution, args map[string]any) error {
	// Default status code
	statusCode := http.StatusOK

	// Extract status if provided
	if status, ok := toStatusCode(args["status"]); ok {
		statusCode = status
	}

	// Extract and set headers if provided
	if headers, ok := args["headers"].(map[string]any); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				c.Header(key, strValue)
			}
		}
	}

	// Extract body - can be nil, map, or other types
	body := args["body"]
	if body == nil {
		body = gin.H{}
	}

	c.JSON(statusCode, body)
	return nil
}

// HTMLResponseHandler handles HTML responses
type HTMLResponseHandler struct{}

func (h *HTMLResponseHandler) Handle(c *gin.Context, exec *Execution, args map[string]any) error {
	log := exec.Logger()
	// Default status code
	statusCode := http.StatusOK

	// Extract status if provided
	if status, ok := toStatusCode(args["status"]); ok {
		statusCode = status
	}

	// Extract and validate body (must be string)
	body, ok := args["body"].(string)
	if !ok {
		log.Error("HTML response body must be a string",
			"bodyType", fmt.Sprintf("%T", args["body"]))
		return fmt.Errorf("html response body must be a string, got %T", args["body"])
	}

	// Extract and set headers if provided
	if headers, ok := args["headers"].(map[string]any); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				c.Header(key, strValue)
			}
		}
	}

	c.Data(statusCode, "text/html; charset=utf-8", []byte(body))
	return nil
}

// RedirectResponseHandler handles HTTP redirects
type RedirectResponseHandler struct{}

func (h *RedirectResponseHandler) Handle(c *gin.Context, exec *Execution, args map[string]any) error {
	log := exec.Logger()
	// Extract location (required)
	location, ok := args["location"].(string)
	if !ok || location == "" {
		log.Error("Redirect response requires a location")
		return fmt.Errorf("redirect response requires a 'location' argument")
	}

	// Default to 302 (Found)
	statusCode := http.StatusFound

	// Extract status if provided and validate it's a redirect code
	if status, ok := toStatusCode(args["status"]); ok {
		if status < 300 || status >= 400 {
			log.Error("Invalid redirect status code",
				"status", status)
			return fmt.Errorf("redirect status must be 3xx, got %d", status)
		}
		statusCode = status
	}

	c.Redirect(statusCode, location)
	return nil
}

func toStatusCode(v any) (int, bool) {
	switch s := v.(type) {
	case int:
		return s, true
	case int64:
		return int(s), true
	case float64:
		return int(s), true
	default:
		return 0, false
	}
}
