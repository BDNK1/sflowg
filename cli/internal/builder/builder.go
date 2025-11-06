package builder

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Builder handles Go binary compilation
type Builder struct {
	WorkspacePath string
	OutputDir     string
	BinaryName    string
}

// NewBuilder creates a new builder
func NewBuilder(workspacePath, outputDir, binaryName string) *Builder {
	return &Builder{
		WorkspacePath: workspacePath,
		OutputDir:     outputDir,
		BinaryName:    binaryName,
	}
}

// DownloadDependencies runs go mod tidy to resolve and download all dependencies
func (b *Builder) DownloadDependencies() error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = b.WorkspacePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed in workspace %q: %w", b.WorkspacePath, err)
	}

	return nil
}

// Build compiles the binary
func (b *Builder) Build() error {
	outputPath := filepath.Join(b.WorkspacePath, b.BinaryName)

	cmd := exec.Command("go", "build", "-o", outputPath)
	cmd.Dir = b.WorkspacePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed in workspace %q, output %q: %w", b.WorkspacePath, outputPath, err)
	}

	return nil
}

// CopyBinary copies the compiled binary to the output directory
func (b *Builder) CopyBinary() error {
	sourcePath := filepath.Join(b.WorkspacePath, b.BinaryName)
	destPath := filepath.Join(b.OutputDir, b.BinaryName)

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open binary at %q: %w", sourcePath, err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create output file at %q: %w", destPath, err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy binary from %q to %q: %w", sourcePath, destPath, err)
	}

	// Make executable
	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable at %q: %w", destPath, err)
	}

	return nil
}
