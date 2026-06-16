package generator

// config.go loads the optional protorm.yaml layout config (passed via the
// `config=<path>` plugin option) and resolves a proto package to its target
// database and postgres schema. This is what lets a multi-service monorepo split
// into several databases with clean schema names without annotating every file.
// Precedence: a per-file (protorm.v1.datasource) annotation wins over the
// config, which in turn wins over the package-path defaults.

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// layoutConfig is the parsed protorm.yaml.
type layoutConfig struct {
	Datasources []matchRule `yaml:"datasources"`
	// StripVersion, when true, flattens the API version out of every derived
	// schema name ("bookstore.v1" → schema "bookstore" instead of "bookstore_v1").
	// It applies to resource-type-derived and config-derived schema names, but
	// never to an explicit (protorm.v1.datasource).schema annotation. A per-rule
	// strip_version overrides this default for that rule.
	StripVersion bool `yaml:"strip_version"`

	// DedupeSchemaTable, when true, renames a table whose name would stutter with
	// its schema in a schema-qualified identifier ("booking" schema + "bookings"
	// table → "bookingBookings" in tools that join schema+table, e.g. Hasura).
	// The redundant leading schema word is stripped from the table name, or — for
	// the schema's eponymous/primary table, where stripping leaves nothing — the
	// table is renamed to a generic word ("resource", then "entity"/…). Only the
	// generated table name changes; proto/model names are untouched. See destutter.go.
	DedupeSchemaTable bool `yaml:"dedupe_schema_table"`
}

// matchRule assigns every proto package matching Match to a database and schema.
// Match is a dotted glob over the package: a trailing "**" matches any remaining
// segments ("fleet.tracking.**"); "*" matches exactly one segment; other
// segments match literally.
type matchRule struct {
	Match    string `yaml:"match"`
	Database string `yaml:"database"`
	// Schema is a literal or a template using {leaf} (the last package segment
	// with a trailing API version dropped). Empty falls back to SchemaDepth.
	Schema string `yaml:"schema"`
	// SchemaDepth, when >0 and Schema is empty, joins the first N package
	// segments with "_" to form the schema name.
	SchemaDepth int `yaml:"schema_depth"`
	// StripVersion overrides the top-level strip_version for packages this rule
	// matches. Nil (the default) inherits the global setting; set it explicitly
	// (true/false) to force version stripping on or off for this datasource.
	StripVersion *bool `yaml:"strip_version"`
}

// loadLayoutConfig reads protorm.yaml from path. A blank path yields nil (no
// config; defaults apply).
func loadLayoutConfig(path string) (*layoutConfig, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var c layoutConfig
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	return &c, nil
}

// resolve returns the database, schema, and version-stripping decision for a
// proto package under the first matching rule, or the global default when no
// rule matches (nil-safe). stripVer reflects the per-rule strip_version when
// set, otherwise the top-level strip_version.
func (c *layoutConfig) resolve(pkg string) (database, schema string, stripVer bool) {
	if c == nil {
		return "", "", false
	}
	segs := strings.Split(pkg, ".")
	for _, r := range c.Datasources {
		if matchPackage(r.Match, segs) {
			sv := c.StripVersion
			if r.StripVersion != nil {
				sv = *r.StripVersion
			}
			return r.Database, ruleSchema(r, segs), sv
		}
	}
	return "", "", c.StripVersion
}

// ruleSchema computes the schema name a matched rule assigns to a package.
func ruleSchema(r matchRule, segs []string) string {
	switch {
	case r.Schema != "":
		return strings.ReplaceAll(r.Schema, "{leaf}", leafSegment(segs))
	case r.SchemaDepth > 0 && r.SchemaDepth <= len(segs):
		return strings.Join(segs[:r.SchemaDepth], "_")
	default:
		return ""
	}
}

// leafSegment returns the last package segment, dropping a trailing API version
// ("store.apps.calendar.v1" → "calendar").
func leafSegment(segs []string) string {
	i := len(segs) - 1
	if i > 0 && isVersionSegment(segs[i]) {
		i--
	}
	if i < 0 {
		return ""
	}
	return segs[i]
}

// matchPackage reports whether the dotted glob pattern matches package segments.
func matchPackage(pattern string, segs []string) bool {
	pats := strings.Split(pattern, ".")
	for i, p := range pats {
		if p == "**" {
			return true // trailing wildcard: the rest matches
		}
		if i >= len(segs) {
			return false
		}
		if p != "*" && p != segs[i] {
			return false
		}
	}
	return len(pats) == len(segs)
}
