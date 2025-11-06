package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/sflowg/sflowg/cli/internal/security"
)

// Workspace represents a temporary build workspace
type Workspace struct {
	Path       string
	ProjectDir string
	UUID       string
}

// Create creates a new temporary workspace directory
func Create(projectDir string) (*Workspace, error) {
	// Generate unique ID for this build
	buildUUID := uuid.New().String()[:8] // Use first 8 chars for brevity

	// Create temp directory
	workspacePath := filepath.Join(os.TempDir(), fmt.Sprintf("sflowg-build-%s", buildUUID))

	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory at %q: %w", workspacePath, err)
	}

	return &Workspace{
		Path:       workspacePath,
		ProjectDir: projectDir,
		UUID:       buildUUID,
	}, nil
}

// Cleanup removes the temporary workspace directory
func (w *Workspace) Cleanup() error {
	if w.Path == "" {
		return nil
	}

	if err := os.RemoveAll(w.Path); err != nil {
		return fmt.Errorf("failed to cleanup workspace at %q: %w", w.Path, err)
	}

	return nil
}

// CopyFlows copies flow YAML files from project directory to workspace
func (w *Workspace) CopyFlows() error {
	// Look for flows directory in project
	flowsDir := filepath.Join(w.ProjectDir, "flows")

	// Security: Validate flowsDir is within project boundaries
	if err := security.ValidatePathWithinBoundary(w.ProjectDir, flowsDir); err != nil {
		return fmt.Errorf("invalid flows directory path: %w", err)
	}

	// Check if flows directory exists
	if _, err := os.Stat(flowsDir); os.IsNotExist(err) {
		return fmt.Errorf("flows directory not found in project: %s", flowsDir)
	}

	// Create flows directory in workspace
	workspaceFlowsDir := filepath.Join(w.Path, "flows")
	if err := os.MkdirAll(workspaceFlowsDir, 0755); err != nil {
		return fmt.Errorf("failed to create flows directory in workspace at %q: %w", workspaceFlowsDir, err)
	}

	// Copy all .yaml and .yml files from flows directory
	entries, err := os.ReadDir(flowsDir)
	if err != nil {
		return fmt.Errorf("failed to read flows directory at %q: %w", flowsDir, err)
	}

	copiedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) == ".yaml" || filepath.Ext(name) == ".yml" {
			src := filepath.Join(flowsDir, name)
			dst := filepath.Join(workspaceFlowsDir, name)

			// Security: Validate source file is within flows directory
			if err := security.ValidatePathWithinBoundary(flowsDir, src); err != nil {
				return fmt.Errorf("invalid flow file path %q: %w", name, err)
			}

			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("failed to copy flow file %s (src=%s, dst=%s): %w", name, src, dst, err)
			}
			copiedCount++
		}
	}

	if copiedCount == 0 {
		return fmt.Errorf("no flow files (.yaml/.yml) found in flows directory")
	}

	return nil
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file at %q: %w", src, err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file at %q: %w", dst, err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy data from %q to %q: %w", src, dst, err)
	}

	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file at %q: %w", dst, err)
	}

	return nil
}
