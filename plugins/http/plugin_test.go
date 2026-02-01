package http

import (
	"testing"
)

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
