package dsl

import (
	"context"
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/flowinput"
	"github.com/BDNK1/sflowg/core/validation/schema"
	"github.com/deepnoodle-ai/risor/v2/pkg/ast"
	risorparser "github.com/deepnoodle-ai/risor/v2/pkg/parser"
)

type LiteralKind string

const (
	LiteralString  LiteralKind = "string"
	LiteralInteger LiteralKind = "integer"
	LiteralNumber  LiteralKind = "number"
	LiteralBoolean LiteralKind = "boolean"
	LiteralNil     LiteralKind = "nil"
)

type LiteralValue struct {
	Kind  LiteralKind
	Value any
}

type FlowCallRef struct {
	TargetName string
	ArgsStatic bool
	ArgKeys    map[string]struct{}
	ArgValues  map[string]LiteralValue
}

type FlowValidator struct{}

func NewFlowValidator() *FlowValidator { return &FlowValidator{} }

func (v *FlowValidator) ValidateFlows(flows map[string]runtime.Flow) error {
	edges := make(map[string][]string)
	for flowID, flow := range flows {
		if err := validateCoreSemantics(flow); err != nil {
			return fmt.Errorf("flow %q: %w", flowID, err)
		}
		refs, err := extractFlowCallsFromFlow(flow)
		if err != nil {
			return fmt.Errorf("flow %q: %w", flowID, err)
		}
		for _, ref := range refs {
			if ref.TargetName == "" {
				continue
			}
			target, ok := flows[ref.TargetName]
			if !ok {
				return fmt.Errorf("flow %q calls undefined subflow %q", flowID, ref.TargetName)
			}
			if target.Entrypoint.Type != "flow" {
				return fmt.Errorf("flow.call target %q must be entrypoint.flow, got %q", ref.TargetName, target.Entrypoint.Type)
			}
			if ref.ArgsStatic {
				if err := validateLiteralArgs(flowID, ref.TargetName, ref, target.Entrypoint.Input.FieldSchemas(flowinput.InputNamespace)); err != nil {
					return err
				}
			}
			edges[flowID] = append(edges[flowID], ref.TargetName)
		}
	}
	if cycle := findFlowCallCycle(edges); len(cycle) > 0 {
		return fmt.Errorf("flow.call cycle detected: %s", strings.Join(cycle, " -> "))
	}
	return nil
}

func validateCoreSemantics(flow runtime.Flow) error {
	contract := flow.ResponseContract
	if contract.IsZero() {
		return nil
	}
	if err := validateResponseCallsInFlow(flow, contract); err != nil {
		return err
	}
	if flow.Entrypoint.Type == "kafka" {
		if err := validateKafkaFlowSemantics(flow); err != nil {
			return err
		}
	} else if contract.RequiresResponse {
		if err := validateRequiredResponseReturn(flow, contract); err != nil {
			return err
		}
	}
	return nil
}

func validateResponseCallsInFlow(flow runtime.Flow, contract runtime.ResponseContract) error {
	for _, step := range flowStepsForValidation(flow) {
		if err := ValidateResponseCalls(step.Body, contract); err != nil {
			return fmt.Errorf("step %q: %w", step.ID, err)
		}
		if err := ValidateResponseCalls(step.FallbackBody, contract); err != nil {
			return fmt.Errorf("fallback %q: %w", step.ID, err)
		}
		if err := ValidateResponseCalls(step.CompensateBody, contract); err != nil {
			return fmt.Errorf("compensate %q: %w", step.ID, err)
		}
	}
	if err := ValidateResponseCalls(flow.OnErrorBody, contract); err != nil {
		return fmt.Errorf("on_error: %w", err)
	}
	if err := ValidateResponseCalls(flow.Return.Body, contract); err != nil {
		return fmt.Errorf("return: %w", err)
	}
	return nil
}

func validateKafkaFlowSemantics(flow runtime.Flow) error {
	valueConfig, ok := flow.Entrypoint.Config["value"].(map[string]any)
	if !ok {
		return fmt.Errorf("entrypoint.kafka value must be configured")
	}
	valueType, ok := valueConfig["type"].(string)
	if !ok || valueType != "json" {
		return fmt.Errorf("entrypoint.kafka value.type must be json")
	}

	returnBody := strings.TrimSpace(strings.TrimSuffix(flow.Return.Body, ";"))
	if returnBody != "response.ack()" && returnBody != "response.nack()" {
		return fmt.Errorf("entrypoint.kafka requires top-level return response.ack() or response.nack()")
	}

	for _, step := range flowStepsForValidation(flow) {
		if step.ID == "__return" {
			continue
		}
		if err := rejectKafkaAckNack(step.Body); err != nil {
			return fmt.Errorf("step %q: %w", step.ID, err)
		}
		if err := rejectKafkaAckNack(step.FallbackBody); err != nil {
			return fmt.Errorf("fallback %q: %w", step.ID, err)
		}
		if err := rejectKafkaAckNack(step.CompensateBody); err != nil {
			return fmt.Errorf("compensate %q: %w", step.ID, err)
		}
	}
	return nil
}

