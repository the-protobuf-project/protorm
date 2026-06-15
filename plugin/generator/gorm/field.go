package gorm

// field.go holds the column-level rendering helpers used by packageView in
// view.go: Go type selection, the gorm/json/validate struct tag, the FK
// constraint fragment, and name deduplication for association fields.

import (
	"strconv"
	"strings"

	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/plugin/generator/types"
)

// goType returns the Go type for a column: enums use their generated Go type,
// nullable scalars become pointers. Slice types ([]byte, json.RawMessage,
// arrays) are not re-wrapped — their nil zero value already encodes SQL NULL.
func goType(col *schema.Column) string {
	var base string
	if col.Enum != nil {
		base = col.Enum.LocalName // package-namespaced: bare enum type name
	} else {
		base = types.GoType(col.SQLType)
	}
	if col.Optional && !strings.HasPrefix(base, "[]") && base != "json.RawMessage" {
		return "*" + base
	}
	return base
}

// structTag builds the combined gorm + json + validate struct tag for a column.
func structTag(col *schema.Column) string {
	gormParts := []string{"column:" + col.Name}
	if col.PrimaryKey {
		gormParts = append(gormParts, "primaryKey")
	}
	if col.NotNull {
		gormParts = append(gormParts, "not null")
	}
	if col.Unique {
		gormParts = append(gormParts, "uniqueIndex")
	}
	switch {
	case col.AutoCreate:
		gormParts = append(gormParts, "autoCreateTime")
	case col.AutoUpdate:
		gormParts = append(gormParts, "autoUpdateTime")
	case col.Generated == "uuid":
		gormParts = append(gormParts, "default:gen_random_uuid()")
	case col.Enum != nil && col.Default != "":
		gormParts = append(gormParts, "default:'"+col.Default+"'") // enum label literal
	case col.Default != "":
		gormParts = append(gormParts, "default:"+col.Default)
	}
	if col.Index {
		gormParts = append(gormParts, "index")
	}
	tag := `gorm:"` + strings.Join(gormParts, ";") + `"`

	if col.Optional {
		tag += ` json:"` + col.Name + `,omitempty"`
	} else {
		tag += ` json:"` + col.Name + `"`
	}

	// Required validation for non-PK NOT NULL fields the application supplies —
	// DB-managed columns (generated ids, timestamps) are excluded.
	if col.NotNull && !col.PrimaryKey && !col.AutoCreate && !col.AutoUpdate && col.Generated == "" {
		tag += ` validate:"required"`
	}
	return tag
}

// constraintTag renders the GORM constraint fragment for the FK on column
// colName, e.g. ";constraint:OnDelete:CASCADE,OnUpdate:SET NULL". Empty when
// the FK declares no referential actions.
func constraintTag(t *schema.Table, colName string) string {
	for _, fk := range t.ForeignKeys {
		if fk.Column != colName {
			continue
		}
		var parts []string
		if fk.OnDelete != "" {
			parts = append(parts, "OnDelete:"+fk.OnDelete)
		}
		if fk.OnUpdate != "" {
			parts = append(parts, "OnUpdate:"+fk.OnUpdate)
		}
		if len(parts) > 0 {
			return ";constraint:" + strings.Join(parts, ",")
		}
	}
	return ""
}

// uniqueGoName returns base, or base with the smallest numeric suffix free in
// used, recording the result — keeps association fields from colliding with
// struct columns or one another.
func uniqueGoName(base string, used map[string]bool) string {
	name := base
	for i := 2; used[name]; i++ {
		name = base + strconv.Itoa(i)
	}
	used[name] = true
	return name
}

// commentOr returns comment when non-empty, otherwise the fallback.
func commentOr(comment, fallback string) string {
	if comment != "" {
		return comment
	}
	return fallback
}
