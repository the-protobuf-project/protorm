package types

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestScalarPostgres covers the proto-kind → PostgreSQL scalar mapping directly,
// including the unsigned widening (uint32→BIGINT, uint64→NUMERIC) that AIP-0141
// forbids expressing in an API proto, so it can't live in the golden fixtures.
func TestScalarPostgres(t *testing.T) {
	cases := map[protoreflect.Kind]string{
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
	for kind, want := range cases {
		if got := scalarPostgres[kind]; got != want {
			t.Errorf("scalarPostgres[%v] = %q, want %q", kind, got, want)
		}
	}
}

// TestWellKnownPostgres covers the well-known-type → PostgreSQL mapping,
// including the type-fidelity choices (P2.3): Duration → INTERVAL (queryable,
// not String) and the uint64 wrappers widening to NUMERIC(20,0).
func TestWellKnownPostgres(t *testing.T) {
	cases := map[string]string{
		"google.protobuf.Timestamp":   "TIMESTAMPTZ",
		"google.protobuf.Duration":    "INTERVAL",
		"google.protobuf.UInt64Value": "NUMERIC(20,0)",
		"google.protobuf.FieldMask":   "TEXT",
		"google.type.Date":            "DATE",
		"google.type.Interval":        "TSTZRANGE",
		"example.v1.CustomMessage":    "JSONB", // user messages fall back to JSONB
	}
	for in, want := range cases {
		if got := messagePostgres(in); got != want {
			t.Errorf("messagePostgres(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRelationalizable covers the message-classification rule that decides
// whether a message-typed field becomes a related table (PK + FK) or keeps a
// scalar / JSONB mapping. Well-known scalar types and the freeform google.protobuf
// wrappers stay; every other message — imported value types and user-defined
// nested messages alike — is relationalized.
func TestRelationalizable(t *testing.T) {
	cases := map[string]bool{
		// Native single-column mappings stay scalar (not a table).
		"google.protobuf.Timestamp":   false,
		"google.protobuf.Duration":    false,
		"google.protobuf.Int64Value":  false,
		"google.protobuf.StringValue": false,
		"google.protobuf.FieldMask":   false,
		"google.type.Date":            false,
		"google.type.LatLng":          false,
		// Freeform / type-erased wrappers stay JSONB (not a table).
		"google.protobuf.Struct":    false,
		"google.protobuf.Value":     false,
		"google.protobuf.ListValue": false,
		"google.protobuf.Any":       false,
		"google.protobuf.Empty":     false,
		// Imported value types relationalize into their own table.
		"google.type.Money":         true,
		"google.type.PostalAddress": true,
		"google.type.PhoneNumber":   true,
		// User-defined nested messages relationalize too.
		"example.v1.CustomMessage": true,
	}
	for in, want := range cases {
		if got := Relationalizable(in); got != want {
			t.Errorf("Relationalizable(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBaseType(t *testing.T) {
	cases := []struct {
		in    string
		base  string
		array bool
	}{
		{"VARCHAR(255)", "VARCHAR", false},
		{"VARCHAR(255)[]", "VARCHAR", true},
		{"DOUBLE PRECISION", "DOUBLE PRECISION", false},
		{"NUMERIC(20,0)", "NUMERIC", false},
		{"text", "TEXT", false},
		{" INTEGER[] ", "INTEGER", true},
	}
	for _, c := range cases {
		base, array := BaseType(c.in)
		if base != c.base || array != c.array {
			t.Errorf("BaseType(%q) = (%q, %v), want (%q, %v)", c.in, base, array, c.base, c.array)
		}
	}
}

func TestGoType(t *testing.T) {
	cases := map[string]string{
		"VARCHAR(255)":     "string",
		"VARCHAR(255)[]":   "[]string",
		"INTEGER":          "int32",
		"BIGINT":           "int64",
		"NUMERIC(20,0)":    "string", // precision-safe: no lossless Go primitive
		"DOUBLE PRECISION": "float64",
		"BOOLEAN":          "bool",
		"BYTEA":            "[]byte",
		"JSONB":            "json.RawMessage",
		"TIMESTAMPTZ":      "time.Time",
		"DATE":             "time.Time",
		"TSTZRANGE":        "string",
		"INTERVAL":         "string", // no lossless Go primitive; stays driver-agnostic
	}
	for in, want := range cases {
		if got := GoType(in); got != want {
			t.Errorf("GoType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrismaType(t *testing.T) {
	cases := map[string]string{
		"VARCHAR(255)":     "String",
		"TEXT[]":           "String[]",
		"INTEGER":          "Int",
		"BIGINT":           "BigInt",
		"NUMERIC(20,0)":    "Decimal",
		"DOUBLE PRECISION": "Float",
		"JSONB":            "Json",
		"TIMESTAMPTZ":      "DateTime",
		"POINT":            "String", // no native Prisma scalar
		"INTERVAL":         "String", // Prisma has no Interval scalar; DB type stays INTERVAL
	}
	for in, want := range cases {
		if got := PrismaType(in); got != want {
			t.Errorf("PrismaType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMongoPrismaType(t *testing.T) {
	cases := map[string]string{
		"VARCHAR(255)":  "String",
		"NUMERIC(20,0)": "Float", // Mongo has no Decimal: collapses to Float
		"POINT":         "Json",
		"TSTZRANGE":     "Json",
		"TIMESTAMPTZ":   "DateTime",
	}
	for in, want := range cases {
		if got := MongoPrismaType(in); got != want {
			t.Errorf("MongoPrismaType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseProvider(t *testing.T) {
	for _, s := range []string{"", "postgres", "postgresql"} {
		if p, err := ParseProvider(s); err != nil || p != Postgres {
			t.Errorf("ParseProvider(%q) = (%v, %v), want Postgres", s, p, err)
		}
	}
	for _, s := range []string{"mongodb", "mongo"} {
		if p, err := ParseProvider(s); err != nil || p != MongoDB {
			t.Errorf("ParseProvider(%q) = (%v, %v), want MongoDB", s, p, err)
		}
	}
	if _, err := ParseProvider("mysql"); err == nil {
		t.Error("ParseProvider(mysql): want error, got nil")
	}
}
