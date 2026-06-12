package sql

// view.go prepares the schema.sql template view: column definitions, enum
// CREATE TYPE data, FK constraints, and index statements.

import (
	"fmt"
	"strings"

	"github.com/oh-tarnished/protorm/plugin/generator/schema"
)

type itemView struct {
	Comment, Def string
	Last         bool
}

type enumDDLView struct {
	Comment, SQLName, ValueList string
}

type tableView struct {
	Comment, Name string
	Items         []itemView
	Indexes       []string
}

// schemaView assembles the template data for one schema's DDL file.
func schemaView(db *schema.Database, s *schema.Schema) map[string]any {
	var enums []enumDDLView
	for _, e := range s.Enums {
		vals := make([]string, len(e.Values))
		for i, v := range e.Values {
			vals[i] = "'" + v.MapName + "'"
		}
		comment := e.Comment
		if comment == "" {
			comment = e.Name + " enum values."
		}
		enums = append(enums, enumDDLView{
			Comment:   comment,
			SQLName:   e.SQLName,
			ValueList: strings.Join(vals, ", "),
		})
	}

	var tables []tableView
	for _, t := range s.Tables {
		tables = append(tables, tableViewOf(s, t))
	}

	return map[string]any{
		"Database": db.Name,
		"Schema":   s.Name,
		"URL":      db.URL,
		"Enums":    enums,
		"Tables":   tables,
	}
}

// tableViewOf renders one table's column defs, FK constraints, and indexes.
func tableViewOf(s *schema.Schema, t *schema.Table) tableView {
	tv := tableView{Comment: t.Comment, Name: t.Name}

	for _, col := range t.Columns {
		tv.Items = append(tv.Items, itemView{Comment: col.Comment, Def: colDef(s, col)})
	}
	for _, fk := range t.ForeignKeys {
		tv.Items = append(tv.Items, itemView{Def: fkDef(t.Name, fk)})
	}
	if n := len(tv.Items); n > 0 {
		tv.Items[n-1].Last = true
	}

	for _, idx := range t.Indexes {
		name := idx.Name
		if name == "" {
			name = "idx_" + t.Name + "_" + strings.Join(idx.Columns, "_")
		}
		unique := ""
		if idx.Unique {
			unique = "UNIQUE "
		}
		tv.Indexes = append(tv.Indexes, fmt.Sprintf(
			"CREATE %sINDEX %s ON %s.%s (%s);",
			unique, name, s.Name, t.Name, strings.Join(idx.Columns, ", "),
		))
	}
	return tv
}

// colDef renders a single column definition fragment.
// Enum columns use the schema-qualified type created by CREATE TYPE.
func colDef(s *schema.Schema, col *schema.Column) string {
	sqlType := col.SQLType
	if col.Enum != nil {
		sqlType = s.Name + "." + col.Enum.SQLName
	}
	def := col.Name + "  " + sqlType
	if col.NotNull {
		def += "  NOT NULL"
	}
	if col.PrimaryKey {
		def += "  PRIMARY KEY"
	}
	if col.Unique && !col.PrimaryKey {
		def += "  UNIQUE"
	}
	switch {
	case col.Generated == "uuid":
		def += "  DEFAULT gen_random_uuid()"
	case col.Generated == "ulid":
		// PostgreSQL has no native ulid(); the value is generated client-side
		// (Prisma @default(ulid()) or application code).
	case col.Default != "":
		def += "  DEFAULT " + col.Default
	}
	return def
}

// fkDef renders a FOREIGN KEY constraint using the schema-qualified referenced
// table and the actual PK column resolved by the IR build pass.
func fkDef(tableName string, fk *schema.ForeignKey) string {
	refTable := fk.ReferencedTable
	if fk.ReferencedSchema != "" {
		refTable = fk.ReferencedSchema + "." + fk.ReferencedTable
	}
	def := fmt.Sprintf(
		"CONSTRAINT fk_%s_%s FOREIGN KEY (%s) REFERENCES %s(%s)",
		tableName, fk.Column, fk.Column, refTable, fk.ReferencedColumn,
	)
	if fk.OnDelete != "" {
		def += " ON DELETE " + fk.OnDelete
	}
	if fk.OnUpdate != "" {
		def += " ON UPDATE " + fk.OnUpdate
	}
	return def
}
