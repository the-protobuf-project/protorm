package generator

// oneof.go restores the exclusivity invariant a proto oneof carries but a
// relational table loses. The base mapping flattens a oneof's members into
// independent nullable columns, so the database can represent two members set at
// once — a state the proto forbids. For every real oneof we add a
// <oneof>_case discriminator enum column recording which member is populated
// (null = none), giving consumers a single source of truth to read and the app
// layer a column to enforce against.

import (
	"google.golang.org/protobuf/compiler/protogen"

	"github.com/the-protobuf-project/protorm/plugin/generator/naming"
	"github.com/the-protobuf-project/protorm/plugin/generator/schema"
)

// addOneofDiscriminators appends a <oneof>_case enum column for each real
// (non-synthetic) oneof in msg. proto3 `optional` fields are modeled as
// synthetic oneofs and carry no exclusivity contract, so they are skipped.
func (ctx *buildCtx) addOneofDiscriminators(s *schema.Schema, t *schema.Table, msg *protogen.Message) {
	for _, o := range msg.Oneofs {
		if o.Desc.IsSynthetic() {
			continue
		}
		name := string(o.Desc.Name())
		t.Columns = append(t.Columns, &schema.Column{
			Name:     name + "_case",
			Comment:  "Discriminator: which " + name + " oneof member is set (null = none).",
			Enum:     oneofEnum(s, msg, o),
			Optional: true,
		})
	}
}

// oneofEnum builds (once per oneof) the discriminator enum for o under schema s,
// with one value per member field. The enum is keyed in the IR by the oneof's
// full proto name so dedupeEnums never merges two distinct oneofs that happen to
// share a simple name.
func oneofEnum(s *schema.Schema, msg *protogen.Message, o *protogen.Oneof) *schema.Enum {
	protoName := string(o.Desc.FullName()) + ".case"
	for _, ex := range s.Enums {
		if ex.ProtoName == protoName {
			return ex
		}
	}
	srcPath := o.Desc.ParentFile().Path()
	caseName := string(msg.Desc.Name()) + naming.PascalGo(string(o.Desc.Name())) + "Case"
	en := &schema.Enum{
		Name:        caseName,
		LocalName:   caseName,
		ProtoName:   protoName,
		PgSchema:    s.Name,
		Comment:     "Discriminator for the " + string(o.Desc.Name()) + " oneof of " + string(msg.Desc.Name()) + ".",
		SourceFile:  sourceFileBase(srcPath),
		SourceProto: srcPath,
		SourceDir:   protoDirNoVersion(srcPath),
	}
	en.SQLName = naming.SnakeCase(en.Name)
	en.LocalSQLName = en.SQLName
	for _, f := range o.Fields {
		v := naming.ScreamingSnake(string(f.Desc.Name()))
		en.Values = append(en.Values, &schema.EnumValue{
			Name: v, MapName: v,
			Comment: "The " + string(f.Desc.Name()) + " member is set.",
		})
	}
	s.Enums = append(s.Enums, en)
	return en
}
