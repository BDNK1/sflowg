package yaml

import (
	"encoding/base64"
	"fmt"

	"github.com/expr-lang/expr"
)

// Custom expression functions available in all flows
var exprFunctions = []expr.Option{
	expr.Function("base64_encode", func(params ...any) (any, error) {
		s, _ := params[0].(string)
		return base64.StdEncoding.EncodeToString([]byte(s)), nil
	}),
	expr.Function("base64_decode", func(params ...any) (any, error) {
		s, _ := params[0].(string)
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}),
}

// ExpressionEvaluator evaluates expressions using the expr-lang library.
// It formats keys and expressions using the flat underscore convention.
type ExpressionEvaluator struct{}

func NewExpressionEvaluator() *ExpressionEvaluator {
	return &ExpressionEvaluator{}
}

func (e *ExpressionEvaluator) Eval(expression string, context map[string]any) (any, error) {
	// Add null as alias for nil (JSON/YAML compatibility)
	context["null"] = nil

	// defined() checks if a path exists in context (distinguishes missing from null)
	// Usage: defined("step.result.field") returns true if key exists, even if value is null
	definedFn := expr.Function(
		"defined",
		func(params ...any) (any, error) {
			path, ok := params[0].(string)
			if !ok {
				return false, fmt.Errorf("defined() expects string path argument, got %T", params[0])
			}
			key := FormatKey(path)
			_, exists := context[key]
			return exists, nil
		},
		new(func(string) bool),
	)

	// NOTE: expr.Env MUST come before AllowUndefinedVariables for it to work
	opts := []expr.Option{
		expr.Env(context),
		expr.AllowUndefinedVariables(), // Missing variables return nil instead of compile error
		definedFn,
	}
	opts = append(opts, exprFunctions...)

	program, err := expr.Compile(FormatExpression(expression), opts...)
	if err != nil {
		return nil, err
	}
	return expr.Run(program, context)
}
