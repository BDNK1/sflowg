package dsl

import (
	"context"

	"github.com/BDNK1/sflowg/runtime"
)

// LocalStepRunner executes one step in-process against isolated per-step state.
type LocalStepRunner struct {
	executor runtime.StepExecutor
}

func NewLocalStepRunner(executor runtime.StepExecutor) *LocalStepRunner {
	return &LocalStepRunner{executor: executor}
}

func (r *LocalStepRunner) RunStep(ctx context.Context, execution *runtime.Execution, input runtime.StepInput) (runtime.StepOutput, error) {
	store := runtime.NewValueStore()
	for k, v := range input.Input {
		store.SetNested(k, v)
	}

	isolatedState := runtime.NewRunState(store)
	isolatedExec := execution.WithIsolatedState(isolatedState)
	step := runtime.Step{
		ID:      input.StepID,
		Body:    input.Body,
		Timeout: input.Timeout,
	}

	next, err := r.executor.ExecuteStep(ctx, isolatedExec, step)
	if err != nil {
		return runtime.StepOutput{}, err
	}

	result, _ := isolatedState.Store().Get(input.StepID)
	return runtime.StepOutput{
		Result:   result,
		Response: isolatedState.Response(),
		Next:     next,
	}, nil
}
