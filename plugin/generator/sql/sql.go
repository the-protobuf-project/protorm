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

	"github.com/the-protobuf-project/protorm/plugin/generator/docs"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/plugin/generator/templates"
	"github.com/the-protobuf-project/protorm/plugin/generator/types"
)

// Generator implements schema.Target for PostgreSQL DDL output.
type Generator struct{}

// Name returns the target identifier used in buf.gen.yaml opt: [target=sql].
func (g *Generator) Name() string { return "sql" }

// Generate writes one .postgres.sql file per schema plus a consolidated
// migrate.sql into the plugin response.
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
		// Consolidated single-file migration: every schema, ordered so it applies
		// in one transaction (foreign keys deferred to ALTER statements).
		migratePath := db.Name + "/migrate.sql"
		mf := p.NewGeneratedFile(migratePath, "")
		if err := templates.Render(mf, "migrate.sql.tpl", migrateView(db)); err != nil {
			return fmt.Errorf("sql: %s: %w", migratePath, err)
		}
		rf := p.NewGeneratedFile(db.Name+"/README.md", "")
		md := docs.Render(db, docs.Meta{
			Title:   "PostgreSQL schema",
			Tagline: "CREATE SCHEMA / TYPE / TABLE DDL with foreign keys and indexes.",
			Outputs: []string{
				"`migrate.sql` — the whole database in one transactional file; apply with `psql -f migrate.sql`.",
				"`<schema>.postgres.sql` — one DDL file per schema (apply referenced tables before referencing ones).",
				"Auto-update triggers keep updated-at columns current; COMMENT ON persists field docs to the catalog.",
			},
			Naming: docs.Local(db),
		})
		if _, err := rf.Write([]byte(md)); err != nil {
			return fmt.Errorf("sql: %s/README.md: %w", db.Name, err)
		}
	}
	return nil
}
