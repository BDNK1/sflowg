package generator

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/cli/internal/config"
	"github.com/BDNK1/sflowg/runtime"
)

func TestGenerate_IncludesObservabilityConfig(t *testing.T) {
	gen := NewMainGoGenerator(
		"github.com/example/ecom",
		"8080",
		false,
		nil,
		config.ObservabilityConfig{
			Logging: runtime.LoggingConfig{
				Export: runtime.LogExportConfig{
					Enabled:  true,
					Mode:     runtime.LogExportModes{"stdout", "otlp"},
					Endpoint: "localhost:4317",
					Insecure: true,
					Attributes: map[string]string{
						"service.name": "ecommerce-api",
					},
				},
			},
			Tracing: config.TracingConfig{
				Enabled:  true,
				Endpoint: "localhost:4317",
			},
			Metrics: runtime.MetricsConfig{
				Enabled:          true,
				Endpoint:         "localhost:4317",
				Insecure:         true,
				ExportIntervalMS: 10000,
				Attributes: map[string]string{
					"service.name": "ecommerce-api",
				},
				HistogramBuckets: runtime.HistogramBuckets{
					HTTPRequestMS: []float64{5, 25, 100},
					FlowMS:        []float64{10, 50, 250},
				},
			},
		},
	)

	content, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	checks := []string{
		"container := runtime.NewContainer(runtime.NewLogger(nil))",
		"if err := container.InitObservability(observabilityCfg); err != nil {",
		"if err := container.ShutdownObservability(shutdownCtx); err != nil {",
		"Export: runtime.LogExportConfig{",
		"Mode: runtime.LogExportModes{",
		"Metrics: runtime.MetricsConfig{",
		"Enabled:          true,",
		`Endpoint:         "localhost:4317",`,
		"Insecure:         true,",
		"ExportIntervalMS: 10000,",
		`"service.name": "ecommerce-api",`,
		"HTTPRequestMS: []float64{",
		"FlowMS: []float64{",
	}

	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Fatalf("generated main.go missing %q\n%s", check, content)
		}
	}
}
