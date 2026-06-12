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

	"github.com/oh-tarnished/protorm/plugin/generator/naming"
	"github.com/oh-tarnished/protorm/plugin/generator/schema"
	"github.com/oh-tarnished/protorm/plugin/generator/templates"
	"github.com/oh-tarnished/protorm/plugin/generator/types"
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
		}
	}
	return nil
}
