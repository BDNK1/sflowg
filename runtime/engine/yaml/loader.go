package yaml

import (
	"fmt"
	"os"

	"github.com/BDNK1/sflowg/runtime"
	goyaml "gopkg.in/yaml.v3"
)

// FlowLoader loads flow definitions from YAML files.
type FlowLoader struct{}

func NewFlowLoader() *FlowLoader {
	return &FlowLoader{}
}

func (l *FlowLoader) Extensions() []string {
	return []string{"*.yaml"}
}

func (l *FlowLoader) Load(filePath string) (runtime.Flow, error) {
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return runtime.Flow{}, fmt.Errorf("error reading YAML file: %w", err)
	}

	var flow runtime.Flow
	err = goyaml.Unmarshal(yamlFile, &flow)
	if err != nil {
		return runtime.Flow{}, fmt.Errorf("error unmarshalling YAML: %w", err)
	}

	// Convert return config to a final step
	if flow.Return.Type != "" {
		flow.Steps = append(flow.Steps, runtime.Step{
			ID:   "__return",
			Type: "return",
		})
	}

	return flow, nil
}
