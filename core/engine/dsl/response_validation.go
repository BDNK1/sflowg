package dsl

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/deepnoodle-ai/risor/v2/pkg/ast"
	risorparser "github.com/deepnoodle-ai/risor/v2/pkg/parser"
)

func ValidateResponseCalls(source string, allowedSubtypes []string) error {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	if len(allowedSubtypes) == 0 {
		allowedSubtypes = []string{"json"}
	}

	program, err := risorparser.Parse(context.Background(), source, nil)
	if err != nil {
		return err
	}

	for node := range ast.Preorder(program) {
		subtype, ok := responseCallSubtype(node)
		if !ok {
			continue
		}
		if !slices.Contains(allowedSubtypes, subtype) {
			return fmt.Errorf("response.%s is not valid for this entrypoint", subtype)
		}
	}
	return nil
}

func responseCallSubtype(node ast.Node) (string, bool) {
	if objectCall, ok := node.(*ast.ObjectCall); ok {
		ident, ok := objectCall.X.(*ast.Ident)
		if !ok || ident.Name != "response" || objectCall.Call == nil {
			return "", false
		}
		method, ok := objectCall.Call.Fun.(*ast.Ident)
		if !ok {
			return "", false
		}
		return method.Name, true
	}

	call, ok := node.(*ast.Call)
	if !ok {
		return "", false
	}
	getAttr, ok := call.Fun.(*ast.GetAttr)
	if !ok || getAttr.Attr == nil {
		return "", false
	}
	ident, ok := getAttr.X.(*ast.Ident)
	if !ok || ident.Name != "response" {
		return "", false
	}
	return getAttr.Attr.Name, true
}
