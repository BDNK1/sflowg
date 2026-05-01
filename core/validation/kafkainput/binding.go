package kafkainput

import (
	"fmt"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/schema"
)

const ValueNamespace = "message.value"

func ParseBinding(config map[string]any) (*runtime.InputContract, error) {
	valueRaw, ok := config["value"]
	if !ok {
		return nil, nil
	}
	valueMap, ok := valueRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("value must be a map")
	}
	schemaRaw, ok := valueMap["schema"]
	if !ok {
		return nil, nil
	}
	valueSchema, err := schema.ParseRoot(schemaRaw)
	if err != nil {
		return nil, fmt.Errorf("value.schema: %w", err)
	}
	contract := runtime.NewInputContract()
	contract.SetRoot(ValueNamespace, valueSchema)
	return contract, nil
}
