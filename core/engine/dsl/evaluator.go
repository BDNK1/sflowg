package dsl

import (
	"fmt"

	"github.com/BDNK1/sflowg/core"
	risor "github.com/deepnoodle-ai/risor/v2"
	"github.com/deepnoodle-ai/risor/v2/pkg/bytecode"
)

// ExpressionEvaluator implements core.ExpressionEvaluator using Risor.
// The *Execution carries both the variable namespace (Values()) and the
// cancellation context, so a single parameter covers both concerns.
// Calls Risor directly rather than going through Interpreter, since expression
// evaluation doesn't need the enriched globals (plugins, raise(), etc.) that
// step body execution requires.
type ExpressionEvaluator struct{}

func NewExpressionEvaluator() *ExpressionEvaluator {
	return &ExpressionEvaluator{}
}

func (e *ExpressionEvaluator) Eval(execution *runtime.Execution, expression string) (result any, err error) {
	return e.EvalExpression(execution, expression, nil, nil, nil)
}

func (e *ExpressionEvaluator) EvalWithEnv(execution *runtime.Execution, expression string, extraVars map[string]any) (result any, err error) {
	return e.EvalExpression(execution, expression, nil, nil, extraVars)
}

func (e *ExpressionEvaluator) EvalExpression(execution *runtime.Execution, expr string, program runtime.ExpressionProgram, storeKeys []string, extra map[string]any) (result any, err error) {
	defer recoverFlowError(&err)
	merged := execution.Values()
	if storeKeys != nil {
		merged = execution.State().Store().SnapshotKeys(storeKeys)
	}
	for k, v := range extra {
		merged[k] = v
	}
	applyAsyncFutures(merged, execution, execution.EvalConsumer())
	if execution.DSLMode() == runtime.DSLExecutionModeCompiled && program != nil {
		code, ok := program.(*bytecode.Code)
		if !ok {
			return nil, fmt.Errorf("compiled mode invariant violated: expression is missing compiled bytecode")
		}
		preSeedMissingKeys(merged, code)
		result, err = (&Interpreter{}).Run(execution, code, merged)
	} else {
		result, err = risor.Eval(execution, expr, risor.WithEnv(convertGlobals(merged)))
	}
	if err != nil {
		return nil, err
	}
	if fe := asyncFutureError(merged); fe != nil {
		return nil, fe
	}
	return result, nil
}
