package web

import (
	"bytes"
	"context"
	"io"

	"github.com/a-h/templ"
	"github.com/yuin/goldmark"
)

// markdown renders CommonMark. It leaves raw HTML out, so what an agent
// writes can't add markup to the page.
var markdown = goldmark.New()

// Markdown is text an agent wrote, such as a Run's closing notes, rendered
// from its Markdown in a block of class prose.
func Markdown(text string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		var b bytes.Buffer
		if err := markdown.Convert([]byte(text), &b); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="prose">`); err != nil {
			return err
		}
		if _, err := b.WriteTo(w); err != nil {
			return err
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
}
