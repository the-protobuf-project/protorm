// Package csv generates a flat CSV schema manifest from the protorm IR.
//
// One _schema.csv file is emitted per database. The manifest is suitable as
// input to MongoDB schema-validation tooling, DynamoDB table planners, and
// any document-store setup script that prefers tabular metadata over SQL DDL.
//
// Columns (fixed header row):
//
//	database, schema, table, column, sql_type, not_null, primary_key, unique, default, description
//
// All fields are RFC 4180-compliant: values containing commas, quotes, or
// newlines are enclosed in double-quotes, with internal quotes doubled.
// The "description" column carries the proto field's leading comment verbatim.
package csv

import (
	"bytes"
	"encoding/csv"
	"strings"

	"github.com/oh-tarnished/protorm/plugin/generator/schema"
	"google.golang.org/protobuf/compiler/protogen"
)

// Generator implements schema.Target for CSV manifest output.
type Generator struct{}

// Name returns the target identifier used in buf.gen.yaml opt: [target=csv].
func (g *Generator) Name() string { return "csv" }

// csvHeader is the fixed header written at the top of every manifest.
var csvHeader = []string{
	"database", "schema", "table", "column",
	"sql_type", "not_null", "primary_key", "unique", "default", "description",
}

// Generate writes one <db>/schema.csv manifest per database into the plugin response.
func (g *Generator) Generate(p *protogen.Plugin, dbs []*schema.Database) error {
	for _, db := range dbs {
		f := p.NewGeneratedFile(db.Name+"/schema.csv", "")

		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write(csvHeader)

		for _, s := range db.Schemas {
			for _, t := range s.Tables {
				for _, col := range t.Columns {
					_ = w.Write(row(db.Name, s.Name, t.Name, col))
				}
			}
		}

		w.Flush()
		// Write each CSV line individually so protogen tracks the output correctly.
		for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			f.P(line)
		}
	}
	return nil
}

// row formats one CSV record for a column.
// Enum columns report "enum:<sql_name>" in the sql_type field.
func row(dbName, schemaName, tableName string, col *schema.Column) []string {
	sqlType := col.SQLType
	if col.Enum != nil {
		sqlType = "enum:" + col.Enum.SQLName
	}
	return []string{
		dbName,
		schemaName,
		tableName,
		col.Name,
		sqlType,
		boolStr(col.NotNull),
		boolStr(col.PrimaryKey),
		boolStr(col.Unique),
		col.Default,
		col.Comment,
	}
}

// boolStr renders a bool as "true" or "false" for the CSV output.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
