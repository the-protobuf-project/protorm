package generator

// embed_fk.go holds the foreign-key plumbing for embedded children: choosing a
// default referential action, guaranteeing each child has a primary key to point
// at, and synthesizing the scalar FK column + constraint that links parent and
// child. Split out of embed.go to keep both files small and single-purpose.

import "github.com/the-protobuf-project/protorm/plugin/generator/schema"

// embedAction picks the default ON DELETE for a synthesized embed relation when
// the proto sets none. An embedded message is a value object owned by its
// parent, so a required link cascades (deleting the owner removes the owned row)
// and an optional one nulls out. An explicit protorm.v1.col.on_delete wins.
func embedAction(explicit string, required bool) string {
	if explicit != "" {
		return explicit
	}
	if required {
		return "CASCADE"
	}
	return "SET NULL"
}

// ensurePK guarantees the table has a primary key so FK references resolve:
// an existing IDENTIFIER PK is kept; otherwise an existing "id" column is
// promoted; otherwise a ULID `id` column is synthesized.
func ensurePK(t *schema.Table) {
	if t.PKColumn != "" {
		return
	}
	for _, c := range t.Columns {
		if c.Name == "id" {
			c.PrimaryKey, c.NotNull, c.Optional = true, true, false
			t.PKColumn = "id"
			return
		}
	}
	id := &schema.Column{
		Name: "id", Comment: "Unique identifier for the record.",
		PrimaryKey: true, NotNull: true, SQLType: "CHAR(26)", Generated: "ulid",
	}
	t.Columns = append([]*schema.Column{id}, t.Columns...)
	t.PKColumn = "id"
}

// addFKColumn appends a synthesized scalar FK column and its ForeignKey to t,
// unless the column already exists (idempotent for multi-parent children).
// refProto pins the exact target message so resolution survives same-named
// models. ReferencedColumn/type are finalized by resolveRelations.
func addFKColumn(t *schema.Table, col, refModel, refProto string, notNull bool, onDelete, onUpdate string) {
	for _, c := range t.Columns {
		if c.Name == col {
			return
		}
	}
	t.Columns = append(t.Columns, &schema.Column{
		Name:     col,
		Comment:  "Foreign key to " + refModel + ".",
		SQLType:  "CHAR(26)",
		NotNull:  notNull,
		Optional: !notNull,
		FKModel:  refModel,
	})
	t.ForeignKeys = append(t.ForeignKeys, &schema.ForeignKey{
		Column:          col,
		ReferencedModel: refModel,
		ReferencedProto: refProto,
		OnDelete:        onDelete,
		OnUpdate:        onUpdate,
	})
}
