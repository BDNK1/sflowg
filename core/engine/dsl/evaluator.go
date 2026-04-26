package dsl

import (
	"github.com/BDNK1/sflowg/core"
	risor "github.com/deepnoodle-ai/risor/v2"
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

func (e *ExpressionEvaluator) Eval(execution *runtime.Execution, expression string) (any, error) {
	return risor.Eval(execution, expression, risor.WithEnv(convertGlobals(execution.Values())))
}

func (e *ExpressionEvaluator) EvalWithEnv(execution *runtime.Execution, expression string, extraVars map[string]any) (any, error) {
	merged := execution.Values()
	for k, v := range extraVars {
		merged[k] = v
	}
	return risor.Eval(execution, expression, risor.WithEnv(convertGlobals(merged)))
}
