package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/cli/internal/constants"
)

func TestScanProjectFlowsFindsKafkaAndValidatesCoreSemantics(t *testing.T) {
	dir := t.TempDir()
	flowsDir := filepath.Join(dir, "flows")
	if err := os.Mkdir(flowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	source := `entrypoint.kafka {
	topic: orders
	group_id: orders-service
	auto_offset_reset: earliest
	value: { type: json }
}
return response.ack()`
	if err := os.WriteFile(filepath.Join(flowsDir, "orders.flow"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	scan, err := scanProjectFlows(dir)
	if err != nil {
		t.Fatalf("scanProjectFlows failed: %v", err)
	}
	if !scan.HasSupported || !scan.HasKafka {
		t.Fatalf("scan = %#v, want supported kafka flow", scan)
	}
}

func TestScanProjectFlowsRejectsKafkaSemanticError(t *testing.T) {
	dir := t.TempDir()
	flowsDir := filepath.Join(dir, "flows")
	if err := os.Mkdir(flowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	source := `entrypoint.kafka {
	topic: orders
	group_id: orders-service
	auto_offset_reset: earliest
	value: { type: text }
}
return response.ack()`
	if err := os.WriteFile(filepath.Join(flowsDir, "orders.flow"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := scanProjectFlows(dir); err == nil {
		t.Fatal("expected Kafka semantic validation error")
	}
}

func TestScanProjectFlowsRejectsUnsupportedEntrypoint(t *testing.T) {
	dir := t.TempDir()
	flowsDir := filepath.Join(dir, "flows")
	if err := os.Mkdir(flowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	valid := `entrypoint.http {
	method: GET
	path: /health
}
return response.json({ status: 200, body: { ok: true } })`
	unsupported := `entrypoint.grpc {
	service: Orders
}
return response.json({ status: 200, body: {} })`
	if err := os.WriteFile(filepath.Join(flowsDir, "health.flow"), []byte(valid), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "orders.flow"), []byte(unsupported), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := scanProjectFlows(dir)
	if err == nil {
		t.Fatal("expected unsupported entrypoint error")
	}
	if !strings.Contains(err.Error(), `unsupported flow entrypoint "grpc"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEmbeddedFlowsRejectsUnsupportedEntrypoint(t *testing.T) {
	flowsDir := filepath.Join(t.TempDir(), "flows")
	if err := os.Mkdir(flowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	source := `entrypoint.grpc {
	service: Orders
}
return response.json({ status: 200, body: {} })`
	if err := os.WriteFile(filepath.Join(flowsDir, "orders.flow"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	err := validateEmbeddedFlows(flowsDir)
	if err == nil {
		t.Fatal("expected unsupported entrypoint error")
	}
	if !strings.Contains(err.Error(), `unsupported flow entrypoint "grpc"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKafkaTransportInfoRequiresTransportPathWithLocalRuntime(t *testing.T) {
	_, err := externalTransportInfo(true, "Kafka", constants.KafkaTransportModulePath, "kafka", "v0.0.0", "/repo/core", "")
	if err == nil || !strings.Contains(err.Error(), "--transport-path") {
		t.Fatalf("expected transport path error, got %v", err)
	}
}

func TestKafkaTransportInfoUsesLocalReplaceWhenTransportPathProvided(t *testing.T) {
	info, err := externalTransportInfo(true, "Kafka", constants.KafkaTransportModulePath, "kafka", "v0.0.0", "/repo/core", "/repo/transports")
	if err != nil {
		t.Fatalf("kafkaTransportInfo failed: %v", err)
	}
	if info.ModulePath != constants.KafkaTransportModulePath {
		t.Fatalf("ModulePath = %q", info.ModulePath)
	}
	if info.Version != "v0.0.0" {
		t.Fatalf("Version = %q, want v0.0.0", info.Version)
	}
	if info.LocalPath != filepath.Join("/repo/transports", "kafka") {
		t.Fatalf("LocalPath = %q", info.LocalPath)
	}
}

func TestHTTPTransportInfoRequiresTransportPathWithLocalRuntime(t *testing.T) {
	_, err := externalTransportInfo(true, "HTTP", constants.HTTPTransportModulePath, "http", "v0.0.0", "/repo/core", "")
	if err == nil || !strings.Contains(err.Error(), "--transport-path") {
		t.Fatalf("expected transport path error, got %v", err)
	}
}
