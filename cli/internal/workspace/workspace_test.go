package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyFlowsCopiesOnlyFlowFiles(t *testing.T) {
	projectDir := t.TempDir()
	flowsDir := filepath.Join(projectDir, "flows")
	if err := os.Mkdir(flowsDir, 0755); err != nil {
		t.Fatalf("Mkdir flows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "pay.flow"), []byte("entrypoint.http {}"), 0644); err != nil {
		t.Fatalf("WriteFile .flow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "legacy.yaml"), []byte("id: legacy"), 0644); err != nil {
		t.Fatalf("WriteFile .yaml: %v", err)
	}

	w := &Workspace{Path: t.TempDir(), ProjectDir: projectDir}
	if err := w.CopyFlows(); err != nil {
		t.Fatalf("CopyFlows() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(w.Path, "flows", "pay.flow")); err != nil {
		t.Fatalf("expected .flow file to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(w.Path, "flows", "legacy.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected .yaml file not to be copied, stat err = %v", err)
	}
}

func TestCopyFlowsRequiresFlowFiles(t *testing.T) {
	projectDir := t.TempDir()
	flowsDir := filepath.Join(projectDir, "flows")
	if err := os.Mkdir(flowsDir, 0755); err != nil {
		t.Fatalf("Mkdir flows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "legacy.yaml"), []byte("id: legacy"), 0644); err != nil {
		t.Fatalf("WriteFile .yaml: %v", err)
	}

	w := &Workspace{Path: t.TempDir(), ProjectDir: projectDir}
	err := w.CopyFlows()
	if err == nil {
		t.Fatal("expected CopyFlows() error")
	}
	if !strings.Contains(err.Error(), "no flow files (.flow)") {
		t.Fatalf("expected .flow error, got %v", err)
	}
}
