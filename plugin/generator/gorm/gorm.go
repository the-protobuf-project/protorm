// Package gorm generates production-ready Go structs with GORM struct tags.
//
// Output layout follows Go package conventions — one directory per schema,
// package name matching its directory (underscores stripped):
//
//	<db>/<schemapkg>/models.go    e.g. bookstore_db/bookstorev1/models.go
//
// Nullable fields are pointer types (*string, *int32, …). Proto enums become
// string-typed Go enums with one const per value. Imports are conditional.
package gorm

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/the-protobuf-project/protorm/plugin/generator/docs"
	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/plugin/generator/templates"
	"github.com/the-protobuf-project/protorm/plugin/generator/types"
)

// Generator implements schema.Target for GORM Go struct output.
type Generator struct{}

// Name returns the target identifier used in buf.gen.yaml opt: [target=gorm].
func (g *Generator) Name() string { return "gorm" }

// Generate writes one Go package per schema into the plugin response.
func (g *Generator) Generate(p *protogen.Plugin, dbs []*schema.Database) error {
	for _, db := range dbs {
		if types.Provider(db.Provider) == types.MongoDB {
			return fmt.Errorf("gorm: database %q uses provider mongodb — the gorm target only supports postgres", db.Name)
		}
		for _, s := range db.Schemas {
			pkg := naming.GoPackage(s.Name)
			f := p.NewGeneratedFile(fmt.Sprintf("%s/%s/models.go", db.Name, pkg), "")
			if err := templates.Render(f, "models.go.tpl", packageView(db, s, pkg)); err != nil {
				return fmt.Errorf("gorm: %s/%s: %w", db.Name, pkg, err)
			}
			// Opt-in: a typed CRUD store per resource, one file per model, sharing
			// the models package. Skip empty schemas so we never emit the shared
			// options file with no store using it.
			if db.Stores && len(s.Tables) > 0 {
				of := p.NewGeneratedFile(fmt.Sprintf("%s/%s/store_options.go", db.Name, pkg), "")
				if err := templates.Render(of, "store_options.go.tpl", storeOptionsView(db, s, pkg)); err != nil {
					return fmt.Errorf("gorm: %s/%s/store_options.go: %w", db.Name, pkg, err)
				}
				for _, t := range s.Tables {
					sf := p.NewGeneratedFile(fmt.Sprintf("%s/%s/%s", db.Name, pkg, storeFileName(t)), "")
					if err := templates.Render(sf, "store_model.go.tpl", storeModelView(db, s, pkg, t)); err != nil {
						return fmt.Errorf("gorm: %s/%s/%s: %w", db.Name, pkg, storeFileName(t), err)
					}
				}
			}
		}
		// The migration aggregator imports each per-schema package by its full Go
		// import path, so it can only be generated when go_module gives us the
		// output tree's base import path.
		if db.GoModule != "" {
			mf := p.NewGeneratedFile(fmt.Sprintf("%s/migrate.go", db.Name), "")
			if err := templates.Render(mf, "migrate.go.tpl", aggregateView(db)); err != nil {
				return fmt.Errorf("gorm: %s/migrate.go: %w", db.Name, err)
			}
		}
		if err := writeReadme(p, db); err != nil {
			return err
		}
	}
	return nil
}

// writeReadme documents the generated package tree: an ER diagram and per-model
// reference under the bare, schema-local names the Go packages use.
func writeReadme(p *protogen.Plugin, db *schema.Database) error {
	rf := p.NewGeneratedFile(db.Name+"/README.md", "")
	outputs := []string{
		"`<schema>/models.go` — one Go package per schema, one struct per table.",
		"`migrate.go` — a factory `Registry` (with a preloaded `Default`) that migrates every model in one call; emitted when the `go_module` opt is set.",
		"Nullable columns are pointer types; proto enums become string-typed Go enums.",
		"Attach in main: `Default.Migrate(db)`, or wire the structs into a `*gorm.DB` and run AutoMigrate yourself.",
	}
	if db.Stores {
		outputs = append(outputs,
			"`<schema>/<model>_store.go` — a typed CRUD store per resource (Create, GetByID, List, Count, Update, DeleteByID, plus GetBy/ListBy finders for unique and foreign-key columns), sharing `<schema>/store_options.go`; emitted when the `stores` opt is set. Requires `gorm.io/gorm`.",
		)
	}
	if db.OTel {
		outputs = append(outputs,
			"`Registry.Instrument(db)` in `migrate.go` — installs the OpenTelemetry GORM tracing plugin; on by default (set the `otel` opt false to omit), emitted with `go_module`. Requires `gorm.io/plugin/opentelemetry`.",
		)
	}
	md := docs.Render(db, docs.Meta{
		Title:   "GORM models",
		Tagline: "Go structs with GORM struct tags — one package per schema.",
		Outputs: outputs,
		Naming:  docs.Local(db),
	})
	if _, err := rf.Write([]byte(md)); err != nil {
		return fmt.Errorf("gorm: %s/README.md: %w", db.Name, err)
	}
	return nil
}
