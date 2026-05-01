package generator

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/cli/internal/config"
)

func TestMainGoIncludesKafkaTransportOnlyWhenEnabled(t *testing.T) {
	gen := NewMainGoGenerator("github.com/example/app", "8080", false, nil, config.ObservabilityConfig{})
	content, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if strings.Contains(content, `github.com/BDNK1/sflowg/transports/kafka`) {
		t.Fatalf("generated main.go unexpectedly imports Kafka transport\n%s", content)
	}

	gen.EnableKafka(config.KafkaRuntimeConfig{Brokers: map[string]config.KafkaBrokerConfig{
		"default": {
			Brokers:               []string{"localhost:9092"},
			ClientID:              "orders-service",
			NackRedeliveryDelayMS: 100,
		},
	}})
	content, err = gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	for _, want := range []string{
		`kafkatransport "github.com/BDNK1/sflowg/transports/kafka"`,
		`kafkatransport.New(kafkatransport.Config{`,
		`"default": {`,
		`"localhost:9092"`,
		`ClientID: "orders-service"`,
		`NackRedeliveryDelayMS: 100`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated main.go missing %q\n%s", want, content)
		}
	}
}
