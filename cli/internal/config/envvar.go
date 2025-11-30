package config

import (
	"fmt"
	"regexp"
	"strings"
)

// EnvVarSpec represents a parsed environment variable specification
type EnvVarSpec struct {
	// VarName is the environment variable name (e.g., "REDIS_ADDR")
	VarName string

	// HasDefault indicates if a default value was provided
	HasDefault bool

	// DefaultValue is the default value if HasDefault is true
	DefaultValue string

	// IsLiteral indicates if this is a literal value (not an env var)
	IsLiteral bool

	// LiteralValue is the literal value if IsLiteral is true
	LiteralValue string
}

// envVarPattern matches ${VAR} and ${VAR:default} syntax
var envVarPattern = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)(:[^}]*)?\}$`)

// ParseEnvVar parses a config value that may contain environment variable syntax
//
// Supported formats:
//   - ${VAR}         - Required environment variable
//   - ${VAR:default} - Optional environment variable with default
//   - literal        - Plain literal value (no env var)
//
// Examples:
//
//	ParseEnvVar("${REDIS_ADDR}") -> required env var "REDIS_ADDR"
//	ParseEnvVar("${REDIS_ADDR:localhost:6379}") -> env var with default
//	ParseEnvVar("localhost:6379") -> literal value
//	ParseEnvVar("${INVALID-NAME}") -> error (invalid variable name)
func ParseEnvVar(value string) (*EnvVarSpec, error) {
	if value == "" {
		return &EnvVarSpec{
			IsLiteral:    true,
			LiteralValue: "",
		}, nil
	}

	// Check if it matches env var pattern
	matches := envVarPattern.FindStringSubmatch(value)
	if matches == nil {
		// Not an env var pattern - treat as literal
		return &EnvVarSpec{
			IsLiteral:    true,
			LiteralValue: value,
		}, nil
	}

	varName := matches[1]
	defaultPart := matches[2] // Will be ":default" or empty

	// Validate variable name (already validated by regex, but double-check)
	if !isValidEnvVarName(varName) {
		return nil, fmt.Errorf("invalid environment variable name: %s", varName)
	}

	spec := &EnvVarSpec{
		VarName:    varName,
		IsLiteral:  false,
		HasDefault: defaultPart != "",
	}

	// Extract default value if present
	if spec.HasDefault {
		// Remove leading colon
		spec.DefaultValue = strings.TrimPrefix(defaultPart, ":")
	}

	return spec, nil
}

// isValidEnvVarName checks if a string is a valid environment variable name
// Valid names: Start with A-Z or underscore, contain only A-Z, 0-9, underscore
func isValidEnvVarName(name string) bool {
	if name == "" {
		return false
	}

	// First character must be A-Z or underscore
	first := name[0]
	if !((first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}

	// Remaining characters must be A-Z, 0-9, or underscore
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}

	return true
}

// ParseConfigValue parses a config value from flow-config.yaml
// This is a convenience wrapper around ParseEnvVar for use in config loading
func ParseConfigValue(value interface{}) (*EnvVarSpec, error) {
	// Handle different value types
	switch v := value.(type) {
	case string:
		return ParseEnvVar(v)
	case int, int32, int64, float32, float64, bool:
		// Numeric/boolean literal values
		return &EnvVarSpec{
			IsLiteral:    true,
			LiteralValue: fmt.Sprintf("%v", v),
		}, nil
	case nil:
		// Null value
		return &EnvVarSpec{
			IsLiteral:    true,
			LiteralValue: "",
		}, nil
	default:
		// Complex types (maps, slices) are not supported for env var substitution
		return nil, fmt.Errorf("unsupported config value type: %T", value)
	}
}

// MustParseEnvVar is like ParseEnvVar but panics on error
// Useful for testing and static initialization
func MustParseEnvVar(value string) *EnvVarSpec {
	spec, err := ParseEnvVar(value)
	if err != nil {
		panic(fmt.Sprintf("MustParseEnvVar failed: %v", err))
	}
	return spec
}
