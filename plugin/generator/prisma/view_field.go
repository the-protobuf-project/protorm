package prisma

// view_field.go holds the column-level rendering helpers used by modelViewOf in
// view.go: one field declaration, referential-action formatting, doc fallbacks,
// and the name-deduplication used to keep relation fields from colliding.

import (
	"strconv"
	"strings"

	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/plugin/generator/types"
)

// uniqueName returns base, or base with the smallest numeric suffix that is not
// already in used, and records the result. Keeps generated relation field names
// from colliding with scalar columns or one another within a model.
func uniqueName(base string, used map[string]bool) string {
	name := base
	for i := 2; used[name]; i++ {
		name = base + strconv.Itoa(i)
	}
	used[name] = true
	return name
}

// fieldDecl renders one column declaration: name, type, and attributes.
func fieldDecl(col *schema.Column, provider types.Provider) string {
	var b strings.Builder
	b.WriteString(naming.Camel(col.Name))
	b.WriteByte(' ')
	var typeName string
	if col.Enum != nil {
		typeName = col.Enum.Name
	} else {
		typeName = types.PrismaTypeFor(provider, col.SQLType)
	}
	b.WriteString(typeName)
	// A Prisma list is implicitly empty-not-null: an optional list (`Type[]?`)
	// is a schema error, so only scalar columns take the optional marker.
	if col.Optional && !strings.HasSuffix(typeName, "[]") {
		b.WriteByte('?')
	}
	if col.PrimaryKey {
		b.WriteString(" @id")
	}
	if col.Unique {
		b.WriteString(" @unique")
	}
	switch {
	case col.Generated != "":
		b.WriteString(" @default(")
		b.WriteString(col.Generated)
		b.WriteString("())") // ulid() / uuid()
	case col.AutoUpdate:
		b.WriteString(" @updatedAt") // Prisma maintains the value; no @default
	case col.Default != "":
		b.WriteString(" @default(")
		b.WriteString(col.Default)
		b.WriteString(")")
	}
	mapName := col.Name
	if provider == types.MongoDB && col.PrimaryKey {
		mapName = "_id" // Mongo documents key on _id; Prisma requires the mapping.
	}
	b.WriteString(` @map("`)
	b.WriteString(mapName)
	b.WriteString(`")`)
	return b.String()
}

// prismaAction converts a SQL referential action to Prisma's identifier form:
// "SET NULL" → "SetNull", "CASCADE" → "Cascade". Empty stays empty.
func prismaAction(sqlAction string) string {
	if sqlAction == "" {
		return ""
	}
	var b strings.Builder
	for _, word := range strings.Fields(sqlAction) {
		b.WriteString(strings.ToUpper(word[:1]))
		b.WriteString(strings.ToLower(word[1:]))
	}
	return b.String()
}

// fieldDoc returns the /// documentation for a column: the proto comment when
// present, otherwise a generated description.
func fieldDoc(col *schema.Column) string {
	if col.Comment != "" {
		return col.Comment
	}
	switch {
	case col.PrimaryKey:
		return `Unique identifier for the record. Primary key mapped to "` + col.Name + `".`
	case col.Optional:
		return `Optional column mapped to "` + col.Name + `".`
	default:
		return `Required column mapped to "` + col.Name + `".`
	}
}

// withFallbackComments fills empty enum/value comments so every line still
// carries /// documentation, matching the hand-written schema convention.
func withFallbackComments(e *schema.Enum) *schema.Enum {
	if e.Comment == "" {
		e.Comment = "Enum representing " + e.Name + " values."
	}
	for _, v := range e.Values {
		if v.Comment == "" {
			v.Comment = "Represents the " + v.MapName + " value."
		}
	}
	return e
}

// commentOr returns comment when non-empty, otherwise the fallback.
func commentOr(comment, fallback string) string {
	if comment != "" {
		return comment
	}
	return fallback
}
