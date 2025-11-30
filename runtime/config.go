package runtime

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strings"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
)

// Package-level validator instance
var validate *validator.Validate

// init initializes the validator and registers custom validation functions
func init() {
	validate = validator.New()

	// Register custom validators
	registerCustomValidators()
}

// InitializeConfig is the CLI API for config preparation
// This is the ONLY function CLI-generated code should call for config handling.
// It combines: defaults → value merging → validation in one call.
//
// Usage in generated code:
//
//	httpConfig := http.Config{}
//	rawValues := map[string]any{
//	    "timeout":     os.Getenv("HTTP_TIMEOUT"),
//	    "max_retries": 3,
//	}
//	if err := runtime.InitializeConfig(&httpConfig, rawValues); err != nil {
//	    panic(err)
//	}
//
// The function:
// 1. Applies defaults from struct tags (`default:"..."`)
// 2. Merges rawValues (from env vars + flow-config.yaml)
// 3. Validates using validation tags (`validate:"..."`)
//
// Returns error if any step fails.
func InitializeConfig(config any, rawValues map[string]any) error {
	// Step 1: Apply defaults from struct tags
	if err := prepareConfig(config); err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
	}

	// Step 2: Merge raw values (env vars + literals from flow-config.yaml)
	if len(rawValues) > 0 {
		if err := mapToStruct(rawValues, config); err != nil {
			return fmt.Errorf("failed to apply config values: %w", err)
		}
	}

	// Step 3: Validate final config
	// Extract the actual value if config is a pointer
	configValue := reflect.ValueOf(config)
	if configValue.Kind() == reflect.Ptr {
		configValue = configValue.Elem()
	}

	if err := validateConfig(configValue.Interface()); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// registerCustomValidators registers framework-provided custom validation functions
func registerCustomValidators() {
	// hostname_port validates "host:port" format with numeric port
	validate.RegisterValidation("hostname_port", func(fl validator.FieldLevel) bool {
		addr := fl.Field().String()
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" || port == "" {
			return false
		}
		// Verify port is a valid number in range 1-65535
		_, err = net.LookupPort("tcp", port)
		return err == nil
	})

	// url_format validates URL structure
	validate.RegisterValidation("url_format", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		u, err := url.Parse(s)
		return err == nil && u.Scheme != "" && u.Host != ""
	})

	// dsn validates database connection string format
	// Checks for either URL format (scheme://...) or traditional DSN (user@host...)
	validate.RegisterValidation("dsn", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		// Check for URL format (postgres://..., mysql://..., etc.)
		if strings.Contains(s, "://") {
			_, err := url.Parse(s)
			return err == nil
		}
		// Check for traditional DSN format (user:pass@host/db)
		return strings.Contains(s, "@") && strings.Contains(s, "/")
	})
}

// ApplyDefaults applies default values from struct tags to the config.
//
// This function reads `default:"..."` tags on struct fields and sets the field
// values if they are currently zero/empty.
//
// Supported default value formats:
//   - Strings: default:"localhost"
//   - Numbers: default:"8080"
//   - Booleans: default:"true" or default:"false"
//   - Durations: default:"30s", default:"5m", default:"1h"
//   - Lists: default:"[1,2,3]"
//
// Example:
//
//	type Config struct {
//	    Addr     string        `default:"localhost:6379"`
//	    Port     int           `default:"6379"`
//	    Timeout  time.Duration `default:"30s"`
//	    Enabled  bool          `default:"true"`
//	}
//
//	config := Config{}
//	err := ApplyDefaults(&config)
//	// config.Addr = "localhost:6379"
//	// config.Port = 6379
//	// config.Timeout = 30 * time.Second
//	// config.Enabled = true
//
// Note: This function is framework-internal. Plugin developers never call it directly.
// The framework automatically applies defaults before plugin initialization.
func ApplyDefaults(config any) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := defaults.Set(config); err != nil {
		return fmt.Errorf("failed to apply default values: %w", err)
	}

	return nil
}

