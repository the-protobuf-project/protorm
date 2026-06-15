package generator

// keys.go holds the column-synthesis passes that run after a table's proto
// fields are mapped: recognizing AIP system fields, choosing and applying the
// primary-key strategy, and appending audit timestamps. Kept separate from
// table.go so each file stays small and single-purpose.

import (
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/protorm/protormpbv1"
)

// applyAIPSystemFields gives the AIP-148/164 standard fields their conventional
// database behavior with no annotation, matching the hand-written
// createdAt/updatedAt/deletedAt convention: create_time/update_time become
// auto-managed NOT NULL audit timestamps, delete_time becomes a nullable indexed
// soft-delete marker, and uid (the server-assigned id) is marked UNIQUE.
func applyAIPSystemFields(t *schema.Table) {
	for _, c := range t.Columns {
		switch c.Name {
		case "create_time":
			c.SQLType, c.Enum = "TIMESTAMPTZ", nil
			c.NotNull, c.Optional, c.AutoCreate = true, false, true
			if c.Default == "" {
				c.Default = "now()"
			}
		case "update_time":
			c.SQLType, c.Enum = "TIMESTAMPTZ", nil
			c.NotNull, c.Optional, c.AutoUpdate = true, false, true
		case "delete_time":
			c.SQLType, c.Enum = "TIMESTAMPTZ", nil
			c.NotNull, c.Optional = false, true // null = live row
			t.Indexes = append(t.Indexes, &schema.Index{Columns: []string{"delete_time"}})
		case "uid":
			c.Unique = true
		}
	}
}

// idStrategyOf returns the table's id strategy, defaulting to a ULID surrogate
// key when unset. This is the production pattern: the AIP resource name becomes a
// UNIQUE lookup key while a generated id is the storage primary key. An explicit
// protorm.v1.table.id (ULID or UUID) overrides the default.
func idStrategyOf(tOpts *protormpbv1.TableOptions) protormpbv1.IdStrategy {
	if s := tOpts.GetId(); s != protormpbv1.IdStrategy_ID_STRATEGY_UNSPECIFIED {
		return s
	}
	return protormpbv1.IdStrategy_ID_STRATEGY_ULID
}

// applyIDStrategy synthesizes a generated `id` PK column and demotes any
// IDENTIFIER-derived primary key to a UNIQUE constraint. If the message already
// declares an `id` column, that one is promoted to the PK instead of
// synthesizing a duplicate (server-assigned ids carry no client-side default).
func applyIDStrategy(t *schema.Table, st protormpbv1.IdStrategy) {
	if st == protormpbv1.IdStrategy_ID_STRATEGY_UNSPECIFIED {
		return
	}
	for _, c := range t.Columns {
		if c.Name == "id" {
			for _, o := range t.Columns {
				if o.PrimaryKey && o != c {
					o.PrimaryKey, o.Unique = false, true
				}
			}
			c.PrimaryKey, c.NotNull, c.Optional = true, true, false
			t.PKColumn = "id"
			return
		}
	}
	for _, c := range t.Columns {
		if c.PrimaryKey {
			c.PrimaryKey, c.Unique = false, true
		}
	}
	id := &schema.Column{
		Name:       "id",
		Comment:    "Unique identifier for the record.",
		PrimaryKey: true,
		NotNull:    true,
	}
	switch st {
	case protormpbv1.IdStrategy_ID_STRATEGY_ULID:
		id.SQLType, id.Generated = "CHAR(26)", "ulid"
	case protormpbv1.IdStrategy_ID_STRATEGY_UUID:
		id.SQLType, id.Generated = "UUID", "uuid"
	}
	t.Columns = append([]*schema.Column{id}, t.Columns...)
	t.PKColumn = "id"
}

// applyTimestamps appends created_at / updated_at TIMESTAMPTZ columns.
func applyTimestamps(t *schema.Table, on bool) {
	if !on {
		return
	}
	t.Columns = append(t.Columns,
		&schema.Column{
			Name: "created_at", Comment: "Timestamp when the record was created.",
			SQLType: "TIMESTAMPTZ", NotNull: true, Default: "now()", AutoCreate: true,
		},
		&schema.Column{
			Name: "updated_at", Comment: "Timestamp when the record was last updated.",
			SQLType: "TIMESTAMPTZ", NotNull: true, Default: "now()", AutoUpdate: true,
		},
	)
}
