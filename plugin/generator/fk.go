package generator

// fk.go holds the foreign-key post-passes that run after resolveRelations:
// indexing FK columns and degrading unresolved references into soft FKs.

import "github.com/the-protobuf-project/protorm/plugin/generator/schema"

// indexForeignKeys adds a single-column index for every foreign-key column that
// isn't already covered by one. PostgreSQL indexes primary keys and unique
// columns automatically but never foreign keys, so without this every join and
// cascade falls back to a sequential scan. Default-on because an unindexed FK is
// almost never intentional; a PK/unique/explicit index on the same column
// suppresses the duplicate. Runs after resolveRelations so only kept FKs (not
// dropped, unresolved ones) are indexed.
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
				if len(idx.Columns) == 1 {
					indexed[idx.Columns[0]] = true
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
