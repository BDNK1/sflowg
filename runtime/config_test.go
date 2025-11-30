package runtime

import (
	"strings"
	"testing"
	"time"
)

// Test configs for various scenarios

type BasicConfig struct {
	Name    string `default:"default-name"`
	Port    int    `default:"8080"`
	Enabled bool   `default:"true"`
}

type RequiredFieldConfig struct {
	Required string `validate:"required"`
}

type PortValidationConfig struct {
	Port int `validate:"gte=1,lte=65535"`
}

type EmailValidationConfig struct {
	Email string `validate:"email"`
}

type LevelValidationConfig struct {
	Level string `validate:"oneof=debug info warn error"`
}

type DurationConfig struct {
	Timeout       time.Duration `default:"30s"`
	RetryInterval time.Duration `default:"5m"`
	MaxWait       time.Duration `default:"1h"`
}

type ComplexConfig struct {
	Addr     string        `default:"localhost:6379" validate:"required,hostname_port"`
	Password string        `yaml:"password"`
	DB       int           `default:"0" validate:"gte=0,lte=15"`
	PoolSize int           `default:"10" validate:"gte=1,lte=1000"`
	Timeout  time.Duration `default:"30s" validate:"gte=1s"`
}

type HostnamePortValidatorConfig struct {
	HostPort string `validate:"hostname_port"`
}

type URLValidatorConfig struct {
	URL string `validate:"url_format"`
}

type DSNValidatorConfig struct {
	DSN string `validate:"dsn"`
}

// Tests for ApplyDefaults

func TestApplyDefaults_BasicTypes(t *testing.T) {
	config := BasicConfig{}

	err := ApplyDefaults(&config)
	if err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	if config.Name != "default-name" {
		t.Errorf("Expected Name='default-name', got '%s'", config.Name)
	}
	if config.Port != 8080 {
		t.Errorf("Expected Port=8080, got %d", config.Port)
	}
	if !config.Enabled {
		t.Errorf("Expected Enabled=true, got false")
	}
}

func TestApplyDefaults_Durations(t *testing.T) {
	config := DurationConfig{}

	err := ApplyDefaults(&config)
	if err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout=30s, got %v", config.Timeout)
	}
	if config.RetryInterval != 5*time.Minute {
		t.Errorf("Expected RetryInterval=5m, got %v", config.RetryInterval)
	}
	if config.MaxWait != 1*time.Hour {
		t.Errorf("Expected MaxWait=1h, got %v", config.MaxWait)
	}
}

func TestApplyDefaults_NonZeroValuesUnchanged(t *testing.T) {
	config := BasicConfig{
		Name:    "custom-name",
		Port:    9000,
		Enabled: false,
	}

	err := ApplyDefaults(&config)
	if err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	// Non-zero values should remain unchanged
	if config.Name != "custom-name" {
		t.Errorf("Expected Name='custom-name', got '%s'", config.Name)
	}
	if config.Port != 9000 {
		t.Errorf("Expected Port=9000, got %d", config.Port)
	}
	// Note: false is zero value for bool, so default would apply
}

func TestApplyDefaults_NilConfig(t *testing.T) {
	err := ApplyDefaults(nil)
	if err == nil {
		t.Error("Expected error for nil config, got nil")
	}
}

// Tests for validateConfig

func TestValidateConfig_RequiredField(t *testing.T) {
	// Valid config
	config := RequiredFieldConfig{Required: "value"}
	err := validateConfig(config)
	if err != nil {
		t.Errorf("validateConfig failed for valid config: %v", err)
	}

	// Invalid config (missing required field)
	invalidConfig := RequiredFieldConfig{}
	err = validateConfig(invalidConfig)
	if err == nil {
		t.Error("Expected validation error for missing required field, got nil")
	}
	if !strings.Contains(err.Error(), "Required") {
		t.Errorf("Expected error to mention 'Required', got: %v", err)
	}
}

func TestValidateConfig_NumericRange(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		shouldErr bool
	}{
		{"valid minimum", 1, false},
		{"valid middle", 8080, false},
		{"valid maximum", 65535, false},
		{"invalid too low", 0, true},
		{"invalid too high", 70000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := PortValidationConfig{
				Port: tt.port,
			}
			err := validateConfig(config)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected validation error for port %d, got nil", tt.port)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error for port %d, got: %v", tt.port, err)
			}
		})
	}
}

func TestValidateConfig_EmailFormat(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		shouldErr bool
	}{
		{"valid email", "user@example.com", false},
		{"valid with subdomain", "user@mail.example.com", false},
		{"invalid no @", "userexample.com", true},
		{"invalid no domain", "user@", true},
		{"invalid no user", "@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := EmailValidationConfig{
				Email: tt.email,
			}
			err := validateConfig(config)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected validation error for email '%s', got nil", tt.email)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error for email '%s', got: %v", tt.email, err)
			}
		})
	}
}

func TestValidateConfig_OneOf(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}
	for _, level := range validLevels {
		config := LevelValidationConfig{
			Level: level,
		}
		err := validateConfig(config)
		if err != nil {
			t.Errorf("Expected no error for level '%s', got: %v", level, err)
		}
	}

	// Invalid level
	config := LevelValidationConfig{
		Level: "invalid",
	}
	err := validateConfig(config)
	if err == nil {
		t.Error("Expected validation error for invalid level, got nil")
	}
}

