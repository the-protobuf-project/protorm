package types

// mongoScalar maps a bare canonical-PostgreSQL keyword to the Prisma scalar
// used under the mongodb provider. Mongo has no VARCHAR/NUMERIC distinctions:
// strings collapse to String, arbitrary precision to Float, ranges/geo to Json.
var mongoScalar = map[string]string{
	"BOOLEAN": "Boolean", "BOOL": "Boolean",
	"SMALLINT": "Int", "INT2": "Int", "INTEGER": "Int", "INT": "Int", "INT4": "Int", "SERIAL": "Int",
	"BIGINT": "BigInt", "INT8": "BigInt", "BIGSERIAL": "BigInt",
	"REAL": "Float", "FLOAT4": "Float", "DOUBLE PRECISION": "Float", "FLOAT8": "Float",
	"NUMERIC": "Float", "DECIMAL": "Float", "MONEY": "Float",
	"BYTEA": "Bytes",
	"JSON":  "Json", "JSONB": "Json",
	"TIMESTAMPTZ": "DateTime", "TIMESTAMP": "DateTime",
	"DATE": "DateTime", "TIME": "DateTime", "TIMETZ": "DateTime",
	"POINT": "Json", "TSTZRANGE": "Json", "INTERVAL": "String",
}

// MongoPrismaType projects a canonical PostgreSQL type onto a Prisma scalar
// for the mongodb provider. Arrays become Prisma lists; unknown → String.
func MongoPrismaType(pgType string) string {
	base, isArray := BaseType(pgType)
	t, ok := mongoScalar[base]
	if !ok {
		t = "String"
	}
	if isArray {
		return t + "[]"
	}
	return t
}

// PrismaTypeFor projects a canonical PostgreSQL type for the given provider.
func PrismaTypeFor(p Provider, pgType string) string {
	if p == MongoDB {
		return MongoPrismaType(pgType)
	}
	return PrismaType(pgType)
}