// ValidateConfig validates the config struct using validation tags.
//
// This function reads `validate:"..."` tags on struct fields and validates
// the field values according to the specified rules.
//
// Common validation rules:
//   - required: Field must not be empty/zero
//   - gte=N, lte=N: Numeric greater/less than or equal
//   - min=N, max=N: String/slice length constraints
//   - email: Email format validation
//   - url: URL format validation
//   - oneof=a b c: Value must be one of the listed options
//
// Custom validators provided by framework:
//   - hostname_port: Validates "host:port" format
//   - url_format: Validates URL structure with scheme and host
//   - dsn: Validates database connection string format
//
// Example:
//
//	type Config struct {
//	    Port    int    `validate:"gte=1,lte=65535"`
//	    Email   string `validate:"required,email"`
//	    Addr    string `validate:"required,hostname_port"`
//	    Level   string `validate:"oneof=debug info warn error"`
//	}
//
//	config := Config{Port: 70000, Email: "invalid", Addr: "localhost"}
//	err := ValidateConfig(config)
//	// Returns validation errors with details about which fields failed
//
// validateConfig validates a config struct using validation tags (internal function)
func validateConfig(config any) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := validate.Struct(config); err != nil {
		// Format validation errors for better readability
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			var errMessages []string
			for _, fieldErr := range validationErrors {
				errMessages = append(errMessages, fmt.Sprintf(
					"field '%s' failed validation: %s (rule: %s)",
					fieldErr.Field(),
					fieldErr.Error(),
					fieldErr.Tag(),
				))
			}
			return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errMessages, "\n  - "))
		}
		return fmt.Errorf("config validation failed: %w", err)
	}

	return nil
}

// PrepareConfig is a convenience function that applies defaults and then validates the config.
//
// This combines ApplyDefaults and ValidateConfig in the correct order:
//  1. Apply default values from tags
//  2. Validate the config with all rules
//
// This is the recommended way to prepare plugin configs as it ensures defaults
// are applied before validation, which is the expected order.
//
// Example:
//
//	type Config struct {
//	    Addr     string        `default:"localhost:6379" validate:"required,hostname_port"`
//	    Port     int           `default:"6379" validate:"gte=1,lte=65535"`
//	    Timeout  time.Duration `default:"30s" validate:"gte=1s"`
//	}
//
//	config := Config{}
//	if err := PrepareConfig(&config); err != nil {
//	    log.Fatal("Config preparation failed:", err)
//	}
//	// config is now ready with defaults applied and validation passed
//
// prepareConfig applies defaults and validates config (internal function)
func prepareConfig(config any) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Step 1: Apply defaults
	if err := ApplyDefaults(config); err != nil {
		return fmt.Errorf("failed to prepare config (defaults): %w", err)
	}

	// Step 2: Validate
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("failed to prepare config (validation): %w", err)
	}

	return nil
}

// RegisterCustomValidator allows registering additional custom validation functions.
//
// This can be used by plugins or the framework to add domain-specific validators
// beyond the standard ones provided by go-playground/validator.
//
// Example:
//
//	// Register a custom validator for specific format
//	RegisterCustomValidator("custom_format", func(fl validator.FieldLevel) bool {
//	    value := fl.Field().String()
//	    // Custom validation logic
//	    return isValidCustomFormat(value)
//	})
//
//	// Use in config struct
//	type Config struct {
//	    Code string `validate:"required,custom_format"`
//	}
//
// Note: This function is provided for extensibility but is rarely needed.
// Most common validation rules are already available.
func RegisterCustomValidator(tag string, fn validator.Func) error {
	if err := validate.RegisterValidation(tag, fn); err != nil {
		return fmt.Errorf("failed to register custom validator '%s': %w", tag, err)
	}
	return nil
}

// GetValidator returns the package-level validator instance.
//
// This is provided for advanced use cases where direct access to the validator
// is needed (e.g., for programmatic validation or inspection).
//
// Most users should use ValidateConfig or PrepareConfig instead.
func GetValidator() *validator.Validate {
	return validate
}
