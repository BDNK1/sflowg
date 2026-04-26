package schema

import (
	"fmt"
	"strconv"
)

func toBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(v)
		return err == nil && parsed
	default:
		return false
	}
}

func optionalFloat(value any) (float64, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	switch v := value.(type) {
	case int:
		return float64(v), true, nil
	case int64:
		return float64(v), true, nil
	case float64:
		return v, true, nil
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, true, err
		}
		return parsed, true, nil
	default:
		return 0, true, fmt.Errorf("expected number, got %T", value)
	}
}

func optionalInt(value any) (int, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	switch v := value.(type) {
	case int:
		return v, true, nil
	case int64:
		return int(v), true, nil
	case float64:
		return int(v), true, nil
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, true, err
		}
		return parsed, true, nil
	default:
		return 0, true, fmt.Errorf("expected integer, got %T", value)
	}
}
