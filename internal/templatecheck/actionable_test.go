package templatecheck

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

func TestDictPartialProvenance(t *testing.T) {
	for _, selection := range []string{`.root.title`, `$.root.title`, `(index . "root").title`, `$r.title`} {
		source := `{{template "card" (dict "root" $ "case" .data.case)}}{{define "card"}}{{$r := .root}}` + `{{` + selection + `}}/{{.case.name}}{{end}}`
		_, s, raw := fixture(t, source, `{"title":"Root","data":{"case":{"name":"Case"},"stale":"unused"}}`, nil)
		if s.Errors != 0 || string(raw) != "Root/Case" || count(s, "template.unused", diagnostic.Warning) != 1 || !strings.Contains(messages(s), "data.stale") {
			t.Fatalf("%s: %s %s", selection, raw, messages(s))
		}
	}
}

func TestDictInvalidArguments(t *testing.T) {
	for _, source := range []string{`{{dict "key"}}`, `{{dict 42 "value"}}`} {
		_, s, _ := fixture(t, source, `{}`, nil)
		if s.Errors != 1 || !strings.Contains(messages(s), "dict") {
			t.Fatal(messages(s))
		}
	}
}

func TestOptionalDirectGuards(t *testing.T) {
	for _, config := range []string{`{}`, `{"data":null}`, `{"data":{}}`, `{"data":{"note":null}}`, `{"data":{"note":""}}`, `{"data":{"note":"yes"}}`} {
		for _, source := range []string{`{{with .data.note}}{{.}}{{else}}absent{{end}}`, `{{with .data.note}}{{$.data.note}}{{else}}absent{{end}}`, `{{if .data.note}}{{.data.note}}{{else}}absent{{end}}`, `{{$r := $}}{{with $r.data.note}}{{.}}{{else}}absent{{end}}`} {
			_, s, raw := fixture(t, source, config, nil)
			if s.Errors != 0 || count(s, "template.empty", diagnostic.Warning) != 0 || count(s, "template.inactive", diagnostic.Warning) != 0 {
				t.Fatalf("%s %s: %s", source, config, messages(s))
			}
			expected := "absent"
			if strings.Contains(config, "yes") {
				expected = "yes"
			}
			if string(raw) != expected {
				t.Fatalf("got %s want %s", raw, expected)
			}
		}
	}
	_, s, _ := fixture(t, `{{.data.note}}`, `{"data":{}}`, nil)
	if s.Errors != 1 {
		t.Fatal(messages(s))
	}
	_, s, _ = fixture(t, `{{with .data.note}}{{.}}{{end}}`, `{"data":42}`, nil)
	if s.Errors == 0 {
		t.Fatal("guard hid wrong shape")
	}
}

func TestMissingPathFullReferenceAndClosestKeys(t *testing.T) {
	_, s, _ := fixture(t, "first\n{{.data.operationSourceKey.name}}", `{"data":{"a":1,"b":2,"c":3,"d":4,"e":5,"f":6,"g":7,"h":8,"operationSource":{}}}`, nil)
	for _, d := range s.Diagnostics {
		if d.Rule == "template.bindings" {
			if d.Path != "data.operationSourceKey.name" || d.Location.File != "index.html.tmpl" || d.Location.Line != 2 || !strings.Contains(d.Hint, `Available keys: "operationSource"`) {
				t.Fatalf("%+v", d)
			}
			return
		}
	}
	t.Fatal(messages(s))
}

