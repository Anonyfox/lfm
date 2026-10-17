package jsonsrc

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// Property: Parent inverts Child and array indexing, and a child path is Within
// its parent but never within a sibling.
func FuzzChildParent(f *testing.F) {
	for _, seed := range [][3]string{{"", "a", "b"}, {"data", "a.b", "title"}, {"a", `b\"c`, "[]"}, {"rows", "1x", "\xff"}, {"", "", ""}, {"é", "_9", "9_"}} {
		f.Add(seed[0], seed[1], seed[2], uint8(2))
	}
	f.Fuzz(func(t *testing.T, a, b, key string, index uint8) {
		parent := Child(Child("", a), b)
		if Parent(parent) != Child("", a) || Parent(Child("", a)) != "" {
			t.Fatalf("Parent does not invert Child for %q, %q", a, b)
		}
		if item := fmt.Sprintf("%s[%d]", parent, index); Parent(item) != parent || !Within(item, parent) {
			t.Fatalf("array element %q does not belong to %q", item, parent)
		}
		child := Child(parent, key)
		if Parent(child) != parent {
			t.Fatalf("Parent(Child(%q, %q)) = %q", parent, key, Parent(child))
		}
		if !Within(child, parent) || !Within(child, "") || Within(parent, child) {
			t.Fatalf("Within boundaries broken for %q under %q", child, parent)
		}
		if sibling := Child(parent, key+"x"); Within(sibling, child) {
			t.Fatalf("sibling %q is within %q", sibling, child)
		}
	})
}

// Property: Index is deterministic, every location points inside its file at the
// position diagnostic.At reports, and every indexed path's ancestors are indexed.
func FuzzIndex(f *testing.F) {
	for _, seed := range []string{"{\r\n  \"data\": {\"a.b\": {\"title\": \"secret\", \"title\": \"other\"}},\r\n  \"rows\": [1, null, {}]\r\n}", `[{"answer":"a","answer":"b"}]`, "", `{"a":`, `{} {}`, `{"":{"\"":[[]]}}`, `"x"`} {
		f.Add(seed, `{"q":[1,{"a":null}]}`)
	}
	f.Fuzz(func(t *testing.T, config, data string) {
		if len(config)+len(data) > 16384 {
			t.Skip()
		}
		files := map[string][]byte{"lfm.json": []byte(config), "data/faq.json": []byte(data)}
		s := Index(files)
		if again := Index(files); !reflect.DeepEqual(s.Locations, again.Locations) || !reflect.DeepEqual(s.Issues, again.Issues) {
			t.Fatal("Index is not deterministic")
		}
		for key, loc := range s.Locations {
			raw := files[loc.File]
			if loc.Offset < 0 || loc.Offset > len(raw) || loc != diagnostic.At(loc.File, raw, loc.Offset) {
				t.Fatalf("%s: bad location %+v", key, loc)
			}
			if parent := Parent(key); parent != "" && parent != "data" {
				if _, ok := s.Locations[parent]; !ok {
					t.Fatalf("%s: parent %q not indexed", key, parent)
				}
			}
		}
		for key := range s.Leaves {
			if _, ok := s.Locations[key]; !ok {
				t.Fatalf("leaf %q has no location", key)
			}
		}
		for _, issue := range s.Issues {
			if loc := issue.Location; loc.Offset < 0 || loc.Offset > len(files[loc.File]) {
				t.Fatalf("issue outside its file: %+v", issue)
			}
		}
	})
}
