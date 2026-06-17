// Command protoc-gen-protorm is a protoc plugin that reads proto descriptors
// annotated with google.api.* and protorm.v1.* options, then generates
// database schema artifacts for the requested backend.
//
// # Install
//
//	go install github.com/the-protobuf-project/protorm/plugin/cmd/protoc-gen-protorm@latest
//
// # Usage via buf.gen.yaml
//
//	plugins:
//	  - local: protoc-gen-protorm
//	    out: generated/
//	    opt:
//	      - target=prisma   # prisma | gorm | sql
//
// # Inference priority
//
//  1. google.api.* annotations   — drives table, column, FK inference (80 %)
//  2. protorm.v1.* options       — overrides: type, name, skip, unique, index
//  3. buf.gen.yaml opt:          — global defaults (target backend)
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/the-protobuf-project/protorm/plugin/generator"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

// Build metadata, injected at release time via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// resolveVersion returns the build version to stamp into generated files.
// A release sets `version` via ldflags and wins outright. Otherwise we recover
// it from the build info the Go toolchain embeds: `go install …@v0.1.2` records
// the tag as the main module version, and when protorm is consumed as a
// dependency its module entry carries the version. Only genuine local builds
// (`go build`/`go run`, which report "(devel)") fall back to "dev".
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, dep := range bi.Deps {
		if dep.Path == "github.com/the-protobuf-project/protorm" && dep.Version != "" {
			return dep.Version
		}
	}
	return version
}

func main() {
	ver := resolveVersion()

	// When invoked directly with -version (not by protoc), print and exit before
	// protogen tries to read a CodeGeneratorRequest from stdin.
	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Printf("protoc-gen-protorm %s (commit %s, built %s)\n", ver, commit, date)
		return
	}

	// flags are populated by protogen before the Run closure is called.
	// ParamFunc maps each "key=value" from buf.gen.yaml opt: to flags.Set.
	var flags flag.FlagSet

	target := flags.String(
		"target", "",
		"output backend: prisma | gorm | sql",
	)
	strict := flags.String(
		"strict", "",
		"per-rule severity for schema problems: \"\"=all warn, \"true\"=all error, "+
			"or \"ref:error,collision:warn,index:error,lint:warn\"",
	)
	config := flags.String(
		"config", "",
		"path to a protorm.yaml mapping proto packages to databases/schemas",
	)
	goModule := flags.String(
		"go_module", "",
		"Go import path of the output directory (e.g. github.com/me/gen); the gorm "+
			"target needs it to generate the migration aggregator that imports each schema package",
	)

	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(p *protogen.Plugin) error {
		// Proto3 `optional` is fully supported (it only affects field presence,
		// which protorm reads via field_behavior, not synthetic oneofs); declare
		// it so buf/protoc don't warn for files that use it.
		p.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return generator.Generate(p, generator.Options{
			// *target/*strict are dereferenced inside the closure so that
			// ParamFunc has already populated them before we read the values.
			Target:     *target,
			Strict:     *strict,
			Version:    ver,
			ConfigPath: *config,
			GoModule:   *goModule,
		})
	})
}
