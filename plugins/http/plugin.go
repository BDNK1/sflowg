package http

import (
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/BDNK1/sflowg/runtime/plugin"
)

// Config holds the HTTP plugin configuration with declarative tags
type Config struct {
	Timeout     time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s"`
	MaxRetries  int           `yaml:"max_retries" default:"3" validate:"gte=0,lte=10"`
	Debug       bool          `yaml:"debug" default:"false"`
	RetryWaitMS int           `yaml:"retry_wait_ms" default:"100" validate:"gte=0,lte=10000"`
}

// RequestInput defines the typed input for HTTP requests
type RequestInput struct {
	URL         string            `json:"url" validate:"required,url"`
	Method      string            `json:"method" validate:"required,oneof=GET POST PUT PATCH DELETE HEAD OPTIONS"`
	Headers     map[string]string `json:"headers"`
	QueryParams map[string]string `json:"query_parameters"`
	Body        map[string]any    `json:"body"`
	ContentType string            `json:"content_type" validate:"omitempty,oneof=json form"` // "json" (default) or "form"
}

// RequestOutput defines the typed output for HTTP requests
type RequestOutput struct {
	Status     string            `json:"status"`
	StatusCode int               `json:"status_code"`
	IsError    bool              `json:"is_error"`
	Headers    map[string]string `json:"headers"`
	Body       map[string]any    `json:"body"`
}

// HTTPPlugin implements HTTP request functionality as a plugin
type HTTPPlugin struct {
	Config Config // Exported so CLI can set it during initialization
	client *resty.Client
}

// Initialize implements the plugin.Initializer interface
// Config is already validated by the framework before this is called
func (h *HTTPPlugin) Initialize() error {
	h.client = resty.New().
		SetTimeout(h.Config.Timeout).
		SetRetryCount(h.Config.MaxRetries).
		SetRetryWaitTime(time.Duration(h.Config.RetryWaitMS) * time.Millisecond).
		SetDebug(h.Config.Debug)

	return nil
}

// Request executes an HTTP request using typed input/output
// The framework automatically validates input and converts between map and struct
func (h *HTTPPlugin) Request(exec *plugin.Execution, input RequestInput) (RequestOutput, error) {
	response := map[string]any{}
	errorResponse := map[string]any{}

	req := h.client.R().
		SetHeaders(input.Headers).
		SetQueryParams(input.QueryParams).
		SetResult(&response).
		SetError(&errorResponse)

	// Set body based on content type
	if input.ContentType == "form" {
		formData := flattenToFormData(input.Body, "")
		req.SetFormData(formData)
	} else {
		// Default to JSON
		req.SetBody(input.Body)
	}

	resp, err := req.Execute(input.Method, input.URL)
	if err != nil {
		return RequestOutput{}, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Extract response headers (first value for each header)
	headers := make(map[string]string)
	for key, values := range resp.Header() {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Build typed output
	output := RequestOutput{
		Status:     resp.Status(),
		StatusCode: resp.StatusCode(),
		IsError:    resp.IsError(),
		Headers:    headers,
	}

	// Use appropriate response based on error status
	if resp.IsError() {
		output.Body = errorResponse
	} else {
		output.Body = response
	}

	return output, nil
}

// flattenToFormData converts a nested map to form-encoded format with bracket notation
// Example: {"metadata": {"key": "value"}} -> {"metadata[key]": "value"}
func flattenToFormData(data map[string]any, prefix string) map[string]string {
	result := make(map[string]string)

	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "[" + key + "]"
		}

		switch v := value.(type) {
		case map[string]any:
			// Recursively flatten nested maps
			for k, val := range flattenToFormData(v, fullKey) {
				result[k] = val
			}
		case []any:
			// Handle arrays: key[0], key[1], etc.
			for i, item := range v {
				arrayKey := fmt.Sprintf("%s[%d]", fullKey, i)
				if nested, ok := item.(map[string]any); ok {
					for k, val := range flattenToFormData(nested, arrayKey) {
						result[k] = val
					}
				} else {
					result[arrayKey] = fmt.Sprintf("%v", item)
				}
			}
		default:
			result[fullKey] = fmt.Sprintf("%v", v)
		}
	}

	return result
}

// Shutdown implements the plugin.Shutdowner interface
func (h *HTTPPlugin) Shutdown() error {
	// Resty doesn't require explicit cleanup, but we can nil the client
	h.client = nil
	return nil
}
