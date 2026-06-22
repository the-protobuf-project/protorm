package generator

// parent.go materializes the ownership hierarchy encoded in an AIP resource
// pattern into real foreign-key columns. A pattern like
// "users/{user}/events/{event}" means an Event is owned by a User, so protorm
// adds a user_id FK column → User: indexed by indexForeignKeys and resolved by
// resolveRelations (degrading to a soft FK when the parent lives outside this
// generation set). It cascades on delete, matching AIP parent→child ownership.

import (
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"

	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

// materializeParents adds a FK column for every parent segment of the resource's
// pattern — all collection/{var} pairs except the final one, which names the
// resource itself. The variable is the singular parent ("{user}"), so the column
// is "<var>_id" referencing model PascalGo(var) ("User"). A pair whose column
// already exists (an explicit field or resource_reference covers that parent) is
// left untouched.
func materializeParents(t *schema.Table, res *annotations.ResourceDescriptor) {
	patterns := res.GetPattern()
	if len(patterns) == 0 {
		return
	}
	segs := strings.Split(patterns[0], "/")
	for i := 0; i < len(segs)/2-1; i++ {
		collection, variable := segs[2*i], strings.Trim(segs[2*i+1], "{}")
		if collection == "" || variable == "" {
			continue
		}
		// Record the parent regardless of whether its FK column already exists, so a
		// nested resource stays recognizable downstream (an M2M join to it captures
		// the full hierarchical key, not just the leaf id).
		t.Parents = append(t.Parents, variable)
		col := variable + "_id"
		if hasColumn(t, col) {
			continue
		}
		refModel := naming.PascalGo(variable)
		t.Columns = append(t.Columns, &schema.Column{
			Name:    col,
			Comment: "Parent reference to " + refModel + " (from the AIP resource pattern).",
			SQLType: "CHAR(26)",
			NotNull: true,
			FKModel: refModel,
		})
		t.ForeignKeys = append(t.ForeignKeys, &schema.ForeignKey{
			Column:          col,
			ReferencedModel: refModel,
			OnDelete:        "CASCADE", // an owned child cascades from its parent
		})
	}
}
