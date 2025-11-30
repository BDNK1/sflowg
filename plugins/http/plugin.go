package http

import (
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sflowg/sflowg/runtime/plugin"
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
}

// RequestOutput defines the typed output for HTTP requests
type RequestOutput struct {
	Status     string         `json:"status"`
	StatusCode int            `json:"status_code"`
	IsError    bool           `json:"is_error"`
	Body       map[string]any `json:"body"`
}

// HTTPPlugin implements HTTP request functionality as a plugin
type HTTPPlugin struct {
	Config Config // Exported so CLI can set it during initialization
	client *resty.Client
}

// Initialize implements the plugin.Initializer interface
// Config is already validated by the framework before this is called
func (h *HTTPPlugin) Initialize(exec *plugin.Execution) error {
	// Create resty client with validated config
	// exec implements context.Context, so it can be used anywhere a context is needed
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

	// Execute request with resty
	resp, err := h.client.R().
		SetHeaders(input.Headers).
		SetQueryParams(input.QueryParams).
		SetBody(input.Body).
		SetResult(&response).
		SetError(&errorResponse).
		Execute(input.Method, input.URL)

	if err != nil {
		return RequestOutput{}, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Build typed output
	output := RequestOutput{
		Status:     resp.Status(),
		StatusCode: resp.StatusCode(),
		IsError:    resp.IsError(),
	}

	// Use appropriate response based on error status
	if resp.IsError() {
		output.Body = errorResponse
	} else {
		output.Body = response
	}

	return output, nil
}

// Shutdown implements the plugin.Shutdowner interface
func (h *HTTPPlugin) Shutdown(exec *plugin.Execution) error {
	// Resty doesn't require explicit cleanup, but we can nil the client
	h.client = nil
	return nil
}
