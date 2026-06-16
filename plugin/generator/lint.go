package generator

// lint.go runs validate-on-generate checks: proto shapes that produce valid
// output but usually signal a mistake, surfaced as "lint" diagnostics before
// Prisma ever sees the schema. Severity is governed by the strict spec like any
// other rule (default: warn).

import (
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
)

// lint emits advisory diagnostics for resource messages:
//   - a resource type whose host disagrees with the file's package (the bug that
//     produced two divergent "calendar" schemas in the evaluation), and
//   - a resource with no IDENTIFIER field (AIP-122 expects a `name`).
func lint(p *protogen.Plugin, diags *diagnostics) {
	for _, f := range p.Files {
		if !f.Generate {
			continue
		}
		pkg := string(f.Desc.Package())
		for _, msg := range f.Messages {
			res := resourceOf(msg)
			if res == nil {
				continue
			}
			if host := typeHost(res.GetType()); host != "" && host != pkg {
				diags.warnf("lint", "message %q: resource type host %q disagrees with package %q",
					msg.Desc.Name(), host, pkg)
			}
			if !hasIdentifier(msg) {
				diags.warnf("lint", "message %q: resource has no IDENTIFIER field (AIP-122 expects `name`)",
					msg.Desc.Name())
			}
		}
	}
}

// typeHost is the service-host portion of a resource type
// ("bookstore.v1/Book" → "bookstore.v1").
func typeHost(resourceType string) string {
	if i := strings.IndexByte(resourceType, '/'); i >= 0 {
		return resourceType[:i]
	}
	return ""
}

// hasIdentifier reports whether any field carries IDENTIFIER field_behavior.
func hasIdentifier(msg *protogen.Message) bool {
	for _, fld := range msg.Fields {
		for _, b := range fieldBehaviors(fld) {
			if b == annotations.FieldBehavior_IDENTIFIER {
				return true
			}
		}
	}
	return false
}
