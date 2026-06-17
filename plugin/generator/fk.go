package generator

// fk.go holds the foreign-key post-passes that run after resolveRelations:
// indexing FK columns and degrading unresolved references into soft FKs.

import (
	"strings"

	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

// indexForeignKeys adds a single-column index for every foreign-key column that
// isn't already covered by one. PostgreSQL indexes primary keys and unique
// columns automatically but never foreign keys, so without this every join and
// cascade falls back to a sequential scan. Default-on because an unindexed FK is
// almost never intentional; a PK/unique/explicit index on the same column
// suppresses the duplicate. Runs after resolveRelations so only kept FKs (not
// dropped, unresolved ones) are indexed.
//
// A multi-column index covers its leading column for single-column lookups (a
// PostgreSQL B-tree serves the leftmost prefix), so a composite index whose
// first column is the FK suppresses the standalone FK index — emitting both
// would just add write and storage overhead for no read benefit.
func indexForeignKeys(db *schema.Database) {
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			indexed := map[string]bool{}
			for _, c := range t.Columns {
				if c.PrimaryKey || c.Unique || c.Index {
					indexed[c.Name] = true
				}
			}
			for _, idx := range t.Indexes {
				if len(idx.Columns) > 0 {
					indexed[idx.Columns[0]] = true // leading column is covered by the index prefix
				}
			}
			for _, fk := range t.ForeignKeys {
				if indexed[fk.Column] {
					continue
				}
				indexed[fk.Column] = true
				t.Indexes = append(t.Indexes, &schema.Index{Columns: []string{fk.Column}})
			}
		}
	}
}

// nameIndexes assigns a deterministic name to every index that doesn't declare
// one, so all targets reference the same identifier. The scheme matches what the
// SQL target historically generated inline ("idx_<table>_<col>_<col>"); GORM's
// index struct tags reuse it so the two backends produce the same physical
// schema. Runs after indexForeignKeys so synthesized FK indexes are named too.
func nameIndexes(db *schema.Database) {
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			for _, idx := range t.Indexes {
				if idx.Name == "" {
					idx.Name = "idx_" + t.Name + "_" + strings.Join(idx.Columns, "_")
				}
			}
		}
	}
}

// softFK degrades an unresolved resource_reference into a plain indexed column
// instead of dropping it. The scalar value is kept, the FK association is cleared
// (so no relation is rendered to a model that isn't in this generation set), a
// single-column index is added so cross-service lookups aren't a sequential
// scan, and a TODO note records the unresolved target for review. This is the
// common case in a multi-service monorepo, where references routinely cross
// service boundaries that a single generation run can't see.
func softFK(t *schema.Table, col, refModel string) {
	note := "TODO: unresolved reference to " + refModel +
		" (not in this generation set); kept as a plain indexed column."
	for _, c := range t.Columns {
		if c.Name != col {
			continue
		}
		c.FKModel = "" // drop the association so targets don't emit a broken relation
		if c.Comment == "" {
			c.Comment = note
		} else {
			c.Comment += " " + note
		}
	}
	for _, idx := range t.Indexes {
		if len(idx.Columns) == 1 && idx.Columns[0] == col {
			return
		}
	}
	t.Indexes = append(t.Indexes, &schema.Index{Columns: []string{col}})
}
