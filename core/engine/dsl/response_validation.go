package dsl

import (
	"context"
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/core"
	"github.com/deepnoodle-ai/risor/v2/pkg/ast"
	risorparser "github.com/deepnoodle-ai/risor/v2/pkg/parser"
)

func ValidateResponseCalls(source string, contract runtime.ResponseContract) error {
	if strings.TrimSpace(source) == "" {
		return nil
	}

	program, err := risorparser.Parse(context.Background(), source, nil)
	if err != nil {
		return err
	}

	for node := range ast.Preorder(program) {
		call, subtype, ok := responseCall(node)
		if !ok {
			continue
		}
		if err := contract.ValidateStaticArgCount(subtype, len(call.Args)); err != nil {
			return err
		}
	}
	return nil
}

func rejectResponseCallsInSurface(stepID string, surface string, source string) error {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	program, err := risorparser.Parse(context.Background(), source, nil)
	if err != nil {
		return err
	}
	for node := range ast.Preorder(program) {
		_, subtype, ok := responseCall(node)
		if ok {
			return fmt.Errorf("parallel branch %q %s cannot call response.%s", stepID, surface, subtype)
		}
	}
	return nil
}

func responseCallSubtype(node ast.Node) (string, bool) {
	_, subtype, ok := responseCall(node)
	return subtype, ok
}

func responseCall(node ast.Node) (*ast.Call, string, bool) {
	if objectCall, ok := node.(*ast.ObjectCall); ok {
		ident, ok := objectCall.X.(*ast.Ident)
		if !ok || ident.Name != "response" || objectCall.Call == nil {
			return nil, "", false
		}
		method, ok := objectCall.Call.Fun.(*ast.Ident)
		if !ok {
			return nil, "", false
		}
		return objectCall.Call, method.Name, true
	}

	call, ok := node.(*ast.Call)
	if !ok {
		return nil, "", false
	}
	getAttr, ok := call.Fun.(*ast.GetAttr)
	if !ok || getAttr.Attr == nil {
		return nil, "", false
	}
	ident, ok := getAttr.X.(*ast.Ident)
	if !ok || ident.Name != "response" {
		return nil, "", false
	}
	return call, getAttr.Attr.Name, true
}
