package dsl

import (
	"context"

	"github.com/BDNK1/sflowg/core"
)

// LocalStepRunner executes one step in-process against isolated per-step state.
type LocalStepRunner struct {
	executor core.StepExecutor
}

func NewLocalStepRunner(executor core.StepExecutor) *LocalStepRunner {
	return &LocalStepRunner{executor: executor}
}

func (r *LocalStepRunner) RunStep(ctx context.Context, execution *core.Execution, input core.StepInput) (core.StepOutput, error) {
	store := core.NewValueStore()
	for k, v := range input.Input {
		store.SetNested(k, v)
	}
	for k, v := range input.ExtraEnv {
		store.SetNested(k, v)
	}

	isolatedState := core.NewRunState(store)
	isolatedExec := execution.WithIsolatedState(isolatedState)
	step := core.Step{
		ID:       input.StepID,
		Body:     input.Body,
		Timeout:  input.Timeout,
		Compiled: input.Compiled,
	}

	next, err := r.executor.ExecuteStep(ctx, isolatedExec, step)
	if err != nil {
		return core.StepOutput{}, err
	}

	result, _ := isolatedState.Store().Get(input.StepID)
	return core.StepOutput{
		Result:   result,
		Response: isolatedState.Response(),
		Next:     next,
	}, nil
}
