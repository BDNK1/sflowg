package generator

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/cli/internal/constants"
)

func TestGoModIncludesKafkaTransportOnlyWhenConfigured(t *testing.T) {
	gen := NewGoModGenerator("abc123", "v0.1.4", "")
	if content := gen.Generate(); strings.Contains(content, constants.KafkaTransportModulePath) {
		t.Fatalf("go.mod unexpectedly includes Kafka transport\n%s", content)
	}

	gen.SetKafkaTransport(TransportInfo{
		ModulePath: constants.KafkaTransportModulePath,
		Version:    "v0.1.4",
		LocalPath:  "/repo/transports/kafka",
	})
	content := gen.Generate()
	for _, want := range []string{
		constants.KafkaTransportModulePath + " v0.1.4",
		constants.KafkaTransportModulePath + " => /repo/transports/kafka",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("go.mod missing %q\n%s", want, content)
		}
	}
}

func TestGoModIncludesHTTPTransportOnlyWhenConfigured(t *testing.T) {
	gen := NewGoModGenerator("abc123", "v0.1.4", "")
	if content := gen.Generate(); strings.Contains(content, constants.HTTPTransportModulePath) {
		t.Fatalf("go.mod unexpectedly includes HTTP transport\n%s", content)
	}

	gen.SetHTTPTransport(TransportInfo{
		ModulePath: constants.HTTPTransportModulePath,
		Version:    "v0.1.4",
		LocalPath:  "/repo/transports/http",
	})
	content := gen.Generate()
	for _, want := range []string{
		constants.HTTPTransportModulePath + " v0.1.4",
		constants.HTTPTransportModulePath + " => /repo/transports/http",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("go.mod missing %q\n%s", want, content)
		}
	}
}

func TestGoModIncludesCronTransportOnlyWhenConfigured(t *testing.T) {
	gen := NewGoModGenerator("abc123", "v0.1.4", "")
	if content := gen.Generate(); strings.Contains(content, constants.CronTransportModulePath) {
		t.Fatalf("go.mod unexpectedly includes Cron transport\n%s", content)
	}

	gen.SetCronTransport(TransportInfo{
		ModulePath: constants.CronTransportModulePath,
		Version:    "v0.1.4",
		LocalPath:  "/repo/transports/cron",
	})
	content := gen.Generate()
	for _, want := range []string{
		constants.CronTransportModulePath + " v0.1.4",
		constants.CronTransportModulePath + " => /repo/transports/cron",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("go.mod missing %q\n%s", want, content)
		}
	}
}
