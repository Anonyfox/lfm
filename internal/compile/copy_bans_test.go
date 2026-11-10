package compile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

func TestProjectCopyBans(t *testing.T) {
	for _, tc := range []struct {
		name, entry, html, problem string
		count                      int
		severity                   diagnostic.Severity
	}{
		{name: "literal inline", entry: `{"phrase":"HIDDEN GEM","severity":"error","hint":"Name the place."}`, html: `<p>A hidden <em>gem</em>.</p>`, count: 1, severity: check.Error},
		{name: "Unicode boundaries", entry: `{"phrase":"Oase","severity":"warning"}`, html: `<p>Öoase Oase2 Oasen</p>`},
		{name: "warning metadata", entry: `{"phrase":"Oase","severity":"warning"}`, html: `<meta name="description" content="OASE">`, count: 1, severity: check.Warning},
		{name: "regex inflection", entry: `{"regex":"\\b(traumhaft|malerisch)[\\p{L}]*","severity":"warning"}`, html: `<p>Malerische Küste.</p>`, count: 1, severity: check.Warning},
		{name: "regex substring", entry: `{"regex":"oase","severity":"error"}`, html: `<p>Oasen</p>`, count: 1, severity: check.Error},
		{name: "literal escaping", entry: `{"phrase":"gem.*","severity":"error"}`, html: `<p>gemstones</p>`},
		{name: "skip", entry: `{"phrase":"hidden gem","severity":"error"}`, html: `<p hidden>hidden gem</p><blockquote>hidden gem</blockquote><p data-slop-skip>hidden gem</p><script>console.log("hidden gem")</script>`},
		{name: "blocks", entry: `{"phrase":"hidden gem","severity":"error"}`, html: `<p>hidden</p><p>gem</p>`},
		{name: "empty", entry: `{"phrase":"  ","severity":"error"}`, problem: "phrase"},
		{name: "both", entry: `{"phrase":"gem","regex":"gem","severity":"error"}`, problem: "phrase"},
		{name: "invalid regex", entry: `{"regex":"[","severity":"error"}`, problem: "regex"},
		{name: "unsupported regex", entry: `{"regex":"(gem)\\1","severity":"error"}`, problem: "regex"},
		{name: "missing severity", entry: `{"phrase":"gem"}`, problem: "severity"},
		{name: "invalid severity", entry: `{"phrase":"gem","severity":"warn"}`, problem: "severity"},
		{name: "unknown field", entry: `{"phrase":"gem","severity":"error","typo":true}`, problem: "typo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			var entry any
			if err := json.Unmarshal([]byte(tc.entry), &entry); err != nil {
				t.Fatal(err)
			}
			changeConfig(t, root, func(c map[string]any) {
				c["build"].(map[string]any)["validation"] = map[string]any{"copyBans": []any{entry}}
			})
			p, err := project.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			loaded := project.Load(p, ruleCheck)
			if tc.problem != "" {
				want := "build.validation.copyBans[0]." + tc.problem
				for _, problem := range loaded.Problems {
					if problem.Path == want {
						return
					}
				}
				t.Fatalf("missing %s: %+v", want, loaded.Problems)
			}
			if len(loaded.Problems) != 0 {
				t.Fatal(loaded.Problems)
			}
			html := tc.html
			template := fixtureHTML
			if strings.HasPrefix(html, "<meta") {
				template = strings.Replace(template, "</head>", html+"</head>", 1)
			} else {
				template = strings.Replace(template, "</body>", html+"</body>", 1)
			}
			write(t, root, "index.html.tmpl", template)
			result, compileErr := Compile(context.Background(), root, Options{})
			if tc.severity == check.Error && tc.count > 0 && compileErr == nil {
				t.Fatal("ban did not block compile")
			}
			if (tc.count == 0 || tc.severity != check.Error) && compileErr != nil {
				t.Fatal(compileErr)
			}
			options, err := ProjectCheckOptions(root, check.Options{File: "live.html"})
			if err != nil {
				t.Fatal(err)
			}
			report, err := check.Analyze([]byte(template), options)
			if err != nil {
				t.Fatal(err)
			}
			for _, findings := range [][]diagnostic.Diagnostic{result.Validation.Diagnostics, report.Diagnostics} {
				var bans []diagnostic.Diagnostic
				for _, finding := range findings {
					if finding.Rule == "copy.banned" {
						bans = append(bans, finding)
					}
				}
				if len(bans) != tc.count {
					t.Fatalf("got %+v, want %d bans", bans, tc.count)
				}
				for _, ban := range bans {
					if ban.Severity != tc.severity || ban.Location.Line < 1 || ban.Location.File == "" || ban.Hint == "" {
						t.Fatalf("incomplete ban: %+v", ban)
					}
					if tc.name == "literal inline" && ban.Hint != "Name the place." {
						t.Fatal(ban)
					}
					raw, err := json.Marshal(ban)
					if err != nil || !strings.Contains(string(raw), `"rule":"copy.banned"`) || !strings.Contains(string(raw), `"hint":`) {
						t.Fatalf("JSON finding: %s %v", raw, err)
					}
				}
			}
		})
	}
}
