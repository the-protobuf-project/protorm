package gorm

// store.go prepares the per-resource store template views: one typed CRUD store
// per table, generated from the same Table IR that drives the structs in
// view.go. The generated surface mirrors a Hasura/PostGraphile-style auto-CRUD
// API (create, get-by-id, list with limit/offset/order/where, count, update,
// delete-by-id) plus typed finders (GetBy<Col> for unique columns, ListBy<FK>
// for foreign keys) — derived generically from each resource, no per-proto code.
//
// Stores share the models package (so they reference the model types directly
// with no import-path requirement) but are emitted as one file per model to keep
// each file small. Opt-in via the stores plugin opt: it introduces a
// gorm.io/gorm dependency the dependency-free models package otherwise avoids.

import (
	"strings"

	"github.com/the-protobuf-project/protorm/plugin/generator/header"
	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

// finderView is one typed finder method: GetBy<Col> for a unique column or
// ListBy<Col> for a foreign-key column.
type finderView struct {
	Method  string // Go method name, e.g. "GetByName" or "ListByAuthorID"
	Column  string // db column name used in the WHERE clause
	ArgType string // Go type of the lookup argument (never a pointer)
}

// storeOptionsView assembles the data for the shared store_options.go file: just
// the banner and package name. The ListOptions type and apply helper are static.
func storeOptionsView(db *schema.Database, s *schema.Schema, pkg string) map[string]any {
	return map[string]any{
		"Header":  storeHeader(db, s),
		"Package": pkg,
	}
}

// storeModelView assembles the data for one resource's <model>_store.go file.
func storeModelView(db *schema.Database, s *schema.Schema, pkg string, t *schema.Table) map[string]any {
	var unique, fks []finderView
	var pkArgType string
	hasPK := false
	for _, col := range t.Columns {
		if col.Name == t.PKColumn {
			hasPK = true
			pkArgType = baseGoType(col)
		}
		if col.Unique && !col.PrimaryKey {
			unique = append(unique, finderView{
				Method:  "GetBy" + gormFieldName(col),
				Column:  col.Name,
				ArgType: baseGoType(col),
			})
		}
		if col.FKModel != "" {
			fks = append(fks, finderView{
				Method:  "ListBy" + gormFieldName(col),
				Column:  col.Name,
				ArgType: baseGoType(col),
			})
		}
	}

	return map[string]any{
		"Header":        storeHeader(db, s),
		"Package":       pkg,
		"Comment":       commentOr(t.Comment, t.LocalName+" model."),
		"Name":          t.LocalName,
		"Store":         t.LocalName + "Store",
		"TableName":     s.Name + "." + t.Name,
		"HasPK":         hasPK,
		"PKColumn":      t.PKColumn,
		"PKArgType":     pkArgType,
		"UniqueFinders": unique,
		"FKFinders":     fks,
	}
}

// storeFileName is the output base name for a table's store file
// (Author → "author_store.go").
func storeFileName(t *schema.Table) string {
	return naming.SnakeCase(t.LocalName) + "_store.go"
}

// baseGoType is the column's Go type with any nullable pointer stripped — lookup
// arguments take the plain value (a *string FK column is still queried by string).
func baseGoType(col *schema.Column) string {
	return strings.TrimPrefix(goType(col), "*")
}

// storeHeader renders the generated-file banner shared by every store file in a
// schema package, matching the models.go banner.
func storeHeader(db *schema.Database, s *schema.Schema) string {
	return header.Render("//", header.Info{
		PluginVersion: db.PluginVersion,
		ProtocVersion: db.ProtocVersion,
		Source:        strings.Join(s.SourceProtos(), ", "),
		Database:      db.Name,
		Schema:        s.Name,
	})
}
