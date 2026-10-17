package compile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
)

func TestValidationGateAndExactBytes(t *testing.T) {
	root := fixture(t)
	report, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	if report.Validation == nil || report.Validation.Stats.HTMLBytes != len(before) || !report.Published {
		t.Fatal("report is not for exact published bytes")
	}
	for _, tc := range []struct{ name, fragment, rule, file string }{
		{"content model", `<ul><div>Invalid child</div></ul>`, "html.content-model", "index.html"},
		{"missing end tag", `<div><span>Missing span close</div>`, "html.syntax", "rendered-template.html"},
		{"duplicate attribute", `<p id=one id=two>Duplicate</p>`, "html.syntax", "rendered-template.html"},
		{"missing reference", `<label for=missing>Name</label>`, "html.references", "index.html"},
		{"invalid JSON", `<script type=application/ld+json>{"x":1,}</script>`, "html.json", "index.html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", tc.fragment+"</body>", 1))
			report, err := Run(root, true)
			var failure *check.Failure
			if !errors.As(err, &failure) || report.Published || report.Validation == nil {
				t.Fatalf("%+v %v", report, err)
			}
			found := false
			for _, issue := range report.Validation.Diagnostics {
				if issue.Rule == tc.rule && issue.Location.File == tc.file && issue.Severity == check.Error {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s: %+v", tc.rule, report.Validation)
			}
			if read(t, root, "output/index.html") != before {
				t.Fatal("failed validation replaced the last good build")
			}
			entries, _ := os.ReadDir(filepath.Join(root, ".micro"))
			if len(entries) != 0 {
				t.Fatalf("scratch leaked: %v", entries)
			}
		})
	}
}

func TestOversizedArtifactGivesSourceAndFileRemedy(t *testing.T) {
	root := fixture(t)
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	// Media bytes are intentionally opaque here: this test concerns embedding and
	// final HTML accounting, not image codec validity.
	write(t, root, "assets/images/large.png", strings.Repeat("a", 1_550_000))
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<img src="assets/images/large.png" width=1 height=1 alt=""></body>`, 1))
	report, err := Run(root, true)
	if err == nil || report.Validation.Stats.HTMLBytes <= check.HTMLLimit {
		t.Fatalf("%+v %v", report, err)
	}
	found := false
	for _, issue := range report.Validation.Diagnostics {
		if issue.Rule == "perf.html-size" && issue.Severity == check.Error && strings.Contains(issue.Hint, `build.assets.overrides["assets/images/large.png"]`) && strings.Contains(issue.Hint, `"file"`) {
			found = true
		}
		if len(issue.Hint) > 10_000 {
			t.Fatal("huge payload leaked into diagnostics")
		}
	}
	if !found {
		t.Fatal("missing actionable asset remedy")
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("size failure replaced output")
	}
	changeConfig(t, root, func(config map[string]any) {
		config["build"].(map[string]any)["assets"].(map[string]any)["overrides"] = map[string]any{"assets/images/large.png": "file"}
	})
	report, err = Run(root, true)
	if err != nil || report.Validation.Stats.HTMLBytes > check.HTMLWarning || !report.Published {
		t.Fatalf("file remedy failed: %+v %v", report, err)
	}
	if len(read(t, root, "output/large.png")) != 1_550_000 {
		t.Fatal("external asset lost bytes")
	}
}

func TestRuleConfiguration(t *testing.T) {
	root := fixture(t)
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<p id=duplicate>One</p><p id=duplicate>Two</p></body>`, 1))
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"html.ids": "warn"}}
	})
	if report, err := Run(root, true); err != nil || !report.Published {
		t.Fatalf("warn override: %+v %v", report, err)
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"html.ids": "off"}}
	})
	report, err := Run(root, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range report.Validation.Diagnostics {
		if issue.Rule == "html.ids" {
			t.Fatal("disabled rule ran")
		}
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"typo": "off"}}
	})
	if _, err := Run(root, false); err == nil || !strings.Contains(err.Error(), "unknown rule") {
		t.Fatalf("%v", err)
	}
}

