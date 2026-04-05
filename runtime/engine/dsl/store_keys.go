package dsl

import (
	"context"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/risor/v2/pkg/ast"
	risorparser "github.com/deepnoodle-ai/risor/v2/pkg/parser"
)

func ExtractStoreKeys(source string, knownStoreKeys []string, frameworkKeys map[string]struct{}) ([]string, error) {
	if strings.TrimSpace(source) == "" {
		return nil, nil
	}

	program, err := risorparser.Parse(context.Background(), source, nil)
	if err != nil {
		return nil, err
	}

	known := make(map[string]struct{}, len(knownStoreKeys))
	for _, key := range knownStoreKeys {
		known[key] = struct{}{}
	}

	found := make(map[string]struct{})
	for node := range ast.Preorder(program) {
		ident, ok := node.(*ast.Ident)
		if !ok {
			continue
		}
		if _, ok := known[ident.Name]; !ok {
			continue
		}
		if _, ok := frameworkKeys[ident.Name]; ok {
			continue
		}
		found[ident.Name] = struct{}{}
	}

	if len(found) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(found))
	for key := range found {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}
