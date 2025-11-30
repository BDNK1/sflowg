package config

import (
	"testing"
)

func TestParseEnvVar_RequiredVariable(t *testing.T) {
	spec, err := ParseEnvVar("${REDIS_ADDR}")
	if err != nil {
		t.Fatalf("ParseEnvVar failed: %v", err)
	}

	if spec.IsLiteral {
		t.Error("Expected IsLiteral=false for env var")
	}

	if spec.VarName != "REDIS_ADDR" {
		t.Errorf("Expected VarName='REDIS_ADDR', got '%s'", spec.VarName)
	}

	if spec.HasDefault {
		t.Error("Expected HasDefault=false for required variable")
	}

	if spec.DefaultValue != "" {
		t.Errorf("Expected empty DefaultValue, got '%s'", spec.DefaultValue)
	}
}

func TestParseEnvVar_WithDefault(t *testing.T) {
	spec, err := ParseEnvVar("${REDIS_ADDR:localhost:6379}")
	if err != nil {
		t.Fatalf("ParseEnvVar failed: %v", err)
	}

	if spec.IsLiteral {
		t.Error("Expected IsLiteral=false for env var")
	}

	if spec.VarName != "REDIS_ADDR" {
		t.Errorf("Expected VarName='REDIS_ADDR', got '%s'", spec.VarName)
	}

	if !spec.HasDefault {
		t.Error("Expected HasDefault=true")
	}

	if spec.DefaultValue != "localhost:6379" {
		t.Errorf("Expected DefaultValue='localhost:6379', got '%s'", spec.DefaultValue)
	}
}

func TestParseEnvVar_LiteralValue(t *testing.T) {
	spec, err := ParseEnvVar("localhost:6379")
	if err != nil {
		t.Fatalf("ParseEnvVar failed: %v", err)
	}

	if !spec.IsLiteral {
		t.Error("Expected IsLiteral=true for plain string")
	}

	if spec.LiteralValue != "localhost:6379" {
		t.Errorf("Expected LiteralValue='localhost:6379', got '%s'", spec.LiteralValue)
	}

	if spec.VarName != "" {
		t.Errorf("Expected empty VarName for literal, got '%s'", spec.VarName)
	}
}

func TestParseEnvVar_EmptyString(t *testing.T) {
	spec, err := ParseEnvVar("")
	if err != nil {
		t.Fatalf("ParseEnvVar failed: %v", err)
	}

	if !spec.IsLiteral {
		t.Error("Expected IsLiteral=true for empty string")
	}

	if spec.LiteralValue != "" {
		t.Errorf("Expected empty LiteralValue, got '%s'", spec.LiteralValue)
	}
}

func TestParseEnvVar_Underscore(t *testing.T) {
	tests := []string{
		"${_PRIVATE}",
		"${REDIS_CACHE_ADDR}",
		"${MY_VAR_123}",
	}

	for _, test := range tests {
		spec, err := ParseEnvVar(test)
		if err != nil {
			t.Errorf("ParseEnvVar(%q) failed: %v", test, err)
		}

		if spec.IsLiteral {
			t.Errorf("Expected IsLiteral=false for %q", test)
		}
	}
}

func TestParseEnvVar_WithEmptyDefault(t *testing.T) {
	spec, err := ParseEnvVar("${API_KEY:}")
	if err != nil {
		t.Fatalf("ParseEnvVar failed: %v", err)
	}

	if spec.IsLiteral {
		t.Error("Expected IsLiteral=false")
	}

	if spec.VarName != "API_KEY" {
		t.Errorf("Expected VarName='API_KEY', got '%s'", spec.VarName)
	}

	if !spec.HasDefault {
		t.Error("Expected HasDefault=true even for empty default")
	}

	if spec.DefaultValue != "" {
		t.Errorf("Expected empty DefaultValue, got '%s'", spec.DefaultValue)
	}
}

func TestParseEnvVar_DefaultWithSpecialChars(t *testing.T) {
	tests := []struct {
		input           string
		expectedVar     string
		expectedDefault string
	}{
		{"${DB_URL:postgres://localhost:5432/db}", "DB_URL", "postgres://localhost:5432/db"},
		{"${REDIS_ADDR:127.0.0.1:6379}", "REDIS_ADDR", "127.0.0.1:6379"},
		{"${PATH:/usr/local/bin}", "PATH", "/usr/local/bin"},
		{"${MESSAGE:Hello, World!}", "MESSAGE", "Hello, World!"},
	}

	for _, test := range tests {
		spec, err := ParseEnvVar(test.input)
		if err != nil {
			t.Errorf("ParseEnvVar(%q) failed: %v", test.input, err)
			continue
		}

		if spec.VarName != test.expectedVar {
			t.Errorf("ParseEnvVar(%q): expected VarName=%q, got %q",
				test.input, test.expectedVar, spec.VarName)
		}

		if spec.DefaultValue != test.expectedDefault {
			t.Errorf("ParseEnvVar(%q): expected DefaultValue=%q, got %q",
				test.input, test.expectedDefault, spec.DefaultValue)
		}
	}
}

