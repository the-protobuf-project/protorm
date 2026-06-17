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
