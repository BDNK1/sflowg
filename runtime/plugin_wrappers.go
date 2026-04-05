package runtime

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/BDNK1/sflowg/runtime/internal/pluginexec"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func newTaskExecutor(binding pluginexec.TaskBinding) Task {
	return &pluginTaskWrapper{
		binding:  binding,
		spanName: fmt.Sprintf("plugin %s.%s", binding.PluginName, binding.MethodName),
	}
}

func newResponseHandler(binding pluginexec.ResponseBinding) ResponseHandler {
	return &pluginResponseHandlerWrapper{binding: binding}
}

type pluginTaskWrapper struct {
	binding  pluginexec.TaskBinding
	spanName string
}

func (w *pluginTaskWrapper) Execute(exec *Execution, args map[string]any) (map[string]any, error) {
	parentCtx := exec.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	spanCtx, span := exec.Tracer().Start(parentCtx, w.spanName,
		trace.WithAttributes(
			attribute.String("plugin.name", w.binding.PluginName),
			attribute.String("plugin.method", w.binding.MethodName),
		),
	)
	defer span.End()
	start := time.Now()

	var result map[string]any
	var err error
	pluginExec := exec.WithContext(spanCtx).WithActivePlugin(w.binding.PluginName)
	result, err = pluginexec.CallTask(w.binding, pluginExec, args)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	duration := time.Since(start)
	exec.Metrics().RecordPluginCall(
		spanCtx,
		execFlowID(exec),
		exec.activeStepID,
		w.binding.PluginName,
		w.binding.MethodName,
		classifyMetricOutcome(err),
		duration,
	)
	if state := exec.State(); state != nil {
		state.RecordSideEffect(SideEffect{
			Plugin:    w.binding.PluginName,
			Method:    w.binding.MethodName,
			Input:     summarizeSideEffectInput(args),
			Output:    summarizeSideEffectOutput(result),
			Error:     summarizeSideEffectError(err),
			Duration:  duration,
			Timestamp: start,
		})
	}
	return result, err
}

func summarizeSideEffectInput(args map[string]any) any {
	return summarizeSideEffectMap(args)
}

func summarizeSideEffectOutput(result map[string]any) any {
	return summarizeSideEffectMap(result)
}

func summarizeSideEffectError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func summarizeSideEffectMap(values map[string]any) any {
	if values == nil {
		return nil
	}

	summary := make(map[string]any, len(values))
	for key, value := range values {
		summary[key] = summarizeSideEffectValue(value)
	}
	return summary
}

func summarizeSideEffectValue(value any) any {
	if isSideEffectScalar(value) {
		return value
	}

	switch v := value.(type) {
	case map[string]any:
		return map[string]any{
			"type": "object",
			"keys": sortedStringKeys(v),
			"size": len(v),
		}
	case []any:
		return map[string]any{
			"type": "array",
			"size": len(v),
		}
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil
	}

	switch rv.Kind() {
	case reflect.Map:
		return map[string]any{
			"type": "object",
			"size": rv.Len(),
		}
	case reflect.Array, reflect.Slice:
		return map[string]any{
			"type": "array",
			"size": rv.Len(),
		}
	default:
		return fmt.Sprintf("%T", value)
	}
}

func isSideEffectScalar(value any) bool {
	switch value.(type) {
	case nil,
		bool,
		string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		time.Time,
		time.Duration:
		return true
	default:
		return false
	}
}

func sortedStringKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func execFlowID(exec *Execution) string {
	if exec == nil || exec.Flow == nil {
		return ""
	}
	return exec.Flow.ID
}

type pluginResponseHandlerWrapper struct {
	binding pluginexec.ResponseBinding
}

func (w *pluginResponseHandlerWrapper) Handle(c *gin.Context, exec *Execution, args map[string]any) error {
	return pluginexec.CallResponseHandler(w.binding, c, exec, args)
}
