package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	core "github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/plugin"
)

func TestRequestSetsJSONContentTypeWhenExplicit(t *testing.T) {
	var gotContentType string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("Decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	p := &HTTPPlugin{Config: Config{Timeout: 5_000_000_000}}
	if err := p.Initialize(core.NewLogger(nil)); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	out, err := p.Request(newPluginExecution(), RequestInput{
		URL:         server.URL,
		Method:      "POST",
		ContentType: "json",
		Body: map[string]any{
			"metadata": map[string]any{"order_id": "123"},
		},
	})
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if out.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want 200", out.StatusCode)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	metadata, ok := gotBody["metadata"].(map[string]any)
	if !ok || metadata["order_id"] != "123" {
		t.Fatalf("metadata = %#v, want order_id 123", gotBody["metadata"])
	}
}

func TestRequestDoesNotOverrideExplicitContentType(t *testing.T) {
	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	p := &HTTPPlugin{Config: Config{Timeout: 5_000_000_000}}
	if err := p.Initialize(core.NewLogger(nil)); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	_, err := p.Request(newPluginExecution(), RequestInput{
		URL:     server.URL,
		Method:  "POST",
		Headers: map[string]string{"content-type": "application/vnd.api+json"},
		Body:    map[string]any{"ok": true},
	})
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if gotContentType != "application/vnd.api+json" {
		t.Fatalf("Content-Type = %q, want explicit value", gotContentType)
	}
}

func newPluginExecution() *plugin.Execution {
	return core.NewExecution(&core.Flow{ID: "test"}, core.NewContainer(core.NewLogger(nil)), nil, core.NewValueStore())
}

func TestFlattenToFormData(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]string
	}{
		{
			name: "simple values",
			input: map[string]any{
				"amount":   1099,
				"currency": "usd",
			},
			expected: map[string]string{
				"amount":   "1099",
				"currency": "usd",
			},
		},
		{
			name: "nested map",
			input: map[string]any{
				"amount": 1099,
				"metadata": map[string]any{
					"order_id": "12345",
					"user":     "john",
				},
			},
			expected: map[string]string{
				"amount":             "1099",
				"metadata[order_id]": "12345",
				"metadata[user]":     "john",
			},
		},
		{
			name: "deeply nested",
			input: map[string]any{
				"shipping": map[string]any{
					"address": map[string]any{
						"city":    "NYC",
						"country": "US",
					},
				},
			},
			expected: map[string]string{
				"shipping[address][city]":    "NYC",
				"shipping[address][country]": "US",
			},
		},
		{
			name: "array values",
			input: map[string]any{
				"items": []any{"item1", "item2"},
			},
			expected: map[string]string{
				"items[0]": "item1",
				"items[1]": "item2",
			},
		},
		{
			name: "array of objects",
			input: map[string]any{
				"line_items": []any{
					map[string]any{"price": "price_123", "quantity": 2},
					map[string]any{"price": "price_456", "quantity": 1},
				},
			},
			expected: map[string]string{
				"line_items[0][price]":    "price_123",
				"line_items[0][quantity]": "2",
				"line_items[1][price]":    "price_456",
				"line_items[1][quantity]": "1",
			},
		},
		{
			name: "stripe payment intent example",
			input: map[string]any{
				"amount":               1099,
				"currency":             "usd",
				"payment_method_types": []any{"card"},
				"metadata": map[string]any{
					"order_id": "order_123",
				},
			},
			expected: map[string]string{
				"amount":                  "1099",
				"currency":                "usd",
				"payment_method_types[0]": "card",
				"metadata[order_id]":      "order_123",
			},
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]string{},
		},
		{
			name: "boolean and float",
			input: map[string]any{
				"enabled": true,
				"rate":    0.15,
			},
			expected: map[string]string{
				"enabled": "true",
				"rate":    "0.15",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := flattenToFormData(tt.input, "")

			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d\ngot: %v\nwant: %v",
					len(result), len(tt.expected), result, tt.expected)
				return
			}

			for key, expectedVal := range tt.expected {
				if gotVal, ok := result[key]; !ok {
					t.Errorf("missing key %q", key)
				} else if gotVal != expectedVal {
					t.Errorf("key %q: got %q, want %q", key, gotVal, expectedVal)
				}
			}
		})
	}
}