func TestValidateConfig_NilConfig(t *testing.T) {
	err := validateConfig(nil)
	if err == nil {
		t.Error("Expected error for nil config, got nil")
	}
}

// Tests for custom validators

func TestCustomValidator_HostnamePort(t *testing.T) {
	tests := []struct {
		name      string
		hostPort  string
		shouldErr bool
	}{
		{"valid localhost", "localhost:6379", false},
		{"valid IP", "192.168.1.1:8080", false},
		{"valid hostname", "redis.example.com:6379", false},
		{"valid IPv6", "[::1]:8080", false},
		{"invalid no port", "localhost", true},
		{"invalid no host", ":8080", true},
		{"invalid format", "localhost:port", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := HostnamePortValidatorConfig{
				HostPort: tt.hostPort,
			}
			err := validateConfig(config)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected validation error for '%s', got nil", tt.hostPort)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error for '%s', got: %v", tt.hostPort, err)
			}
		})
	}
}

func TestCustomValidator_URLFormat(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		shouldErr bool
	}{
		{"valid HTTP", "http://example.com", false},
		{"valid HTTPS", "https://example.com/path", false},
		{"valid with port", "https://example.com:8080", false},
		{"valid with query", "https://example.com?key=value", false},
		{"invalid no scheme", "example.com", true},
		{"invalid no host", "http://", true},
		{"invalid malformed", "http://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := URLValidatorConfig{
				URL: tt.url,
			}
			err := validateConfig(config)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected validation error for '%s', got nil", tt.url)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error for '%s', got: %v", tt.url, err)
			}
		})
	}
}

func TestCustomValidator_DSN(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		shouldErr bool
	}{
		{"valid postgres URL", "postgres://user:pass@localhost:5432/db", false},
		{"valid mysql URL", "mysql://user:pass@localhost:3306/db", false},
		{"valid traditional DSN", "user:pass@tcp(localhost:3306)/db", false},
		{"valid with special chars", "user:p@ss@localhost/db", false},
		{"invalid no scheme or @", "localhost:5432/db", true},
		{"invalid no database", "user:pass@localhost", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DSNValidatorConfig{
				DSN: tt.dsn,
			}
			err := validateConfig(config)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected validation error for '%s', got nil", tt.dsn)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error for '%s', got: %v", tt.dsn, err)
			}
		})
	}
}

// Tests for prepareConfig

func TestPrepareConfig_Success(t *testing.T) {
	config := ComplexConfig{}

	err := prepareConfig(&config)
	if err != nil {
		t.Fatalf("prepareConfig failed: %v", err)
	}

	// Check defaults were applied
	if config.Addr != "localhost:6379" {
		t.Errorf("Expected Addr='localhost:6379', got '%s'", config.Addr)
	}
	if config.DB != 0 {
		t.Errorf("Expected DB=0, got %d", config.DB)
	}
	if config.PoolSize != 10 {
		t.Errorf("Expected PoolSize=10, got %d", config.PoolSize)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout=30s, got %v", config.Timeout)
	}

	// All fields should pass validation
}

func TestPrepareConfig_ValidationFailsAfterDefaults(t *testing.T) {
	config := ComplexConfig{
		Addr:     "invalid-format", // Will fail hostname_port validation
		PoolSize: 5000,             // Will fail gte/lte validation
	}

	err := prepareConfig(&config)
	if err == nil {
		t.Error("Expected prepareConfig to fail validation, got nil")
	}

	// Should mention validation failure
	if !strings.Contains(err.Error(), "validation") {
		t.Errorf("Expected error to mention 'validation', got: %v", err)
	}
}

func TestPrepareConfig_NilConfig(t *testing.T) {
	err := prepareConfig(nil)
	if err == nil {
		t.Error("Expected error for nil config, got nil")
	}
}

// Tests for complex scenarios

func TestComplexConfig_FullFlow(t *testing.T) {
	// Simulate CLI-generated code flow
	config := ComplexConfig{}

	// Step 1: Prepare (defaults + validation)
	if err := prepareConfig(&config); err != nil {
		t.Fatalf("prepareConfig failed: %v", err)
	}

	// Step 2: Apply overrides (simulating environment variables or YAML literals)
	config.Addr = "redis.prod.com:6379"
	config.Password = "secret123"
	config.PoolSize = 20

	// Step 3: Validate again after overrides
	if err := validateConfig(config); err != nil {
		t.Fatalf("validateConfig failed after overrides: %v", err)
	}

	// Verify final values
	if config.Addr != "redis.prod.com:6379" {
		t.Errorf("Expected overridden Addr, got '%s'", config.Addr)
	}
	if config.Password != "secret123" {
		t.Errorf("Expected overridden Password, got '%s'", config.Password)
	}
	if config.PoolSize != 20 {
		t.Errorf("Expected overridden PoolSize, got %d", config.PoolSize)
	}
	// DB should still have default
	if config.DB != 0 {
		t.Errorf("Expected default DB=0, got %d", config.DB)
	}
}

// Benchmark tests

func BenchmarkApplyDefaults(b *testing.B) {
	for i := 0; i < b.N; i++ {
		config := ComplexConfig{}
		ApplyDefaults(&config)
	}
}

func BenchmarkValidateConfig(b *testing.B) {
	config := ComplexConfig{
		Addr:     "localhost:6379",
		DB:       0,
		PoolSize: 10,
		Timeout:  30 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateConfig(config)
	}
}

func BenchmarkPrepareConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		config := ComplexConfig{}
		prepareConfig(&config)
	}
}
