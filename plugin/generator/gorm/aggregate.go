package gorm

// aggregate.go builds the per-database migration aggregator: a single Go file
// (one package at the database root) exposing a factory Registry preloaded with
// every generated model, so an application migrates the whole database in one
// call. It needs the go_module opt to import each per-schema models package by
// its full import path; without it, no aggregator is emitted.

import (
	"strings"

	"github.com/the-protobuf-project/protorm/plugin/generator/header"
	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

// importView is one per-schema models package the aggregator imports.
type importView struct{ Alias, Path string }

// aggregateView assembles the migrate.go template data: the package name, the
// per-schema imports, and the fully-qualified model expressions (pkg.Model) the
// Default registry is preloaded with, in schema-then-declaration order.
func aggregateView(db *schema.Database) map[string]any {
	var imports []importView
	var models []string
	seen := map[string]bool{}
	var schemaNames []string

	for _, s := range db.Schemas {
		schemaNames = append(schemaNames, s.Name)
		pkg := naming.GoPackage(s.Name)
		for _, t := range s.Tables {
			// Import a package only once it contributes a model, so a schema with
			// no tables never leaves an unused import in the generated file.
			if !seen[pkg] {
				seen[pkg] = true
				imports = append(imports, importView{
					Alias: pkg,
					Path:  db.GoModule + "/" + db.Name + "/" + pkg,
				})
			}
			models = append(models, pkg+"."+t.LocalName)
		}
	}

	return map[string]any{
		"Header": header.Render("//", header.Info{
			PluginVersion: db.PluginVersion,
			ProtocVersion: db.ProtocVersion,
			Database:      db.Name,
			SchemaLabel:   "schemas",
			Schema:        strings.Join(schemaNames, ", "),
			Notes:         []string{"Migration aggregator: every model in one factory Registry."},
		}),
		"Package":     naming.GoPackage(db.Name),
		"Database":    db.Name,
		"Imports":     imports,
		"Models":      models,
		"Schemas":     schemaNames,
		"OTel":        db.OTel,
		"OTelMetrics": db.OTelMetrics,
	}
}
