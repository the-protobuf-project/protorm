package generator

// Golden-file tests: every directory under testdata/cases/ is one case. Its
// .proto files are compiled in-process (no protoc/buf needed), run through
// every backend, and the outputs are compared byte-for-byte against the
// committed files under <case>/golden/<target>/.
//
// Regenerate goldens after an intentional output change:
//
//	go test ./plugin/generator -run TestGolden -update
//
// A case may contain a "targets" file (comma-separated list) to restrict
// which backends run; the default is all of them.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

var update = flag.Bool("update", false, "rewrite golden files with current output")

var allTargets = []string{"prisma", "gorm", "sql"}

func TestGolden(t *testing.T) {
	cases, err := os.ReadDir("testdata/cases")
	if err != nil {
		t.Fatalf("read cases: %v", err)
	}
	for _, c := range cases {
		if !c.IsDir() {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			runCase(t, filepath.Join("testdata", "cases", c.Name()))
		})
	}
}

// runCase compiles the case's protos and checks every target's output.
func runCase(t *testing.T, dir string) {
	req := buildRequest(t, dir)

	// A case may ship a protorm.yaml to exercise the layout config (P1.1).
	configPath := ""
	if _, err := os.Stat(filepath.Join(dir, "protorm.yaml")); err == nil {
		configPath = filepath.Join(dir, "protorm.yaml")
	}

	for _, target := range caseTargets(t, dir) {
		t.Run(target, func(t *testing.T) {
			files := runTarget(t, req, configPath, target)
			goldenDir := filepath.Join(dir, "golden", target)

			if *update {
				writeGolden(t, goldenDir, files)
				return
			}
			compareGolden(t, goldenDir, files)
		})
	}
}

// runTarget executes one backend through the real plugin entry point and
// returns the generated files as path → content.
func runTarget(t *testing.T, req *pluginpb.CodeGeneratorRequest, configPath, target string) map[string]string {
	t.Helper()

	p, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen: %v", err)
	}
	// GoModule mirrors the go_module plugin opt so the gorm target emits its
	// migration aggregator; other targets ignore it.
	opts := Options{Target: target, ConfigPath: configPath, GoModule: "example.com/test/gen"}
	if err := Generate(p, opts); err != nil {
		t.Fatalf("generate %s: %v", target, err)
	}
	resp := p.Response()
	if resp.GetError() != "" {
		t.Fatalf("response error: %s", resp.GetError())
	}

	files := map[string]string{}
	for _, f := range resp.GetFile() {
		files[f.GetName()] = f.GetContent()
	}
	if len(files) == 0 {
		t.Fatalf("target %s produced no files", target)
	}
	return files
}

// writeGolden replaces goldenDir with the current output.
func writeGolden(t *testing.T, goldenDir string, files map[string]string) {
	t.Helper()
	if err := os.RemoveAll(goldenDir); err != nil {
		t.Fatalf("clean golden: %v", err)
	}
	for name, content := range files {
		path := filepath.Join(goldenDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	t.Logf("updated %d golden files in %s", len(files), goldenDir)
}

// compareGolden diffs current output against the committed golden tree.
func compareGolden(t *testing.T, goldenDir string, files map[string]string) {
	t.Helper()

	want := map[string]string{}
	err := filepath.WalkDir(goldenDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(goldenDir, path)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		want[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("read golden tree %s (run with -update to create): %v", goldenDir, err)
	}

	for name, got := range files {
		w, ok := want[name]
		if !ok {
			t.Errorf("unexpected output file %s (run with -update if intentional)", name)
			continue
		}
		if got != w {
			t.Errorf("output mismatch for %s (run with -update if intentional):\n%s",
				name, firstDiff(w, got))
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("missing output file %s (golden exists, nothing generated)", name)
	}
}

// firstDiff renders the first differing line with one line of context.
func firstDiff(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		w, g := line(wl, i), line(gl, i)
		if w != g {
			return fmt.Sprintf("line %d:\n  want: %q\n  got:  %q", i+1, w, g)
		}
	}
	return "contents differ"
}

func line(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<EOF>"
}
