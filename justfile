# protorm dev tasks — run `just` (or `just --list`) to see recipes.
#
# Common flows:
#   just dev        # build+install the dev plugin, test, regen examples with it
#   just ci         # what CI verifies: lint, build, test (mutates nothing)
#   just regen      # rewrite every committed artifact: stubs, goldens, examples
#
# Requires: go, buf, protoc-gen-go (for `stubs`). Install buf via
# `brew install bufbuild/buf/buf`.
#
# Note: a Homebrew-installed protoc-gen-protorm can sit earlier on PATH and
# shadow `just install`'s build. The recipes that run the plugin (examples) use
# ./bin via a PATH override so the build under test always wins; `just which`
# shows the resolution order.

# The dev plugin is built into ./bin and that dir is prepended to PATH for buf,
# so a brew/global protoc-gen-protorm never shadows the build under test.
bin := justfile_directory() / "bin"

# -buildvcs=false stamps the generated-file version banner as "dev" (matching the
# committed goldens/examples) instead of the git tag + "+dirty" working-tree
# version, so regeneration doesn't churn every banner line.
_flags := "-buildvcs=false"

# List recipes (default when you run bare `just`).
_default:
    @just --list

# Build the dev plugin into ./bin (version banner: "dev").
build:
    mkdir -p {{bin}}
    go build {{_flags}} -o {{bin}}/protoc-gen-protorm ./plugin/cmd/protoc-gen-protorm

# Install the dev plugin onto your Go bin (GOBIN) for use in other projects.
install:
    go install {{_flags}} ./plugin/cmd/protoc-gen-protorm

# Show every protoc-gen-protorm on PATH, in resolution order, with versions.
which:
    @for p in $(which -a protoc-gen-protorm 2>/dev/null); do printf '%s\t' "$p"; "$p" --version; done || echo "none on PATH (run: just install)"

# Regenerate the protorm option Go stubs (protorm/protormpbv1/*.pb.go).
stubs:
    buf generate

# Format Go sources in place.
fmt:
    gofmt -w plugin

# Static checks: gofmt diff, go vet, buf lint (mutates nothing).
lint:
    @test -z "$(gofmt -l plugin)" || { echo "unformatted files (run: just fmt):"; gofmt -l plugin; exit 1; }
    go vet ./...
    buf lint

# Run unit + golden tests.
test:
    go test ./...

# Rewrite golden fixtures after an intentional output change, then re-test.
update-goldens:
    go test ./plugin/generator -run TestGolden -update
    go test ./...

# Regenerate examples/ with the freshly-built ./bin plugin (not a global one).
examples: build
    PATH="{{bin}}:$PATH" buf generate --template buf.gen.example.yaml
    @echo "examples regenerated with $(./bin/protoc-gen-protorm --version)"

# Build the examples module to confirm the generated GORM structs compile.
build-examples:
    cd examples && go build ./...

# Regenerate every committed artifact: stubs, goldens, examples.
regen: stubs update-goldens examples

# Verify everything CI cares about, mutating nothing: lint, build, test.
ci: lint build test

# End-to-end "test the dev build": install+build the plugin, test, regen examples.
dev: install build test examples build-examples
    @echo "dev plugin installed, tests passed, examples regenerated with the dev build"

# Remove build artifacts.
clean:
    rm -rf {{bin}}
