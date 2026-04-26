package schema

import "fmt"

type FieldError struct {
	Path       string `json:"path"`
	Pointer    string `json:"pointer"`
	Constraint string `json:"constraint"`
	Got        any    `json:"got,omitempty"`
	Message    string `json:"detail"`
}

func fieldError(path, constraint string, got any, format string, args ...any) FieldError {
	return FieldError{
		Path:       path,
		Pointer:    pointer(path),
		Constraint: constraint,
		Got:        got,
		Message:    fmt.Sprintf(format, args...),
	}
}

func pointer(path string) string {
	if path == "" {
		return "#"
	}
	out := "#"
	start := 0
	for i := 0; i <= len(path); i++ {
		if i != len(path) && path[i] != '.' {
			continue
		}
		part := path[start:i]
		out += "/" + escapePointerPart(part)
		start = i + 1
	}
	return out
}

func escapePointerPart(part string) string {
	out := ""
	for _, r := range part {
		switch r {
		case '~':
			out += "~0"
		case '/':
			out += "~1"
		default:
			out += string(r)
		}
	}
	return out
}

func FieldsToMaps(fields []FieldError) []map[string]any {
	out := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		out = append(out, map[string]any{
			"path":       field.Path,
			"pointer":    field.Pointer,
			"constraint": field.Constraint,
			"detail":     field.Message,
		})
	}
	return out
}
