package generator

// golden_compile_test.go holds the in-process proto compilation the golden
// tests run on: turning a case directory's .proto files into the
// CodeGeneratorRequest a real protoc invocation would deliver, without needing
// protoc or buf on PATH. golden_test.go consumes the request and diffs output.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

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

	// A case may ship an imports/ directory of .proto files that are compiled and
	// importable but never added to file_to_generate — standing in for external,
	// non-generated dependencies (google/type/*, a vendored common.proto). They
	// exercise the imported-message relationalization path without depending on
	// genproto having those descriptors linked into the test binary.
	importPaths := []string{protoDir}
	if fi, err := os.Stat(filepath.Join(dir, "imports")); err == nil && fi.IsDir() {
		importPaths = append(importPaths, filepath.Join(dir, "imports"))
	}

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(protocompile.CompositeResolver{
			// Case protos (and any imports/ deps) are compiled from source so doc
			// comments forward.
			&protocompile.SourceResolver{ImportPaths: importPaths},
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
