package schema

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func Validate(value any, s *Schema, path string) (any, []FieldError) {
	return validatePresent(value, true, s, path)
}

func ValidateField(value any, present bool, s *Schema, path string) (any, []FieldError) {
	return validatePresent(value, present, s, path)
}

func validatePresent(value any, present bool, s *Schema, path string) (any, []FieldError) {
	if s == nil {
		return value, nil
	}
	if !present {
		if s.Default != nil {
			value = s.Default
			present = true
		} else if s.Required {
			return nil, []FieldError{fieldError(path, "required", nil, "%s is required", path)}
		} else {
			return nil, nil
		}
	}

	switch s.Type {
	case TypeObject:
		return validateObject(value, s, path)
	case TypeArray:
		return validateArray(value, s, path)
	case TypeInteger:
		return validateInteger(value, s, path)
	case TypeNumber:
		return validateNumber(value, s, path)
	case TypeBoolean:
		return validateBoolean(value, s, path)
	default:
		return validateString(value, s, path)
	}
}

func validateObject(value any, s *Schema, path string) (any, []FieldError) {
	obj, ok := value.(map[string]any)
	if !ok {
		return value, []FieldError{fieldError(path, "type", value, "%s must be an object", path)}
	}
	normalized := make(map[string]any, len(obj))
	for key, val := range obj {
		normalized[key] = val
	}

	var errs []FieldError
	for key, fieldSchema := range s.Properties {
		fieldPath := joinPath(path, key)
		raw, present := obj[key]
		normalizedValue, fieldErrs := validatePresent(raw, present, fieldSchema, fieldPath)
		if len(fieldErrs) > 0 {
			errs = append(errs, fieldErrs...)
			continue
		}
		if normalizedValue != nil || present || fieldSchema.Default != nil {
			normalized[key] = normalizedValue
		}
	}
	return normalized, errs
}

func validateArray(value any, s *Schema, path string) (any, []FieldError) {
	arr, ok := toAnySlice(value)
	if !ok {
		return value, []FieldError{fieldError(path, "type", value, "%s must be an array", path)}
	}
	if s.Items == nil {
		return arr, nil
	}

	normalized := make([]any, 0, len(arr))
	var errs []FieldError
	for i, item := range arr {
		itemPath := fmt.Sprintf("%s.%d", path, i)
		normalizedItem, itemErrs := validatePresent(item, true, s.Items, itemPath)
		if len(itemErrs) > 0 {
			errs = append(errs, itemErrs...)
			continue
		}
		normalized = append(normalized, normalizedItem)
	}
	return normalized, errs
}

func validateInteger(value any, s *Schema, path string) (any, []FieldError) {
	v, ok := toInt64(value)
	if !ok {
		return value, []FieldError{fieldError(path, "type", value, "%s must be an integer", path)}
	}
	var errs []FieldError
	if s.Minimum != nil && float64(v) < *s.Minimum {
		errs = append(errs, fieldError(path, "minimum", v, "%s must be >= %v", path, *s.Minimum))
	}
	if s.Maximum != nil && float64(v) > *s.Maximum {
		errs = append(errs, fieldError(path, "maximum", v, "%s must be <= %v", path, *s.Maximum))
	}
	return v, errs
}

func validateNumber(value any, s *Schema, path string) (any, []FieldError) {
	v, ok := toFloat64(value)
	if !ok {
		return value, []FieldError{fieldError(path, "type", value, "%s must be a number", path)}
	}
	var errs []FieldError
	if s.Minimum != nil && v < *s.Minimum {
		errs = append(errs, fieldError(path, "minimum", v, "%s must be >= %v", path, *s.Minimum))
	}
	if s.Maximum != nil && v > *s.Maximum {
		errs = append(errs, fieldError(path, "maximum", v, "%s must be <= %v", path, *s.Maximum))
	}
	return v, errs
}

