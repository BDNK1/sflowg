package httpinput

import (
	"fmt"

	"github.com/BDNK1/sflowg/runtime/validation/schema"
)

type Binding struct {
	Body            *schema.Schema
	PathVariables   map[string]*schema.Schema
	QueryParameters map[string]*schema.Schema
	Headers         map[string]*schema.Schema
}

func ParseBinding(config map[string]any) (*Binding, error) {
	binding := &Binding{}
	hasSchemas := false

	if bodyRaw, ok := config["body"].(map[string]any); ok {
		if schemaRaw, ok := bodyRaw["schema"]; ok {
			body, err := schema.ParseRoot(schemaRaw)
			if err != nil {
				return nil, fmt.Errorf("body.schema: %w", err)
			}
			binding.Body = body
			hasSchemas = true
		}
	}

	for _, entry := range []struct {
		key string
		set func(map[string]*schema.Schema)
	}{
		{key: "pathVariables", set: func(v map[string]*schema.Schema) { binding.PathVariables = v }},
		{key: "queryParameters", set: func(v map[string]*schema.Schema) { binding.QueryParameters = v }},
		{key: "headers", set: func(v map[string]*schema.Schema) { binding.Headers = v }},
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
		entry.set(parsed)
		hasSchemas = true
	}

	if !hasSchemas {
		return nil, nil
	}
	return binding, nil
}
