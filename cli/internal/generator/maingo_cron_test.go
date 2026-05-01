package generator

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/cli/internal/config"
)

func TestMainGoIncludesCronTransportOnlyWhenEnabled(t *testing.T) {
	gen := NewMainGoGenerator("github.com/example/app", "8080", false, nil, config.ObservabilityConfig{})
	content, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if strings.Contains(content, `github.com/BDNK1/sflowg/transports/cron`) {
		t.Fatalf("generated main.go unexpectedly imports Cron transport\n%s", content)
	}

	gen.EnableCron()
	content, err = gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	for _, want := range []string{
		`crontransport "github.com/BDNK1/sflowg/transports/cron"`,
		`crontransport.New(),`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated main.go missing %q\n%s", want, content)
		}
	}
}
