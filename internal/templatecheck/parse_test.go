package templatecheck

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

func TestTemplateDefinitionOverridesAndUnused(t *testing.T) {
	files := map[string][]byte{"index.html.tmpl": []byte(`{{block "card" .}}default{{end}}`), "partials/card": []byte(`{{define "card"}}custom{{end}}{{define "unused"}}extra{{end}}`)}
	template, s := Parse(files, "index.html.tmpl")
	if s.Errors > 0 || count(s, "template.value", diagnostic.Warning) != 1 {
		t.Fatal(messages(s))
	}
	checked := Check(template, Options{Entry: "index.html.tmpl", Files: files, Values: map[string]any{}})
	if count(checked, "template.unused", diagnostic.Warning) != 1 {
		t.Fatal(messages(checked))
	}
	raw, issue := Render(template, Options{Entry: "index.html.tmpl", Files: files, Values: map[string]any{}})
	if issue != nil || string(raw) != "custom" {
		t.Fatalf("%s %+v", raw, issue)
	}
	_, s = Parse(map[string][]byte{}, "index.html.tmpl")
	if s.Errors != 1 {
		t.Fatal(messages(s))
	}
}
func TestRenderExecutionFallback(t *testing.T) {
	files := map[string][]byte{"index.html.tmpl": []byte("line one\n{{.data.missing}}")}
	template, s := Parse(files, "index.html.tmpl")
	if s.Errors > 0 {
		t.Fatal(messages(s))
	}
	raw, issue := Render(template, Options{Entry: "index.html.tmpl", Files: files, Values: map[string]any{"data": map[string]any{}}})
	if raw != nil || issue == nil || issue.Location.Line != 2 || issue.Rule != "template.render" {
		t.Fatalf("%s %+v", raw, issue)
	}
}
func TestResourceLimits(t *testing.T) {
	for _, source := range []string{`{{range 100001}}{{.}}{{end}}`, `{{range 60000}}x{{.}}{{end}}`} {
		_, s, _ := fixture(t, source, `{}`, nil)
		if s.Errors != 1 || !strings.Contains(messages(s), "100,000") {
			t.Fatal(messages(s))
		}
	}
}
func FuzzBindingsMatchGo(f *testing.F) {
	for _, seed := range []string{`{}`, `{"x":null}`, `{"x":[{"label":"A"},{}]}`, `{"x":{"label":"one"}}`, `{"x":false}`, `{"x":"text"}`, `{"x":1.5}`} {
		f.Add(seed, uint8(0))
	}
	templates := []string{
		`{{.x}}`, `{{.x.label}}`, `{{range .x}}{{.label}}{{else}}none{{end}}`,
		`{{with .x}}{{.label}}{{else}}none{{end}}`, `{{if .x}}{{.x.label}}{{end}}`,
		`{{len .x}}`, `{{index .x 0}}`, `{{and false .x.label}}`, `{{or true .x.label}}`,
		`{{$x := .x}}{{with $x}}{{$x.label}}{{end}}`, `{{eq .x 1}}`,
		`{{(index . "x").label}}`, `{{slice .x 0 1}}`, `{{printf "%d" .x}}`,
	}
	f.Fuzz(func(t *testing.T, raw string, choice uint8) {
		if len(raw) > 4096 {
			t.Skip()
		}
		var values map[string]any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&values); err != nil {
			t.Skip()
		}
		files := map[string][]byte{"index.html.tmpl": []byte(templates[int(choice)%len(templates)])}
		template, parsed := Parse(files, "index.html.tmpl")
		if parsed.Errors > 0 {
			t.Fatal(messages(parsed))
		}
		options := Options{Entry: "index.html.tmpl", Files: files, Values: values}
		checked := Check(template, options)
		_, issue := Render(template, options)
		if (checked.Errors > 0) != (issue != nil) {
			t.Fatalf("template %s raw %s preflight %s render %+v", files[options.Entry], raw, messages(checked), issue)
		}
	})
}

func TestArbitraryKeysAndIndexUsage(t *testing.T) {
	_, s, raw := fixture(t, `{{index .data "a.b" "x[0]"}}{{index .data "a.b" "list" 0 "used"}}`, `{"data":{"a.b":{"x[0]":"yes","list":[{"used":"ok","unused[0]":1},{"unused[0]":2}]}}}`, nil)
	if s.Errors > 0 || string(raw) != "yesok" || !strings.Contains(messages(s), `data["a.b"].list[*]["unused[0]"] (2 values)`) {
		t.Fatalf("%s %s", raw, messages(s))
	}
}
