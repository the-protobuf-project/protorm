package gorm

// view.go prepares the models.go template view: naming, Go types, struct tags,
// enum consts, and conditional imports. The template is presentation only.

import (
	"sort"
	"strings"

	"github.com/the-protobuf-project/protorm/plugin/generator/header"
	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

type fieldView struct{ Comment, Decl string }

type modelView struct {
	Comment, Name, TableName string
	Fields                   []fieldView
}

type enumValueView struct{ Comment, ConstName, TypeName, MapName string }

type enumView struct {
	Comment, Name string
	Values        []enumValueView
}

// packageView assembles the template data for one schema package.
func packageView(db *schema.Database, s *schema.Schema, pkg string) map[string]any {
	var models []modelView
	needTime, needJSON, needPQ := false, false, false

	// Go packages are per-schema, so structs use the bare LocalName. Related
	// models (FK / has-many targets) carry the globally-qualified ModelName,
	// which loc translates back to the local name they're declared under.
	loc := localNameFunc(db)
	// inThisSchema reports whether a model is declared in the schema being
	// rendered. Same-package targets get a direct association field. A cross-schema
	// target would need importing another package, which risks an import cycle, so
	// it is normally omitted — except a value object (a leaf table nothing points
	// back through), which is safe to import so the generic engine can Preload it
	// (see the cross-schema branch below). The scalar FK column is always kept.
	inThisSchema := modelSchemaSet(db, s.Name)
	tableByModel := tableByModelFunc(db)
	voEdges := crossSchemaVOEdges(db)
	assocImports := map[string]bool{} // cross-schema value-object packages to import

	for _, t := range s.Tables {
		m := modelView{
			Comment:   commentOr(t.Comment, t.LocalName+" model."),
			Name:      t.LocalName,
			TableName: s.Name + "." + t.Name,
		}
		// Reserve scalar Go field names so association fields stay unique — two
		// FKs to the same model must not produce two identically-named fields.
		used := map[string]bool{}
		for _, col := range t.Columns {
			used[gormFieldName(col)] = true
		}
		idxTags := indexTagsByColumn(t)
		for _, col := range t.Columns {
			gt := goType(col)
			needTime = needTime || strings.Contains(gt, "time.Time")
			needJSON = needJSON || strings.Contains(gt, "json.RawMessage")
			needPQ = needPQ || strings.HasPrefix(gt, "pq.")

			goField := gormFieldName(col)
			extra := idxTags[col.Name]
			if col.Enum != nil {
				if chk := enumCheck(t.Name, col); chk != "" {
					extra = append(extra, chk)
				}
			}
			m.Fields = append(m.Fields, fieldView{
				Comment: col.Comment,
				Decl:    goField + " " + gt + " `" + structTag(col, extra) + "`",
			})
			// BelongsTo association: emitted alongside the FK column. The field is
			// named after the FK column (minus _id) so multiple references to the
			// same model stay distinct; GORM resolves the link via foreignKey.
			if col.FKModel != "" && inThisSchema(col.FKModel) {
				assoc := uniqueGoName(naming.PascalGo(naming.StripIDSuffix(col.Name)), used)
				m.Fields = append(m.Fields, fieldView{
					Decl: assoc + " *" + loc(col.FKModel) +
						" `gorm:\"foreignKey:" + goField + constraintTag(t, col.Name) +
						"\" json:\"" + strings.ToLower(assoc) + ",omitempty\"`",
				})
			} else if tgt := tableByModel(col.FKModel); col.FKModel != "" && tgt != nil &&
				tgt.ValueObject && db.GoModule != "" && emitCrossAssoc(s.Name, tgt.PgSchema, voEdges) {
				// Cross-schema belongs-to to a value object: a leaf table, so importing
				// its package can't form a cycle. Emitting the association lets the
				// generic engine Preload it instead of hydrating Money/Address by hand.
				tgtPkg := naming.GoPackage(tgt.PgSchema)
				assoc := uniqueGoName(naming.PascalGo(naming.StripIDSuffix(col.Name)), used)
				m.Fields = append(m.Fields, fieldView{
					Decl: assoc + " *" + tgtPkg + "." + tgt.LocalName +
						" `gorm:\"foreignKey:" + goField + constraintTag(t, col.Name) +
						"\" json:\"" + strings.ToLower(assoc) + ",omitempty\"`",
				})
				assocImports[db.GoModule+"/"+db.Name+"/"+tgtPkg] = true
			}
		}
		// HasMany back-references (e.g. Author.Books []Book). Same-schema only:
		// the child type lives in another package otherwise (see inThisSchema).
		for _, hm := range t.HasMany {
			if !inThisSchema(hm.Model) {
				continue
			}
			field := uniqueGoName(naming.PascalGo(hm.Field), used)
			childModel := loc(hm.Model)
			m.Fields = append(m.Fields, fieldView{
				Comment: "Back-relation: " + childModel + " records that reference this via " + hm.ViaFK + ".",
				Decl: field + " []" + childModel +
					" `gorm:\"foreignKey:" + naming.PascalGo(hm.ViaFK) + "\" json:\"" + strings.ToLower(field) + ",omitempty\"`",
			})
		}
		models = append(models, m)
	}

	var std []string
	if needJSON {
		std = append(std, "encoding/json")
	}
	if needTime {
		std = append(std, "time")
	}
	var third []string
	if needPQ {
		third = append(third, "github.com/lib/pq")
	}
	for imp := range assocImports {
		third = append(third, imp)
	}
	sort.Strings(third)

	return map[string]any{
		"Header": header.Render("//", header.Info{
			PluginVersion: db.PluginVersion,
			ProtocVersion: db.ProtocVersion,
			Source:        strings.Join(s.SourceProtos(), ", "),
			Database:      db.Name,
			Schema:        s.Name,
		}),
		"Package": pkg,
		"Imports": importBlock(std, third),
		"Enums":   enumViews(s),
		"Models":  models,
	}
}

// enumViews renders each schema enum as a Go string type with one const per
// value. The Go type uses the bare LocalName — the package already namespaces
// it, so the global-collision prefix Prisma needs would be redundant here.
func enumViews(s *schema.Schema) []enumView {
	var out []enumView
	for _, e := range s.Enums {
		ev := enumView{
			Comment: commentOr(e.Comment, e.LocalName+" enumerates the "+e.LocalSQLName+" values."),
			Name:    e.LocalName,
		}
		for _, v := range e.Values {
			ev.Values = append(ev.Values, enumValueView{
				Comment:   v.Comment,
				ConstName: e.LocalName + naming.PascalGo(strings.ToLower(v.Name)),
				TypeName:  e.LocalName,
				MapName:   v.MapName,
			})
		}
		out = append(out, ev)
	}
	return out
}

// tableByModelFunc indexes every table in the database by its globally-unique
// ModelName, so an FK target's table (and thus its schema, local name, and
// value-object flag) can be looked up while rendering another schema's package.
func tableByModelFunc(db *schema.Database) func(string) *schema.Table {
	idx := map[string]*schema.Table{}
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			idx[t.ModelName] = t
		}
	}
	return func(m string) *schema.Table { return idx[m] }
}

