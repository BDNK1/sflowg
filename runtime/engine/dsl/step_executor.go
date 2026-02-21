package dsl

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/BDNK1/sflowg/runtime"
)

// StepExecutor executes DSL step bodies via the Risor interpreter.
// Unlike the YAML StepExecutor which dispatches by step type (assign/switch/task),
// the DSL executor always runs the step's Body as Risor code.
type StepExecutor struct {
	interpreter *Interpreter
	l           *slog.Logger
}

func NewStepExecutor(l *slog.Logger) *StepExecutor {
	return &StepExecutor{
		interpreter: &Interpreter{},
		l:           l,
	}
}

func (e *StepExecutor) ExecuteStep(execution *runtime.Execution, step runtime.Step) (string, error) {
	if step.Body == "" {
		return "", nil
	}

	globals := e.buildGlobals(execution)

	e.l.InfoContext(execution, fmt.Sprintf("Executing DSL step: %s", step.ID))

	result, err := e.interpreter.Eval(context.Background(), step.Body, globals)
	if err != nil {
		return "", fmt.Errorf("executing step %s: %w", step.ID, err)
	}

	// Store the step result under the step ID, but skip if a response
	// descriptor was set (the step produced a response, not a stored value)
	if result != nil && execution.ResponseDescriptor == nil {
		if m, ok := result.(map[string]any); ok {
			execution.Store.SetNested(step.ID, m)
		} else {
			execution.Store.Set(step.ID, result)
		}
	}

	return "", nil
}

// buildGlobals assembles the globals map for a step's Risor evaluation.
// Includes: all current store values + plugin modules + promoted properties.
func (e *StepExecutor) buildGlobals(execution *runtime.Execution) map[string]any {
	globals := make(map[string]any)

	// Copy all current store values as globals
	for k, v := range execution.Store.All() {
		globals[k] = v
	}

	// Add plugin task functions grouped by plugin name
	pluginGlobals := BuildPluginGlobals(execution)
	for k, v := range pluginGlobals {
		globals[k] = v
	}

	// Promote properties: make properties.X also available as bare X
	// (unless there's already a top-level name conflict)
	if props, ok := globals["properties"]; ok {
		if propsMap, ok := props.(map[string]any); ok {
			for k, v := range propsMap {
				if _, exists := globals[k]; !exists {
					globals[k] = v
				}
			}
		}
	}

	// Add response module (sets ResponseDescriptor on execution)
	responseGlobals := BuildResponseGlobals(execution)
	for k, v := range responseGlobals {
		globals[k] = v
	}

	// Add string formatting utility
	globals["sprintf"] = fmt.Sprintf

	return globals
}
