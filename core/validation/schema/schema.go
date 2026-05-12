package schema

import (
	"fmt"
	"regexp"
)

type Type string

const (
	TypeObject  Type = "object"
	TypeArray   Type = "array"
	TypeString  Type = "string"
	TypeInteger Type = "integer"
	TypeNumber  Type = "number"
	TypeBoolean Type = "boolean"
)

type Schema struct {
	Type            Type
	Required        bool
	Default         any
	Enum            []any
	enumStringSet   map[string]struct{}
	Minimum         *float64
	Maximum         *float64
	MinLength       *int
	MaxLength       *int
	Pattern         string
	compiledPattern *regexp.Regexp
	Format          string
	Properties      map[string]*Schema
	Items           *Schema
}

func ParseRoot(raw any) (*Schema, error) {
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema must be an object")
	}
	if _, hasType := rawMap["type"]; !hasType {
		fields, err := ParseFieldMap(rawMap)
		if err != nil {
			return nil, err
		}
		return &Schema{Type: TypeObject, Properties: fields}, nil
	}
	return Parse(raw)
}

func ParseFieldMap(raw map[string]any) (map[string]*Schema, error) {
	result := make(map[string]*Schema, len(raw))
	for name, value := range raw {
		parsed, err := Parse(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		result[name] = parsed
	}
	return result, nil
}

func Parse(raw any) (*Schema, error) {
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema must be an object")
	}

	s := &Schema{}
	if typ, ok := rawMap["type"]; ok {
		s.Type = Type(fmt.Sprintf("%v", typ))
	} else if _, ok := rawMap["properties"]; ok {
		s.Type = TypeObject
	}

	if s.Type == "" {
		return nil, fmt.Errorf("type is required")
	}
	if !isKnownType(s.Type) {
		return nil, fmt.Errorf("unknown type %q", s.Type)
	}

	s.Required = toBool(rawMap["required"])
	if v, ok := rawMap["default"]; ok {
		s.Default = v
	}
	if enumRaw, ok := rawMap["enum"].([]any); ok {
		s.Enum = enumRaw
	}
	if v, ok, err := optionalFloat(rawMap["minimum"]); err != nil {
		return nil, fmt.Errorf("minimum: %w", err)
	} else if ok {
		s.Minimum = &v
	}
	if v, ok, err := optionalFloat(rawMap["maximum"]); err != nil {
		return nil, fmt.Errorf("maximum: %w", err)
	} else if ok {
		s.Maximum = &v
	}
	if v, ok, err := optionalInt(rawMap["minLength"]); err != nil {
		return nil, fmt.Errorf("minLength: %w", err)
	} else if ok {
		s.MinLength = &v
	}
	if v, ok, err := optionalInt(rawMap["maxLength"]); err != nil {
		return nil, fmt.Errorf("maxLength: %w", err)
	} else if ok {
		s.MaxLength = &v
	}
	if v, ok := rawMap["pattern"].(string); ok {
		s.Pattern = v
	}
	if v, ok := rawMap["format"].(string); ok {
		s.Format = v
	}

	if propsRaw, ok := rawMap["properties"].(map[string]any); ok {
		props, err := ParseFieldMap(propsRaw)
		if err != nil {
			return nil, fmt.Errorf("properties: %w", err)
		}
		s.Properties = props
	}
	if s.Type == TypeObject && s.Properties == nil {
		props := map[string]any{}
		for key, value := range rawMap {
			if isSchemaKeyword(key) {
				continue
			}
			props[key] = value
		}
		if len(props) > 0 {
			parsed, err := ParseFieldMap(props)
			if err != nil {
				return nil, err
			}
			s.Properties = parsed
		}
	}

	if itemsRaw, ok := rawMap["items"]; ok {
		items, err := Parse(itemsRaw)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		s.Items = items
	}

	if err := s.validateShape(); err != nil {
		return nil, err
	}
	return s, nil
}

func isKnownType(t Type) bool {
	switch t {
	case TypeObject, TypeArray, TypeString, TypeInteger, TypeNumber, TypeBoolean:
		return true
	default:
		return false
	}
}

func isSchemaKeyword(key string) bool {
	switch key {
	case "type", "required", "default", "enum", "minimum", "maximum", "minLength", "maxLength", "pattern", "format", "properties", "items":
		return true
	default:
		return false
	}
}
