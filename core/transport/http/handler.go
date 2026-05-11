package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/httpinput"
	validationschema "github.com/BDNK1/sflowg/core/validation/schema"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func registerFlowRoute(flow *core.Flow, container *core.Container, executor *core.Executor, globalProperties map[string]any, newValueStore func() core.ValueStore, g *gin.Engine) {
	config := flow.Entrypoint.Config
	methodRaw, _ := config["method"].(string)
	path, _ := config["path"].(string)
	if methodRaw == "" || path == "" {
		container.Logger().Error("HTTP entrypoint missing method or path", "flow_id", flow.ID)
		return
	}
	method := strings.ToLower(methodRaw)

	container.Logger().Info("Registering HTTP entrypoint", "method", method, "path", path, "flow_id", flow.ID)

	switch method {
	case "get":
		g.GET(path, handleRequest(flow, path, container, executor, globalProperties, newValueStore, false))
	case "post":
		g.POST(path, handleRequest(flow, path, container, executor, globalProperties, newValueStore, true))
	default:
		container.Logger().Error("Unsupported HTTP method", "method", method, "flow_id", flow.ID)
	}
}

type requestScope struct {
	execution *core.Execution
	flow      *core.Flow
	route     string
	start     time.Time
	spanCtx   context.Context
	span      trace.Span
	cancel    context.CancelFunc
	log       core.Logger
}

func beginRequestScope(c *gin.Context, flow *core.Flow, route string, execution *core.Execution) *requestScope {
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

func handleRequest(flow *core.Flow, route string, container *core.Container, executor *core.Executor, globalProperties map[string]any, newValueStore func() core.ValueStore, withBody bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		e := core.NewExecution(flow, container, globalProperties, newValueStore())
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

		if boundaryErr := extractRequestData(c, flow, e, withBody); boundaryErr != nil {
			requestErr = boundaryErr
			scope.span.RecordError(boundaryErr)
			scope.span.SetStatus(codes.Error, boundaryErr.Error())
			log.Error("HTTP input validation failed",
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"error", boundaryErr)

			_, handlerErr := executor.HandleBoundaryError(e, boundaryErr)
			if handlerErr != nil {
				requestErr = handlerErr
				scope.span.RecordError(handlerErr)
				scope.span.SetStatus(codes.Error, handlerErr.Error())
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": "Error in on_error handler: " + handlerErr.Error(),
				})
				return
			}
			if e.State().Response() != nil {
				if err := dispatchResponse(c, e); err != nil {
					requestErr = err
					scope.span.RecordError(err)
					scope.span.SetStatus(codes.Error, err.Error())
				}
				return
			}
			writeSchemaViolationProblem(c, boundaryErr)
			return
		}

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
func dispatchResponse(c *gin.Context, execution *core.Execution) error {
	log := execution.Logger()
	rd := execution.State().Response()
	if rd == nil {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
		return nil
	}

	subtype := rd.Subtype
	if subtype == "" {
		subtype = strings.TrimPrefix(rd.HandlerName, "http.")
	}
	if err := writeResponse(c, execution, subtype, rd.Args); err != nil {
		log.Error("HTTP response dispatch failed",
			"subtype", subtype,
			"error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return err
	}

	return nil
}

func writeResponse(c *gin.Context, execution *core.Execution, subtype string, args map[string]any) error {
	switch subtype {
	case "json":
		writeJSON(c, args)
		return nil
	case "text":
		writeText(c, args)
		return nil
	case "redirect":
		return writeRedirect(c, execution, args)
	default:
		return fmt.Errorf("unsupported HTTP response subtype %q", subtype)
	}
}

func writeJSON(c *gin.Context, args map[string]any) {
	statusCode := http.StatusOK
	if status, ok := toStatusCode(args["status"]); ok {
		statusCode = status
	}
	writeHeaders(c, args)
	body := args["body"]
	if body == nil {
		body = gin.H{}
	}
	c.JSON(statusCode, body)
}

func writeText(c *gin.Context, args map[string]any) {
	statusCode := http.StatusOK
	if status, ok := toStatusCode(args["status"]); ok {
		statusCode = status
	}
	writeHeaders(c, args)
	body := ""
	if raw, ok := args["body"]; ok && raw != nil {
		body = fmt.Sprintf("%v", raw)
	}
	c.Data(statusCode, "text/plain; charset=utf-8", []byte(body))
}

func writeRedirect(c *gin.Context, execution *core.Execution, args map[string]any) error {
	location, ok := args["location"].(string)
	if !ok || location == "" {
		execution.Logger().Error("Redirect response requires a location")
		return fmt.Errorf("redirect response requires a 'location' argument")
	}

	if err := validateRedirectLocation(location); err != nil {
		execution.Logger().Error("Rejected redirect location", "location", location, "error", err)
		return err
	}

	statusCode := http.StatusFound
	if status, ok := toStatusCode(args["status"]); ok {
		if status < 300 || status >= 400 {
			execution.Logger().Error("Invalid redirect status code", "status", status)
			return fmt.Errorf("redirect status must be 3xx, got %d", status)
		}
		statusCode = status
	}

	c.Redirect(statusCode, location)
	return nil
}

func validateRedirectLocation(location string) error {
	if strings.HasPrefix(location, "//") || strings.HasPrefix(location, "\\\\") {
		return fmt.Errorf("redirect location must not be protocol-relative")
	}
	if strings.HasPrefix(location, "/") {
		return nil
	}
	u, err := url.Parse(location)
	if err != nil {
		return fmt.Errorf("redirect location is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("redirect location must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("redirect location must include a host")
	}
	return nil
}

func writeHeaders(c *gin.Context, args map[string]any) {
	if headers, ok := args["headers"].(map[string]any); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				c.Header(key, strValue)
			}
		}
	}
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

