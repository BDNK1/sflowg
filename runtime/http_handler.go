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

func NewHttpHandler(flow *Flow, container *Container, executor *Executor, globalProperties map[string]any, newValueStore func() ValueStore, g *gin.Engine) {
	config := flow.Entrypoint.Config
	method := strings.ToLower(config["method"].(string))
	path := config["path"].(string)

	fmt.Printf("registering HTTP entrypoint for %s %s \n", method, path)

	switch method {
	case "get":
		g.GET(path, handleRequest(flow, container, executor, globalProperties, newValueStore, false))
	case "post":
		g.POST(path, handleRequest(flow, container, executor, globalProperties, newValueStore, true))
	default:
		fmt.Printf("Method %s is not supported", method)
	}
}

func handleRequest(flow *Flow, container *Container, executor *Executor, globalProperties map[string]any, newValueStore func() ValueStore, withBody bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		e := NewExecution(flow, container, globalProperties, newValueStore())

		extractRequestData(c, flow, &e, withBody)

		if err := executor.ExecuteSteps(&e); err != nil {
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

		dispatchResponse(c, &e)
	}
}

// dispatchResponse handles the HTTP response dispatch based on the execution's ResponseDescriptor.
// If no descriptor was set by any step, returns a default 200 OK.
func dispatchResponse(c *gin.Context, execution *Execution) {
	if execution.ResponseDescriptor == nil {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return
	}

	handler, ok := execution.Container.ResponseHandlers.Get(execution.ResponseDescriptor.HandlerName)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "unknown response handler: " + execution.ResponseDescriptor.HandlerName,
		})
		return
	}

	if err := handler.Handle(c, execution, execution.ResponseDescriptor.Args); err != nil {
		slog.Error("Response handler execution failed",
			"flow", execution.Flow.ID,
			"handler", execution.ResponseDescriptor.HandlerName,
			"error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Error generating response: " + err.Error(),
		})
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
	bodyConfig, ok := f.Entrypoint.Config["body"].(map[string]any)
	if !ok {
		return
	}
	bodyType, ok := bodyConfig["type"].(string)
	if !ok {
		return
	}

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
	e.Store.SetNested(RequestBodyPrefix, parsed)
}
