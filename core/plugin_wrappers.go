package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/BDNK1/sflowg/core/internal/pluginexec"
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
	return result, err
}

func execFlowID(exec *Execution) string {
	if exec == nil || exec.Flow == nil {
		return ""
	}
	return exec.Flow.ID
}
