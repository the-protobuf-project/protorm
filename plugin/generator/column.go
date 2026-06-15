package generator

// column.go maps a single proto field to a *schema.Column and builds the IR
// enum a field references. Type inference proper lives in the types package;
// here we apply protorm.v1.col overrides, field_behavior, and the AIP enum
// defaulting rules.

import (
	"fmt"
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
	"github.com/the-protobuf-project/protorm/plugin/generator/types"
)

// buildColumn maps one proto field to a *schema.Column.
// Returns nil when the field carries protorm.v1.col.skip = true.
func buildColumn(s *schema.Schema, f *protogen.Field) *schema.Column {
	cOpts := colOpts(f)
	if cOpts.GetSkip() {
		return nil
	}
	col := &schema.Column{
		Name:    colName(f, cOpts),
		Comment: cleanComment(f.Comments.Leading),
		Default: cOpts.GetDefaultValue(),
		Unique:  cOpts.GetUnique(),
		Index:   cOpts.GetIndex(),
	}
	switch {
	case cOpts.GetType() != "":
		col.SQLType, col.TypeOverridden = cOpts.GetType(), true // beats all inference
	case cOpts.GetMaxLength() > 0:
		col.SQLType = fmt.Sprintf("VARCHAR(%d)", cOpts.GetMaxLength())
		col.TypeOverridden = true
	case cOpts.GetPrecision() > 0:
		col.SQLType = fmt.Sprintf("NUMERIC(%d,%d)", cOpts.GetPrecision(), cOpts.GetScale())
		col.TypeOverridden = true
	case f.Enum != nil && !f.Desc.IsList():
		col.Enum = enumByName(s, f.Enum)
	default:
		col.SQLType = types.PostgresType(f)
	}
	for _, b := range fieldBehaviors(f) {
		switch b {
		case annotations.FieldBehavior_REQUIRED:
			col.NotNull = true
		case annotations.FieldBehavior_IDENTIFIER:
			col.PrimaryKey, col.NotNull = true, true
		}
	}
	col.Optional = !col.NotNull
	// With the UNSPECIFIED sentinel dropped, a required enum column has no implicit
	// "unset" value, so default it to the first declared variant — mirroring the
	// hand-written `status … @default(ACTIVE)` pattern. An explicit
	// protorm.v1.col.default_value always wins.
	if col.Enum != nil && col.NotNull && col.Default == "" && len(col.Enum.Values) > 0 {
		col.Default = col.Enum.Values[0].Name
	}
	return col
}

// enumByName returns the IR enum for e within schema s, building it on first use.
func enumByName(s *schema.Schema, e *protogen.Enum) *schema.Enum {
	name := string(e.Desc.Name())
	for _, ex := range s.Enums {
		if ex.Name == name {
			return ex
		}
	}
	en := &schema.Enum{
		Name:         name,
		SQLName:      naming.SnakeCase(name),
		LocalName:    name,
		LocalSQLName: naming.SnakeCase(name),
		ProtoName:    string(e.Desc.FullName()),
		PgSchema:     s.Name,
		Comment:      cleanComment(e.Comments.Leading),
		SourceFile:   sourceFileBase(e.Desc.ParentFile().Path()),
		SourceProto:  e.Desc.ParentFile().Path(),
		SourceDir:    protoDirNoVersion(e.Desc.ParentFile().Path()),
	}
	for _, v := range e.Values {
		full := string(v.Desc.Name())
		// AIP-126: the zero value is the *_UNSPECIFIED "not set" sentinel, never a
		// real state. Drop it so the generated enum mirrors a hand-written one — no
		// storable UNSPECIFIED row — and so a required column can default to the
		// first genuine value (see buildColumn).
		if v.Desc.Number() == 0 && strings.HasSuffix(strings.ToUpper(full), "UNSPECIFIED") {
			continue
		}
		// MapName is the SCREAMING_SNAKE stored value (read by every target). Name
		// is the rendered identifier and must start with a letter (a Prisma enum
		// value rule): when stripping the enum prefix leaves a digit-leading value
		// (ASPECT_RATIO_3_4 → "3_4"), fall back to the full proto value name, which
		// is letter-leading. MapName keeps the short form for the DB ("3_4").
		mapName := naming.ScreamingSnake(naming.EnumValueName(name, full))
		valName := mapName
		if !startsWithLetter(valName) {
			valName = naming.ScreamingSnake(full)
		}
		if !startsWithLetter(valName) {
			valName = "V" + valName // rare: a proto value that itself isn't letter-leading
		}
		en.Values = append(en.Values, &schema.EnumValue{
			Name:    valName,
			MapName: mapName,
			Comment: cleanComment(v.Comments.Leading),
		})
	}
	s.Enums = append(s.Enums, en)
	return en
}

// startsWithLetter reports whether s begins with an ASCII letter — the leading
// character Prisma requires for an enum value identifier.
func startsWithLetter(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
