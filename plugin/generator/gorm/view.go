package gorm

// view.go prepares the models.go template view: naming, Go types, struct tags,
// enum consts, and conditional imports. The template is presentation only.

import (
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

type enumValueView struct{ ConstName, TypeName, MapName string }

type enumView struct {
	Comment, Name string
	Values        []enumValueView
}

// packageView assembles the template data for one schema package.
func packageView(db *schema.Database, s *schema.Schema, pkg string) map[string]any {
	var models []modelView
	needTime, needJSON := false, false

	// Go packages are per-schema, so structs use the bare LocalName. Related
	// models (FK / has-many targets) carry the globally-qualified ModelName,
	// which loc translates back to the local name they're declared under.
	loc := localNameFunc(db)

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
			used[naming.PascalGo(col.Name)] = true
		}
		for _, col := range t.Columns {
			gt := goType(col)
			needTime = needTime || strings.Contains(gt, "time.Time")
			needJSON = needJSON || strings.Contains(gt, "json.RawMessage")

			goField := naming.PascalGo(col.Name)
			m.Fields = append(m.Fields, fieldView{
				Comment: col.Comment,
				Decl:    goField + " " + gt + " `" + structTag(col) + "`",
			})
			// BelongsTo association: emitted alongside the FK column. The field is
			// named after the FK column (minus _id) so multiple references to the
			// same model stay distinct; GORM resolves the link via foreignKey.
			if col.FKModel != "" {
				assoc := uniqueGoName(naming.PascalGo(naming.StripIDSuffix(col.Name)), used)
				m.Fields = append(m.Fields, fieldView{
					Decl: assoc + " *" + loc(col.FKModel) +
						" `gorm:\"foreignKey:" + goField + constraintTag(t, col.Name) +
						"\" json:\"" + strings.ToLower(assoc) + ",omitempty\"`",
				})
			}
		}
		// HasMany back-references (e.g. Author.Books []Book).
		for _, hm := range t.HasMany {
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

	var imports []string
	if needJSON {
		imports = append(imports, "encoding/json")
	}
	if needTime {
		imports = append(imports, "time")
	}

	return map[string]any{
		"Header": header.Render("//", header.Info{
			PluginVersion: db.PluginVersion,
			ProtocVersion: db.ProtocVersion,
			Source:        strings.Join(s.SourceProtos(), ", "),
			Database:      db.Name,
			Schema:        s.Name,
		}),
		"Package": pkg,
		"Imports": imports,
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
				ConstName: e.LocalName + naming.PascalGo(strings.ToLower(v.Name)),
				TypeName:  e.LocalName,
				MapName:   v.MapName,
			})
		}
		out = append(out, ev)
	}
	return out
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
