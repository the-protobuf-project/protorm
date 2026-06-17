package generator

// build.go traverses the proto descriptor set and assembles the schema IR.
// Files declaring the same datasource name merge into one schema.Database.
// Table assembly lives in table.go/column.go; the post-merge global-namespace
// passes in qualify.go; FK and HasMany wiring completes in resolve.go.

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/plugin/generator/types"
)

// buildDatabases converts every generate-flagged file in the plugin request
// into the database IR, merging files that share a datasource name. Recoverable
// schema problems (unresolved FKs, unknown index columns) are recorded on diags
// rather than failing here; the caller decides their severity via --strict.
//
// Several production-fidelity defaults are applied automatically (no proto
// annotation needed), each mirroring the hand-written schema: the AIP
// *_UNSPECIFIED enum sentinel is dropped and required enum columns default to
// their first value (column.go), every foreign-key column is indexed
// (indexForeignKeys), and embedded child relations cascade/null on delete
// (embed_fk.go). All remain overridable via protorm.v1.* options.
func buildDatabases(p *protogen.Plugin, diags *diagnostics, layout *layoutConfig) ([]*schema.Database, error) {
	byName := map[string]*schema.Database{}
	var order []*schema.Database
	ctx := newBuildCtx(p, layout)

	for _, f := range p.Files {
		if !f.Generate {
			continue
		}
		db, err := ctx.mergeFile(byName, f)
		if err != nil {
			return nil, err
		}
		if db != nil && byName[db.Name] == db && !contains(order, db) {
			order = append(order, db)
		}
	}

	// Materialize embedded child tables and their FK columns before relations
	// are resolved, so resolveRelations sees the full table set.
	ctx.normalizeEmbeds(diags)
	// Synthesize join tables for repeated resource_reference (many-to-many),
	// after embeds so every potential target table already exists.
	ctx.normalizeM2M(diags)

	dedupeSchemaTable := layout != nil && layout.DedupeSchemaTable
	for _, db := range order {
		// Rename schema-stuttering table names before relations resolve, so FK
		// references pick up the final names (opt-in via protorm.yaml).
		if dedupeSchemaTable {
			deStutterTables(db)
		}
		qualifyModels(db, diags)
		resolveRelations(db, diags)
		dedupeEnums(db, diags)
		indexForeignKeys(db)
		nameIndexes(db)
		validateIndexes(db, diags)
	}
	return order, nil
}

func contains(dbs []*schema.Database, db *schema.Database) bool {
	for _, d := range dbs {
		if d == db {
			return true
		}
	}
	return false
}

// mergeFile folds one proto file into the database keyed by its datasource
// name, creating the database on first sight. Returns nil when the file has
// no resource-annotated messages.
func (ctx *buildCtx) mergeFile(byName map[string]*schema.Database, f *protogen.File) (*schema.Database, error) {
	ds := datasourceOpts(f)
	// Precedence for database/schema: annotation > protorm.yaml config > default.
	cfgDB, cfgSchema, stripVer := ctx.layout.resolve(string(f.Desc.Package()))
	name := ds.GetDatabase()
	if name == "" {
		name = cfgDB
	}
	if name == "" {
		parts := strings.Split(string(f.Desc.Package()), ".")
		name = parts[len(parts)-1]
	}
	provider, err := types.ParseProvider(ds.GetProvider())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Desc.Path(), err)
	}

	db, ok := byName[name]
	if !ok {
		db = &schema.Database{Name: name, URL: ds.GetUrl(), Provider: string(provider)}
		byName[name] = db
	} else {
		if db.URL == "" {
			db.URL = ds.GetUrl()
		}
		if db.Provider != string(provider) && ds.GetProvider() != "" {
			return nil, fmt.Errorf("%s: datasource %q provider %q conflicts with %q",
				f.Desc.Path(), name, provider, db.Provider)
		}
	}

	// An explicit datasource.schema annotation is authoritative and never
	// version-stripped; a config-derived or resource-type-derived schema obeys
	// strip_version.
	schemaOverride := ds.GetSchema()
	stripOK := false
	if schemaOverride == "" {
		schemaOverride = cfgSchema
		stripOK = stripVer
	}
	if added := ctx.addFileTables(db, f, schemaOverride, stripOK); !added && !ok {
		delete(byName, name)
		return nil, nil
	}
	return db, nil
}

// addFileTables appends every resource-annotated message in f to db.
// schemaOverride, when non-empty, replaces the resource-type-derived schema for
// all tables in this file; stripVer additionally flattens a trailing API version
// out of the chosen schema name. Reports whether anything was added.
func (ctx *buildCtx) addFileTables(db *schema.Database, f *protogen.File, schemaOverride string, stripVer bool) bool {
	srcPath := f.Desc.Path()
	src := sourceFileBase(srcPath)
	added := false

	for _, msg := range f.Messages {
		if msg.Desc.IsMapEntry() {
			continue
		}
		tOpts := tableOpts(msg)
		if tOpts.GetSkip() {
			continue
		}
		res := resourceOf(msg)
		if res == nil {
			continue
		}
		sName, tName := schemaTable(res.GetType(), tOpts.GetTable())
		// google.api.resource.plural is the authoritative plural ("shelves"),
		// beating the naive +s inference. protorm.v1.table.table still wins.
		if tOpts.GetTable() == "" && res.GetPlural() != "" {
			tName = naming.SnakeCase(res.GetPlural())
		}
		if schemaOverride != "" {
			sName = schemaOverride
		}
		if stripVer {
			sName = naming.StripPackageVersion(sName)
		}
		s := schemaByName(db, sName)
		t := ctx.buildTable(db, s, msg, tName, src, srcPath)
		t.PgSchema = sName
		t.SourceDir = protoDirNoVersion(srcPath)
		s.Tables = append(s.Tables, t)
		added = true
	}
	return added
}

// schemaByName returns the named schema in db, creating it on first use.
func schemaByName(db *schema.Database, name string) *schema.Schema {
	for _, s := range db.Schemas {
		if s.Name == name {
			return s
		}
	}
	s := &schema.Schema{Name: name}
	db.Schemas = append(db.Schemas, s)
	return s
}

// schemaTable parses a google.api.resource.type string (e.g. "bookstore.v1/Book")
// into a schema name ("bookstore_v1") and snake_plural table name ("books").
// nameOverride replaces the inferred table name when non-empty.
func schemaTable(resourceType, nameOverride string) (sName, tName string) {
	parts := strings.SplitN(resourceType, "/", 2)
	if len(parts) != 2 {
		return "public", naming.SnakePlural(resourceType)
	}
	sName = strings.ReplaceAll(strings.ToLower(parts[0]), ".", "_")
	tName = naming.SnakePlural(parts[1])
	if nameOverride != "" {
		tName = nameOverride
	}
	return
}
