package generator

import (
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/cli/internal/analyzer"
	"github.com/BDNK1/sflowg/cli/internal/config"
)

// ConfigGenData contains all data needed to generate config initialization code
type ConfigGenData struct {
	// EnvVars are environment variable overrides to generate
	EnvVars []ConfigEnvVar

	// Literals are literal value assignments from flow-config.yaml
	Literals []ConfigLiteral
}

// ConfigEnvVar represents an environment variable override for a config field
type ConfigEnvVar struct {
	// EnvVar is the environment variable name (e.g., "REDIS_ADDR")
	EnvVar string

	// Field is the struct field name (e.g., "Addr")
	Field string

	// YAMLField is the YAML key for this field in rawConfig map (e.g., "addr")
	YAMLField string

	// Required indicates if the env var is required (no default)
	Required bool

	// DefaultValue is the default if not required
	DefaultValue string

	// FieldType is the Go type of the field (e.g., "string", "int", "time.Duration")
	FieldType string
}

// ConfigLiteral represents a literal value assignment from flow-config.yaml
type ConfigLiteral struct {
	// Field is the struct field name (e.g., "DB")
	Field string

	// YAMLField is the YAML key for this field in rawConfig map (e.g., "db")
	YAMLField string

	// Value is the Go code representation of the value (e.g., "2", `"localhost"`)
	Value string

	// FieldType is the Go type of the field
	FieldType string
}

// GenerateConfigInit generates config initialization data from metadata and user config
func GenerateConfigInit(metadata *analyzer.ConfigMetadata, userConfig map[string]interface{}) (*ConfigGenData, error) {
	if metadata == nil {
		return nil, nil
	}

	data := &ConfigGenData{
		EnvVars:  []ConfigEnvVar{},
		Literals: []ConfigLiteral{},
	}

	// Process each field in the Config struct
	for _, field := range metadata.Fields {
		// Check if user provided a value in flow-config.yaml
		yamlKey := field.YAMLTag
		if yamlKey == "" {
			// No yaml tag - use field name as key (lowercase)
			yamlKey = strings.ToLower(field.Name)
		}

		userValue, hasUserValue := userConfig[yamlKey]

		if !hasUserValue {
			// No user value - skip (will use default from struct tag)
			continue
		}

		// Parse the user value to determine if it's an env var or literal
		spec, err := config.ParseConfigValue(userValue)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}

		if spec.IsLiteral {
			// Literal value from flow-config.yaml
			literal := ConfigLiteral{
				Field:     field.Name,
				YAMLField: yamlKey,
				Value:     formatLiteralValue(spec.LiteralValue, field.Type),
				FieldType: field.Type,
			}
			data.Literals = append(data.Literals, literal)
		} else {
			// Environment variable
			envVar := ConfigEnvVar{
				EnvVar:       spec.VarName,
				Field:        field.Name,
				YAMLField:    yamlKey,
				Required:     !spec.HasDefault,
				DefaultValue: spec.DefaultValue,
				FieldType:    field.Type,
			}
			data.EnvVars = append(data.EnvVars, envVar)
		}
	}

	return data, nil
}

// formatLiteralValue converts a string literal to its Go code representation
// based on the target field type
func formatLiteralValue(value string, fieldType string) string {
	switch fieldType {
	case "string":
		// String literals need quotes and escaping
		return fmt.Sprintf("%q", value)

	case "int", "int8", "int16", "int32", "int64":
		// Numeric types - pass through (already parsed as number)
		return value

	case "uint", "uint8", "uint16", "uint32", "uint64":
		return value

	case "float32", "float64":
		return value

	case "bool":
		// Boolean - pass through
		return value

	case "time.Duration":
		// Duration strings like "30s" need quotes
		if value == "" {
			return "0"
		}
		return fmt.Sprintf("time.ParseDuration(%q)", value)

	default:
		// For complex types or unknown types, treat as string literal
		return fmt.Sprintf("%q", value)
	}
}

// formatEnvVarConversion generates the code to convert env var string to target type
func formatEnvVarConversion(varName, fieldType string) string {
	switch fieldType {
	case "string":
		// Direct assignment
		return varName

	case "int":
		return fmt.Sprintf("parseInt(%s)", varName)

	case "int64":
		return fmt.Sprintf("parseInt64(%s)", varName)

	case "bool":
		return fmt.Sprintf("parseBool(%s)", varName)

	case "time.Duration":
		return fmt.Sprintf("parseDuration(%s)", varName)

	default:
		// Default to string for unknown types
		return varName
	}
}

// NeedsTypeConversion checks if a field type needs conversion from string
func NeedsTypeConversion(fieldType string) bool {
	switch fieldType {
	case "string":
		return false
	case "int", "int64", "bool", "time.Duration":
		return true
	default:
		return false
	}
}

// GetConversionFuncName returns the helper function name for type conversion
func GetConversionFuncName(fieldType string) string {
	switch fieldType {
	case "int":
		return "parseInt"
	case "int64":
		return "parseInt64"
	case "bool":
		return "parseBool"
	case "time.Duration":
		return "parseDuration"
	default:
		return ""
	}
}