func TestIdenticalEmbeddedSourcesStayDeterministic(t *testing.T) {
	root := fixture(t)
	write(t, root, "assets/images/copy.svg", read(t, root, "assets/images/picture.svg"))
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<img src="assets/images/copy.svg" alt=""></body>`, 1))
	report, err := Run(root, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, embed := range report.Validation.Stats.Embeds {
		if len(embed.Sources) == 2 {
			if !reflect.DeepEqual(embed.Sources, []string{"assets/images/copy.svg", "assets/images/picture.svg"}) {
				t.Fatal(embed)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("lost origins for identical encoded resources")
	}
}

func TestOperationalNoticesObeyConfigAndPreserveOutput(t *testing.T) {
	root := fixture(t)
	write(t, root, "unused.svg", `<svg xmlns="http://www.w3.org/2000/svg"/>`)
	write(t, root, "index.html.tmpl", helperHTML("", `<link rel="preload" as="image" href="unused.svg">`))
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	original := read(t, root, "output/index.html")
	for _, level := range []string{"error", "off"} {
		changeConfig(t, root, func(c map[string]any) {
			c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"assets.hint": level}}
		})
		r, err := Run(root, true)
		if level == "error" {
			if err == nil || r.Published || read(t, root, "output/index.html") != original {
				t.Fatal("promoted lifecycle notice bypassed gate", err, r.Validation)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range r.Validation.Diagnostics {
				if d.Rule == "assets.hint" {
					t.Fatal("disabled notice was still reported")
				}
			}
		}
	}
}

func TestReleaseNoticesObeyConfigBeforePublishing(t *testing.T) {
	root := fixture(t)
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	original := read(t, root, "output/index.html")
	t.Setenv("LFM_CHROME", "/missing/chrome")
	t.Setenv("LFM_LIGHTHOUSE", "/missing/lighthouse")
	for _, level := range []string{"off", "error"} {
		changeConfig(t, root, func(c map[string]any) {
			c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"release.browser": level, "release.lighthouse": "off"}}
		})
		r, err := RunWithOptions(root, RunOptions{Publish: true, Release: true})
		if level == "off" {
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range r.Validation.Diagnostics {
				if d.Rule == "release.browser" || d.Rule == "release.lighthouse" {
					t.Fatal(d)
				}
			}
		} else if err == nil || r.Published || read(t, root, "output/index.html") != original {
			t.Fatal("promoted release notice published", err, r.Validation)
		}
	}
}

func TestSEOFinalGateAggregatesAndPreservesOutput(t *testing.T) {
	root := fixture(t)
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{"badSocial": "mailto:preview@example.com", "closed": "#closed"}
	})
	page := strings.Replace(fixtureHTML, "</head>", `<meta property="og:url" content="{{.urls.badSocial}}"><meta name="robots" content="nosnippet"></head>`, 1)
	page = strings.Replace(page, "</body>", `<h3>Skipped level</h3><a href="{{.urls.closed}}">Answer</a><section id="closed" hidden>Hidden answer</section></body>`, 1)
	write(t, root, "index.html.tmpl", page)
	r, err := Run(root, true)
	var failure *check.Failure
	if !errors.As(err, &failure) || r.Published || r.Validation == nil {
		t.Fatalf("%+v %v", r, err)
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("SEO rejection replaced the previous build")
	}
	for rule, level := range map[string]check.Severity{"seo.social": check.Error, "seo.snippets": check.Warning, "seo.headings": check.Warning, "links.fragments": check.Warning} {
		found := false
		for _, issue := range r.Validation.Diagnostics {
			if issue.Rule == rule && issue.Severity == level {
				found = true
				if issue.Location.File != "index.html" || issue.Hint == "" {
					t.Fatal(issue)
				}
			}
		}
		if !found {
			t.Fatal("missing aggregate finding", rule, r.Validation)
		}
	}
	// Existing severity controls cover new rules and report every remaining
	// finding before publication. They add no CLI/config mechanism.
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"seo.social": "off", "seo.snippets": "error", "links.fragments": "off"}}
	})
	r, err = Run(root, true)
	if err == nil || r.Published || read(t, root, "output/index.html") != before {
		t.Fatal("promoted SEO warning bypassed publication gate")
	}
	for _, issue := range r.Validation.Diagnostics {
		if issue.Rule == "seo.social" || issue.Rule == "links.fragments" {
			t.Fatal("disabled SEO rule still ran", issue)
		}
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"].(map[string]any)["rules"].(map[string]any)["seo.snippets"] = "warn"
	})
	if r, err = Run(root, true); err != nil || !r.Published {
		t.Fatalf("deliberate override failed: %+v %v", r, err)
	}
}

func TestAccessibilityAndInteractionFinalGate(t *testing.T) {
	root := fixture(t)
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	fragment := `<label for=bad-label>Amount</label><div id=bad-label>Container</div>
<details><summary>One</summary><summary>Two</summary></details>
<div aria-hidden=true><button type=button>Still tabbable</button></div>
<div role=slider tabindex=0 aria-label=Volume aria-valuenow=NaN></div>
<form><button>Local step</button></form>`
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", fragment+"</body>", 1))
	r, err := Run(root, true)
	if err == nil || r.Published || r.Validation == nil || read(t, root, "output/index.html") != before {
		t.Fatal("accessibility rejection replaced output", err)
	}
	for rule, level := range map[string]check.Severity{"a11y.relationships": check.Error, "ux.disclosures": check.Error, "a11y.focus": check.Warning, "a11y.aria": check.Warning, "ux.forms": check.Warning} {
		found := false
		for _, issue := range r.Validation.Diagnostics {
			if issue.Rule == rule && issue.Severity == level {
				found = true
				if issue.Location.File != "index.html" || issue.Hint == "" {
					t.Fatal(issue)
				}
			}
		}
		if !found {
			t.Fatal("missing aggregate rule", rule, r.Validation)
		}
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"a11y.relationships": "off", "ux.disclosures": "off", "a11y.focus": "error"}}
	})
	if r, err = Run(root, true); err == nil || r.Published || read(t, root, "output/index.html") != before {
		t.Fatal("promoted focus warning bypassed gate")
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"].(map[string]any)["rules"].(map[string]any)["a11y.focus"] = "warn"
	})
	if r, err = Run(root, true); err != nil || !r.Published {
		t.Fatal("intentional overrides failed", err)
	}
	for _, issue := range r.Validation.Diagnostics {
		if issue.Rule == "a11y.relationships" || issue.Rule == "ux.disclosures" {
			t.Fatal("disabled rule emitted", issue)
		}
	}
}
