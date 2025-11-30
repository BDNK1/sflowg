package runtime

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mitchellh/mapstructure"
)

func ToStringValueMap(m map[string]any) map[string]string {
	result := make(map[string]string)
	for key, value := range m {
		switch v := value.(type) {
		case string:
			result[key] = v
		case int:
			result[key] = fmt.Sprintf("%d", v)
		case float64:
			result[key] = fmt.Sprintf("%f", v)
		case bool:
			result[key] = fmt.Sprintf("%t", v)
		case nil:
			result[key] = ""
		default:
			result[key] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// mapToStruct converts a map[string]any to a struct using mapstructure.
// It uses json tags for field mapping and supports time.Duration and time.Time conversions.
func mapToStruct(m map[string]any, target any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  target,
		TagName: "json", // Use json tags for field mapping
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToTimeHookFunc(time.RFC3339),
		),
		WeaklyTypedInput: true, // Allow type coercion (e.g., int -> float64)
	})
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(m); err != nil {
		return fmt.Errorf("failed to decode map to struct: %w", err)
	}

	return nil
}

// structToMap converts a struct to map[string]any using JSON round-trip.
// This respects json tags and properly handles nested structs.
func structToMap(s any) (map[string]any, error) {
	// Marshal to JSON first (respects json tags)
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal struct: %w", err)
	}

	// Unmarshal to map
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	return result, nil
}
