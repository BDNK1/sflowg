package http

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sflowg/sflowg/runtime"
)

// Config holds the HTTP plugin configuration
// Phase 1: Simple struct, Phase 2: Will add declarative tags
type Config struct {
	Timeout     time.Duration // HTTP request timeout
	MaxRetries  int           // Maximum number of retries
	Debug       bool          // Enable debug logging
	RetryWaitMS int           // Wait time between retries in milliseconds
}

// HTTPPlugin implements HTTP request functionality as a plugin
type HTTPPlugin struct {
	client *resty.Client
}

// NewHTTPPlugin creates a new HTTP plugin instance
func NewHTTPPlugin() *HTTPPlugin {
	return &HTTPPlugin{}
}

// Initialize implements the Lifecycle interface
// Phase 1: Reads env vars and hardcodes defaults inline
func (h *HTTPPlugin) Initialize(ctx context.Context) error {
	// Phase 1 MVP: Build config from env vars with hardcoded defaults
	config := Config{
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		Debug:       false,
		RetryWaitMS: 100,
	}

	// Read env var overrides
	if timeout := os.Getenv("HTTP_TIMEOUT"); timeout != "" {
		if seconds, err := strconv.Atoi(timeout); err == nil {
			config.Timeout = time.Duration(seconds) * time.Second
		}
	}

	if retries := os.Getenv("HTTP_MAX_RETRIES"); retries != "" {
		if r, err := strconv.Atoi(retries); err == nil {
			config.MaxRetries = r
		}
	}

	if debug := os.Getenv("HTTP_DEBUG"); debug == "true" {
		config.Debug = true
	}

	if wait := os.Getenv("HTTP_RETRY_WAIT_MS"); wait != "" {
		if ms, err := strconv.Atoi(wait); err == nil {
			config.RetryWaitMS = ms
		}
	}

	// Create resty client with config
	h.client = resty.New().
		SetTimeout(config.Timeout).
		SetRetryCount(config.MaxRetries).
		SetRetryWaitTime(time.Duration(config.RetryWaitMS) * time.Millisecond).
		SetDebug(config.Debug)

	return nil
}

// Shutdown implements the Lifecycle interface
// Cleans up HTTP client resources
func (h *HTTPPlugin) Shutdown(ctx context.Context) error {
	// Resty client doesn't require explicit cleanup
	h.client = nil
	return nil
}

// Request executes an HTTP request
// Task name in flows: "http.request"
//
// Usage in flow YAML:
//   - task: http.request
//     args:
//     url: "https://api.example.com/users"
//     method: "POST"
//     headers:
//     Content-Type: "application/json"
//     Authorization: "Bearer ${token}"
//     queryParameters:
//     page: "1"
//     limit: "10"
//     body:
//     name: "${request.body.name}"
//     email: "${request.body.email}"
//     assign:
//     userId: "${response.body.id}"
//     status: "${response.statusCode}"
//
// Returns:
//   - status: HTTP status text (e.g., "200 OK")
//   - statusCode: HTTP status code (e.g., 200)
//   - isError: boolean indicating if request failed
//   - body.*: Flattened response body fields
//
// Task signature: func(exec *Execution, args map[string]any) (map[string]any, error)
func (h *HTTPPlugin) Request(exec *runtime.Execution, args map[string]any) (map[string]any, error) {
	requestConfig, err := h.parseArgs(exec, args)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request args: %w", err)
	}

	response, err := h.executeRequest(requestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return response, nil
}

// httpRequestConfig holds the configuration for an HTTP request
type httpRequestConfig struct {
	uri         string
	method      string
	headers     map[string]string
	queryParams map[string]string
	body        map[string]any
}

// parseArgs parses the task arguments into an httpRequestConfig
func (h *HTTPPlugin) parseArgs(exec *runtime.Execution, args map[string]any) (httpRequestConfig, error) {
	// Parse URL
	uri, ok := args["url"].(string)
	if !ok {
		return httpRequestConfig{}, fmt.Errorf("url not found or not a string")
	}

	// Parse method
	method, ok := args["method"].(string)
	if !ok {
		return httpRequestConfig{}, fmt.Errorf("method not found or not a string")
	}

	// Parse headers (optional)
	headers := make(map[string]any)
	if headersRaw, exists := args["headers"]; exists {
		if headersMap, ok := headersRaw.(map[string]any); ok {
			for key, value := range headersMap {
				if valueStr, ok := value.(string); ok {
					// Evaluate expressions in header values
					headerValue, err := runtime.Eval(valueStr, exec.Values)
					if err != nil {
						return httpRequestConfig{}, fmt.Errorf("error evaluating header %s: %w", key, err)
					}
					headers[key] = headerValue
				} else {
					headers[key] = value
				}
			}
		} else {
			return httpRequestConfig{}, fmt.Errorf("headers must be a map[string]any")
		}
	}

	// Parse query parameters (optional)
	queryParameters := make(map[string]any)
	if queryRaw, exists := args["queryParameters"]; exists {
		if queryMap, ok := queryRaw.(map[string]any); ok {
			for key, value := range queryMap {
				if valueStr, ok := value.(string); ok {
					// Evaluate expressions in query parameter values
					queryValue, err := runtime.Eval(valueStr, exec.Values)
					if err != nil {
						return httpRequestConfig{}, fmt.Errorf("error evaluating query parameter %s: %w", key, err)
					}
					queryParameters[key] = queryValue
				} else {
					queryParameters[key] = value
				}
			}
		} else {
			return httpRequestConfig{}, fmt.Errorf("queryParameters must be a map[string]any")
		}
	}

	// Parse body (optional)
	body := make(map[string]any)
	if bodyRaw, exists := args["body"]; exists {
		if bodyMap, ok := bodyRaw.(map[string]any); ok {
			for key, value := range bodyMap {
				if valueStr, ok := value.(string); ok {
					// Evaluate expressions in body field values
					bodyValue, err := runtime.Eval(valueStr, exec.Values)
					if err != nil {
						return httpRequestConfig{}, fmt.Errorf("error evaluating body field %s: %w", key, err)
					}
					body[key] = bodyValue
				} else {
					body[key] = value
				}
			}
		} else {
			return httpRequestConfig{}, fmt.Errorf("body must be a map[string]any")
		}
	}

	return httpRequestConfig{
		uri:         uri,
		method:      method,
		headers:     runtime.ToStringValueMap(headers),
		queryParams: runtime.ToStringValueMap(queryParameters),
		body:        body,
	}, nil
}

// executeRequest executes the HTTP request using the resty client
func (h *HTTPPlugin) executeRequest(config httpRequestConfig) (map[string]any, error) {
	response := map[string]any{}
	errorResponse := map[string]any{}

	resp, err := h.client.R().
		SetHeaders(config.headers).
		SetQueryParams(config.queryParams).
		SetBody(config.body).
		SetResult(&response).
		SetError(&errorResponse).
		Execute(config.method, config.uri)

	if err != nil {
		return nil, err
	}

	result := make(map[string]any)
	result["status"] = resp.Status()
	result["statusCode"] = resp.StatusCode()
	result["isError"] = resp.IsError()

	// Flatten response body into result
	if resp.IsError() {
		for k, v := range errorResponse {
			result[fmt.Sprintf("body.%s", k)] = v
		}
	} else {
		for k, v := range response {
			result[fmt.Sprintf("body.%s", k)] = v
		}
	}

	return result, nil
}
