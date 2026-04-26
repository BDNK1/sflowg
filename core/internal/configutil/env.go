package configutil

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var envVarPattern = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)(:[^}]*)?\}$`)

func ResolvePropertyMap(props map[string]any) (map[string]any, error) {
	if len(props) == 0 {
		return map[string]any{}, nil
	}

	resolved := make(map[string]any, len(props))
	for key, value := range props {
		resolvedValue, err := resolveEnvVar(value)
		if err != nil {
			return nil, fmt.Errorf("property %s: %w", key, err)
		}
		resolved[key] = resolvedValue
	}

	return resolved, nil
}

func resolveEnvVar(value any) (any, error) {
	strValue, ok := value.(string)
	if !ok {
		return value, nil
	}

	matches := envVarPattern.FindStringSubmatch(strValue)
	if matches == nil {
		return value, nil
	}

	varName := matches[1]
	defaultPart := matches[2]

	envValue, exists := os.LookupEnv(varName)
	if exists {
		return envValue, nil
	}

	if defaultPart != "" {
		return strings.TrimPrefix(defaultPart, ":"), nil
	}

	return nil, fmt.Errorf("required environment variable not set: %s", varName)
}
