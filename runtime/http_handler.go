package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func NewHttpHandler(flow *Flow, container *Container, executor *Executor, globalProperties map[string]any, g *gin.Engine) {
	config := flow.Entrypoint.Config
	method := strings.ToLower(config["method"].(string))
	path := config["path"].(string)

	fmt.Printf("registering HTTP entrypoint for %s %s \n", method, path)

	switch method {
	case "get":
		g.GET(path, handleRequest(flow, container, executor, globalProperties, false))
	case "post":
		g.POST(path, handleRequest(flow, container, executor, globalProperties, true))
	default:
		fmt.Printf("Method %s is not supported", method)
	}
}

func handleRequest(flow *Flow, container *Container, executor *Executor, globalProperties map[string]any, withBody bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		e := NewExecution(flow, container, globalProperties)

		extractRequestData(c, flow, &e, withBody)

		err := executor.ExecuteSteps(&e)

		if err != nil {
			slog.Error("Flow execution failed",
				"flow", flow.ID,
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "Error in task execution: " + err.Error(),
			})
			return
		}

		toResponse(c, &e)
	}
}

const (
	PathVariablesKey   = "pathVariables"
	QueryParametersKey = "queryParameters"
	HeadersKey         = "headers"

	PathVariablesPrefix   = "request.pathVariables"
	QueryParametersPrefix = "request.queryParameters"
	HeadersPrefix         = "request.headers"
	RequestBodyPrefix     = "request.body"
	RequestRawBodyKey     = "request.rawBody"
)

func extractRequestData(c *gin.Context, f *Flow, e *Execution, withBody bool) {
	if pathVariables, ok := f.Entrypoint.Config[PathVariablesKey].([]any); ok {
		extractValues(e, pathVariables, PathVariablesPrefix, c.Param)
	}

	if queryParameters, ok := f.Entrypoint.Config[QueryParametersKey].([]any); ok {
		extractValues(e, queryParameters, QueryParametersPrefix, c.Query)
	}

	if headers, ok := f.Entrypoint.Config[HeadersKey].([]any); ok {
		extractValues(e, headers, HeadersPrefix, c.GetHeader)
	}

	if withBody {
		extractBody(c, f, e)
	}
}

func extractValues(e *Execution, keys []any, prefix string, getValue func(string) string) {
	for _, key := range keys {
		if v, ok := key.(string); ok {
			e.AddValue(fmt.Sprintf("%s.%s", prefix, v), getValue(v))
		}
	}
}

func extractBody(c *gin.Context, f *Flow, e *Execution) {
	bodyConfig := f.Entrypoint.Config["body"].(map[string]any)
	bodyType := bodyConfig["type"].(string)

	if bodyType == "json" {
		extractJsonBody(c, e)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Body type is not supported"})
	}
}

var wrongBodyFormatRes = gin.H{"message": "Wrong request body format"}

func extractJsonBody(c *gin.Context, e *Execution) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, wrongBodyFormatRes)
		return
	}

	// Store raw body for webhook signature verification and similar use cases
	e.AddValue(RequestRawBodyKey, string(body))

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		c.JSON(http.StatusBadRequest, wrongBodyFormatRes)
		return
	}

	// Store values at all levels (intermediate objects + leaf values)
	// This allows both: request.body.metadata.order_id AND request.body.metadata != null
	storeWithIntermediates(e, RequestBodyPrefix, parsed)
}

// storeWithIntermediates recursively stores values at every level of the JSON structure.
// This allows both leaf value access (request.body.metadata.order_id) and
// intermediate object checks (request.body.metadata != null).
func storeWithIntermediates(e *Execution, prefix string, value any) {
	// Always store current value (whether object, array, or leaf)
	e.AddValue(prefix, value)

	// If it's a map, recurse into children
	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			storeWithIntermediates(e, prefix+"."+k, v)
		}
	}

	// If it's an array, recurse with indices
	if arr, ok := value.([]any); ok {
		for i, v := range arr {
			storeWithIntermediates(e, fmt.Sprintf("%s.%d", prefix, i), v)
		}
	}
}

func toResponse(c *gin.Context, e *Execution) {
	// Handle case where no return section is defined
	if e.Flow.Return.Type == "" {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return
	}

	// Default status code
	statusCode := http.StatusOK
	response := make(map[string]any)

	// Process return arguments with expression evaluation
	for key, valueExpr := range e.Flow.Return.Args {
		switch key {
		case "status":
			// Handle status code
			if expr, ok := valueExpr.(string); ok {
				if value, err := Eval(expr, e.Values); err == nil {
					if code, ok := value.(int); ok {
						statusCode = code
					}
				}
			} else if code, ok := valueExpr.(int); ok {
				statusCode = code
			}
		case "body":
			// Handle response body
			if bodyExpr, ok := valueExpr.(string); ok {
				// Body is a string expression - evaluate it
				if value, err := Eval(bodyExpr, e.Values); err == nil {
					// The evaluated value should be a map
					if bodyMap, ok := value.(map[string]any); ok {
						response = bodyMap
					}
				}
			} else if bodyArgs, ok := valueExpr.(map[string]any); ok {
				// Body is a map - evaluate each field
				for bodyKey, bodyValueExpr := range bodyArgs {
					if expr, ok := bodyValueExpr.(string); ok {
						if value, err := Eval(expr, e.Values); err == nil {
							response[bodyKey] = value
						} else {
							// If expression evaluation fails, use the raw value
							response[bodyKey] = bodyValueExpr
						}
					} else {
						response[bodyKey] = bodyValueExpr
					}
				}
			}
		}
	}

	c.JSON(statusCode, response)
}
