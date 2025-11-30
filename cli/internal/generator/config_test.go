package generator

import (
	"testing"

	"github.com/sflowg/sflowg/cli/internal/analyzer"
)

func TestGenerateConfigInit_NoConfig(t *testing.T) {
	data, err := GenerateConfigInit(nil, nil)
	if err != nil {
		t.Fatalf("GenerateConfigInit failed: %v", err)
	}

	if data != nil {
		t.Error("Expected nil data for nil config metadata")
	}
}

func TestGenerateConfigInit_EmptyConfig(t *testing.T) {
	metadata := &analyzer.ConfigMetadata{
		TypeName: "Config",
		Fields:   []analyzer.ConfigField{},
	}

	data, err := GenerateConfigInit(metadata, map[string]interface{}{})
	if err != nil {
		t.Fatalf("GenerateConfigInit failed: %v", err)
	}

	if data == nil {
		t.Fatal("Expected non-nil data")
	}

	if len(data.EnvVars) != 0 {
		t.Errorf("Expected 0 env vars, got %d", len(data.EnvVars))
	}

	if len(data.Literals) != 0 {
		t.Errorf("Expected 0 literals, got %d", len(data.Literals))
	}
}

func TestGenerateConfigInit_LiteralValues(t *testing.T) {
	metadata := &analyzer.ConfigMetadata{
		TypeName: "Config",
		Fields: []analyzer.ConfigField{
			{Name: "Port", Type: "int", YAMLTag: "port"},
			{Name: "Host", Type: "string", YAMLTag: "host"},
			{Name: "Enabled", Type: "bool", YAMLTag: "enabled"},
		},
	}

	userConfig := map[string]interface{}{
		"port":    8080,
		"host":    "localhost",
		"enabled": true,
	}

	data, err := GenerateConfigInit(metadata, userConfig)
	if err != nil {
		t.Fatalf("GenerateConfigInit failed: %v", err)
	}

	// Should have 3 literals, 0 env vars
	if len(data.Literals) != 3 {
		t.Fatalf("Expected 3 literals, got %d", len(data.Literals))
	}

	if len(data.EnvVars) != 0 {
		t.Errorf("Expected 0 env vars, got %d", len(data.EnvVars))
	}

	// Check port
	portLiteral := findLiteral(data.Literals, "Port")
	if portLiteral == nil {
		t.Fatal("Port literal not found")
	}
	if portLiteral.Value != "8080" {
		t.Errorf("Expected Port value='8080', got '%s'", portLiteral.Value)
	}

	// Check host
	hostLiteral := findLiteral(data.Literals, "Host")
	if hostLiteral == nil {
		t.Fatal("Host literal not found")
	}
	if hostLiteral.Value != `"localhost"` {
		t.Errorf("Expected Host value='\"localhost\"', got '%s'", hostLiteral.Value)
	}

	// Check enabled
	enabledLiteral := findLiteral(data.Literals, "Enabled")
	if enabledLiteral == nil {
		t.Fatal("Enabled literal not found")
	}
	if enabledLiteral.Value != "true" {
		t.Errorf("Expected Enabled value='true', got '%s'", enabledLiteral.Value)
	}
}

func TestGenerateConfigInit_EnvVarRequired(t *testing.T) {
	metadata := &analyzer.ConfigMetadata{
		TypeName: "Config",
		Fields: []analyzer.ConfigField{
			{Name: "APIKey", Type: "string", YAMLTag: "api_key"},
		},
	}

	userConfig := map[string]interface{}{
		"api_key": "${API_KEY}",
	}

	data, err := GenerateConfigInit(metadata, userConfig)
	if err != nil {
		t.Fatalf("GenerateConfigInit failed: %v", err)
	}

	if len(data.EnvVars) != 1 {
		t.Fatalf("Expected 1 env var, got %d", len(data.EnvVars))
	}

	envVar := data.EnvVars[0]
	if envVar.EnvVar != "API_KEY" {
		t.Errorf("Expected EnvVar='API_KEY', got '%s'", envVar.EnvVar)
	}

	if envVar.Field != "APIKey" {
		t.Errorf("Expected Field='APIKey', got '%s'", envVar.Field)
	}

	if !envVar.Required {
		t.Error("Expected Required=true for env var without default")
	}

	if envVar.DefaultValue != "" {
		t.Errorf("Expected empty DefaultValue, got '%s'", envVar.DefaultValue)
	}
}

