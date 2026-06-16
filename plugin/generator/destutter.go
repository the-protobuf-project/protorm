package generator

// destutter.go renames generated table names that would stutter against their
// own schema in a schema-qualified identifier. Tools that name objects
// <schema>_<table> (Hasura DDN's GraphQL root fields, some ORMs) turn a schema
// "booking" plus its table "bookings" into "bookingBookings". The proto/API and
// model names must not change, and a derivative schema name ("bookingSvc") still
// stutters, so the fix is on the generated table name:
//
//   - prefixed table  ("schedule_exceptions" in "schedule") → strip the leading
//     schema word: "exceptions"  → schedule_exceptions  reads "scheduleExceptions".
//   - eponymous table ("bookings" in "booking", nothing left after stripping)  →
//     a generic word ("resource", then "entity"/"record"/"item") → "bookingResource".
//
// Opt in with protorm.yaml `dedupe_schema_table: true`. Runs after all tables are
// built (including embeds and m2m joins) and before resolveRelations, so foreign
// keys pick up the final table names. Only schema.Table.Name changes — ModelName
// (Prisma/GORM types, Prisma Client) is left intact, mapped back via @@map /
// gorm:"column".

import (
	"strings"

	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

// genericTableWords are the fallback names for an eponymous table, in order of
// preference. The first that neither stutters with the schema nor collides with
// an existing table is used.
var genericTableWords = []string{"resource", "entity", "record", "item"}

// deStutterTables renames every stuttering table in db (see file comment). The
// Postgres public schema is skipped — Hasura and friends don't prefix it, so no
// stutter occurs.
func deStutterTables(db *schema.Database) {
	for _, s := range db.Schemas {
		if s.Name == "public" {
			continue
		}
		taken := make(map[string]bool, len(s.Tables))
		for _, t := range s.Tables {
			taken[t.Name] = true
		}
		for _, t := range s.Tables {
			nn := deStutteredName(t.Name, s.Name, taken)
			if nn != t.Name {
				delete(taken, t.Name)
				taken[nn] = true
				t.Name = nn
			}
		}
	}
}

// deStutteredName returns a non-stuttering table name for name in schemaName, or
// name unchanged when it doesn't stutter or no free alternative exists. taken is
// the set of table names already used in the schema (so a rename never collides).
func deStutteredName(name, schemaName string, taken map[string]bool) string {
	root := squashIdent(schemaName)
	if root == "" || !strings.HasPrefix(squashIdent(name), root) {
		return name // no stutter
	}

	// Prefixed table: drop the leading underscore segment(s) that spell the
	// schema, keeping the remainder ("schedule_exceptions" → "exceptions").
	segs := strings.Split(name, "_")
	for k := 1; k < len(segs); k++ {
		if squashIdent(strings.Join(segs[:k], "_")) != root {
			continue
		}
		rest := strings.Join(segs[k:], "_")
		switch {
		case rest != "" && !strings.HasPrefix(squashIdent(rest), root) && !taken[rest]:
			return rest
		case rest != "" && !taken[rest+"_link"]:
			return rest + "_link" // remainder collides (e.g. a join vs the entity table)
		}
		break
	}

	// Eponymous table (stripping leaves nothing meaningful): a generic word.
	for _, w := range genericTableWords {
		if !strings.HasPrefix(squashIdent(w), root) && !taken[w] {
			return w
		}
	}
	return name
}

// squashIdent lowercases an identifier and removes underscores so "promo_code"
// and "promocode" compare equal when testing for a shared root word.
func squashIdent(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "_", "")
}
