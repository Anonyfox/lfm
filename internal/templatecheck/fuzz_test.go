package templatecheck

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

// Property: on arbitrary template text and JSON values, Parse, Check and (when
// Check passes, as in the compiler) Render terminate quickly without panicking,
// and every diagnostic points inside its source file.
func FuzzTemplatePipeline(f *testing.F) {
	config := `{"title":"T","data":{"items":[{"name":"a"},{"name":""}],"n":3,"on":true,"url":"https://example.org/","html":"<b>"}}`
	for _, source := range []string{
		`{{.title}}`, `{{range .data.items}}{{.name}}{{else}}none{{end}}`, `{{with .data.missing}}{{.}}{{end}}`, `{{if .data.on}}{{.data.n}}{{end}}`,
		`{{define "card"}}{{.name}}{{end}}{{range .data.items}}{{template "card" .}}{{end}}`, `{{block "x" .}}{{.title}}{{end}}`,
		`{{define "r"}}{{template "r" .}}{{end}}{{template "r" .}}`, `{{range 3}}{{.}}{{end}}`, `{{range 9}}{{range 9}}{{end}}{{end}}`, `{{range $i, $x := .data.items}}{{$i}}{{$x.name}}{{break}}{{end}}`,
		`<a href="{{.data.url}}">x</a><script>var x = {{.data}};</script><style>a{color:{{.data.html}}}</style>`, `{{printf "%v %d" .data.n .title}}`,
		`{{$x := .data}}{{$x = .title}}{{$x}}`, `{{index .data.items 5}}`, `{{slice .title 0 1}}`, `{{len .data}}{{script "a.js"}}{{style "a.css"}}`,
		"line one\n{{.data.missing}}\r\n{{end}}", `<a {{.title}}="x">`, `{{/* comment */}}{{- .title -}}`,
	} {
		f.Add(source, config)
	}
	f.Fuzz(func(t *testing.T, source, config string) {
		if len(source) > 2048 || len(config) > 2048 {
			t.Skip()
		}
		var values map[string]any
		decoder := json.NewDecoder(strings.NewReader(config))
		decoder.UseNumber()
		if decoder.Decode(&values) != nil {
			t.Skip()
		}
		files := map[string][]byte{"index.html.tmpl": []byte(source)}
		inBounds := func(stage string, line, offset int, file string) {
			if raw, ok := files[file]; ok && (offset < 0 || offset > len(raw) || line < 1) {
				t.Fatalf("%s diagnostic outside %s: line %d offset %d", stage, file, line, offset)
			}
		}
		start := time.Now()
		template, parsed := Parse(files, "index.html.tmpl")
		for _, d := range parsed.Diagnostics {
			inBounds("parse", d.Location.Line, d.Location.Offset, d.Location.File)
		}
		if parsed.Errors > 0 {
			return
		}
		options := Options{Entry: "index.html.tmpl", Files: files, Values: values, Sources: jsonsrc.Index(map[string][]byte{"lfm.json": []byte(config)})}
		checked := Check(template, options)
		for _, d := range checked.Diagnostics {
			inBounds("check", d.Location.Line, d.Location.Offset, d.Location.File)
		}
		if checked.Errors == 0 {
			_, issue := Render(template, options)
			if issue != nil && strings.HasPrefix(issue.Message, "Template rendering stopped unexpectedly") {
				t.Fatalf("render panicked: %s", issue.Message)
			}
			if issue != nil {
				inBounds("render", issue.Location.Line, issue.Location.Offset, issue.Location.File)
			}
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("pipeline took %v", elapsed)
		}
	})
}
