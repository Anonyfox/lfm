package jsonsrc

import (
	"strings"
	"testing"
)

func TestSourcesLocationsDuplicatesAndDatasets(t *testing.T) {
	config := "{\r\n  \"data\": {\"a.b\": {\"title\": \"secret\", \"title\": \"other\"}},\r\n  \"rows\": [1, null, {}]\r\n}"
	s := Index(map[string][]byte{"lfm.json": []byte(config), "data/faq.json": []byte("[\n {\"answer\":\"a\",\"answer\":\"b\"}\n]")})
	if len(s.Issues) != 2 {
		t.Fatalf("%+v", s.Issues)
	}
	for _, issue := range s.Issues {
		if issue.Rule != "config.duplicate-key" || len(issue.Related) != 1 || issue.Location.Offset <= issue.Related[0].Offset || issue.Location.Line != 2 {
			t.Fatalf("%+v", issue)
		}
	}
	loc := s.Locations[`data["a.b"].title`]
	if string(s.Raw[loc.File][loc.Offset:loc.Offset+7]) != `"title"` || loc.Line != 2 {
		t.Fatal(loc)
	}
	if loc, ok := s.Nearest(`data["a.b"].title.child`); !ok || loc.Line != 2 {
		t.Fatal(loc)
	}
	if _, ok := s.Leaves["rows[2]"]; !ok {
		t.Fatal("empty container missing")
	}
	for _, key := range []string{`a["b.c"].d`, `a["b\\\"c"].d`, `a["[]"].d`} {
		if Parent(key) != strings.TrimSuffix(key, ".d") {
			t.Fatal(key, Parent(key))
		}
	}
	if !Within(`data["a.b"].title`, `data["a.b"]`) || Within("data.foobar", "data.foo") {
		t.Fatal("path boundaries")
	}
}
func TestInvalidJSONSources(t *testing.T) {
	for _, raw := range []string{"", `{"a":`, `{"a":1,}`, `{} {}`, `[1,]`, "\xef\xbb\xbf{}"} {
		s := Index(map[string][]byte{"lfm.json": []byte(raw)})
		if len(s.Issues) != 1 || s.Issues[0].Rule != "config.invalid" {
			t.Fatalf("%q %+v", raw, s.Issues)
		}
	}
}
