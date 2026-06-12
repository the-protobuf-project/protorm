// Package sql generates PostgreSQL DDL from the protorm IR.
//
// Output layout — one file per schema, mirroring the prisma fragment tree:
//
//	<db>/<schema>.postgres.sql
//
// Each file carries CREATE SCHEMA, CREATE TYPE for every enum, CREATE TABLE
// with inline comments, FK constraints referencing the resolved PK column,
// and CREATE INDEX statements.
package sql

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/oh-tarnished/protorm/plugin/generator/schema"
	"github.com/oh-tarnished/protorm/plugin/generator/templates"
	"github.com/oh-tarnished/protorm/plugin/generator/types"
)

// Generator implements schema.Target for PostgreSQL DDL output.
type Generator struct{}

// Name returns the target identifier used in buf.gen.yaml opt: [target=sql].
func (g *Generator) Name() string { return "sql" }

// Generate writes one .postgres.sql file per schema into the plugin response.
func (g *Generator) Generate(p *protogen.Plugin, dbs []*schema.Database) error {
	for _, db := range dbs {
		if types.Provider(db.Provider) == types.MongoDB {
			return fmt.Errorf("sql: database %q uses provider mongodb — the sql target only supports postgres", db.Name)
		}
		for _, s := range db.Schemas {
			path := fmt.Sprintf("%s/%s.postgres.sql", db.Name, s.Name)
			f := p.NewGeneratedFile(path, "")
			if err := templates.Render(f, "schema.sql.tpl", schemaView(db, s)); err != nil {
				return fmt.Errorf("sql: %s: %w", path, err)
			}
		}
	}
	return nil
}