// crossSchemaVOEdges returns the directed schema pairs (from→to) that carry at
// least one cross-schema FK to a value-object table. emitCrossAssoc uses it to
// break a would-be import cycle when two schemas reference each other's value
// objects.
func crossSchemaVOEdges(db *schema.Database) map[[2]string]bool {
	byModel := tableByModelFunc(db)
	edges := map[[2]string]bool{}
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			for _, col := range t.Columns {
				if col.FKModel == "" {
					continue
				}
				if tgt := byModel(col.FKModel); tgt != nil && tgt.ValueObject && tgt.PgSchema != t.PgSchema {
					edges[[2]string{t.PgSchema, tgt.PgSchema}] = true
				}
			}
		}
	}
	return edges
}

// emitCrossAssoc reports whether the from→to cross-schema value-object
// association should be emitted. When the reverse edge also exists (mutual
// value-object references), only the lexically smaller schema emits its side, so
// the two generated packages never import each other.
func emitCrossAssoc(from, to string, edges map[[2]string]bool) bool {
	return !edges[[2]string{to, from}] || from <= to
}

// modelSchemaSet returns a predicate reporting whether a model (by its
// globally-qualified ModelName) is declared in the named schema — i.e. lives in
// the same Go package as the structs being rendered. Used to gate cross-schema
// association fields, which can't reference another package's type without an
// import (and references can be cyclic).
func modelSchemaSet(db *schema.Database, schemaName string) func(string) bool {
	here := map[string]bool{}
	for _, s := range db.Schemas {
		if s.Name != schemaName {
			continue
		}
		for _, t := range s.Tables {
			here[t.ModelName] = true
		}
	}
	return func(model string) bool { return here[model] }
}

// localNameFunc returns a translator from a model's globally-qualified ModelName
// to the bare LocalName it is declared under in its Go package. Unknown names
// (soft-FK targets outside the generate set) pass through unchanged.
func localNameFunc(db *schema.Database) func(string) string {
	local := map[string]string{}
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			local[t.ModelName] = t.LocalName
		}
	}
	return func(model string) string {
		if v, ok := local[model]; ok {
			return v
		}
		return model
	}
}