func validateRequiredResponseReturn(flow runtime.Flow, contract runtime.ResponseContract) error {
	subtype, ok := topLevelResponseCallSubtype(flow.Return.Body)
	if !ok {
		return fmt.Errorf("entrypoint.%s requires top-level return response.*(...)", contract.EntrypointType)
	}
	if !contract.HasSubtype(subtype) {
		return fmt.Errorf("response.%s is not valid for this entrypoint", subtype)
	}
	return nil
}

func topLevelResponseCallSubtype(source string) (string, bool) {
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(source), ";"))
	if !strings.HasPrefix(body, "response.") {
		return "", false
	}
	rest := strings.TrimPrefix(body, "response.")
	idx := strings.Index(rest, "(")
	if idx <= 0 {
		return "", false
	}
	subtype := strings.TrimSpace(rest[:idx])
	if subtype == "" || strings.ContainsAny(subtype, " \t\n\r.") {
		return "", false
	}
	return subtype, true
}

func rejectKafkaAckNack(source string) error {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	program, err := risorparser.Parse(context.Background(), source, nil)
	if err != nil {
		return err
	}
	for node := range ast.Preorder(program) {
		_, subtype, ok := responseCall(node)
		if !ok {
			continue
		}
		if subtype == "ack" || subtype == "nack" {
			return fmt.Errorf("response.%s is only allowed in top-level return or on_error", subtype)
		}
	}
	return nil
}

