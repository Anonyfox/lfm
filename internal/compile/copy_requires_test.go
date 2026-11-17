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

func TestProjectCopyRequires(t *testing.T) {
	for _, tc := range []struct {
		name, entry, html, problem string
		count                      int
		severity                   diagnostic.Severity
	}{
		{name: "missing beacon", entry: `{"snippet":"static.cloudflareinsights.com/beacon.min.js","scope":"html","before":"</body>","exactly":1,"severity":"error","hint":"Add the beacon."}`, count: 1, severity: check.Error},
		{name: "two beacons", entry: `{"snippet":"beacon.min.js","scope":"html","exactly":1,"severity":"error"}`, html: `<script>window.beacon = "https://static.cloudflareinsights.com/beacon.min.js";</script><script>window.beacon = "https://static.cloudflareinsights.com/beacon.min.js";</script>`, count: 1, severity: check.Error},
		{name: "one beacon", entry: `{"snippet":"beacon.min.js","scope":"html","before":"</body>","exactly":1,"severity":"error"}`, html: `<script>window.beacon = "https://static.cloudflareinsights.com/beacon.min.js";</script>`},
		{name: "copy present", entry: `{"phrase":"Privacy policy","severity":"error"}`, html: "<p>PRIVACY <em>policy</em></p>"},
		{name: "copy absent", entry: `{"phrase":"Privacy policy","min":1,"severity":"error"}`, count: 1, severity: check.Error},
		{name: "warning", entry: `{"phrase":"Privacy policy","severity":"warning"}`, count: 1, severity: check.Warning},
		{name: "selector present", entry: `{"phrase":"Privacy policy","selector":".legal","severity":"error"}`, html: `<p class="legal">Privacy policy</p>`},
		{name: "selector outside", entry: `{"phrase":"Privacy policy","selector":".legal","severity":"error"}`, html: `<p>Privacy policy</p>`, count: 1, severity: check.Error},
		{name: "regex present", entry: `{"regex":"privacy\\s+policy","severity":"error"}`, html: "<p>Privacy policy</p>"},
		{name: "both", entry: `{"phrase":"x","regex":"x","severity":"error"}`, problem: "phrase"},
		{name: "negative", entry: `{"phrase":"x","min":-1,"severity":"error"}`, problem: "min"},
		{name: "negative max", entry: `{"phrase":"x","max":-1,"severity":"error"}`, problem: "max"},
		{name: "negative exactly", entry: `{"phrase":"x","exactly":-1,"severity":"error"}`, problem: "exactly"},
		{name: "mixed min", entry: `{"phrase":"x","exactly":1,"min":1,"severity":"error"}`, problem: "exactly"},
		{name: "mixed max", entry: `{"phrase":"x","exactly":1,"max":1,"severity":"error"}`, problem: "exactly"},
		{name: "reversed", entry: `{"phrase":"x","min":2,"max":1,"severity":"error"}`, problem: "max"},
		{name: "before copy", entry: `{"phrase":"x","before":"</body>","severity":"error"}`, problem: "before"},
		{name: "selector html", entry: `{"phrase":"x","scope":"html","selector":".legal","severity":"error"}`, problem: "selector"},
		{name: "snippet copy", entry: `{"snippet":"x","severity":"error"}`, problem: "snippet"},
		{name: "invalid regex", entry: `{"regex":"[","severity":"error"}`, problem: "regex"},
		{name: "invalid selector", entry: `{"phrase":"x","selector":"[","severity":"error"}`, problem: "selector"},
		{name: "missing severity", entry: `{"phrase":"x"}`, problem: "severity"},
		{name: "unknown field", entry: `{"phrase":"x","pages":["*"],"severity":"error"}`, problem: "pages"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			var entry any
			if err := json.Unmarshal([]byte(tc.entry), &entry); err != nil {
				t.Fatal(err)
			}
			changeConfig(t, root, func(c map[string]any) {
				c["build"].(map[string]any)["validation"] = map[string]any{"copyRequires": []any{entry}}
			})
			p, err := project.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			loaded := project.Load(p, ruleCheck)
			if tc.problem != "" {
				want := "build.validation.copyRequires[0]." + tc.problem
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
				t.Fatal("requirement did not block compile")
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
				var requirements []diagnostic.Diagnostic
				for _, finding := range findings {
					if finding.Rule == "copy.required" {
						requirements = append(requirements, finding)
					}
				}
				if len(requirements) != tc.count {
					t.Fatalf("got %+v, want %d requirements", requirements, tc.count)
				}
				for _, requirement := range requirements {
					if requirement.Severity != tc.severity || requirement.Location.Line < 1 || requirement.Location.File == "" || requirement.Hint == "" {
						t.Fatalf("incomplete requirement: %+v", requirement)
					}
					if tc.name == "missing beacon" && requirement.Hint != "Add the beacon." {
						t.Fatal(requirement)
					}
					raw, err := json.Marshal(requirement)
					if err != nil || !strings.Contains(string(raw), `"rule":"copy.required"`) || !strings.Contains(string(raw), `"hint":`) {
						t.Fatalf("JSON finding: %s %v", raw, err)
					}
				}
			}
		})
	}
}
