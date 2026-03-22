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

type requestScope struct {
	execution *Execution
	flow      *Flow
	route     string
	start     time.Time
	spanCtx   context.Context
	span      trace.Span
	cancel    context.CancelFunc
	log       Logger
}

func beginRequestScope(c *gin.Context, flow *Flow, route string, execution *Execution) *requestScope {
	propagator := otel.GetTextMapPropagator()
	reqCtx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
	spanCtx, span := execution.Tracer().Start(reqCtx, fmt.Sprintf("flow %s", flow.ID),
		trace.WithAttributes(
			attribute.String("flow.id", flow.ID),
			attribute.String("execution.id", execution.ID),
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.path", c.Request.URL.Path),
		),
	)

	cancel := func() {}
	if flow.Timeout > 0 {
		ctx, timeoutCancel := context.WithTimeout(spanCtx, time.Duration(flow.Timeout)*time.Millisecond)
		execution = execution.WithContext(ctx)
		cancel = timeoutCancel
	} else {
		execution = execution.WithContext(spanCtx)
	}

	return &requestScope{
		execution: execution,
		flow:      flow,
		route:     route,
		start:     time.Now(),
		spanCtx:   spanCtx,
		span:      span,
		cancel:    cancel,
		log:       execution.Logger(),
	}
}

func (s *requestScope) finish(c *gin.Context, requestErr error) {
	defer s.cancel()

	duration := time.Since(s.start)
	statusCode := c.Writer.Status()
	finalErr := requestErr
	if finalErr == nil && statusCode >= http.StatusInternalServerError {
		finalErr = fmt.Errorf("request completed with status %d", statusCode)
	}

	s.log.Info("HTTP request completed",
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"status_code", statusCode,
		"duration_ms", duration.Milliseconds())

	s.execution.Metrics().RecordFlow(s.spanCtx, s.flow.ID, classifyMetricOutcome(finalErr), duration)
	s.execution.Metrics().RecordHTTPRequest(
		s.spanCtx,
		s.flow.ID,
		c.Request.Method,
		s.route,
		classifyHTTPStatus(statusCode),
		duration,
	)

	if statusCode > 0 {
		s.span.SetAttributes(attribute.Int("http.status_code", statusCode))
	}
	if requestErr == nil && finalErr != nil {
		s.span.RecordError(finalErr)
		s.span.SetStatus(codes.Error, finalErr.Error())
	}
	s.span.End()
}

func handleRequest(flow *Flow, route string, container *Container, executor *Executor, globalProperties map[string]any, newValueStore func() ValueStore, withBody bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		e := NewExecution(flow, container, globalProperties, newValueStore())
		var requestErr error
		var flowErr error
		scope := beginRequestScope(c, flow, route, e)
		// beginRequestScope derives a new *Execution with the span/timeout context.
		// Use scope.execution so ExecuteSteps and response dispatch run with the right ctx.
		e = scope.execution
		defer func() {
			scope.finish(c, requestErr)
		}()
		log := scope.log

		extractRequestData(c, flow, e, withBody)

		flowErr = executor.ExecuteSteps(e)
		if flowErr != nil {
			requestErr = flowErr
			scope.span.RecordError(flowErr)
			scope.span.SetStatus(codes.Error, flowErr.Error())
			log.Error("Flow execution failed",
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"error", flowErr)
			// on_error handler may have set a response descriptor despite execution failure.
			if e.State().Response() != nil {
				if err := dispatchResponse(c, e); err != nil {
					requestErr = err
					scope.span.RecordError(err)
					scope.span.SetStatus(codes.Error, err.Error())
				}
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "Error in task execution: " + flowErr.Error(),
			})
			return
		}

		if err := dispatchResponse(c, e); err != nil {
			requestErr = err
			scope.span.RecordError(err)
			scope.span.SetStatus(codes.Error, err.Error())
		}
	}
}

// dispatchResponse handles the HTTP response dispatch based on the execution's RunState response.
// If no descriptor was set by any step, returns a default 200 OK.
func dispatchResponse(c *gin.Context, execution *Execution) error {
	log := execution.Logger()
	rd := execution.State().Response()
	if rd == nil {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return nil
	}

	handler, ok := execution.Container.ResponseHandlers.Get(rd.HandlerName)
	if !ok {
		err := fmt.Errorf("unknown response handler: %s", rd.HandlerName)
		log.Error("Unknown response handler",
			"handler", rd.HandlerName,
			"error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return err
	}

	if err := handler.Handle(c, execution, rd.Args); err != nil {
		log.Error("Response handler execution failed",
			"handler", rd.HandlerName,
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

	e.State().Store().SetNested(RequestBodyPrefix, parsed)
}
