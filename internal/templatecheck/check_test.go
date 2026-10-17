package templatecheck

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

func fixture(t *testing.T, source, config string, extras map[string]string) (Options, diagnostic.Summary, []byte) {
	t.Helper()
	files := map[string][]byte{"index.html.tmpl": []byte(source)}
	for name, raw := range extras {
		files[name] = []byte(raw)
	}
	options := Options{Entry: "index.html.tmpl", Files: files, Sources: jsonsrc.Index(map[string][]byte{"lfm.json": []byte(config)})}
	decoder := json.NewDecoder(strings.NewReader(config))
	decoder.UseNumber()
	if err := decoder.Decode(&options.Values); err != nil {
		t.Fatal(err)
	}
	template, parsed := Parse(files, options.Entry)
	if parsed.Errors > 0 {
		return options, parsed, nil
	}
	checked := Check(template, options)
	if checked.Errors > 0 {
		return options, checked, nil
	}
	rendered, issue := Render(template, options)
	if issue != nil {
		checked.Diagnostics = append(checked.Diagnostics, *issue)
		checked = diagnostic.Summarize(checked.Diagnostics)
	}
	return options, checked, rendered
}
func count(s diagnostic.Summary, rule string, severity diagnostic.Severity) int {
	n := 0
	for _, d := range s.Diagnostics {
		if d.Rule == rule && d.Severity == severity {
			n++
		}
	}
	return n
}
func messages(s diagnostic.Summary) string {
	var b strings.Builder
	for _, d := range s.Diagnostics {
		b.WriteString(d.String())
		b.WriteByte('\n')
	}
	return b.String()
}

