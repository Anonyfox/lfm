package project

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/term"
)

// Explain renders lfm.json's reference for the terminal: every top-level
// field for "", the fields of an object or map, or one field in full. Paths
// may name map entries concretely (urls.checkout) or with "*".
func Explain(path string) (string, error) {
	path = strings.TrimSpace(path)
	var b strings.Builder
	if path == "" {
		b.WriteString("lfm.json: the page's facts and build settings. Everything the page sells or shows is under data.\n\n")
		rows(&b, Children(""))
		b.WriteString("\n'lfm help config <field>' shows one field, e.g. 'lfm help config deliverables.printable'.\n")
		return b.String(), nil
	}
	f, ok := Lookup(path)
	if !ok {
		var paths []string
		for _, f := range Spec {
			paths = append(paths, f.Path)
		}
		return "", fmt.Errorf("lfm.json has no field %q.%s 'lfm help config' lists them", path, term.DidYouMean(path, paths))
	}
	fmt.Fprintf(&b, "%s  %s\n  %s\n", f.Path, f.Shape(), term.Wrap(f.Doc, term.Width-2, "  "))
	if f.Required {
		b.WriteString("  required\n")
	}
	if f.Default != "" {
		fmt.Fprintf(&b, "  default: %s\n", f.Default)
	}
	if f.Checked != "" {
		fmt.Fprintf(&b, "  checked by %s ('lfm rules %s')\n", f.Checked, f.Checked)
	}
	if f.Example != "" {
		fmt.Fprintf(&b, "  example: %q: %s\n", f.Name(), f.Example)
	}
	children := Children(f.Path)
	if len(children) > 0 {
		b.WriteString("\n")
		rows(&b, children)
	}
	return b.String(), nil
}

// rows lists fields one per line: name, shape, required or default, meaning.
func rows(b *strings.Builder, fields []Field) {
	width := 0
	for _, f := range fields {
		width = max(width, len(f.Name()))
	}
	for _, f := range fields {
		note := ""
		switch {
		case f.Required:
			note = "required · "
		case f.Default != "":
			note = "default " + f.Default + " · "
		}
		more := ""
		if len(Children(f.Path)) > 0 {
			more = " (more: 'lfm help config " + strings.ReplaceAll(f.Path, ".*", "") + "')"
		}
		indent := strings.Repeat(" ", width+4)
		line := fmt.Sprintf("%s · %s%s%s", f.Shape(), note, f.Doc, more)
		fmt.Fprintf(b, "  %-*s  %s\n", width, f.Name(), term.Wrap(line, max(40, term.Width-len(indent)), indent))
	}
}

// Markdown renders the whole reference for docs/config.md.
func Markdown() string {
	var b strings.Builder
	b.WriteString("# lfm.json reference\n\nGenerated from `internal/project/spec.go` by `go test ./internal/project` with `LFM_UPDATE_DOCS=1`; edit the spec, not this file. `lfm help config [field]` prints the same in the terminal, and every config finding quotes the field it is about.\n\n")
	b.WriteString("| Field | Takes | Default | Meaning |\n| --- | --- | --- | --- |\n")
	for _, f := range Spec {
		def := f.Default
		if f.Required {
			def = "required"
		}
		doc := f.Doc
		if f.Example != "" {
			doc += " Example: `" + f.Example + "`."
		}
		if f.Checked != "" {
			doc += " Checked by `" + f.Checked + "`."
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", f.Path, f.Shape(), def, strings.ReplaceAll(doc, "|", "\\|"))
	}
	b.WriteString("\n## Moved keys\n\nAn lfm.json of an earlier version is told where these went:\n\n")
	for _, old := range slices.Sorted(maps.Keys(Moved)) {
		fmt.Fprintf(&b, "- `%s` is now `%s`\n", old, Moved[old])
	}
	return b.String()
}
