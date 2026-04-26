package runtime_test

import (
	"testing"

	"github.com/BDNK1/sflowg/core"
	dslengine "github.com/BDNK1/sflowg/core/engine/dsl"
	"github.com/BDNK1/sflowg/core/validation/schema"
)

func TestFlowCallHappyPath(t *testing.T) {
	app, caller := newFlowCallTestApp()

	exec := runtime.NewExecution(caller, app.Container, nil, runtime.NewValueStore())
	if err := app.Executor().ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}

	got, ok := exec.State().Store().Get("charge.y")
	if !ok || got != int64(2) {
		t.Fatalf("charge.y = %#v, ok=%v", got, ok)
	}
}

func TestFlowCallErrorResponseFallsBackWithErrorScope(t *testing.T) {
	app, caller := newFlowCallTestApp()
	sub := app.Flows["sub"]
	sub.Steps = []runtime.Step{{ID: "__return", Body: `response.error({code: "SUB_FAILED", message: "nope"})`}}
	app.Flows["sub"] = sub
	caller.Steps[0].FallbackBody = `{handled: error.code}`

	exec := runtime.NewExecution(caller, app.Container, nil, runtime.NewValueStore())
	if err := app.Executor().ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}

	got, ok := exec.State().Store().Get("charge.handled")
	if !ok || got != "SUB_FAILED" {
		t.Fatalf("charge.handled = %#v, ok=%v", got, ok)
	}
}

func newFlowCallTestApp() (*runtime.App, *runtime.Flow) {
	contract := runtime.NewInputContract()
	contract.SetFields("input", map[string]*schema.Schema{
		"x": {Type: schema.TypeInteger, Required: true},
	})

	caller := &runtime.Flow{
		ID:         "caller",
		Entrypoint: runtime.Entrypoint{Type: "http"},
		Steps: []runtime.Step{{
			ID:   "charge",
			Body: `flow.call("sub", {x: 1})`,
		}},
	}
	sub := runtime.Flow{
		ID:               "sub",
		Entrypoint:       runtime.Entrypoint{Type: "flow", Input: contract},
		ResponseSubtypes: []string{"value", "error"},
		Steps: []runtime.Step{{
			ID:   "__return",
			Body: `response.value({y: input.x + 1})`,
		}},
	}

	container := runtime.NewContainer(runtime.NewLogger(nil))
	stepExecutor := dslengine.NewStepExecutor()
	stepRunner := dslengine.NewLocalStepRunner(stepExecutor)
	newValueStore := func() runtime.ValueStore { return runtime.NewValueStore() }
	app := runtime.NewApp(container, dslengine.NewFlowLoader(), dslengine.NewExpressionEvaluator(), stepExecutor, stepRunner, nil, newValueStore)
	stepExecutor.SetSubflowInvoker(app)
	app.Flows["sub"] = sub
	return app, caller
}
