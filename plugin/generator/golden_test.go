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
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var update = flag.Bool("update", false, "rewrite golden files with current output")

var allTargets = []string{"prisma", "gorm", "sql", "csv"}

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

	for _, target := range caseTargets(t, dir) {
		t.Run(target, func(t *testing.T) {
			files := runTarget(t, req, target)
			goldenDir := filepath.Join(dir, "golden", target)

			if *update {
				writeGolden(t, goldenDir, files)
				return
			}
			compareGolden(t, goldenDir, files)
		})
	}
}

// buildRequest compiles the case protos in-process and assembles the
// CodeGeneratorRequest a protoc invocation would deliver.
//
// A case directory holds its own .proto files, unless it contains a "source"
// file naming another directory (path relative to this package) to compile
// instead — used so the bookstore case exercises the real example protos rather
// than a drift-prone copy.
func buildRequest(t *testing.T, dir string) *pluginpb.CodeGeneratorRequest {
	t.Helper()

	protoDir := dir
	if b, err := os.ReadFile(filepath.Join(dir, "source")); err == nil {
		protoDir = filepath.Clean(strings.TrimSpace(string(b)))
	}

	entries, err := os.ReadDir(protoDir)
	if err != nil {
		t.Fatalf("read %s: %v", protoDir, err)
	}
	var protos []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".proto") {
			protos = append(protos, e.Name())
		}
	}
	sort.Strings(protos)

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(protocompile.CompositeResolver{
			// Case protos are compiled from source so doc comments forward.
			&protocompile.SourceResolver{ImportPaths: []string{protoDir}},
			// google/api/* and protorm/v1/* are served from the already-compiled
			// global registry (linked in via the generator's imports), so no
			// vendored .proto copies live in the workspace to drift or get linted.
			registryResolver{},
		}),
		SourceInfoMode: protocompile.SourceInfoStandard, // keep comments for doc forwarding
	}
	compiled, err := compiler.Compile(context.Background(), protos...)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Collect the transitive descriptor set in dependency order.
	seen := map[string]bool{}
	var fdps []*descriptorpb.FileDescriptorProto
	var add func(fd protoreflect.FileDescriptor)
	add = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true
		for i := 0; i < fd.Imports().Len(); i++ {
			add(fd.Imports().Get(i).FileDescriptor)
		}
		fdps = append(fdps, toFDP(t, fd))
	}
	for _, fd := range compiled {
		add(fd)
	}

	// protogen demands a Go import path for every generated file; supply the
	// M mappings a buf.gen.yaml would.
	mappings := make([]string, len(protos))
	for i, p := range protos {
		mappings[i] = "M" + p + "=example.com/test/gen"
	}
	return &pluginpb.CodeGeneratorRequest{
		FileToGenerate: protos,
		Parameter:      proto.String(strings.Join(mappings, ",")),
		ProtoFile:      fdps,
	}
}

// registryResolver serves import paths from the global protobuf registry, which
// holds every .proto linked into the test binary (google/api/* via genproto,
// protorm/v1/* via the generated stubs). Returning a compiled Desc lets
// protocompile reuse it directly without needing the original source.
type registryResolver struct{}

func (registryResolver) FindFileByPath(path string) (protocompile.SearchResult, error) {
	fd, err := protoregistry.GlobalFiles.FindFileByPath(path)
	if err != nil {
		return protocompile.SearchResult{}, err
	}
	return protocompile.SearchResult{Desc: fd}, nil
}

// toFDP extracts the FileDescriptorProto, preferring the compiler's own copy
// (which always carries SourceCodeInfo) over a protodesc reconstruction.
//
// The result is round-tripped through the wire format: protocompile stores
// custom options as dynamic extension messages, which would panic when read
// via the linked-in generated extension types. Re-unmarshalling against the
// global registry re-interns them — matching what a real protoc run delivers.
func toFDP(t *testing.T, fd protoreflect.FileDescriptor) *descriptorpb.FileDescriptorProto {
	t.Helper()
	var fdp *descriptorpb.FileDescriptorProto
	if r, ok := fd.(linker.Result); ok {
		fdp = r.FileDescriptorProto()
	} else {
		fdp = protodesc.ToFileDescriptorProto(fd)
	}
	b, err := proto.Marshal(fdp)
	if err != nil {
		t.Fatalf("marshal %s: %v", fd.Path(), err)
	}
	out := &descriptorpb.FileDescriptorProto{}
	if err := proto.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal %s: %v", fd.Path(), err)
	}
	return out
}

// runTarget executes one backend through the real plugin entry point and
// returns the generated files as path → content.
func runTarget(t *testing.T, req *pluginpb.CodeGeneratorRequest, target string) map[string]string {
	t.Helper()

	p, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen: %v", err)
	}
	if err := Generate(p, Options{Target: target}); err != nil {
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
