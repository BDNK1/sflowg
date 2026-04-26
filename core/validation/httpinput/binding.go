package httpinput

import (
	"fmt"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/schema"
)

const (
	BodyNamespace            = "request.body"
	PathVariablesNamespace   = "request.pathVariables"
	QueryParametersNamespace = "request.queryParameters"
	HeadersNamespace         = "request.headers"
)

func ParseBinding(config map[string]any) (*runtime.InputContract, error) {
	contract := runtime.NewInputContract()
	hasSchemas := false

	if bodyRaw, ok := config["body"].(map[string]any); ok {
		if schemaRaw, ok := bodyRaw["schema"]; ok {
			body, err := schema.ParseRoot(schemaRaw)
			if err != nil {
				return nil, fmt.Errorf("body.schema: %w", err)
			}
			contract.SetRoot(BodyNamespace, body)
			hasSchemas = true
		}
	}

	for _, entry := range []struct {
		key       string
		namespace string
	}{
		{key: "pathVariables", namespace: PathVariablesNamespace},
		{key: "queryParameters", namespace: QueryParametersNamespace},
		{key: "headers", namespace: HeadersNamespace},
	} {
		raw, ok := config[entry.key]
		if !ok {
			continue
		}
		rawMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		parsed, err := schema.ParseFieldMap(rawMap)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.key, err)
		}
		contract.SetFields(entry.namespace, parsed)
		hasSchemas = true
	}

	if !hasSchemas {
		return nil, nil
	}
	return contract, nil
}
