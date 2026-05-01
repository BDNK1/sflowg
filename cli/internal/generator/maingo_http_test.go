package generator

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/cli/internal/config"
)

func TestMainGoIncludesHTTPTransportOnlyWhenEnabled(t *testing.T) {
	gen := NewMainGoGenerator("github.com/example/app", "8080", false, nil, config.ObservabilityConfig{})
	content, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if strings.Contains(content, `github.com/BDNK1/sflowg/transports/http`) {
		t.Fatalf("generated main.go unexpectedly imports HTTP transport\n%s", content)
	}

	gen.EnableHTTP()
	content, err = gen.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	for _, want := range []string{
		`httptransport "github.com/BDNK1/sflowg/transports/http"`,
		`httptransport.New(httptransport.Config{Addr: ":" + *port}),`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated main.go missing %q\n%s", want, content)
		}
	}
}
