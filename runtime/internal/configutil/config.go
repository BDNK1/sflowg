package configutil

import (
	"fmt"

	"github.com/creasty/defaults"
)

func ApplyDefaults(config any) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := defaults.Set(config); err != nil {
		return fmt.Errorf("failed to apply default values: %w", err)
	}

	return nil
}

func Prepare(config any, rawValues map[string]any) error {
	if err := ApplyDefaults(config); err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
	}

	if len(rawValues) > 0 {
		if err := MapToStructFromYAML(rawValues, config); err != nil {
			return fmt.Errorf("failed to apply config values: %w", err)
		}
	}

	if err := ValidateStruct(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}