func ExtractLiteralFlowCalls(source string) ([]FlowCallRef, error) {
	if strings.TrimSpace(source) == "" {
		return nil, nil
	}
	program, err := risorparser.Parse(context.Background(), source, nil)
	if err != nil {
		return nil, err
	}

	var refs []FlowCallRef
	for node := range ast.Preorder(program) {
		call, ok := flowCall(node)
		if !ok {
			continue
		}
		ref := FlowCallRef{}
		if len(call.Args) > 0 {
			if s, ok := call.Args[0].(*ast.String); ok {
				ref.TargetName = s.Value
			}
		}
		if len(call.Args) > 1 {
			if m, ok := call.Args[1].(*ast.Map); ok {
				ref.ArgsStatic = true
				ref.ArgKeys = map[string]struct{}{}
				ref.ArgValues = map[string]LiteralValue{}
				for _, item := range m.Items {
					key, ok := literalMapKey(item.Key)
					if !ok {
						ref.ArgsStatic = false
						ref.ArgKeys = nil
						ref.ArgValues = nil
						break
					}
					ref.ArgKeys[key] = struct{}{}
					if lit, ok := simpleLiteral(item.Value); ok {
						ref.ArgValues[key] = lit
					}
				}
			}
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func flowCall(node ast.Node) (*ast.Call, bool) {
	if objectCall, ok := node.(*ast.ObjectCall); ok {
		ident, ok := objectCall.X.(*ast.Ident)
		if !ok || ident.Name != "flow" || objectCall.Call == nil {
			return nil, false
		}
		method, ok := objectCall.Call.Fun.(*ast.Ident)
		if !ok || method.Name != "call" {
			return nil, false
		}
		return objectCall.Call, true
	}

	call, ok := node.(*ast.Call)
	if !ok {
		return nil, false
	}
	getAttr, ok := call.Fun.(*ast.GetAttr)
	if !ok || getAttr.Attr == nil || getAttr.Attr.Name != "call" {
		return nil, false
	}
	ident, ok := getAttr.X.(*ast.Ident)
	if !ok || ident.Name != "flow" {
		return nil, false
	}
	return call, true
}

func literalMapKey(node ast.Expr) (string, bool) {
	switch n := node.(type) {
	case *ast.Ident:
		return n.Name, true
	case *ast.String:
		return n.Value, true
	default:
		return "", false
	}
}

func simpleLiteral(node ast.Expr) (LiteralValue, bool) {
	switch n := node.(type) {
	case *ast.String:
		return LiteralValue{Kind: LiteralString, Value: n.Value}, true
	case *ast.Int:
		return LiteralValue{Kind: LiteralInteger, Value: n.Value}, true
	case *ast.Float:
		return LiteralValue{Kind: LiteralNumber, Value: n.Value}, true
	case *ast.Bool:
		return LiteralValue{Kind: LiteralBoolean, Value: n.Value}, true
	case *ast.Nil:
		return LiteralValue{Kind: LiteralNil, Value: nil}, true
	default:
		return LiteralValue{}, false
	}
}

func extractFlowCallsFromFlow(flow runtime.Flow) ([]FlowCallRef, error) {
	var refs []FlowCallRef
	add := func(body string) error {
		bodyRefs, err := ExtractLiteralFlowCalls(body)
		if err != nil {
			return err
		}
		refs = append(refs, bodyRefs...)
		return nil
	}
	for _, step := range flowStepsForValidation(flow) {
		if err := add(step.Body); err != nil {
			return nil, fmt.Errorf("step %q: %w", step.ID, err)
		}
		if err := add(step.FallbackBody); err != nil {
			return nil, fmt.Errorf("fallback %q: %w", step.ID, err)
		}
		if err := add(step.CompensateBody); err != nil {
			return nil, fmt.Errorf("compensate %q: %w", step.ID, err)
		}
	}
	if err := add(flow.OnErrorBody); err != nil {
		return nil, fmt.Errorf("on_error: %w", err)
	}
	if err := add(flow.Return.Body); err != nil {
		return nil, fmt.Errorf("return: %w", err)
	}
	return refs, nil
}

func flowStepsForValidation(flow runtime.Flow) []runtime.Step {
	nodes := runtime.NormalizeFlowNodes(&flow)
	if len(nodes) == 0 {
		return nil
	}
	steps := []runtime.Step{}
	for _, node := range nodes {
		switch node.Kind {
		case runtime.FlowNodeStep:
			if node.Step != nil {
				steps = append(steps, *node.Step)
			}
		case runtime.FlowNodeParallel:
			if node.Parallel != nil {
				steps = append(steps, node.Parallel.Branches...)
			}
		case runtime.FlowNodeForeach:
			if node.Foreach != nil {
				steps = append(steps, node.Foreach.Steps...)
			}
		}
	}
	return steps
}

func validateLiteralArgs(flowID, targetName string, ref FlowCallRef, fields map[string]*schema.Schema) error {
	for name, fieldSchema := range fields {
		if fieldSchema.Required {
			if _, ok := ref.ArgKeys[name]; !ok {
				return fmt.Errorf("flow %q calls %q missing required arg %q", flowID, targetName, name)
			}
		}
		lit, ok := ref.ArgValues[name]
		if !ok {
			continue
		}
		if err := validateLiteralValue(lit, fieldSchema); err != nil {
			return fmt.Errorf("flow %q calls %q arg %q: %w", flowID, targetName, name, err)
		}
	}
	return nil
}

func validateLiteralValue(lit LiteralValue, s *schema.Schema) error {
	if lit.Kind == LiteralNil {
		if s.Required {
			return fmt.Errorf("required value cannot be nil")
		}
		return nil
	}

	switch s.Type {
	case schema.TypeString:
		if lit.Kind != LiteralString {
			return fmt.Errorf("expected string literal, got %s", lit.Kind)
		}
	case schema.TypeInteger:
		if lit.Kind != LiteralInteger {
			return fmt.Errorf("expected integer literal, got %s", lit.Kind)
		}
	case schema.TypeNumber:
		if lit.Kind != LiteralInteger && lit.Kind != LiteralNumber {
			return fmt.Errorf("expected number literal, got %s", lit.Kind)
		}
	case schema.TypeBoolean:
		if lit.Kind != LiteralBoolean {
			return fmt.Errorf("expected boolean literal, got %s", lit.Kind)
		}
	default:
		return nil
	}

	if len(s.Enum) > 0 && !literalInEnum(lit.Value, s.Enum) {
		return fmt.Errorf("literal is not in enum")
	}
	if lit.Kind == LiteralInteger || lit.Kind == LiteralNumber {
		n := literalNumber(lit)
		if s.Minimum != nil && n < *s.Minimum {
			return fmt.Errorf("literal must be >= %v", *s.Minimum)
		}
		if s.Maximum != nil && n > *s.Maximum {
			return fmt.Errorf("literal must be <= %v", *s.Maximum)
		}
	}
	return nil
}

func literalInEnum(value any, enum []any) bool {
	for _, allowed := range enum {
		if fmt.Sprintf("%v", allowed) == fmt.Sprintf("%v", value) {
			return true
		}
	}
	return false
}

func literalNumber(lit LiteralValue) float64 {
	switch v := lit.Value.(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	default:
		return 0
	}
}

func findFlowCallCycle(edges map[string][]string) []string {
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var stack []string

	var visit func(string) []string
	visit = func(node string) []string {
		if visiting[node] {
			for i, n := range stack {
				if n == node {
					return append(append([]string{}, stack[i:]...), node)
				}
			}
			return []string{node, node}
		}
		if visited[node] {
			return nil
		}
		visiting[node] = true
		stack = append(stack, node)
		for _, next := range edges[node] {
			if cycle := visit(next); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		visiting[node] = false
		visited[node] = true
		return nil
	}

	for node := range edges {
		if cycle := visit(node); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}