func TestGenerateConfigInit_EnvVarWithDefault(t *testing.T) {
	metadata := &analyzer.ConfigMetadata{
		TypeName: "Config",
		Fields: []analyzer.ConfigField{
			{Name: "Addr", Type: "string", YAMLTag: "addr"},
		},
	}

	userConfig := map[string]interface{}{
		"addr": "${REDIS_ADDR:localhost:6379}",
	}

	data, err := GenerateConfigInit(metadata, userConfig)
	if err != nil {
		t.Fatalf("GenerateConfigInit failed: %v", err)
	}

	if len(data.EnvVars) != 1 {
		t.Fatalf("Expected 1 env var, got %d", len(data.EnvVars))
	}

	envVar := data.EnvVars[0]
	if envVar.EnvVar != "REDIS_ADDR" {
		t.Errorf("Expected EnvVar='REDIS_ADDR', got '%s'", envVar.EnvVar)
	}

	if envVar.Field != "Addr" {
		t.Errorf("Expected Field='Addr', got '%s'", envVar.Field)
	}

	if envVar.Required {
		t.Error("Expected Required=false for env var with default")
	}

	if envVar.DefaultValue != "localhost:6379" {
		t.Errorf("Expected DefaultValue='localhost:6379', got '%s'", envVar.DefaultValue)
	}
}

func TestGenerateConfigInit_MixedValues(t *testing.T) {
	metadata := &analyzer.ConfigMetadata{
		TypeName: "Config",
		Fields: []analyzer.ConfigField{
			{Name: "Addr", Type: "string", YAMLTag: "addr"},
			{Name: "Password", Type: "string", YAMLTag: "password"},
			{Name: "DB", Type: "int", YAMLTag: "db"},
		},
	}

	userConfig := map[string]interface{}{
		"addr":     "${REDIS_ADDR:localhost:6379}",
		"password": "${REDIS_PASSWORD}",
		"db":       2,
	}

	data, err := GenerateConfigInit(metadata, userConfig)
	if err != nil {
		t.Fatalf("GenerateConfigInit failed: %v", err)
	}

	// Should have 2 env vars (addr, password) and 1 literal (db)
	if len(data.EnvVars) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(data.EnvVars))
	}

	if len(data.Literals) != 1 {
		t.Errorf("Expected 1 literal, got %d", len(data.Literals))
	}

	// Check DB literal
	dbLiteral := findLiteral(data.Literals, "DB")
	if dbLiteral == nil {
		t.Fatal("DB literal not found")
	}
	if dbLiteral.Value != "2" {
		t.Errorf("Expected DB value='2', got '%s'", dbLiteral.Value)
	}
}

func TestGenerateConfigInit_NoUserConfig(t *testing.T) {
	metadata := &analyzer.ConfigMetadata{
		TypeName: "Config",
		Fields: []analyzer.ConfigField{
			{Name: "Port", Type: "int", YAMLTag: "port", DefaultTag: "8080"},
			{Name: "Host", Type: "string", YAMLTag: "host", DefaultTag: "localhost"},
		},
	}

	// No user config - should use defaults from struct tags
	data, err := GenerateConfigInit(metadata, map[string]interface{}{})
	if err != nil {
		t.Fatalf("GenerateConfigInit failed: %v", err)
	}

	// Should have no env vars or literals (using struct tag defaults)
	if len(data.EnvVars) != 0 {
		t.Errorf("Expected 0 env vars, got %d", len(data.EnvVars))
	}

	if len(data.Literals) != 0 {
		t.Errorf("Expected 0 literals, got %d", len(data.Literals))
	}
}

func TestFormatLiteralValue_String(t *testing.T) {
	result := formatLiteralValue("localhost", "string")
	expected := `"localhost"`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatLiteralValue_Int(t *testing.T) {
	result := formatLiteralValue("8080", "int")
	if result != "8080" {
		t.Errorf("Expected '8080', got '%s'", result)
	}
}

func TestFormatLiteralValue_Bool(t *testing.T) {
	result := formatLiteralValue("true", "bool")
	if result != "true" {
		t.Errorf("Expected 'true', got '%s'", result)
	}
}

func TestFormatLiteralValue_Duration(t *testing.T) {
	result := formatLiteralValue("30s", "time.Duration")
	expected := `time.ParseDuration("30s")`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestNeedsTypeConversion(t *testing.T) {
	tests := []struct {
		fieldType       string
		needsConversion bool
	}{
		{"string", false},
		{"int", true},
		{"int64", true},
		{"bool", true},
		{"time.Duration", true},
		{"float64", false},
		{"CustomType", false},
	}

	for _, test := range tests {
		result := NeedsTypeConversion(test.fieldType)
		if result != test.needsConversion {
			t.Errorf("NeedsTypeConversion(%q) = %v, expected %v",
				test.fieldType, result, test.needsConversion)
		}
	}
}

func TestGetConversionFuncName(t *testing.T) {
	tests := []struct {
		fieldType string
		funcName  string
	}{
		{"int", "parseInt"},
		{"int64", "parseInt64"},
		{"bool", "parseBool"},
		{"time.Duration", "parseDuration"},
		{"string", ""},
		{"unknown", ""},
	}

	for _, test := range tests {
		result := GetConversionFuncName(test.fieldType)
		if result != test.funcName {
			t.Errorf("GetConversionFuncName(%q) = %q, expected %q",
				test.fieldType, result, test.funcName)
		}
	}
}

// Helper function to find a literal by field name
func findLiteral(literals []ConfigLiteral, field string) *ConfigLiteral {
	for i := range literals {
		if literals[i].Field == field {
			return &literals[i]
		}
	}
	return nil
}