func TestCollectIndependentBindings(t *testing.T) {
	_, s, _ := fixture(t, "{{.data.titel}}\n{{.data.price}}\n{{.links.checkout.url}}", `{"data":{"title":"secret title"},"links":{"checkout":"secret URL"}}`, nil)
	if s.Errors != 3 {
		t.Fatalf("%+v", s)
	}
	if !strings.Contains(messages(s), "Did you mean data.title") || strings.Contains(messages(s), "secret") {
		t.Fatal(messages(s))
	}
	for _, d := range s.Diagnostics {
		if d.Rule != "template.bindings" {
			continue
		}
		if d.Location.File != "index.html.tmpl" || d.Location.Line < 1 || len(d.Related) == 0 {
			t.Fatalf("%+v", d)
		}
	}
}
func TestScopesAndUsage(t *testing.T) {
	source := `{{$offer := .data.offer}}{{with $offer}}{{.label}}{{end}}{{range $i,$row := .data.rows}}{{$i}}:{{template "row" $row}}{{end}}{{define "row"}}{{$.label}}{{end}}{{if false}}{{.data.dormant}}{{end}}`
	_, s, raw := fixture(t, source, `{"data":{"offer":{"label":"Offer","stale":true},"rows":[{"label":"One","extra":1},{"label":"Two","extra":2}],"dormant":"Saved"}}`, nil)
	if s.Errors > 0 || string(raw) != "Offer0:One1:Two" {
		t.Fatalf("%s %s", raw, messages(s))
	}
	if count(s, "template.unused", diagnostic.Warning) != 2 || !strings.Contains(messages(s), "data.rows[*].extra (2 values)") || strings.Contains(messages(s), "data.dormant") {
		t.Fatal(messages(s))
	}
}
func TestEmptyFalseZeroAndShapes(t *testing.T) {
	_, s, raw := fixture(t, `{{.data.blank}}/{{.data.nil}}/{{.data.off}}/{{.data.zero}}/{{range .data.rows}}{{.noSchema}}{{else}}empty{{end}}{{with .data.optional}}{{.alsoUnknown}}{{end}}`, `{"data":{"blank":"  ","nil":null,"off":false,"zero":0,"rows":[],"optional":null}}`, nil)
	if s.Errors > 0 || count(s, "template.empty", diagnostic.Warning) != 4 || !bytes.Contains(raw, []byte("false/0/empty")) {
		t.Fatalf("%s %s", raw, messages(s))
	}
}
func TestDormantAndShortCircuit(t *testing.T) {
	for _, source := range []string{`{{if .off}}{{.missing}}{{else}}ok{{end}}`, `{{and .off .missing}}`, `{{or .on .missing}}`} {
		_, s, _ := fixture(t, source, `{"off":false,"on":true}`, nil)
		if s.Errors > 0 || !strings.Contains(messages(s), "Inactive branch") {
			t.Fatal(messages(s))
		}
	}
	_, s, _ := fixture(t, `{{if false}}{{template "missing" .}}{{end}}`, `{}`, nil)
	if s.Errors != 1 {
		t.Fatal(messages(s))
	}
}
func TestGoBuiltinSemantics(t *testing.T) {
	tests := []struct {
		source, config string
		errors         int
		output         string
	}{
		{`{{len .data.rows}}/{{index .data.rows 0 "label"}}`, `{"data":{"rows":[{"label":"one"}]}}`, 0, "1/one"},
		{`{{index .data.rows 7}} {{len .data.off}} {{eq .data.number 1}}`, `{"data":{"rows":[],"off":false,"number":1}}`, 3, ""},
		{`{{printf "%s" .data.number}}`, `{"data":{"number":1}}`, 0, "1"},
		{`{{.data.number.Int64 | printf "%d"}}`, `{"data":{"number":1}}`, 0, "1"},
		{`{{"hi" | printf "%s:%s" "prefix"}}`, `{}`, 0, "prefix:hi"},
		{`{{index .data "missing"}}`, `{"data":{}}`, 0, ""},
		{`{{(index .data "missing").label}}`, `{"data":{}}`, 1, ""},
		{`{{$x := .data}}{{ $x.label }}{{$x = .other}}{{$x.label}}`, `{"data":{"label":"A"},"other":{"label":"B"}}`, 0, "AB"},
		{`{{$ = .data}}{{$.label}}`, `{"data":{"label":"A"}}`, 0, "A"},
		{`{{if true}}{{$ = .data}}{{end}}{{$.label}}`, `{"data":{"label":"A"}}`, 0, "A"},
		{`{{$x := "before"}}{{if false}}{{$x = "wrong"}}{{end}}{{$x}}`, `{}`, 0, "before"},
	}
	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			_, s, raw := fixture(t, tc.source, tc.config, nil)
			if s.Errors != tc.errors || tc.errors == 0 && string(raw) != tc.output {
				t.Fatalf("%s %s", raw, messages(s))
			}
		})
	}
}
func TestRangeControl(t *testing.T) {
	tests := []struct{ source, config, output string }{
		{`{{range .rows}}{{if .skip}}{{continue}}{{end}}{{.label}}{{end}}`, `{"rows":[{"skip":true},{"skip":false,"label":"yes"}]}`, "yes"},
		{`{{range .rows}}{{.label}}{{break}}{{.missing}}{{end}}`, `{"rows":[{"label":"yes"},{}]}`, "yes"},
		{`{{$x := "old"}}{{range $x = .rows}}{{.}}{{end}}{{$x}}`, `{"rows":["one","two"]}`, "onetwotwo"},
		{`{{range $x := .rows}}{{.label}}{{else}}{{len $x}}{{end}}`, `{"rows":[]}`, "0"},
		{`{{range 3}}{{.}}{{end}}`, `{}`, "012"},
		{`{{range $k,$v := .rows}}{{$k}}={{$v}};{{end}}`, `{"rows":{"b":"B","a":"A"}}`, "a=A;b=B;"},
	}
	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			_, s, raw := fixture(t, tc.source, tc.config, nil)
			if s.Errors > 0 || string(raw) != tc.output {
				t.Fatalf("%s %s", raw, messages(s))
			}
		})
	}
}
func TestHeterogeneousArrays(t *testing.T) {
	_, s, _ := fixture(t, `{{range .rows}}{{.title}} {{.price}}{{end}}`, `{"rows":[{"title":"A"},{"price":"B"},{}]}`, nil)
	if s.Errors != 4 {
		t.Fatal(messages(s))
	}
}
func TestParseEveryFileAndRenderContext(t *testing.T) {
	_, s, _ := fixture(t, `{{if}}`, `{}`, map[string]string{"partials/a": "{{undefined}}", "partials/b": "{{range .x}}"})
	if s.Errors != 3 {
		t.Fatal(messages(s))
	}
	_, s, _ = fixture(t, `{{if .on}}<a href="{{end}}{{.value}}`, `{"on":true,"value":"ok"}`, nil)
	if count(s, "template.render", diagnostic.Error) != 1 {
		t.Fatal(messages(s))
	}
	_, s, _ = fixture(t, `<a href="{{.value}}">go</a>`, `{"value":"javascript:alert(1)"}`, nil)
	if count(s, "template.render", diagnostic.Error) != 1 {
		t.Fatal(messages(s))
	}
}
func TestRecursivePartials(t *testing.T) {
	_, s, _ := fixture(t, `{{template "loop" .}}{{define "loop"}}{{template "loop" .}}{{end}}`, `{}`, nil)
	if s.Errors != 1 {
		t.Fatal(messages(s))
	}
	_, s, raw := fixture(t, `{{template "tree" .node}}{{define "tree"}}{{.name}}{{with .child}}{{template "tree" .}}{{end}}{{end}}`, `{"node":{"name":"A","child":{"name":"B","child":null}}}`, nil)
	if s.Errors > 0 || string(raw) != "AB" {
		t.Fatalf("%s %s", raw, messages(s))
	}
}
func TestBrowserDataAndPolicies(t *testing.T) {
	options, _, _ := fixture(t, `{{.title}}`, `{"title":"Title","data":{"runtime":{"score":1},"old":2}}`, nil)
	options.BrowserData = []string{"data.runtime", "data.missing", "title.child", "data..bad"}
	template, _ := Parse(options.Files, options.Entry)
	s := Check(template, options)
	if count(s, "template.browser-data", diagnostic.Error) != 3 || strings.Contains(messages(s), "Unused config/data field: data.runtime") {
		t.Fatal(messages(s))
	}
	options.BrowserData = []string{"data.runtime"}
	options.Rules = map[string]diagnostic.Severity{"template.unused": diagnostic.Off}
	s = Check(template, options)
	if s.Errors > 0 || s.Warnings > 0 {
		t.Fatal(messages(s))
	}
	options.Files[options.Entry] = []byte(`{{.title}}{{.data.old}}`)
	template, _ = Parse(options.Files, options.Entry)
	options.Values["title"] = ""
	options.Rules["template.empty"] = diagnostic.Error
	s = Check(template, options)
	if s.Errors != 1 {
		t.Fatal(messages(s))
	}
}
