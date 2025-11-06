package security

import (
	"path/filepath"
	"testing"
)

func TestValidatePathWithinBoundary_Valid(t *testing.T) {
	boundary := "/Users/test/project"
	validPaths := []string{
		"/Users/test/project/flows",
		"/Users/test/project/flows/test.yaml",
		"/Users/test/project",
		"/Users/test/project/plugins/http",
	}

	for _, path := range validPaths {
		err := ValidatePathWithinBoundary(boundary, path)
		if err != nil {
			t.Errorf("Expected path %q to be valid within boundary %q, but got error: %v", path, boundary, err)
		}
	}
}

func TestValidatePathWithinBoundary_PathTraversal(t *testing.T) {
	boundary := "/Users/test/project"
	maliciousPaths := []string{
		"/Users/test/project/../../../etc/passwd",
		"/Users/test/project/../other-project",
		"/Users/test",
		"/etc/passwd",
		"/Users/other-user/data",
	}

	for _, path := range maliciousPaths {
		err := ValidatePathWithinBoundary(boundary, path)
		if err == nil {
			t.Errorf("Expected path %q to be REJECTED (path traversal), but it was allowed", path)
		}
	}
}

func TestValidatePathWithinBoundary_RelativePaths(t *testing.T) {
	// Test with relative paths - should be resolved to absolute
	boundary := "."
	absBoundary, _ := filepath.Abs(boundary)

	tests := []struct {
		name        string
		targetPath  string
		shouldError bool
	}{
		{
			name:        "current directory",
			targetPath:  ".",
			shouldError: false,
		},
		{
			name:        "subdirectory",
			targetPath:  "./flows",
			shouldError: false,
		},
		{
			name:        "parent directory escape",
			targetPath:  "../",
			shouldError: true,
		},
		{
			name:        "double parent escape",
			targetPath:  "../../etc",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathWithinBoundary(absBoundary, tt.targetPath)
			if tt.shouldError && err == nil {
				t.Errorf("Expected error for %q but got none", tt.targetPath)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error for %q but got: %v", tt.targetPath, err)
			}
		})
	}
}

func TestValidatePathsWithinBoundary(t *testing.T) {
	boundary := "/Users/test/project"

	t.Run("all valid paths", func(t *testing.T) {
		paths := []string{
			"/Users/test/project/file1.txt",
			"/Users/test/project/flows/flow.yaml",
			"/Users/test/project/plugins/http",
		}
		err := ValidatePathsWithinBoundary(boundary, paths...)
		if err != nil {
			t.Errorf("Expected all paths to be valid, but got error: %v", err)
		}
	})

	t.Run("one invalid path", func(t *testing.T) {
		paths := []string{
			"/Users/test/project/file1.txt",
			"/Users/test/project/../../../etc/passwd", // Malicious
			"/Users/test/project/plugins/http",
		}
		err := ValidatePathsWithinBoundary(boundary, paths...)
		if err == nil {
			t.Error("Expected error for malicious path, but got none")
		}
	})
}
