package generator

// qualify.go holds the post-build IR passes that enforce Prisma's global
// namespaces: model and enum names must be unique per database regardless of
// @@schema. They run after every file is merged (see buildDatabases) and before
// relations are resolved, so FK rewiring sees the final names.

import (
	"sort"
	"strconv"
	"strings"

	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

// qualifyModels enforces the Prisma rule that model names occupy one global
// namespace per database (independent of @@schema). Normalizing embedded value
// types pulls same-named messages (a per-package "Media", "Location", …) into
// one database; here the colliding ones gain a schema-domain prefix
// ("calendar" + "Media" → "CalendarMedia") so every model name is unique. Runs
// before resolveRelations, which then reflects the new names onto every FK.
//
// Every participant in a collision is qualified — no occurrence keeps the bare
// name. This makes names stable as the schema grows: adding another same-named
// model later cannot silently rename (and force a destructive migration on) the
// ones already generated, which positional "first stays bare" qualification did.
func qualifyModels(db *schema.Database, diags *diagnostics) {
	byName := map[string][]*schema.Table{}
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			byName[t.ModelName] = append(byName[t.ModelName], t)
		}
	}
	used := map[string]bool{}
	for name, group := range byName {
		if len(group) < 2 {
			used[name] = true
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		group := byName[name]
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			if group[i].PgSchema != group[j].PgSchema {
				return group[i].PgSchema < group[j].PgSchema
			}
			return group[i].ProtoMessage < group[j].ProtoMessage
		})
		for _, t := range group {
			base := naming.PascalGo(naming.SchemaDomain(t.PgSchema)) + t.ModelName
			q := base
			for n := 2; used[q]; n++ {
				q = base + strconv.Itoa(n)
			}
			used[q] = true
			diags.warnf("collision", "model %q is defined in multiple schemas; qualified to %q "+
				"(Prisma model names are global)", t.ModelName, q)
			t.ModelName = q
		}
	}
}

// dedupeEnums enforces the Prisma rule that enum type names occupy one global
// namespace per database (independent of @@schema). It (1) collapses the same
// proto enum built separately under multiple schemas onto a single canonical
// definition, repointing every column to it, and (2) qualifies the names of
// distinct enums that happen to share a simple name with a schema-derived
// prefix ("State" in calendar_app and alarm_app → "State" / "AlarmAppState").
func dedupeEnums(db *schema.Database, diags *diagnostics) {
	// Pass 1: choose one canonical *Enum per proto full name (first seen wins).
	canonical := map[string]*schema.Enum{}
	for _, s := range db.Schemas {
		for _, e := range s.Enums {
			if _, ok := canonical[e.ProtoName]; !ok {
				canonical[e.ProtoName] = e
			}
		}
	}
	// Repoint every enum column to its canonical definition, then keep only the
	// canonical enum in its home schema and drop the duplicates elsewhere.
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			for _, c := range t.Columns {
				if c.Enum != nil {
					c.Enum = canonical[c.Enum.ProtoName]
				}
			}
		}
		kept := s.Enums[:0]
		for _, e := range s.Enums {
			if canonical[e.ProtoName] == e {
				kept = append(kept, e)
			}
		}
		s.Enums = kept
	}

	// Pass 2: qualify simple-name collisions among distinct enums with a
	// schema-domain prefix ("calendar" + "State" → "CalendarState"). Sorted for
	// determinism; the first keeps the bare name, the rest gain a unique prefix.
	byName := map[string][]*schema.Enum{}
	for _, e := range canonical {
		byName[e.Name] = append(byName[e.Name], e)
	}
	used := map[string]bool{}
	for name, group := range byName {
		if len(group) < 2 {
			used[name] = true
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		group := byName[name]
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			if group[i].PgSchema != group[j].PgSchema {
				return group[i].PgSchema < group[j].PgSchema
			}
			return group[i].ProtoName < group[j].ProtoName
		})
		for _, e := range group {
			base := naming.PascalGo(naming.SchemaDomain(e.PgSchema)) + e.Name
			q := base
			for n := 2; used[q]; n++ {
				q = base + strconv.Itoa(n)
			}
			used[q] = true
			diags.warnf("collision", "enum %q is defined in multiple schemas; qualified to %q "+
				"(Prisma enum names are global)", name, q)
			e.Name = q
			e.SQLName = naming.SnakeCase(q)
		}
	}
}

// validateIndexes reports any index that names a column absent from its table,
// so a typo in protorm.v1.table.indexes is caught at generation time instead of
// surfacing as invalid DDL when the schema is applied.
func validateIndexes(db *schema.Database, diags *diagnostics) {
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			cols := make(map[string]bool, len(t.Columns))
			for _, c := range t.Columns {
				cols[c.Name] = true
			}
			for _, idx := range t.Indexes {
				label := idx.Name
				if label == "" {
					label = "(" + strings.Join(idx.Columns, ",") + ")"
				}
				for _, c := range idx.Columns {
					if !cols[c] {
						diags.warnf("index", "table %q index %s names unknown column %q",
							t.Name, label, c)
					}
				}
			}
		}
	}
}
