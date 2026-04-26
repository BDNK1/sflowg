package generator

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/cli/internal/config"
	"github.com/BDNK1/sflowg/core/bootstrap"
)

func TestGenerate_IncludesObservabilityConfig(t *testing.T) {
	gen := NewMainGoGenerator(
		"github.com/example/ecom",
		"8080",
		false,
		nil,
		config.ObservabilityConfig{
			Logging: bootstrap.LoggingConfig{
				Export: bootstrap.LogExportConfig{
					Enabled:  true,
					Mode:     bootstrap.LogExportModes{"stdout", "otlp"},
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
			Metrics: bootstrap.MetricsConfig{
				Enabled:          true,
				Endpoint:         "localhost:4317",
				Insecure:         true,
				ExportIntervalMS: 10000,
				Attributes: map[string]string{
					"service.name": "ecommerce-api",
				},
				HistogramBuckets: bootstrap.HistogramBuckets{
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
		`"github.com/BDNK1/sflowg/core/bootstrap"`,
		`httptransport "github.com/BDNK1/sflowg/core/transport/http"`,
		"if err := bootstrap.Run(ctx, bootstrap.Config{",
		"Observability:    observabilityCfg,",
		"Transports: []bootstrap.Transport{",
		`httptransport.New(httptransport.Config{Addr: ":" + *port}),`,
		"Export: bootstrap.LogExportConfig{",
		"Mode: bootstrap.LogExportModes{",
		"Metrics: bootstrap.MetricsConfig{",
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

	if strings.Contains(content, `"github.com/BDNK1/sflowg/core"`) {
		t.Fatalf("generated main.go imports root runtime directly\n%s", content)
	}
	if strings.Contains(content, "runtime.ObservabilityConfig") || strings.Contains(content, "runtime.Transport") || strings.Contains(content, "*runtime.Container") {
		t.Fatalf("generated main.go still exposes root runtime API\n%s", content)
	}
}

func TestGenerate_DelegatesRuntimeAssemblyToBootstrap(t *testing.T) {
	gen := NewMainGoGenerator(
		"github.com/example/ecom",
		"8080",
		false,
		nil,
		config.ObservabilityConfig{},
	)

	content, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(content, "bootstrap.Run(ctx, bootstrap.Config{") {
		t.Fatalf("generated main.go missing bootstrap Run call\n%s", content)
	}

	if strings.Contains(content, "newValueStore := func() runtime.ValueStore { return runtime.NewValueStore() }") {
		t.Fatalf("generated main.go still assembles runtime value store directly\n%s", content)
	}

	if strings.Contains(content, "dslengine.NewValueStore()") {
		t.Fatalf("generated main.go still references removed dslengine.NewValueStore constructor\n%s", content)
	}
}

func TestGenerate_IncludesUserMetricsDeclarations(t *testing.T) {
	gen := NewMainGoGenerator(
		"github.com/example/ecom",
		"8080",
		false,
		nil,
		config.ObservabilityConfig{
			Metrics: bootstrap.MetricsConfig{
				Enabled:  true,
				Endpoint: "localhost:4317",
				User: bootstrap.UserMetricsConfig{
					Declarations: map[string]bootstrap.UserMetricDecl{
						"payment_attempts": {
							Type:        "counter",
							Description: "Payment attempts by provider and outcome",
							Labels: map[string]bootstrap.UserMetricLabel{
								"provider": {Type: "enum", Values: []string{"stripe"}},
								"outcome":  {Type: "enum", Values: []string{"success", "error", "queued"}},
							},
						},
						"latency": {
							Type:    "histogram",
							Unit:    "ms",
							Buckets: []float64{10, 50, 100, 500},
						},
					},
				},
			},
		},
	)

	content, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	checks := []string{
		"User: bootstrap.UserMetricsConfig{",
		"Declarations: map[string]bootstrap.UserMetricDecl{",
		`"payment_attempts"`,
		`Type:        "counter"`,
		`Description: "Payment attempts by provider and outcome"`,
		"Labels: map[string]bootstrap.UserMetricLabel{",
		`"provider"`,
		`Type: "enum"`,
		`"stripe"`,
		`"outcome"`,
		`"success"`,
		`"error"`,
		`"queued"`,
		`"latency"`,
		`Type:        "histogram"`,
		`Unit:        "ms"`,
		"Buckets: []float64{",
	}

	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Fatalf("generated main.go missing %q\n\nGenerated content:\n%s", check, content)
		}
	}
}
