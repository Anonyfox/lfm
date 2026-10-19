package compile

import (
	"context"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

func TestMappingGateCollectsHardErrorsAndPreservesOutput(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	if _, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["browserData"] = []string{"data.missingOne", "data.missingTwo"}
	})
	write(t, root, "index.html.tmpl", `<img src="assets/images/does-not-exist.svg">{{.data.firstMissing}}{{.data.secondMissing}}{{.data.unpublished}}`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err == nil || report.Published || report.Validation == nil || report.Validation.Stage != "template" || report.Validation.Errors != 4 || report.Validation.Warnings != 0 || report.Validation.Stats.HTMLBytes != 0 {
		t.Fatalf("%+v %v", report, err)
	}
	for _, d := range report.Validation.Diagnostics {
		if d.Severity != diagnostic.Error || strings.Contains(d.String(), "PRIVATE_CONFIG_MARKER") || strings.HasPrefix(d.Rule, "html.") || strings.Contains(d.Message, "does-not-exist") {
			t.Fatalf("%+v", d)
		}
	}
	if read(t, root, "output/index.html") != before {
		t.Fatal("rejected template changed output")
	}
	assertClean(t, root)
}
func TestMappingWarningsMergeWithHTMLGate(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) { c["data"].(map[string]any)["blank"] = "" })
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<p>{{.data.blank}}</p><img src="assets/images/picture.svg"></body>`, 1))
	report, err := Compile(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, d := range report.Validation.Diagnostics {
		found[d.Rule] = true
	}
	for _, rule := range []string{"template.empty", "template.unused", "a11y.images"} {
		if !found[rule] {
			t.Fatalf("missing %s: %+v", rule, report.Validation)
		}
	}
	if report.Validation.Stage != "html" || report.Validation.Stats.HTMLBytes == 0 {
		t.Fatal(report.Validation)
	}
}
func TestMappingPoliciesAndRuntimeOnlyConsumption(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"template.unused": "off", "template.inactive": "off"}}
	})
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `{{if false}}{{.data.dormantMissing}}{{end}}</body>`, 1))
	report, err := Compile(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range report.Validation.Diagnostics {
		if strings.HasPrefix(d.Rule, "template.") {
			t.Fatal(d)
		}
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"template.bindings": "off"}}
	})
	report, err = Compile(context.Background(), root, Options{})
	if err == nil || report.Validation.Errors != 1 || !strings.Contains(report.Validation.Diagnostics[0].Message, "is required and must stay error") {
		t.Fatal(report, err)
	}
}
func TestEveryTemplateSyntaxFailureReported(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	write(t, root, "index.html.tmpl", `{{if}}`)
	write(t, root, "partials/bad.html.tmpl", `{{range .data}}`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err == nil || report.Validation.Errors != 2 || report.Validation.Warnings != 0 {
		t.Fatal(report, err)
	}
	for _, d := range report.Validation.Diagnostics {
		if d.Rule != "template.syntax" {
			t.Fatal(d)
		}
	}
}
func TestDuplicateJSONRejectedBeforeBinding(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	write(t, root, "lfm.json", strings.Replace(read(t, root, "lfm.json"), `"enabled":true`, `"enabled":true,"enabled":false`, 1))
	write(t, root, "data/faq.json", `[{"id":"one","id":"two","question":"Q","answer":"A"}]`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err == nil || report.Validation.Errors != 2 || report.Validation.Warnings != 0 {
		t.Fatal(report, err)
	}
	for _, d := range report.Validation.Diagnostics {
		if d.Rule != "config.duplicate-key" || len(d.Related) != 1 {
			t.Fatal(d)
		}
	}
}
func TestContextualEscapingFailureIsUniform(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	write(t, root, "index.html.tmpl", `{{if .data.enabled}}<a href="{{end}}{{.title}}`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err == nil || report.Validation.Errors != 1 || report.Validation.Diagnostics[0].Rule != "template.render" || report.Validation.Warnings != 0 {
		t.Fatal(report, err)
	}
}

func TestConfigProblemsCollectedWithLocations(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["schemaVersion"] = 2
		c["url"] = "relative"
		c["title"] = ""
		c["language"] = ""
		c["build"].(map[string]any)["assets"].(map[string]any)["default"] = "wrong"
	})
	write(t, root, "data/bad1.json", `{"a":`)
	write(t, root, "data/bad2.json", `[1,]`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err == nil || report.Validation.Errors != 7 || report.Validation.Warnings != 0 {
		t.Fatalf("%+v %v", report.Validation, err)
	}
	for _, d := range report.Validation.Diagnostics {
		if d.Rule != "config.invalid" {
			t.Fatal(d)
		}
		if d.Path != "" && d.Location.Offset == 0 {
			t.Fatal("setting location lost", d)
		}
	}
}
func TestReservedConfigCaseAndUnknowns(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["Title"] = "duplicate through Go case folding"
		c["other"] = true
		c["build"].(map[string]any)["wrong"] = 1
	})
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err == nil || report.Validation.Errors != 3 || report.Validation.Warnings != 0 {
		t.Fatalf("%+v %v", report.Validation, err)
	}
	found := false
	for _, d := range report.Validation.Diagnostics {
		if strings.Contains(d.Hint, "Use the exact key title") {
			found = true
		}
		if d.Location.Offset == 0 {
			t.Fatal(d)
		}
	}
	if !found {
		t.Fatal("missing exact-key hint")
	}
}