func extractRequestData(c *gin.Context, f *core.Flow, e *core.Execution, withBody bool) *core.FlowError {
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
		if err := extractBody(c, f, e); err != nil {
			return err
		}
	}

	return validateRequestData(c, f, e)
}

func extractValues(e *core.Execution, keys []any, prefix string, getValue func(string) string) {
	for _, key := range keys {
		if v, ok := key.(string); ok {
			e.AddValue(fmt.Sprintf("%s.%s", prefix, v), getValue(v))
		}
	}
}

func extractBody(c *gin.Context, f *core.Flow, e *core.Execution) *core.FlowError {
	bodyConfig, ok := f.Entrypoint.Config["body"].(map[string]any)
	if !ok {
		return nil
	}
	bodyType, ok := bodyConfig["type"].(string)
	if !ok {
		return nil
	}

	if bodyType == "json" {
		return extractJsonBody(c, e)
	}
	return schemaViolation([]validationschema.FieldError{{
		Path:       "body",
		Pointer:    "#/body",
		Constraint: "type",
		Got:        bodyType,
		Message:    "body type is not supported",
	}})
}

var wrongBodyFormatRes = gin.H{"message": "Wrong request body format"}

const maxRequestBodyBytes = 1 << 20

func extractJsonBody(c *gin.Context, e *core.Execution) *core.FlowError {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		constraint := "read"
		if _, ok := err.(*http.MaxBytesError); ok {
			constraint = "size"
		}
		return schemaViolation([]validationschema.FieldError{{
			Path:       "body",
			Pointer:    "#/body",
			Constraint: constraint,
			Message:    wrongBodyFormatRes["message"].(string),
		}})
	}

	e.AddValue(RequestRawBodyKey, string(body))

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return schemaViolation([]validationschema.FieldError{{
			Path:       "body",
			Pointer:    "#/body",
			Constraint: "json",
			Got:        string(body),
			Message:    wrongBodyFormatRes["message"].(string),
		}})
	}

	e.State().Store().SetNested(RequestBodyPrefix, parsed)
	return nil
}

