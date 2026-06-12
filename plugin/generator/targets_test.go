package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
)

// caseTargets returns the backends a case runs: the comma-separated content
// of its optional "targets" file, defaulting to every registered target.
func caseTargets(t *testing.T, dir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "targets"))
	if err != nil {
		if os.IsNotExist(err) {
			return allTargets
		}
		t.Fatalf("read targets: %v", err)
	}
	var out []string
	for _, s := range strings.Split(strings.TrimSpace(string(b)), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// TestRelationalTargetsRejectMongo verifies gorm and sql fail loudly when a
// datasource declares provider mongodb instead of silently emitting nonsense.
func TestRelationalTargetsRejectMongo(t *testing.T) {
	req := buildRequest(t, filepath.Join("testdata", "cases", "mongo"))

	for _, target := range []string{"gorm", "sql"} {
		p, err := protogen.Options{}.New(req)
		if err != nil {
			t.Fatalf("protogen: %v", err)
		}
		err = Generate(p, Options{Target: target})
		if err == nil || !strings.Contains(err.Error(), "mongodb") {
			t.Errorf("%s on mongodb provider: want provider error, got %v", target, err)
		}
	}
}

// TestUnknownTarget verifies the registry rejects unknown and missing targets.
func TestUnknownTarget(t *testing.T) {
	req := buildRequest(t, filepath.Join("testdata", "cases", "wkt"))

	for _, target := range []string{"", "typeorm"} {
		p, err := protogen.Options{}.New(req)
		if err != nil {
			t.Fatalf("protogen: %v", err)
		}
		if err := Generate(p, Options{Target: target}); err == nil {
			t.Errorf("target %q: want error, got nil", target)
		}
	}
}
