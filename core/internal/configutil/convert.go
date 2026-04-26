package configutil

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mitchellh/mapstructure"
)

func MapToStruct(m map[string]any, target any) error {
	return mapToStructWithTag(m, target, "json")
}

func MapToStructFromYAML(m map[string]any, target any) error {
	return mapToStructWithTag(m, target, "yaml")
}

func mapToStructWithTag(m map[string]any, target any, tagName string) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  target,
		TagName: tagName,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToTimeHookFunc(time.RFC3339),
		),
		WeaklyTypedInput: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(m); err != nil {
		return fmt.Errorf("failed to decode map to struct: %w", err)
	}

	return nil
}

func StructToMap(s any) (map[string]any, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal struct: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	return result, nil
}
