package templatecheck

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

func jsonOptions(t *testing.T, config string) Options {
	t.Helper()
	options := Options{Sources: jsonsrc.Index(map[string][]byte{"lfm.json": []byte(config)})}
	decoder := json.NewDecoder(strings.NewReader(config))
	decoder.UseNumber()
	if err := decoder.Decode(&options.Values); err != nil {
		t.Fatal(err)
	}
	return options
}

func TestJSONLDTypesEscapingAndSinglePass(t *testing.T) {
	options := jsonOptions(t, `{"title":"Quotes \" & </script>\n{{.missing}}", "data":{"number":9007199254740993,"off":false,"nil":null,"items":[{"name":"{{.neverRender}}"}],"key-name":"value"},"jsonld":{"name":"{{.title}}","n":"{{.data.number}}","off":"{{$.data.off}}","nil":"{{.data.nil}}","items":"{{.data.items}}","mixed":"Price {{.data.number}} €","key":"{{index .data \"key-name\"}}","first":"{{(index .data.items 0).name}}"}}`)
	resolved, used, result := ResolveJSONLD(options)
	if result.Errors != 0 || result.Warnings != 1 {
		t.Fatal(messages(result))
	}
	object := resolved.(map[string]any)
	if object["n"] != json.Number("9007199254740993") || object["off"] != false || object["nil"] != nil || object["key"] != "value" || object["mixed"] != "Price 9007199254740993 €" || object["first"] != "{{.neverRender}}" {
		t.Fatalf("%#v", resolved)
	}
	if !reflect.DeepEqual(object["items"], options.Values["data"].(map[string]any)["items"]) {
		t.Fatal("copied subtree changed")
	}
	raw, err := json.Marshal(resolved)
	if err != nil || strings.Contains(string(raw), "</script>") || !strings.Contains(string(raw), `\u003c/script\u003e`) || !strings.Contains(string(raw), "{{.missing}}") {
		t.Fatalf("%s %v", raw, err)
	}
	options.Consumed = used
	options.Files = map[string][]byte{"index.html.tmpl": []byte("OK")}
	options.Entry = "index.html.tmpl"
	tmpl, _ := Parse(options.Files, options.Entry)
	if result := Check(tmpl, options); result.Errors != 0 || result.Warnings != 0 {
		t.Fatal(messages(result))
	}
}

func TestJSONLDBindingErrorsCollectedAtConfigLeaves(t *testing.T) {
	options := jsonOptions(t, `{
"data":{"name":"secret"},
"jsonld":{
"one":"{{.data.nam}}",
"two":"{{index .data \"missing\"}}",
"three":"{{.jsonld.one}}",
"four":"{{index . \"jsonld\"}}",
"five":"{{.}}",
"six":"prefix {{.data}}",
"seven":"{{if true}}bad{{end}}",
"eight":"{{printf \"%s\" .data.name}}",
"nine":"{{.data.name | html}}",
"ten":"{{define \"x\"}}bad{{end}}",
"eleven":"{{.bad",
"twelve":"{{define \"jsonld\"}}{{.data.name}}{{end}}",
"thirteen":"{{1+2i}}",
"{{.data.name}}":"key"
}}`)
	_, _, result := ResolveJSONLD(options)
	if result.Errors != 14 {
		t.Fatal(messages(result))
	}
	if strings.Contains(messages(result), "secret") || !strings.Contains(messages(result), "Did you mean data.name") {
		t.Fatal(messages(result))
	}
	for _, issue := range result.Diagnostics {
		if issue.Rule != "jsonld.bindings" || issue.Location.File != "lfm.json" || issue.Location.Line < 4 || !strings.HasPrefix(issue.Path, "jsonld") {
			t.Fatalf("%+v", issue)
		}
	}
}

func TestJSONLDOptionalRootsAndEmptyPolicy(t *testing.T) {
	for _, raw := range []string{`{}`, `{"jsonld":null}`} {
		resolved, _, result := ResolveJSONLD(jsonOptions(t, raw))
		if resolved != nil || result.Errors != 0 {
			t.Fatal(result)
		}
	}
	for _, raw := range []string{`{"jsonld":false}`, `{"jsonld":"{{.data}}"}`, `{"jsonld":3}`} {
		_, _, result := ResolveJSONLD(jsonOptions(t, raw))
		if result.Errors != 1 {
			t.Fatal(result)
		}
	}
	options := jsonOptions(t, `{"data":{"empty":""},"jsonld":{"name":"{{.data.empty}}"}}`)
	options.Rules = map[string]diagnostic.Severity{"jsonld.empty": diagnostic.Off}
	if _, _, result := ResolveJSONLD(options); result.Errors != 0 || result.Warnings != 0 {
		t.Fatal(result)
	}
	options.Rules["jsonld.empty"] = diagnostic.Error
	if _, _, result := ResolveJSONLD(options); result.Errors != 1 {
		t.Fatal(result)
	}
}

func TestJSONLDDynamicIndexConsumption(t *testing.T) {
	options := jsonOptions(t, `{"urls":{"selected":"https://example.com/selected","unused":"https://example.com/unused"},"data":{"key":"selected","stale":"unused"},"jsonld":{"url":"{{index .urls .data.key}}"}}`)
	_, options.Consumed, _ = ResolveJSONLD(options)
	options.Entry = "index.html.tmpl"
	options.Files = map[string][]byte{options.Entry: []byte("OK")}
	tmpl, _ := Parse(options.Files, options.Entry)
	result := Check(tmpl, options)
	if result.Errors != 0 || result.Warnings != 2 || !strings.Contains(messages(result), "urls.unused") || !strings.Contains(messages(result), "data.stale") || strings.Contains(messages(result), "data.key") {
		t.Fatal(messages(result))
	}
}

func FuzzJSONLDSubstitution(f *testing.F) {
	for _, value := range []string{"{{.data.n}}", "{{index .data 0}}", "{{define \"x\"}}{{end}}", "</script>", "{{.}}", "{{(index .data.items 0).name}}", "{{nil}}"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 4096 {
			t.Skip()
		}
		raw, _ := json.Marshal(map[string]any{"data": map[string]any{"n": 1, "items": []any{}}, "jsonld": map[string]any{"name": source}})
		options := jsonOptions(t, string(raw))
		resolved, _, result := ResolveJSONLD(options)
		if result.Errors == 0 {
			serialized, err := json.Marshal(resolved)
			if err != nil || !json.Valid(serialized) || strings.Contains(string(serialized), "</script>") {
				t.Fatalf("unsafe result %s %v", serialized, err)
			}
		}
	})
}
