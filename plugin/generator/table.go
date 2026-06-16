package generator

// table.go assembles a *schema.Table from a proto message: its column list,
// synthesized id/timestamp columns, and the foreign keys inferred from
// resource_reference. Scalar/enum column mapping lives in column.go.

import (
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/plugin/pb/protormpbv1"
)

// buildTable maps one resource-annotated message to a *schema.Table.
func (ctx *buildCtx) buildTable(db *schema.Database, s *schema.Schema, msg *protogen.Message, name, src, srcPath string) *schema.Table {
	t := &schema.Table{
		Name:         name,
		Comment:      cleanComment(msg.Comments.Leading),
		ModelName:    string(msg.Desc.Name()),
		LocalName:    string(msg.Desc.Name()),
		ProtoMessage: string(msg.Desc.FullName()),
		SourceFile:   src,
		SourceProto:  srcPath,
	}

	ctx.populateColumns(db, s, t, msg)
	applyAIPSystemFields(t)
	if res := resourceOf(msg); res != nil {
		materializeParents(t, res)
	}

	tOpts := tableOpts(msg)
	applyIDStrategy(t, idStrategyOf(tOpts))
	applyTimestamps(t, tOpts.GetTimestamps())

	for _, idx := range tOpts.GetIndexes() {
		t.Indexes = append(t.Indexes, &schema.Index{
			Name: idx.GetIndex(), Columns: idx.GetColumns(), Unique: idx.GetUnique(),
		})
	}
	return t
}

// populateColumns maps msg's fields onto t. Scalar/enum fields become columns;
// string fields with google.api.resource_reference become FK columns; and
// user message-typed fields always become embed requests (normalized into
// related tables with a primary key + foreign key by normalizeEmbeds) instead
// of lossy JSONB blobs — unless the field is skipped or pins an explicit column
// type. Maps and google.* well-known types are not normalizable and keep their
// scalar/JSONB mapping. Shared by buildTable (resources) and materialize
// (embedded children).
func (ctx *buildCtx) populateColumns(db *schema.Database, s *schema.Schema, t *schema.Table, msg *protogen.Message) {
	for _, f := range msg.Fields {
		cOpts := colOpts(f)
		if target := normalizableMessage(f); target != "" && cOpts.GetType() == "" {
			if cOpts.GetSkip() {
				continue
			}
			ctx.embeds = append(ctx.embeds, &embedReq{
				db: db, schemaName: s.Name, parent: t, field: f,
				targetMsg: target, repeated: f.Desc.IsList(),
				optional: !isRequiredField(f),
				onDelete: refAction(cOpts.GetOnDelete()),
				onUpdate: refAction(cOpts.GetOnUpdate()),
			})
			continue
		}

		col := buildColumn(s, f)
		if col == nil {
			continue
		}
		t.Columns = append(t.Columns, col)
		if col.PrimaryKey && t.PKColumn == "" {
			t.PKColumn = col.Name
		}
		if ref := resourceRef(f); ref != nil {
			// A repeated resource_reference is a list of resource names, not a
			// single foreign key — a scalar FK column can't hold many parents.
			// Keep it as the array column buildColumn already produced (T[]); a
			// proper relation here would need a join table, which protorm does not
			// synthesize. Modeling it as a single FK would silently drop the list
			// (and collide field names, e.g. exceptions/exceptions2).
			if f.Desc.IsList() {
				continue
			}
			refSchema, refTable := schemaTable(ref.GetType(), "")
			refModel := modelNameFromType(ref.GetType())
			col.FKModel = refModel
			t.ForeignKeys = append(t.ForeignKeys, &schema.ForeignKey{
				Column:           col.Name,
				ReferencedSchema: refSchema,
				ReferencedTable:  refTable,
				ReferencedModel:  refModel,
				OnDelete:         refAction(cOpts.GetOnDelete()),
				OnUpdate:         refAction(cOpts.GetOnUpdate()),
				// ReferencedColumn filled by resolveRelations after all tables built.
			})
		}
	}
	ctx.addOneofDiscriminators(s, t, msg)
}

// refAction converts a ReferentialAction enum to its SQL clause form.
func refAction(a protormpbv1.ReferentialAction) string {
	switch a {
	case protormpbv1.ReferentialAction_REFERENTIAL_ACTION_CASCADE:
		return "CASCADE"
	case protormpbv1.ReferentialAction_REFERENTIAL_ACTION_RESTRICT:
		return "RESTRICT"
	case protormpbv1.ReferentialAction_REFERENTIAL_ACTION_SET_NULL:
		return "SET NULL"
	case protormpbv1.ReferentialAction_REFERENTIAL_ACTION_SET_DEFAULT:
		return "SET DEFAULT"
	case protormpbv1.ReferentialAction_REFERENTIAL_ACTION_NO_ACTION:
		return "NO ACTION"
	default:
		return ""
	}
}