func validateRequestData(c *gin.Context, f *core.Flow, e *core.Execution) *core.FlowError {
	input := f.Entrypoint.Input
	if input == nil {
		return nil
	}

	var fields []validationschema.FieldError

	if bodySchema, ok := input.RootSchema(httpinput.BodyNamespace); ok {
		raw, present := e.State().Store().Get(RequestBodyPrefix)
		normalized, errs := validationschema.ValidateField(raw, present, bodySchema, "body")
		fields = append(fields, errs...)
		if len(errs) == 0 && normalized != nil {
			e.State().Store().SetNested(RequestBodyPrefix, normalized)
		}
	}

	fields = append(fields, validateNamedInputs(input.FieldSchemas(httpinput.PathVariablesNamespace), PathVariablesPrefix, "pathVariables", func(name string, s *validationschema.Schema) (any, bool) {
		value := c.Param(name)
		return value, value != ""
	}, e)...)

	fields = append(fields, validateNamedInputs(input.FieldSchemas(httpinput.QueryParametersNamespace), QueryParametersPrefix, "queryParameters", func(name string, s *validationschema.Schema) (any, bool) {
		values, ok := c.GetQueryArray(name)
		if !ok {
			return nil, false
		}
		if s.Type == validationschema.TypeArray {
			return values, true
		}
		if len(values) == 0 {
			return "", true
		}
		return values[0], true
	}, e)...)

	fields = append(fields, validateNamedInputs(input.FieldSchemas(httpinput.HeadersNamespace), HeadersPrefix, "headers", func(name string, s *validationschema.Schema) (any, bool) {
		values := c.Request.Header.Values(name)
		if len(values) == 0 {
			return nil, false
		}
		if s.Type == validationschema.TypeArray {
			return values, true
		}
		return values[0], true
	}, e)...)

	if len(fields) > 0 {
		return schemaViolation(fields)
	}
	return nil
}

func validateNamedInputs(schemas map[string]*validationschema.Schema, prefix string, root string, source func(string, *validationschema.Schema) (any, bool), e *core.Execution) []validationschema.FieldError {
	var fields []validationschema.FieldError
	for name, inputSchema := range schemas {
		raw, present := source(name, inputSchema)
		normalized, errs := validationschema.ValidateField(raw, present, inputSchema, root+"."+name)
		fields = append(fields, errs...)
		if len(errs) == 0 && (present || inputSchema.Default != nil) {
			e.State().Store().SetNested(prefix+"."+name, normalized)
		}
	}
	return fields
}

func schemaViolation(fields []validationschema.FieldError) *core.FlowError {
	return &core.FlowError{
		Type:    core.ErrorTypePermanent,
		Code:    string(core.ErrorCodeSchemaViolation),
		Message: "Request validation failed",
		Meta: map[string]any{
			"fields": validationschema.FieldsToMaps(fields),
		},
	}
}

func classifyMetricOutcome(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}

	var flowErr *core.FlowError
	if errors.As(err, &flowErr) {
		if flowErr.Type == core.ErrorTypeTimeout ||
			flowErr.Code == string(core.ErrorCodeDeadlineExceeded) ||
			flowErr.Code == string(core.ErrorCodeContextCancelled) {
			return "timeout"
		}
	}

	return "error"
}

func classifyHTTPStatus(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500 && statusCode < 600:
		return "5xx"
	default:
		return "unknown"
	}
}

func writeSchemaViolationProblem(c *gin.Context, fe *core.FlowError) {
	errors, _ := fe.Meta["fields"].([]map[string]any)
	c.Header("Content-Type", "application/problem+json")
	c.JSON(http.StatusBadRequest, gin.H{
		"title":  "Request validation failed",
		"status": http.StatusBadRequest,
		"detail": "One or more inputs did not match the flow contract.",
		"errors": errors,
	})
}
