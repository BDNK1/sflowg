package runtime

import (
	"fmt"
	"log/slog"
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
func InitializeConfig(config any, rawValues map[string]any) error {
	// Step 1: Apply defaults from struct tags
	if err := ApplyDefaults(config); err != nil {
		slog.Error("Plugin config: failed to apply defaults",
			"config_type", reflect.TypeOf(config).String(),
			"error", err)
		return fmt.Errorf("failed to apply defaults: %w", err)
	}

	// Step 2: Merge raw values (env vars + literals from flow-config.yaml)
	// Use YAML tags because Config structs use yaml tags for field mapping
	if len(rawValues) > 0 {
		if err := mapToStructFromYAML(rawValues, config); err != nil {
			slog.Error("Plugin config: failed to apply config values",
				"config_type", reflect.TypeOf(config).String(),
				"raw_values", rawValues,
				"error", err)
			return fmt.Errorf("failed to apply config values: %w", err)
		}
	}

	// Step 3: Validate final config (AFTER rawValues are merged)
	// Extract the actual value if config is a pointer
	configValue := reflect.ValueOf(config)
	if configValue.Kind() == reflect.Ptr {
		configValue = configValue.Elem()
	}

	if err := validateConfig(configValue.Interface()); err != nil {
		slog.Error("Plugin config validation failed",
			"config_type", reflect.TypeOf(config).String(),
			"config_value", configValue.Interface(),
			"error", err)
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

func ApplyDefaults(config any) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := defaults.Set(config); err != nil {
		return fmt.Errorf("failed to apply default values: %w", err)
	}

	return nil
}

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

func RegisterCustomValidator(tag string, fn validator.Func) error {
	if err := validate.RegisterValidation(tag, fn); err != nil {
		return fmt.Errorf("failed to register custom validator '%s': %w", tag, err)
	}
	return nil
}
