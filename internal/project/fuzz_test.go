package project

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const fuzzConfig = `{"schemaVersion":1,"url":"https://example.org/p/","language":"en","title":"T","description":"D","links":{"x":1},"build":{"browserData":["data.a"],"assets":{"default":"auto","overrides":{"a.css":"inline"}},"validation":{"rules":{"x":"off"}}},"data":{"a":{"b":[1,"2"]}}}`

// Property: DecodeJSON accepts exactly the single valid JSON documents and
// its result survives a marshal/decode round trip unchanged.
func FuzzDecodeJSON(f *testing.F) {
	for _, seed := range []string{fuzzConfig, "", "{} {}", `{"a":1,}`, "\xef\xbb\xbf{}", `1e400`, `"\ud800"`, " [1] \n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		var value any
		err := DecodeJSON([]byte(raw), &value)
		if (err == nil) != json.Valid([]byte(raw)) {
			t.Fatalf("DecodeJSON error %v disagrees with json.Valid", err)
		}
		if err != nil {
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var again any
		if err := DecodeJSON(encoded, &again); err != nil || !reflect.DeepEqual(value, again) {
			t.Fatalf("round trip changed %s: %v", encoded, err)
		}
	})
}

// Property: Load never panics, reports every key that strict decoding rejects,
// and a problem-free load exposes exactly the authored data.
func FuzzLoad(f *testing.F) {
	for _, seed := range [][2]string{{fuzzConfig, `{"q":[1]}`}, {`{"URL":"x","data":{}}`, `null`}, {`{"socialImage":{"src":"a","Alt":""}}`, `{`}, {`{"build":{"assets":{"overrides":{"../x":"file"}}}}`, `[]`}, {`{"data":{"faq":1}}`, `2`}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, config, data string) {
		if len(config)+len(data) > 16384 {
			t.Skip()
		}
		root := t.TempDir()
		if os.WriteFile(filepath.Join(root, ConfigFile), []byte(config), 0o644) != nil || os.Mkdir(filepath.Join(root, DataDir), 0o755) != nil ||
			os.WriteFile(filepath.Join(root, DataDir, "faq.json"), []byte(data), 0o644) != nil {
			t.Fatal("cannot write fixture")
		}
		p, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		loaded := Load(p, nil)
		unknown := false
		for _, problem := range loaded.Problems {
			if problem.File == "" || problem.Err == nil {
				t.Fatalf("incomplete problem %+v", problem)
			}
			unknown = unknown || strings.Contains(problem.Err.Error(), "unknown field")
		}
		strict := json.NewDecoder(strings.NewReader(config))
		strict.DisallowUnknownFields()
		if err := strict.Decode(new(Config)); json.Valid([]byte(config)) && err != nil && strings.Contains(err.Error(), "unknown field") && !unknown {
			t.Fatalf("strict decoding rejects %q but Load reports no unknown field", err)
		}
		if len(loaded.Problems) > 0 {
			return
		}
		var want any
		if DecodeJSON([]byte(data), &want) != nil {
			t.Fatal("invalid data file loaded without a problem")
		}
		values, _ := loaded.Values["data"].(map[string]any)
		if got := values["faq"]; !reflect.DeepEqual(roundTrip(t, got), roundTrip(t, want)) {
			t.Fatalf("data.faq = %v, want %v", got, want)
		}
	})
}

// Property: BrowserData copies exactly the selected values, leaves its input
// untouched and emits JSON that cannot close a <script> element.
func FuzzBrowserData(f *testing.F) {
	for _, seed := range [][2]string{{`{"data":{"a":{"b":[1]},"c":"</script>"}}`, "data.a.b,data.c"}, {`{"a":{"b":{"c":1}}}`, "a,a.b.c"}, {`{"a":1}`, "a."}, {`{"a":{}}`, "a.b"}, {`{"":1}`, ""}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, raw, selection string) {
		var values map[string]any
		if len(raw) > 8192 || DecodeJSON([]byte(raw), &values) != nil {
			t.Skip()
		}
		before := roundTrip(t, values)
		paths := strings.Split(selection, ",")
		out, err := BrowserData(values, paths)
		if !reflect.DeepEqual(before, roundTrip(t, values)) {
			t.Fatal("BrowserData mutated its input")
		}
		if err != nil {
			return
		}
		if bytes.ContainsAny(out, "<>&") {
			t.Fatalf("unescaped markup in %s", out)
		}
		var selected map[string]any
		if err := DecodeJSON(out, &selected); err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if got, want := lookup(selected, path), lookup(values, path); !reflect.DeepEqual(roundTrip(t, got), roundTrip(t, want)) {
				t.Fatalf("%s: selected %v, want %v", path, got, want)
			}
		}
	})
}

func lookup(values map[string]any, path string) any {
	var value any = values
	for _, part := range strings.Split(path, ".") {
		object, _ := value.(map[string]any)
		value = object[part]
	}
	return value
}

// roundTrip normalizes a decoded JSON value so json.Number and float forms compare equal.
func roundTrip(t *testing.T, value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var normalized any
	if err := DecodeJSON(raw, &normalized); err != nil {
		t.Fatal(err)
	}
	return normalized
}
