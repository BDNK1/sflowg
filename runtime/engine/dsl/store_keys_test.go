package dsl

import (
	"reflect"
	"testing"
)

func TestExtractStoreKeys(t *testing.T) {
	knownStoreKeys := []string{"request", "properties", "create_order", "error", "compensation"}
	frameworkKeys := map[string]struct{}{
		"response": {},
		"postgres": {},
		"log":      {},
		"metric":   {},
		"sprintf":  {},
	}

	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "request body",
			source: `request.body.email`,
			want:   []string{"request"},
		},
		{
			name:   "framework response",
			source: `response.json({status: 200})`,
			want:   nil,
		},
		{
			name:   "plugin call",
			source: `postgres.get({query: "select 1"})`,
			want:   nil,
		},
		{
			name:   "multi key body",
			source: `{email: request.body.email, id: create_order.id, api: properties.api_key}`,
			want:   []string{"create_order", "properties", "request"},
		},
		{
			name:   "empty body",
			source: `  `,
			want:   nil,
		},
		{
			name:   "error key",
			source: `error.code`,
			want:   []string{"error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractStoreKeys(tt.source, knownStoreKeys, frameworkKeys)
			if err != nil {
				t.Fatalf("ExtractStoreKeys() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExtractStoreKeys() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
