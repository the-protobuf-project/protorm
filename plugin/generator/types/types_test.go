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
