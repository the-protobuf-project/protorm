// Package templates embeds the text/template files that render every protorm
// output format. Backends prepare plain view-model structs; templates contain
// presentation only — no naming or type logic.
package templates

import (
	"embed"
	"io"
	"text/template"
)

//go:embed prisma/*.tpl gorm/*.tpl sql/*.tpl
var files embed.FS

// set parses all embedded templates once at init. Template names are the
// unique file base names: "schema.prisma.tpl", "fragment.prisma.tpl",
// "models.go.tpl", "schema.sql.tpl".
var set = template.Must(template.ParseFS(files,
	"prisma/*.tpl", "gorm/*.tpl", "sql/*.tpl",
))

// Render executes the named template with data into w.
func Render(w io.Writer, name string, data any) error {
	return set.ExecuteTemplate(w, name, data)
}
