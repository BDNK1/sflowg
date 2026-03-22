package runtime

import (
	"context"
	"fmt"
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
	exec.Metrics().RecordPluginCall(
		spanCtx,
		execFlowID(exec),
		exec.activeStepID,
		w.binding.PluginName,
		w.binding.MethodName,
		classifyMetricOutcome(err),
		time.Since(start),
	)
	return result, err
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
