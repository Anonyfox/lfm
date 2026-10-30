package project

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The structs lfm decodes and Spec must describe the same file: a field added
// to one without the other fails here, so docs, help and checks never drift.
func TestSpecMatchesTheConfigStructs(t *testing.T) {
	var paths []string
	var walk func(t reflect.Type, prefix string)
	walk = func(typ reflect.Type, prefix string) {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		switch typ.Kind() {
		case reflect.Struct:
			for i := range typ.NumField() {
				name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
				if name == "" || name == "-" {
					continue
				}
				path := name
				if prefix != "" {
					path = prefix + "." + name
				}
				paths = append(paths, path)
				walk(typ.Field(i).Type, path)
			}
		case reflect.Slice:
			if typ.Elem().Kind() == reflect.Struct {
				paths = append(paths, prefix+".*")
				walk(typ.Elem(), prefix+".*")
			}
		case reflect.Map:
			if typ.Elem().Kind() != reflect.Interface {
				paths = append(paths, prefix+".*")
				walk(typ.Elem(), prefix+".*")
			}
		}
	}
	walk(reflect.TypeOf(Config{}), "")
	var spec []string
	for _, f := range Spec {
		if !f.Reserved {
			spec = append(spec, f.Path)
		}
	}
	for _, p := range paths {
		if !slices.Contains(spec, p) {
			t.Errorf("Config has %s, Spec does not", p)
		}
	}
	for _, p := range spec {
		if !slices.Contains(paths, p) {
			t.Errorf("Spec has %s, Config does not", p)
		}
	}
	for _, f := range Spec {
		if f.Doc == "" || !strings.HasSuffix(f.Doc, ".") {
			t.Errorf("%s needs a one-sentence Doc", f.Path)
		}
		if f.Kind != Object && f.Kind != Free && f.Kind != Map && f.Example == "" && len(f.Values) == 0 {
			t.Errorf("%s needs an Example", f.Path)
		}
	}
}

func TestShapeErrorsExplainTheField(t *testing.T) {
	problems := checkShape([]byte(`{"schemaVersion":1,"url":"https://e.com/","language":"en","title":"T","description":"D",
		"deliverables":{"printable":{"sell":"shop","Path":"x"},"bundle":{"sell":"page"}},"build":{"browserData":"data.x"}}`))
	got := map[string]Problem{}
	for _, p := range problems {
		got[p.Path] = p
	}
	if p := got["deliverables.printable.sell"]; !strings.Contains(p.Err.Error(), `"shop" is not page, shops or bundle`) || !strings.Contains(p.Hint, "lfm help config deliverables.printable") {
		t.Errorf("sell: %+v", p)
	}
	if p := got["deliverables.printable.Path"]; !strings.Contains(p.Hint, "Use the exact key deliverables.printable.path") {
		t.Errorf("casing: %+v", p)
	}
	if p := got["deliverables.bundle.name"]; p.Err == nil || !strings.Contains(p.Err.Error(), "is required") {
		t.Errorf("required: %+v", got)
	}
	if p := got["build.browserData"]; p.Err == nil || !strings.Contains(p.Err.Error(), "must be a list") {
		t.Errorf("kind: %+v", got)
	}
}

// docs/config.md is generated from Spec; LFM_UPDATE_DOCS=1 rewrites it.
func TestConfigReferenceIsCurrent(t *testing.T) {
	path := "../../docs/config.md"
	want := Markdown()
	if os.Getenv("LFM_UPDATE_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatal("docs/config.md is stale; run LFM_UPDATE_DOCS=1 go test ./internal/project")
	}
}

func TestExplain(t *testing.T) {
	top, _ := Explain("")
	sub, _ := Explain("deliverables.printable")
	leaf, _ := Explain("urls.checkout")
	if !strings.Contains(top, "deliverables") || !strings.Contains(sub, "sell") || !strings.Contains(sub, "default page") || !strings.Contains(leaf, "urls.*") {
		t.Fatalf("%s\n%s\n%s", top, sub, leaf)
	}
	if _, err := Explain("nope.nothing"); err == nil {
		t.Fatal("unknown field explained")
	}
}

func TestMovedKeysNameTheirNewPlace(t *testing.T) {
	problems := checkShape([]byte(`{"schemaVersion":1,"url":"https://e.com/","language":"en","title":"T","description":"D","deliverables":{"printable":{"shop":"X"}}}`))
	if len(problems) != 1 || !strings.Contains(problems[0].Hint, "It moved to seller.name") {
		t.Fatalf("%+v", problems)
	}
	for old, to := range Moved {
		if _, ok := Lookup(to); !ok {
			t.Errorf("%s moved to %s, which Spec does not have", old, to)
		}
	}
}
