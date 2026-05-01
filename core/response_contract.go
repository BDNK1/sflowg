package runtime

import (
	"fmt"
	"slices"
)

type ResponseArgRule int

const (
	ResponseArgsMap ResponseArgRule = iota
	ResponseArgsNone
)

type ResponseSubtypeContract struct {
	Name string
	Args ResponseArgRule
}

type ResponseContract struct {
	EntrypointType   string
	RequiresResponse bool
	subtypes         []ResponseSubtypeContract
	byName           map[string]ResponseSubtypeContract
}

func NewResponseContract(entrypointType string, subtypes ...ResponseSubtypeContract) ResponseContract {
	byName := make(map[string]ResponseSubtypeContract, len(subtypes))
	names := make([]ResponseSubtypeContract, len(subtypes))
	copy(names, subtypes)
	for _, subtype := range names {
		byName[subtype.Name] = subtype
	}
	return ResponseContract{
		EntrypointType:   entrypointType,
		RequiresResponse: true,
		subtypes:         names,
		byName:           byName,
	}
}

func HTTPResponseContract() ResponseContract {
	return NewResponseContract("http",
		ResponseSubtypeContract{Name: "json", Args: ResponseArgsMap},
		ResponseSubtypeContract{Name: "text", Args: ResponseArgsMap},
		ResponseSubtypeContract{Name: "redirect", Args: ResponseArgsMap},
	)
}

func FlowResponseContract() ResponseContract {
	return NewResponseContract("flow",
		ResponseSubtypeContract{Name: "value", Args: ResponseArgsMap},
		ResponseSubtypeContract{Name: "error", Args: ResponseArgsMap},
	)
}

func KafkaResponseContract() ResponseContract {
	return NewResponseContract("kafka",
		ResponseSubtypeContract{Name: "ack", Args: ResponseArgsNone},
		ResponseSubtypeContract{Name: "nack", Args: ResponseArgsNone},
	)
}

func CronResponseContract() ResponseContract {
	return ResponseContract{
		EntrypointType:   "cron",
		RequiresResponse: false,
		subtypes:         []ResponseSubtypeContract{},
		byName:           map[string]ResponseSubtypeContract{},
	}
}

func BuiltInResponseContract(entrypointType string) (ResponseContract, bool) {
	switch entrypointType {
	case "", "http":
		return HTTPResponseContract(), true
	case "flow":
		return FlowResponseContract(), true
	case "kafka":
		return KafkaResponseContract(), true
	case "cron":
		return CronResponseContract(), true
	default:
		return ResponseContract{}, false
	}
}

func (c ResponseContract) Subtypes() []string {
	result := make([]string, 0, len(c.subtypes))
	for _, subtype := range c.subtypes {
		result = append(result, subtype.Name)
	}
	return result
}

func (c ResponseContract) IsZero() bool {
	return c.EntrypointType == "" && !c.RequiresResponse && len(c.subtypes) == 0
}

func (c ResponseContract) HasSubtype(name string) bool {
	_, ok := c.byName[name]
	return ok
}

func (c ResponseContract) ValidateArgs(subtype string, args []any) (map[string]any, error) {
	spec, ok := c.byName[subtype]
	if !ok {
		return nil, fmt.Errorf("response.%s is not valid for this entrypoint", subtype)
	}
	switch spec.Args {
	case ResponseArgsNone:
		if len(args) != 0 {
			return nil, fmt.Errorf("response.%s expects no arguments", subtype)
		}
		return map[string]any{}, nil
	case ResponseArgsMap:
		if len(args) != 1 {
			return nil, fmt.Errorf("response.%s expects one map argument", subtype)
		}
		m, ok := args[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("response.%s expects map argument, got %T", subtype, args[0])
		}
		return m, nil
	default:
		return nil, fmt.Errorf("response.%s has unsupported argument rule", subtype)
	}
}

func (c ResponseContract) ValidateStaticArgCount(subtype string, count int) error {
	spec, ok := c.byName[subtype]
	if !ok {
		return fmt.Errorf("response.%s is not valid for this entrypoint", subtype)
	}
	switch spec.Args {
	case ResponseArgsNone:
		if count != 0 {
			return fmt.Errorf("response.%s expects no arguments", subtype)
		}
	case ResponseArgsMap:
		if count != 1 {
			return fmt.Errorf("response.%s expects one map argument", subtype)
		}
	}
	return nil
}

func (c ResponseContract) EqualSubtypes(names []string) bool {
	return slices.Equal(c.Subtypes(), names)
}
