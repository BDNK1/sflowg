package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BDNK1/sflowg/core"
)

// FlowLoader loads flow definitions from .flow DSL files.
type FlowLoader struct{}

func NewFlowLoader() *FlowLoader {
	return &FlowLoader{}
}

func (l *FlowLoader) Extensions() []string {
	return []string{"*.flow"}
}

func (l *FlowLoader) Load(filePath string) (core.Flow, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return core.Flow{}, fmt.Errorf("error reading DSL file: %w", err)
	}

	flow, err := Parse(string(data))
	if err != nil {
		return core.Flow{}, fmt.Errorf("error parsing DSL file %s: %w", filePath, err)
	}

	// Derive flow ID from filename (strip extension and path)
	flow.ID = strings.TrimSuffix(filepath.Base(filePath), ".flow")

	// Convert return body to a final step (return is just an unconditional step)
	if flow.Return.Body != "" {
		flow.Steps = append(flow.Steps, core.Step{
			ID:   "__return",
			Body: flow.Return.Body,
		})
	}

	return flow, nil
}