func validateBoolean(value any, s *Schema, path string) (any, []FieldError) {
	v, ok := toBoolValue(value)
	if !ok {
		return value, []FieldError{fieldError(path, "type", value, "%s must be a boolean", path)}
	}
	return v, nil
}

func validateString(value any, s *Schema, path string) (any, []FieldError) {
	v, ok := value.(string)
	if !ok {
		return value, []FieldError{fieldError(path, "type", value, "%s must be a string", path)}
	}
	var errs []FieldError
	if s.MinLength != nil && len(v) < *s.MinLength {
		errs = append(errs, fieldError(path, "minLength", v, "%s is too short", path))
	}
	if s.MaxLength != nil && len(v) > *s.MaxLength {
		errs = append(errs, fieldError(path, "maxLength", v, "%s is too long", path))
	}
	if s.compiledPattern != nil {
		if !s.compiledPattern.MatchString(v) {
			errs = append(errs, fieldError(path, "pattern", v, "%s does not match pattern", path))
		}
	} else if s.Pattern != "" {
		errs = append(errs, fieldError(path, "pattern", v, "%s has invalid pattern %q", path, s.Pattern))
	}
	if s.Format != "" && !validFormat(s.Format, v) {
		errs = append(errs, fieldError(path, "format", v, "%s must be a valid %s", path, s.Format))
	}
	if !s.enumContains(v) {
		errs = append(errs, fieldError(path, "enum", v, "%s must be one of the allowed values", path))
	}
	return v, errs
}

func (s *Schema) validateShape() error {
	if s.Pattern != "" {
		compiled, err := regexp.Compile(s.Pattern)
		if err != nil {
			return fmt.Errorf("invalid pattern %q: %w", s.Pattern, err)
		}
		s.compiledPattern = compiled
	}
	if len(s.Enum) > 0 {
		set := make(map[string]struct{}, len(s.Enum))
		for _, item := range s.Enum {
			if str, ok := item.(string); ok {
				set[str] = struct{}{}
			}
		}
		s.enumStringSet = set
	}
	if (s.Minimum != nil || s.Maximum != nil) && s.Type != TypeInteger && s.Type != TypeNumber {
		return fmt.Errorf("%s schema cannot use minimum or maximum", s.Type)
	}
	if (s.MinLength != nil || s.MaxLength != nil || s.Pattern != "" || s.Format != "") && s.Type != TypeString {
		return fmt.Errorf("%s schema cannot use string constraints", s.Type)
	}
	if len(s.Enum) > 0 && s.Type != TypeString {
		return fmt.Errorf("%s schema cannot use enum", s.Type)
	}
	if s.Properties != nil && s.Type != TypeObject {
		return fmt.Errorf("%s schema cannot declare properties", s.Type)
	}
	if s.Items != nil && s.Type != TypeArray {
		return fmt.Errorf("%s schema cannot declare items", s.Type)
	}
	if s.Type == TypeArray && s.Items == nil {
		return fmt.Errorf("array schema requires items")
	}
	return nil
}

func validFormat(format string, value string) bool {
	switch format {
	case "email":
		_, err := mail.ParseAddress(value)
		return err == nil
	case "uuid":
		_, err := uuid.Parse(value)
		return err == nil
	case "url":
		u, err := url.ParseRequestURI(value)
		return err == nil && u.Scheme != ""
	case "date":
		_, err := time.Parse("2006-01-02", value)
		return err == nil
	case "date-time":
		_, err := time.Parse(time.RFC3339, value)
		return err == nil
	default:
		return true
	}
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func toAnySlice(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		return parsed, err == nil
	}
	return 0, false
}

func toBoolValue(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(v) {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	}
	return false, false
}

func (s *Schema) enumContains(value string) bool {
	if len(s.Enum) == 0 {
		return true
	}
	if s.enumStringSet != nil {
		_, ok := s.enumStringSet[value]
		return ok
	}
	for _, item := range s.Enum {
		enumValue, ok := item.(string)
		if ok && enumValue == value {
			return true
		}
	}
	return false
}