func TestParseEnvVar_InvalidSyntax(t *testing.T) {
	tests := []string{
		"${lowercase}", // Lowercase not allowed
		"${123VAR}",    // Can't start with number
		"${VAR-NAME}",  // Hyphen not allowed
		"${VAR NAME}",  // Space not allowed
		"${VAR.NAME}",  // Dot not allowed
		"$VAR",         // Missing braces
		"${VAR",        // Missing closing brace
		"VAR}",         // Missing opening brace
		"${}",          // Empty variable name
	}

	for _, test := range tests {
		spec, err := ParseEnvVar(test)

		// These should be treated as literals (no error, but IsLiteral=true)
		if err != nil {
			t.Errorf("ParseEnvVar(%q) should not error, got: %v", test, err)
		}

		if !spec.IsLiteral {
			t.Errorf("ParseEnvVar(%q) should treat as literal, got IsLiteral=false", test)
		}

		if spec.LiteralValue != test {
			t.Errorf("ParseEnvVar(%q) should preserve value as literal, got %q",
				test, spec.LiteralValue)
		}
	}
}

func TestParseConfigValue_String(t *testing.T) {
	spec, err := ParseConfigValue("${DATABASE_URL}")
	if err != nil {
		t.Fatalf("ParseConfigValue failed: %v", err)
	}

	if spec.IsLiteral {
		t.Error("Expected IsLiteral=false")
	}

	if spec.VarName != "DATABASE_URL" {
		t.Errorf("Expected VarName='DATABASE_URL', got '%s'", spec.VarName)
	}
}

func TestParseConfigValue_Int(t *testing.T) {
	spec, err := ParseConfigValue(8080)
	if err != nil {
		t.Fatalf("ParseConfigValue failed: %v", err)
	}

	if !spec.IsLiteral {
		t.Error("Expected IsLiteral=true for int")
	}

	if spec.LiteralValue != "8080" {
		t.Errorf("Expected LiteralValue='8080', got '%s'", spec.LiteralValue)
	}
}

func TestParseConfigValue_Bool(t *testing.T) {
	spec, err := ParseConfigValue(true)
	if err != nil {
		t.Fatalf("ParseConfigValue failed: %v", err)
	}

	if !spec.IsLiteral {
		t.Error("Expected IsLiteral=true for bool")
	}

	if spec.LiteralValue != "true" {
		t.Errorf("Expected LiteralValue='true', got '%s'", spec.LiteralValue)
	}
}

func TestParseConfigValue_Nil(t *testing.T) {
	spec, err := ParseConfigValue(nil)
	if err != nil {
		t.Fatalf("ParseConfigValue failed: %v", err)
	}

	if !spec.IsLiteral {
		t.Error("Expected IsLiteral=true for nil")
	}

	if spec.LiteralValue != "" {
		t.Errorf("Expected empty LiteralValue for nil, got '%s'", spec.LiteralValue)
	}
}

func TestParseConfigValue_UnsupportedType(t *testing.T) {
	// Maps and slices are not supported
	tests := []interface{}{
		map[string]string{"key": "value"},
		[]string{"a", "b", "c"},
		[]int{1, 2, 3},
	}

	for _, test := range tests {
		_, err := ParseConfigValue(test)
		if err == nil {
			t.Errorf("ParseConfigValue(%T) should return error for unsupported type", test)
		}
	}
}

func TestMustParseEnvVar_Valid(t *testing.T) {
	// Should not panic
	spec := MustParseEnvVar("${VALID_VAR}")
	if spec.VarName != "VALID_VAR" {
		t.Errorf("Expected VarName='VALID_VAR', got '%s'", spec.VarName)
	}
}

func TestMustParseEnvVar_Panic(t *testing.T) {
	// Note: This test is commented out because MustParseEnvVar currently
	// treats invalid syntax as literals rather than panicking
	// If we want it to panic on invalid syntax, we'd need to change the behavior

	// defer func() {
	// 	if r := recover(); r == nil {
	// 		t.Error("MustParseEnvVar should panic on invalid input")
	// 	}
	// }()
	//
	// MustParseEnvVar("${invalid-name}")
}

func TestIsValidEnvVarName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"VALID_NAME", true},
		{"_PRIVATE", true},
		{"VAR123", true},
		{"MY_VAR_123", true},
		{"lowercase", false},
		{"123VAR", false},
		{"VAR-NAME", false},
		{"VAR NAME", false},
		{"VAR.NAME", false},
		{"", false},
	}

	for _, test := range tests {
		result := isValidEnvVarName(test.name)
		if result != test.valid {
			t.Errorf("isValidEnvVarName(%q) = %v, expected %v",
				test.name, result, test.valid)
		}
	}
}
