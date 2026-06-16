package generator

// lint.go runs validate-on-generate checks: proto shapes that produce valid
// output but usually signal a mistake, surfaced as "lint" diagnostics before
// Prisma ever sees the schema. Severity is governed by the strict spec like any
// other rule (default: warn).

import (
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
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

// lintSchemaStutter warns when a schema name and one of its tables share a root
// word, so a schema-qualified identifier reads the word twice. Downstream layers
// that name objects <schema>_<table> (e.g. Hasura's camelCase root fields) turn
// "booking" + "bookings" into "bookingBookings". protorm can't auto-fix this —
// the table name is the AIP plural and a derivative schema name ("bookingSvc")
// still stutters — so it flags it for a distinct schema name in protorm.yaml.
// Runs on the built IR (schema/table names are final by then).
func lintSchemaStutter(dbs []*schema.Database, diags *diagnostics) {
	for _, db := range dbs {
		for _, s := range db.Schemas {
			root := squashName(s.Name)
			if root == "" {
				continue
			}
			for _, t := range s.Tables {
				if strings.HasPrefix(squashName(t.Name), root) {
					diags.warnf("lint", "schema %q and table %q share a root word, so a "+
						"schema-qualified name stutters (e.g. %q); set a distinct schema in "+
						"protorm.yaml to avoid it", s.Name, t.Name, naming.Camel(s.Name+"_"+t.Name))
				}
			}
		}
	}
}

// squashName lowercases an identifier and drops underscores so "promo_code" and
// "promocode" compare equal when testing for a shared root word.
func squashName(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "_", "")
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
