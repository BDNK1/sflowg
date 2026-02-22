package dsl

import "context"

// ExpressionEvaluator implements runtime.ExpressionEvaluator using Risor.
// Used by the Executor for step conditions and retry conditions.
// Unlike the YAML evaluator which uses expr-lang with flat keys,
// this evaluator passes nested maps directly to Risor for native dot access.
type ExpressionEvaluator struct {
	interpreter *Interpreter
}

func NewExpressionEvaluator() *ExpressionEvaluator {
	return &ExpressionEvaluator{
		interpreter: &Interpreter{},
	}
}

func (e *ExpressionEvaluator) Eval(expression string, ctx map[string]any) (any, error) {
	return e.interpreter.Eval(context.Background(), expression, ctx)
}
