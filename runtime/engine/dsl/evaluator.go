package dsl

import (
	"github.com/BDNK1/sflowg/runtime"
	risor "github.com/deepnoodle-ai/risor/v2"
)

// ExpressionEvaluator implements runtime.ExpressionEvaluator using Risor.
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
