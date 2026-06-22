package generator

// m2m.go models a repeated google.api.resource_reference field as a proper
// many-to-many relation. A scalar foreign-key column can hold only one parent,
// so a repeated reference (e.g. Schedule.exceptions → many AvailabilityException)
// cannot be a single FK. Instead we synthesize an explicit join table with a FK
// to each side, a surrogate id PK, and a unique composite index so a pair links
// at most once. The join is an ordinary table, so FK resolution, the has-many
// back-references, and every target renderer pick it up with no special casing.
//
// When the referenced model is not in the generate set there is no PK to point
// at, so the field falls back to a plain array column (a list of resource names),
// matching how a repeated scalar without a reference is rendered.

import (
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

// m2mReq is one repeated resource_reference field awaiting a join table.
type m2mReq struct {
	db          *schema.Database
	schemaName  string          // schema the parent lives in; the join table joins it
	parent      *schema.Table   // table carrying the repeated reference field
	field       *protogen.Field // the repeated string + resource_reference field
	targetModel string          // referenced model name (from the resource type)
	onDelete    string
	onUpdate    string
}

// normalizeM2M drains the join-table queue: for each repeated reference whose
// target is in the generate set it synthesizes a join table; otherwise it falls
// back to a plain array column on the parent. Runs after normalizeEmbeds (so all
// base and embedded tables exist) and before resolveRelations (which then wires
// the join's FKs and the has-many sides).
func (ctx *buildCtx) normalizeM2M(diags *diagnostics) {
	for _, req := range ctx.m2m {
		child := findTableByModel(req.db, req.targetModel)
		if child == nil {
			ctx.fallbackArrayColumn(req)
			diags.warnf("ref", "table %q repeated reference %q targets unknown model %q; "+
				"kept as a plain array column (no join table)",
				req.parent.Name, req.field.Desc.Name(), req.targetModel)
			continue
		}
		ctx.buildJoinTable(req, child)
	}
}

// buildJoinTable synthesizes the join table linking req.parent and child for the
// repeated field, placed in the parent's schema.
func (ctx *buildCtx) buildJoinTable(req *m2mReq, child *schema.Table) {
	parent := req.parent
	parentCol := naming.SnakeCase(parent.ModelName) + "_id"
	childCol := naming.SnakeCase(child.ModelName) + "_id"
	// Distinct names keep two references to the same child (or a self-reference)
	// from colliding: join name carries both the parent and the field.
	joinTable := naming.SnakeCase(parent.ModelName) + "_" + string(req.field.Desc.Name())
	joinModel := naming.PascalGo(joinTable)

	join := &schema.Table{
		Name: joinTable,
		Comment: "Join table for the many-to-many relation " + parent.ModelName + "." +
			string(req.field.Desc.Name()) + " ↔ " + child.ModelName + ".",
		ModelName:   joinModel,
		LocalName:   joinModel,
		SourceFile:  parent.SourceFile,
		SourceProto: parent.SourceProto,
		SourceDir:   parent.SourceDir,
		PgSchema:    req.schemaName,
	}
	// One FK to each side. A link row is meaningless once either side is gone, so
	// both cascade on delete (an explicit on_delete on the field overrides).
	addFKColumn(join, parentCol, parent.ModelName, parent.ProtoMessage, true, embedAction(req.onDelete, true), req.onUpdate)
	addFKColumn(join, childCol, child.ModelName, child.ProtoMessage, true, embedAction(req.onDelete, true), req.onUpdate)
	// When the child is a nested resource, the leaf id alone can't reconstruct its
	// full name (resources/{resource}/offerings/{offering} loses the {resource}
	// parent). Capture the full resource name in a sibling column so callers can
	// round-trip it without overloading the id.
	if len(child.Parents) > 0 {
		nameCol := naming.SnakeCase(child.ModelName) + "_name"
		join.Columns = append(join.Columns, &schema.Column{
			Name: nameCol,
			Comment: "Full resource name of the referenced " + child.ModelName +
				", capturing the parent hierarchy the " + childCol + " id alone omits.",
			SQLType: "TEXT",
			NotNull: true,
		})
	}
	// A unique composite index makes the pair the logical key (the surrogate id
	// added by ensurePK is just a convenient single-column PK for the ORM).
	join.Indexes = append(join.Indexes, &schema.Index{
		Columns: []string{parentCol, childCol}, Unique: true,
	})
	ensurePK(join)

	s := schemaByName(req.db, req.schemaName)
	s.Tables = append(s.Tables, join)
}

// fallbackArrayColumn renders an unresolved repeated reference as a plain array
// column on the parent (a list of resource names), the same shape a repeated
// scalar without a reference produces.
func (ctx *buildCtx) fallbackArrayColumn(req *m2mReq) {
	s := schemaByName(req.db, req.schemaName)
	col := buildColumn(s, req.field)
	if col == nil {
		return
	}
	req.parent.Columns = append(req.parent.Columns, col)
}

// findTableByModel returns the table whose ModelName matches model, searching
// every schema in db, or nil when no table declares that model.
func findTableByModel(db *schema.Database, model string) *schema.Table {
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			if t.ModelName == model {
				return t
			}
		}
	}
	return nil
}
