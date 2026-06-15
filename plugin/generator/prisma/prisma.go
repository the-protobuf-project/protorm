// Package prisma generates a multi-file Prisma schema tree from the protorm IR,
// replicating the hand-written layout this repository uses:
//
//	<db>/schema.prisma                       — datasource + generator blocks
//	<db>/<schema>/<domain>.<provider>.prisma — models + enums per source proto file
//
// "domain" is the proto file base name, so bookstore/v1/bookstore.proto with
// schema bookstore_v1 renders bookstore_db/bookstore_v1/bookstore.postgres.prisma.
package prisma

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/plugin/generator/templates"
	"github.com/the-protobuf-project/protorm/plugin/generator/types"
)

// Generator implements schema.Target for Prisma schema output.
type Generator struct{}

// Name returns the target identifier used in buf.gen.yaml opt: [target=prisma].
func (g *Generator) Name() string { return "prisma" }

// Generate writes the datasource file and every per-domain fragment for each database.
func (g *Generator) Generate(p *protogen.Plugin, dbs []*schema.Database) error {
	for _, db := range dbs {
		provider := types.Provider(db.Provider)

		f := p.NewGeneratedFile(db.Name+"/schema.prisma", "")
		if err := templates.Render(f, "schema.prisma.tpl", schemaFileView(db, provider)); err != nil {
			return fmt.Errorf("prisma: %s: %w", db.Name, err)
		}

		// Prisma 7: the connection URL lives in <db>.config.ts, not the schema file.
		cf := p.NewGeneratedFile(db.Name+"/"+db.Name+".config.ts", "")
		if err := templates.Render(cf, "config.ts.tpl", configView(db)); err != nil {
			return fmt.Errorf("prisma: %s config: %w", db.Name, err)
		}

		// Project scaffold so the generated folder is a runnable Prisma project.
		if err := writeScaffold(p, db, provider); err != nil {
			return err
		}

		// One fragment per source proto file, placed at a path mirroring the
		// proto directory tree so the Prisma layout matches the protobuf layout.
		// A single proto may contribute models to several @@schemas, so each
		// model/enum carries its own schema rather than the fragment as a whole.
		groups := groupByProto(db)
		for _, g := range groups {
			dir := fragmentDir(db.Name, g.sourceDir, g.fileBase)
			path := fmt.Sprintf("%s/%s.%s.prisma", dir, g.fileBase, provider.FragmentExt())
			ff := p.NewGeneratedFile(path, "")
			if err := templates.Render(ff, "fragment.prisma.tpl", fragmentView(db, g, provider)); err != nil {
				return fmt.Errorf("prisma: %s: %w", path, err)
			}
		}

		// A README.md with a Mermaid ER diagram in every folder of the tree.
		if err := writeReadmes(p, db, groups, provider); err != nil {
			return err
		}
	}
	return nil
}

// fragmentGroup is one source proto file's tables and enums, gathered across
// every schema they were sorted into.
type fragmentGroup struct {
	sourceProto string
	sourceDir   string
	fileBase    string
	tables      []*schema.Table
	enums       []*schema.Enum
}

// groupByProto buckets a database's tables and enums by their source proto file,
// in deterministic order, so each proto renders to exactly one fragment file.
func groupByProto(db *schema.Database) []fragmentGroup {
	idx := map[string]*fragmentGroup{}
	var order []string
	get := func(proto, dir, base string) *fragmentGroup {
		g, ok := idx[proto]
		if !ok {
			g = &fragmentGroup{sourceProto: proto, sourceDir: dir, fileBase: base}
			idx[proto] = g
			order = append(order, proto)
		}
		return g
	}
	for _, s := range db.Schemas {
		for _, t := range s.Tables {
			g := get(t.SourceProto, t.SourceDir, t.SourceFile)
			g.tables = append(g.tables, t)
		}
		for _, e := range s.Enums {
			g := get(e.SourceProto, e.SourceDir, e.SourceFile)
			g.enums = append(g.enums, e)
		}
	}
	sort.Strings(order)
	out := make([]fragmentGroup, 0, len(order))
	for _, p := range order {
		out = append(out, *idx[p])
	}
	return out
}

// fragmentDir mirrors the proto directory under the database root. The proto
// tree's leading segment (the module/service root, e.g. "store") is dropped —
// the database name already stands in for it — and the file base name becomes a
// leaf folder so each proto gets its own directory (with room for a README):
//
//	db "users", dir "store/apps/productivity/calendar", base "event"
//	  → "users/apps/productivity/calendar/event"
func fragmentDir(dbName, sourceDir, fileBase string) string {
	rest := ""
	if i := strings.IndexByte(sourceDir, '/'); i >= 0 {
		rest = sourceDir[i+1:]
	}
	parts := []string{dbName}
	if rest != "" {
		parts = append(parts, rest)
	}
	parts = append(parts, fileBase)
	return strings.Join(parts, "/")
}
