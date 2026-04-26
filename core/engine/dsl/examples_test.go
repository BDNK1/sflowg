package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_DocumentationExamples(t *testing.T) {
	examplesRoot := filepath.Clean(filepath.Join("..", "..", "..", "docs", "examples"))

	err := filepath.WalkDir(examplesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".flow") {
			return nil
		}

		t.Run(path, func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if _, err := Parse(string(source)); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
}
