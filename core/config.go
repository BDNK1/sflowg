package core

import (
	"github.com/BDNK1/sflowg/core/internal/configutil"
	"github.com/go-playground/validator/v10"
)

func InitializeConfig(config any, rawValues map[string]any) error {
	return configutil.Prepare(config, rawValues)
}

func RegisterCustomValidator(tag string, fn validator.Func) error {
	return configutil.RegisterCustomValidator(tag, fn)
}
