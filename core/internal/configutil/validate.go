package configutil

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
	registerCustomValidators()
}

func ValidateStruct(config any) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := validate.Struct(config); err != nil {
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

func RegisterCustomValidator(tag string, fn validator.Func) error {
	if err := validate.RegisterValidation(tag, fn); err != nil {
		return fmt.Errorf("failed to register custom validator '%s': %w", tag, err)
	}
	return nil
}

func registerCustomValidators() {
	validate.RegisterValidation("hostname_port", func(fl validator.FieldLevel) bool {
		addr := fl.Field().String()
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" || port == "" {
			return false
		}
		_, err = net.LookupPort("tcp", port)
		return err == nil
	})

	validate.RegisterValidation("url_format", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		u, err := url.Parse(s)
		return err == nil && u.Scheme != "" && u.Host != ""
	})

	validate.RegisterValidation("dsn", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		if strings.Contains(s, "://") {
			_, err := url.Parse(s)
			return err == nil
		}
		return strings.Contains(s, "@") && strings.Contains(s, "/")
	})
}
