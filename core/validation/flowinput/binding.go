package flowinput

import (
	"fmt"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/schema"
)

const InputNamespace = "input"

func ParseBinding(config map[string]any) (*runtime.InputContract, error) {
	raw, ok := config["input"]
	if !ok {
		return nil, nil
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("input must be a map")
	}
	fields, err := schema.ParseFieldMap(rawMap)
	if err != nil {
		return nil, fmt.Errorf("input: %w", err)
	}
	contract := runtime.NewInputContract()
	contract.SetFields(InputNamespace, fields)
	return contract, nil
}
