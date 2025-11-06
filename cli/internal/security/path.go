package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidatePathWithinBoundary ensures that targetPath is within or equal to boundaryPath.
// This prevents path traversal attacks where malicious paths could escape the
// intended directory using "../" sequences.
//
// Example:
//
//	boundary := "/Users/me/project"
//	target := "/Users/me/project/flows/test.yaml"  // ✅ Valid
//	target := "/Users/me/project/../../../etc/passwd"  // ❌ Invalid
//
// Returns an error if:
//   - Either path cannot be resolved to absolute form
//   - targetPath is outside boundaryPath (escapes using "..")
func ValidatePathWithinBoundary(boundaryPath, targetPath string) error {
	// Convert both paths to absolute paths to handle symlinks and relative paths
	absBoundary, err := filepath.Abs(boundaryPath)
	if err != nil {
		return fmt.Errorf("failed to resolve boundary path %q: %w", boundaryPath, err)
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path %q: %w", targetPath, err)
	}

	// Compute relative path from boundary to target
	rel, err := filepath.Rel(absBoundary, absTarget)
	if err != nil {
		return fmt.Errorf("invalid path relationship between %q and %q: %w", absBoundary, absTarget, err)
	}

	// If relative path starts with "..", target is outside boundary
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path traversal detected: %q escapes boundary %q", targetPath, boundaryPath)
	}

	return nil
}

// ValidatePathsWithinBoundary validates multiple target paths against a single boundary.
// Returns the first validation error encountered.
func ValidatePathsWithinBoundary(boundaryPath string, targetPaths ...string) error {
	for _, target := range targetPaths {
		if err := ValidatePathWithinBoundary(boundaryPath, target); err != nil {
			return err
		}
	}
	return nil
}
