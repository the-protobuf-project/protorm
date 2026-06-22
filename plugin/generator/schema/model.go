package schema

// model.go holds the table-level IR: a Table (one proto resource message), its
// Columns, and the relation records (ForeignKey, HasManyRef, Index) that wire
// tables together. The database/schema/enum types live in schema.go.

// Table maps one proto message (carrying google.api.resource) to one DB table.
type Table struct {
	// Name is the snake_case plural table name; overridable via protorm.v1.table.name.
	Name string

	// Comment is the proto message's leading comment, normalized for embedding in
	// generated output. Stripped of // markers and trimmed to a single line.
	Comment string

	// ModelName is the singular Go/Prisma model name derived from the proto message
	// simple name. "bookstore.v1.Author" → "Author". Always PascalCase. Qualified
	// with a schema-domain prefix on a cross-schema name collision so it stays
	// globally unique (Prisma model names occupy one namespace per database).
	ModelName string

	// LocalName is the bare, schema-local model name (the proto message simple
	// name), never qualified for global uniqueness. Schema-namespaced targets
	// (GORM, SQL) render this so a name that the @@schema / Go package
	// already disambiguates isn't redundantly prefixed (a "Location" shared by two
	// schemas stays "Location", not "CalendarLocation"). Prisma uses ModelName.
	LocalName string

	// PKColumn is the name of the primary key column resolved during build.
	// Referenced by FK resolution to emit correct REFERENCES clauses.
	PKColumn string

	// ProtoMessage is the fully qualified proto message name for diagnostics.
	ProtoMessage string

	// SourceFile is the proto file base name this table came from ("bookstore"
	// for bookstore/v1/bookstore.proto). Drives fragment file splitting:
	// <db>/<schema>/<SourceFile>.<provider>.prisma.
	SourceFile string

	// SourceProto is the full proto import path ("bookstore/v1/bookstore.proto"),
	// shown on the generated-file banner's source line.
	SourceProto string

	// SourceDir is the proto file's directory with the version segment dropped
	// ("store/apps/.../calendar/v1/event.proto" → "store/apps/.../calendar"),
	// driving the mirrored output path so the Prisma tree matches the proto tree.
	SourceDir string

	// PgSchema is the postgres schema this table is placed in. Lets one proto
	// file contribute models to several schemas (per-table @@schema override).
	PgSchema string

	// ValueObject marks a table materialized from an embedded message-typed field
	// (a relationalized value object like Money or PostalAddress) rather than a
	// google.api.resource. Such tables are leaves — nothing points back through
	// them — so the GORM target can safely emit a cross-schema belongs-to
	// association to one (and import its package) without risking an import cycle.
	ValueObject bool

	// Parents lists the parent resource variable names from the AIP resource
	// pattern ("users/{user}/events/{event}" → ["user"]), set by materializeParents.
	// Non-empty marks a nested resource, so an M2M join to it can capture the full
	// hierarchical key, not just the leaf id.
	Parents []string

	Columns     []*Column
	Indexes     []*Index
	ForeignKeys []*ForeignKey
	HasMany     []*HasManyRef
}

// Column maps one proto field to one database column.
type Column struct {
	// Name is the column identifier (snake_case). Overridable via protorm.v1.column.name.
	Name string

	// Comment is the proto field's leading comment, normalized for embedding.
	Comment string

	// SQLType is the normalized PostgreSQL type (e.g. "VARCHAR(255)", "TIMESTAMPTZ").
	// Canonical across targets: each backend maps it to its own type system.
	// Overridable via protorm.v1.column.type. Empty when Enum is set.
	SQLType string

	// Enum points to the enum definition when the proto field is an enum kind.
	// Backends render their native enum reference instead of SQLType.
	Enum *Enum

	// NotNull is true when the field carries REQUIRED or IDENTIFIER field_behavior.
	NotNull bool

	// PrimaryKey is true when the field carries IDENTIFIER field_behavior.
	PrimaryKey bool

	// Optional is the inverse of NotNull. When true, Go generators emit pointer
	// types (*string, *int32, …) so the zero value is distinguishable from absence.
	Optional bool

	// Unique is true when protorm.v1.column.unique = true.
	Unique bool

	// Default is the SQL default expression from protorm.v1.column.default_value.
	Default string

	// Index is true when protorm.v1.column.index = true (single-column index).
	Index bool

	// TypeOverridden is true when the user pinned the type via col.type,
	// col.max_length, or col.precision. FK type alignment skips such columns.
	TypeOverridden bool

	// Generated names the value-generation strategy for synthesized PK columns:
	// "ulid" or "uuid". Empty for ordinary columns.
	Generated string

	// AutoCreate marks a synthesized created_at column (GORM autoCreateTime).
	AutoCreate bool

	// AutoUpdate marks a synthesized updated_at column
	// (Prisma @updatedAt, GORM autoUpdateTime).
	AutoUpdate bool

	// FKModel is the singular Go model name of the resource this column references.
	// Non-empty only when google.api.resource_reference is present on the field.
	// Example: "author_id" with ref to "bookstore.v1/Author" → FKModel = "Author".
	FKModel string
}

// HasManyRef records a back-reference from a parent table to a child table that
// holds a FK pointing back at the parent. Populated during the build second pass.
// Generators that need both sides of a relation (e.g. Prisma) consume this.
type HasManyRef struct {
	// Model is the child model name that owns the FK (e.g. "Book").
	Model string

	// Field is the snake_plural table name of the child, used to derive the
	// back-relation field name (e.g. "books" → Prisma: "books Book[]").
	Field string

	// ViaFK is the FK column name in the child table (snake_case, e.g. "author_id").
	// Each generator converts this to its own naming convention as needed.
	ViaFK string

	// RelationName matches the paired ForeignKey.RelationName, naming the relation
	// on the has-many side when a model pair has more than one relation. Empty
	// otherwise.
	RelationName string
}

// Index describes a multi-column index declared via protorm.v1.table.indexes.
type Index struct {
	Name    string   // explicit name; auto-generated when empty
	Columns []string // snake_case column names in declaration order
	Unique  bool
}

// ForeignKey describes a column-level FK constraint inferred from
// google.api.resource_reference. ReferencedColumn is resolved in a second pass
// after all tables in the database are built.
type ForeignKey struct {
	Column           string // referencing column in this table
	ReferencedSchema string // schema of the target table (e.g. "bookstore_v1")
	ReferencedTable  string // target table name (e.g. "authors")
	ReferencedModel  string // singular Go model name (e.g. "Author")
	ReferencedColumn string // actual PK column; "id" when table not found in database

	// ReferencedProto is the full proto message name of the target, set for
	// synthesized embed relations so resolution picks the exact table even when
	// several models share a simple name (e.g. a per-package "Media"). Empty for
	// resource_reference FKs, which resolve by model name.
	ReferencedProto string

	// OnDelete / OnUpdate are SQL referential actions ("CASCADE", "SET NULL", …)
	// from protorm.v1.column.on_delete / on_update. Empty means database default.
	OnDelete string
	OnUpdate string

	// RelationName disambiguates multiple relations between the same two models.
	// Prisma requires a matching @relation("name") on both the belongs-to and the
	// has-many side when more than one relation connects a model pair. Empty when
	// the pair has a single relation (no name needed). Mirrored onto the paired
	// HasManyRef.RelationName.
	RelationName string
}
