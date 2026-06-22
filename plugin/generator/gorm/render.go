package gorm

// render.go runs every generated Go file through gofmt before it is written.
// protogen reformats .go output with go/printer (which aligns fields) but does
// not sort imports, so without this pass the import order would depend on template
// emission order. Running go/format here makes the output canonically formatted —
// sorted imports within each group, aligned declarations — independent of the
// template's whitespace, and surfaces a malformed template as a clear gofmt error
// instead of emitting broken Go.

import (
	"bytes"
	"fmt"
	"go/format"
	"io"

	"github.com/the-protobuf-project/protorm/plugin/generator/templates"
)

// renderGo executes the named template into w, gofmt-formatting the rendered Go
// first. Use it for every .go output; non-Go files (README.md) use templates.Render.
func renderGo(w io.Writer, name string, data any) error {
	var buf bytes.Buffer
	if err := templates.Render(&buf, name, data); err != nil {
		return err
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt %s: %w\nrendered source:\n%s", name, err, buf.String())
	}
	_, err = w.Write(formatted)
	return err
}
