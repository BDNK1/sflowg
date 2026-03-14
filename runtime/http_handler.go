package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func NewHttpHandler(flow *Flow, container *Container, executor *Executor, globalProperties map[string]any, newValueStore func() ValueStore, g *gin.Engine) {
	config := flow.Entrypoint.Config
	method := strings.ToLower(config["method"].(string))
	path := config["path"].(string)

	container.logger.Info("Registering HTTP entrypoint", "method", method, "path", path, "flow_id", flow.ID)

	switch method {
	case "get":
		g.GET(path, handleRequest(flow, path, container, executor, globalProperties, newValueStore, false))
	case "post":
		g.POST(path, handleRequest(flow, path, container, executor, globalProperties, newValueStore, true))
	default:
		container.logger.Error("Unsupported HTTP method", "method", method, "flow_id", flow.ID)
	}
}

func handleRequest(flow *Flow, route string, container *Container, executor *Executor, globalProperties map[string]any, newValueStore func() ValueStore, withBody bool) gin.HandlerFunc {
	propagator := otel.GetTextMapPropagator()

	return func(c *gin.Context) {
		e := NewExecution(flow, container, globalProperties, newValueStore())
		start := time.Now()
		var requestErr error
		var flowErr error
		reqCtx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		spanCtx, span := container.Tracer().Start(reqCtx, fmt.Sprintf("flow %s", flow.ID),
			trace.WithAttributes(
				attribute.String("flow.id", flow.ID),
				attribute.String("execution.id", e.ID),
				attribute.String("http.method", c.Request.Method),
				attribute.String("http.path", c.Request.URL.Path),
			),
		)
		defer func() {
			statusCode := c.Writer.Status()
			if statusCode > 0 {
				span.SetAttributes(attribute.Int("http.status_code", statusCode))
			}
			if requestErr == nil && statusCode >= http.StatusInternalServerError {
				requestErr = fmt.Errorf("request completed with status %d", statusCode)
				span.RecordError(requestErr)
				span.SetStatus(codes.Error, requestErr.Error())
			}
			span.End()
		}()

		// Apply flow-level timeout if configured.
		// The execution embeds the context so all downstream code (Risor, retry
		// sleeps, slog) automatically respects the deadline via e.Done().
		if flow.Timeout > 0 {
			ctx, cancel := context.WithTimeout(spanCtx, time.Duration(flow.Timeout)*time.Millisecond)
			defer cancel()
			e = *e.WithContext(ctx)
		} else {
			e = *e.WithContext(spanCtx)
		}
		log := e.Logger()

		defer func() {
			duration := time.Since(start)
			container.Metrics().RecordFlow(spanCtx, flow.ID, classifyMetricOutcome(flowErr), duration)
			container.Metrics().RecordHTTPRequest(
				spanCtx,
				flow.ID,
				c.Request.Method,
				route,
				classifyHTTPStatus(c.Writer.Status()),
				duration,
			)
		}()

		defer func() {
			log.Info("HTTP request completed",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"status_code", c.Writer.Status(),
				"duration_ms", time.Since(start).Milliseconds())
		}()

		extractRequestData(c, flow, &e, withBody)

		flowErr = executor.ExecuteSteps(&e)
		if flowErr != nil {
			requestErr = flowErr
			span.RecordError(flowErr)
			span.SetStatus(codes.Error, flowErr.Error())
			log.Error("Flow execution failed",
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"error", flowErr)
			// on_error handler may have set a response descriptor despite execution failure.
			if e.ResponseDescriptor != nil {
				if err := dispatchResponse(c, &e); err != nil {
					requestErr = err
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
				}
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "Error in task execution: " + flowErr.Error(),
			})
			return
		}

		if err := dispatchResponse(c, &e); err != nil {
			requestErr = err
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}
}

// dispatchResponse handles the HTTP response dispatch based on the execution's ResponseDescriptor.
// If no descriptor was set by any step, returns a default 200 OK.
func dispatchResponse(c *gin.Context, execution *Execution) error {
	log := execution.Logger()
	if execution.ResponseDescriptor == nil {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return nil
	}

	handler, ok := execution.Container.ResponseHandlers.Get(execution.ResponseDescriptor.HandlerName)
	if !ok {
		err := fmt.Errorf("unknown response handler: %s", execution.ResponseDescriptor.HandlerName)
		log.Error("Unknown response handler",
			"handler", execution.ResponseDescriptor.HandlerName,
			"error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return err
	}

	if err := handler.Handle(c, execution, execution.ResponseDescriptor.Args); err != nil {
		log.Error("Response handler execution failed",
			"handler", execution.ResponseDescriptor.HandlerName,
			"error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Error generating response: " + err.Error(),
		})
		return err
	}

	return nil
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

	e.AddValue(RequestRawBodyKey, string(body))

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		c.JSON(http.StatusBadRequest, wrongBodyFormatRes)
		return
	}

	e.Store.SetNested(RequestBodyPrefix, parsed)
}
