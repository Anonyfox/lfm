package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFooterConfig(t *testing.T) {
	for _, tc := range []struct{ value, path string }{
		{`false`, ""}, {`{}`, ""}, {`true`, "footer"}, {`null`, "footer"},
		{`{"unknown":1}`, "footer.unknown"}, {`{"wordmark":42}`, "footer.wordmark"},
		{`{"groups":[{"label":"Product","links":[{"urlKey":"news","text":42}]}]}`, "footer.groups[0].links[0].text"},
		{`{"groups":[{"label":"Product","links":[{"text":"News"}]}]}`, "footer.groups[0].links[0].urlKey"},
		{`{"groups":[{"label":"Product","links":[{"urlKey":"typo","text":"News"}]}]}`, "footer.groups[0].links[0].urlKey"},
		{`{"badges":[{"urlKey":"news","srcKey":"news","alt":"Badge","width":0,"height":20}]}`, "footer.badges[0].width"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			dir := t.TempDir()
			raw := `{"schemaVersion":1,"url":"https://example.org/","language":"en","title":"Title","description":"Description","urls":{"news":"https://example.org/news/"},"footer":` + tc.value + `}`
			if err := os.WriteFile(filepath.Join(dir, "lfm.json"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			p, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			loaded := Load(p, nil)
			if tc.path == "" {
				if len(loaded.Problems) > 0 {
					t.Fatal(loaded.Problems)
				}
				return
			}
			for _, problem := range loaded.Problems {
				if problem.Path == tc.path && strings.Contains(problem.Hint+problem.Err.Error(), "lfm help config footer") {
					if _, command, ok := strings.Cut(problem.Hint, "'lfm help config "); ok {
						path, _, _ := strings.Cut(command, "'")
						if _, err := Explain(path); err != nil {
							t.Fatal(err)
						}
					}
					return
				}
			}
			t.Fatalf("missing %s: %+v", tc.path, loaded.Problems)
		})
	}
}

func FuzzFooter(f *testing.F) {
	for _, s := range []string{`false`, `{}`, `null`, `true`, `{"groups":[]}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		footer := Footer{Disabled: true}
		if json.Unmarshal([]byte(s), &footer) != nil {
			return
		}
		if footer.Disabled != (strings.TrimSpace(s) == "false") {
			t.Fatal("footer object must enable the footer")
		}
		raw, err := json.Marshal(footer)
		if err != nil {
			t.Fatal(err)
		}
		var again Footer
		if err := json.Unmarshal(raw, &again); err != nil {
			t.Fatal(err)
		}
	})
}