func TestHelperHelpMatchesFunctions(t *testing.T) {
	help := HelperHelp(nil)
	for name := range assetFunctions(nil) {
		if !strings.HasPrefix(name, "__lfm") && !strings.Contains(help, "  "+name+" — ") {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestMissingPartialPathLocation(t *testing.T) {
	_, s, _ := fixture(t, `{{template "card" .data}}`, `{"data":{"operationSource":"present"}}`, map[string]string{"partials/card.html.tmpl": "{{define \"card\"}}\n{{.operationSourceKey.name}}{{end}}"})
	for _, d := range s.Diagnostics {
		if d.Rule == "template.bindings" {
			if d.Location.File != "partials/card.html.tmpl" || d.Location.Line != 2 || d.Path != "data.operationSourceKey.name" || !strings.Contains(d.Hint, "operationSource") {
				t.Fatalf("%+v", d)
			}
			return
		}
	}
	t.Fatal(messages(s))
}

func TestIndexMissingKeysSuggestClosest(t *testing.T) {
	_, s, _ := fixture(t, `{{index .data "titel"}}`, `{"data":{"title":"present"}}`, nil)
	for _, d := range s.Diagnostics {
		if d.Rule == "template.empty" && strings.Contains(d.Hint, `Available keys: "title"`) {
			return
		}
	}
	t.Fatal(messages(s))
}

func TestDictRangeProvenance(t *testing.T) {
	_, s, raw := fixture(t, `{{range dict "root" $ "case" .data.case}}{{template "show" .}}{{end}}{{define "show"}}{{with .title}}{{.}}{{end}}{{with .name}}{{.}}{{end}}{{end}}`, `{"title":"Root","data":{"case":{"name":"Case"},"stale":"unused"}}`, nil)
	if s.Errors != 0 || !strings.Contains(string(raw), "Root") || !strings.Contains(string(raw), "Case") || count(s, "template.unused", diagnostic.Warning) != 1 {
		t.Fatalf("%s %s", raw, messages(s))
	}
}

func TestOptionalGuardPreservesNumberMethods(t *testing.T) {
	_, s, raw := fixture(t, `{{if .data.n.String}}{{.data.n.String}}{{end}}`, `{"data":{"n":42}}`, nil)
	if s.Errors != 0 || string(raw) != "42" {
		t.Fatalf("%s %s", raw, messages(s))
	}
}

func TestFieldMethod(t *testing.T) {
	for _, tc := range []struct {
		raw          any
		name         string
		found, fails bool
	}{
		{nil, "String", false, false}, {true, "String", false, false},
		{json.Number("42"), "String", true, false}, {json.Number("42"), "Int64", true, false},
		{json.Number("invalid"), "Int64", true, true},
		{badMethod{}, "String", true, true},
	} {
		_, found, err := fieldMethod(tc.raw, tc.name)
		if found != tc.found || (err != nil) != tc.fails {
			t.Fatalf("%T.%s: found %v error %v", tc.raw, tc.name, found, err)
		}
	}
}

type badMethod struct{}

func (badMethod) String() {}

func TestNumberMethodErrorsMatchRender(t *testing.T) {
	for _, source := range []string{`{{.data.n.Int64}}`, `{{if .data.n.Int64}}yes{{end}}`} {
		files := map[string][]byte{"index.html.tmpl": []byte(source)}
		tmpl, parsed := Parse(files, "index.html.tmpl")
		if parsed.Errors != 0 {
			t.Fatal(messages(parsed))
		}
		options := Options{Entry: "index.html.tmpl", Files: files, Values: map[string]any{"data": map[string]any{"n": json.Number("1.5")}}}
		checked := Check(tmpl, options)
		if checked.Errors != 1 {
			t.Fatal(messages(checked))
		}
		if raw, issue := Render(tmpl, options); issue == nil || raw != nil {
			t.Fatalf("method error was ignored: %s %+v", raw, issue)
		}
	}
}

func TestIndexMissingPathIncludesRemainingSelectors(t *testing.T) {
	_, s, _ := fixture(t, "first\n{{index .data \"titel\" \"rows\" 0 \"name\"}}", `{"data":{"title":"present"}}`, nil)
	for _, d := range s.Diagnostics {
		if d.Rule == "template.empty" && d.Path == "data.titel.rows[0].name" && d.Location.Line == 2 && strings.Contains(d.Hint, `Available keys: "title"`) {
			return
		}
	}
	t.Fatal(messages(s))
}

func TestOptionalDictMember(t *testing.T) {
	_, s, raw := fixture(t, `{{template "card" (dict "root" $)}}{{define "card"}}{{.root.title}}{{if .note}}{{.note}}{{end}}{{end}}`, `{"title":"Root"}`, nil)
	if string(raw) != "Root" || len(s.Diagnostics) != 0 {
		t.Fatalf("%s %s", raw, messages(s))
	}
}

func TestUndefinedPartialNamesFileAndSearchPath(t *testing.T) {
	_, s, _ := fixture(t, `{{template "partials/mice/top.html.tmpl" .}}`, `{}`, nil)
	m := messages(s)
	var hint string
	for _, d := range s.Diagnostics {
		if d.Rule == "template.bindings" {
			hint = d.Hint
		}
	}
	if s.Errors == 0 || !strings.Contains(m, `"partials/mice/top.html.tmpl" is not defined: no such file`) || !strings.Contains(hint, "every file under partials/") || !strings.Contains(hint, "shared partial") {
		t.Fatalf("%s | %s", m, hint)
	}
	_, s, _ = fixture(t, `{{template "crad" .}}{{define "card"}}x{{end}}`, `{}`, nil)
	for _, d := range s.Diagnostics {
		if d.Rule == "template.bindings" && !strings.Contains(d.Hint, "Defined: card") {
			t.Fatal(d.Hint)
		}
	}
}
