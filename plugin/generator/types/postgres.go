package types

import (
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// PostgresType infers the canonical PostgreSQL type for a proto field.
// Repeated scalars become native SQL arrays; maps and repeated messages stay
// JSONB (one JSON document already represents the whole collection).
// Enum fields are handled by the IR builder (schema.Column.Enum), not here.
func PostgresType(f *protogen.Field) string {
	base := postgresBase(f)
	if f.Desc.IsList() && base != "JSONB" {
		return base + "[]"
	}
	return base
}

// postgresBase returns the element PostgreSQL type, ignoring field cardinality.
func postgresBase(f *protogen.Field) string {
	if msg := f.Desc.Message(); msg != nil {
		return messagePostgres(string(msg.FullName()))
	}
	return scalarPostgres[f.Desc.Kind()]
}

// scalarPostgres maps proto scalar kinds to PostgreSQL types. Unsigned 32- and
// 64-bit kinds widen one step (uint32→BIGINT, uint64→NUMERIC) so their full
// value range fits — an unsigned max exceeds the signed type of the same width.
var scalarPostgres = map[protoreflect.Kind]string{
	protoreflect.BoolKind:     "BOOLEAN",
	protoreflect.Int32Kind:    "INTEGER",
	protoreflect.Sint32Kind:   "INTEGER",
	protoreflect.Sfixed32Kind: "INTEGER",
	protoreflect.Uint32Kind:   "BIGINT",
	protoreflect.Fixed32Kind:  "BIGINT",
	protoreflect.Int64Kind:    "BIGINT",
	protoreflect.Sint64Kind:   "BIGINT",
	protoreflect.Sfixed64Kind: "BIGINT",
	protoreflect.Uint64Kind:   "NUMERIC(20,0)",
	protoreflect.Fixed64Kind:  "NUMERIC(20,0)",
	protoreflect.FloatKind:    "REAL",
	protoreflect.DoubleKind:   "DOUBLE PRECISION",
	protoreflect.StringKind:   "VARCHAR(255)",
	protoreflect.BytesKind:    "BYTEA",
}

// wellKnownPostgres maps google.protobuf and google.type messages to PostgreSQL
// types. Structured aggregates (Struct, Any, Money, …) and user-defined nested
// messages fall back to JSONB, which preserves the proto sub-document.
var wellKnownPostgres = map[string]string{
	"google.protobuf.Timestamp":   "TIMESTAMPTZ",
	"google.protobuf.Duration":    "INTERVAL",
	"google.protobuf.DoubleValue": "DOUBLE PRECISION",
	"google.protobuf.FloatValue":  "REAL",
	"google.protobuf.Int32Value":  "INTEGER",
	"google.protobuf.UInt32Value": "BIGINT",
	"google.protobuf.Int64Value":  "BIGINT",
	"google.protobuf.UInt64Value": "NUMERIC(20,0)",
	"google.protobuf.BoolValue":   "BOOLEAN",
	"google.protobuf.StringValue": "VARCHAR(255)",
	"google.protobuf.BytesValue":  "BYTEA",
	"google.protobuf.FieldMask":   "TEXT",
	"google.type.Date":            "DATE",
	"google.type.TimeOfDay":       "TIME",
	"google.type.DateTime":        "TIMESTAMPTZ",
	"google.type.Decimal":         "NUMERIC",
	"google.type.LatLng":          "POINT",
	"google.type.Interval":        "TSTZRANGE",
}

func messagePostgres(fullName string) string {
	if t, ok := wellKnownPostgres[fullName]; ok {
		return t
	}
	return "JSONB"
}

// freeformMessage is the set of google.protobuf messages whose shape is dynamic
// or type-erased: arbitrary JSON (Struct, Value, ListValue), a boxed message of
// any type (Any), or an empty marker (Empty). They have no stable column layout,
// so they stay JSONB rather than being relationalized into a table.
var freeformMessage = map[string]bool{
	"google.protobuf.Struct":    true,
	"google.protobuf.Value":     true,
	"google.protobuf.ListValue": true,
	"google.protobuf.Any":       true,
	"google.protobuf.Empty":     true,
}

// Relationalizable reports whether a message-typed field should be normalized
// into its own table (a primary key plus the foreign key linking it to its
// owner) rather than mapped to a single column. It is false for well-known types
// with a native single-column SQL mapping (Timestamp, Duration, the wrappers,
// Date, LatLng, …) and for the freeform google.protobuf wrappers (Struct, Any,
// …), both of which keep their scalar / JSONB mapping. Every other message —
// a user-defined nested message and an imported value type alike
// (google.type.Money, google.type.PostalAddress, a third-party proto) — is
// relationalized, so its structure stays queryable instead of collapsing into
// an opaque JSONB blob.
func Relationalizable(fullName string) bool {
	if _, ok := wellKnownPostgres[fullName]; ok {
		return false
	}
	return !freeformMessage[fullName]
}

// goScalar maps a bare PostgreSQL keyword (no modifiers, no array suffix) to a
// Go type. Types without a lossless Go primitive (NUMERIC, MONEY, UUID, INET,
// POINT, INTERVAL, ranges, …) map to string to stay driver-agnostic.
var goScalar = map[string]string{
	"BOOLEAN": "bool", "BOOL": "bool",
	"SMALLINT": "int32", "INT2": "int32", "INTEGER": "int32", "INT": "int32", "INT4": "int32", "SERIAL": "int32",
	"BIGINT": "int64", "INT8": "int64", "BIGSERIAL": "int64",
	"REAL": "float32", "FLOAT4": "float32",
	"DOUBLE PRECISION": "float64", "FLOAT8": "float64",
	"BYTEA": "[]byte",
	"JSON":  "json.RawMessage", "JSONB": "json.RawMessage",
	"TIMESTAMPTZ": "time.Time", "TIMESTAMP": "time.Time",
	"DATE": "time.Time", "TIME": "time.Time", "TIMETZ": "time.Time",
}

// GoType projects a canonical PostgreSQL type onto Go.
// Arrays become slices; unknown keywords default to string.
func GoType(pgType string) string {
	base, isArray := BaseType(pgType)
	t, ok := goScalar[base]
	if !ok {
		t = "string"
	}
	if isArray {
		return "[]" + t
	}
	return t
}

// pqArrayType maps an array's element keyword to the github.com/lib/pq array
// type GORM needs: a bare Go slice (`[]string`) fails AutoMigrate with
// "unsupported data type", so a repeated scalar is rendered as the pq.*Array
// type that implements sql.Scanner/driver.Valuer. Element kinds without a
// dedicated pq type fall back to pq.StringArray (see GormGoType).
var pqArrayType = map[string]string{
	"BOOLEAN": "pq.BoolArray", "BOOL": "pq.BoolArray",
	"SMALLINT": "pq.Int32Array", "INT2": "pq.Int32Array", "INTEGER": "pq.Int32Array", "INT": "pq.Int32Array", "INT4": "pq.Int32Array", "SERIAL": "pq.Int32Array",
	"BIGINT": "pq.Int64Array", "INT8": "pq.Int64Array", "BIGSERIAL": "pq.Int64Array",
	"REAL": "pq.Float32Array", "FLOAT4": "pq.Float32Array",
	"DOUBLE PRECISION": "pq.Float64Array", "FLOAT8": "pq.Float64Array",
	"BYTEA": "pq.ByteaArray",
}

// GormGoType is GoType specialized for the GORM target: a repeated scalar maps
// to its github.com/lib/pq array type instead of a bare Go slice, because GORM's
// AutoMigrate cannot map a bare `[]string`/`[]int32`. Non-array types are
// identical to GoType. Callers detect the resulting "pq." prefix to add the
// lib/pq import.
func GormGoType(pgType string) string {
	if base, isArray := BaseType(pgType); isArray {
		if t, ok := pqArrayType[base]; ok {
			return t
		}
		return "pq.StringArray" // text[] and any unmapped element kind
	}
	return GoType(pgType)
}

// gormColumnType maps a scalar PostgreSQL keyword to the explicit GORM `type:`
// tag value needed when GORM's Go-type default disagrees with the canonical
// column type: time.Time defaults to timestamptz (so a TIMESTAMP/DATE/TIME loses
// its kind), and json.RawMessage ([]byte) defaults to bytea (clashing with the
// Prisma/SQL jsonb). Keywords absent here need no override.
var gormColumnType = map[string]string{
	"TIMESTAMPTZ": "timestamptz", "TIMESTAMP": "timestamp",
	"DATE": "date", "TIME": "time", "TIMETZ": "timetz",
	"JSONB": "jsonb", "JSON": "json",
}

// gormArrayColumnType maps an array element keyword to the GORM `type:` array
// value. String-ish elements default to text[] so GORM matches Prisma's
// String[] → text[] mapping.
var gormArrayColumnType = map[string]string{
	"BOOLEAN": "boolean[]", "BOOL": "boolean[]",
	"SMALLINT": "smallint[]", "INT2": "smallint[]", "INTEGER": "integer[]", "INT": "integer[]", "INT4": "integer[]", "SERIAL": "integer[]",
	"BIGINT": "bigint[]", "INT8": "bigint[]", "BIGSERIAL": "bigint[]",
	"REAL": "real[]", "FLOAT4": "real[]",
	"DOUBLE PRECISION": "double precision[]", "FLOAT8": "double precision[]",
	"BYTEA": "bytea[]",
}

// GormColumnType returns the explicit value for a GORM `type:` struct-tag
// fragment, or "" when GORM's Go-type default already matches the canonical
// column type. It keeps the three backends agreeing on timestamptz, jsonb, and
// native arrays so AutoMigrate doesn't fight a Prisma-created column.
func GormColumnType(pgType string) string {
	base, isArray := BaseType(pgType)
	if isArray {
		if t, ok := gormArrayColumnType[base]; ok {
			return t
		}
		return "text[]"
	}
	return gormColumnType[base]
}

// prismaScalar maps a bare PostgreSQL keyword to a Prisma scalar type.
// Types Prisma cannot model natively map to String.
var prismaScalar = map[string]string{
	"BOOLEAN": "Boolean", "BOOL": "Boolean",
	"SMALLINT": "Int", "INT2": "Int", "INTEGER": "Int", "INT": "Int", "INT4": "Int", "SERIAL": "Int",
	"BIGINT": "BigInt", "INT8": "BigInt", "BIGSERIAL": "BigInt",
	"REAL": "Float", "FLOAT4": "Float", "DOUBLE PRECISION": "Float", "FLOAT8": "Float",
	"NUMERIC": "Decimal", "DECIMAL": "Decimal", "MONEY": "Decimal",
	"BYTEA": "Bytes",
	"JSON":  "Json", "JSONB": "Json",
	"TIMESTAMPTZ": "DateTime", "TIMESTAMP": "DateTime",
	"DATE": "DateTime", "TIME": "DateTime", "TIMETZ": "DateTime",
}

// PrismaType projects a canonical PostgreSQL type onto a Prisma scalar.
// Arrays become Prisma lists; unknown keywords default to String.
func PrismaType(pgType string) string {
	base, isArray := BaseType(pgType)
	t, ok := prismaScalar[base]
	if !ok {
		t = "String"
	}
	if isArray {
		return t + "[]"
	}
	return t
}

// prismaNativeType maps a date/time keyword to the bare Prisma native-type
// (without the datasource prefix). Prisma's DateTime defaults to `timestamp(3)`
// (no time zone), so a TIMESTAMPTZ field silently loses its zone — a UTC write
// reads back as a local-wall-clock value. Pinning Timestamptz(6) keeps Prisma
// agreeing with the GORM/SQL columns. Only the date/time family needs an
// override: Json already maps to jsonb and String[] to text[] by default.
var prismaNativeType = map[string]string{
	"TIMESTAMPTZ": "Timestamptz(6)", "TIMESTAMP": "Timestamp(6)",
	"DATE": "Date", "TIME": "Time(6)", "TIMETZ": "Timetz(6)",
}

// PrismaNativeType returns the bare Prisma native-type name (e.g.
// "Timestamptz(6)") for a canonical PostgreSQL type, or "" when Prisma's default
// mapping already matches. The caller prefixes it with the datasource block name
// to form the attribute (@<datasource>.Timestamptz(6)). Postgres-only; Mongo has
// no native-type attributes.
func PrismaNativeType(pgType string) string {
	base, _ := BaseType(pgType)
	return prismaNativeType[base]
}

// BaseType splits a SQL type into its leading keyword and whether it is an
// array, discarding any "(length)"/"(precision,scale)" modifier.
// "VARCHAR(255)[]" → ("VARCHAR", true); "DOUBLE PRECISION" → (same, false).
func BaseType(sqlType string) (base string, isArray bool) {
	base = strings.ToUpper(strings.TrimSpace(sqlType))
	if strings.HasSuffix(base, "[]") {
		base, isArray = strings.TrimSpace(strings.TrimSuffix(base, "[]")), true
	}
	if i := strings.IndexByte(base, '('); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	return base, isArray
}
