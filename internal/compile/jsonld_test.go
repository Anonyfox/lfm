package compile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
)

func TestJSONLDCompilation(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(config map[string]any) {
		config["jsonld"] = map[string]any{"@context": "https://schema.org", "@type": "WebPage", "url": "{{.url}}", "name": "{{.title}}", "description": "{{.description}}", "inLanguage": "{{.language}}", "identifier": "{{.data.quiz.prompt}}"}
	})
	report, err := Run(root, true)
	if err != nil {
		t.Fatalf("%v: %+v", err, report.Validation)
	}
	page := read(t, root, "output/index.html")
	count := 0
	var jsonldPosition, executablePosition int
	for i, node := range parsePage(t, page) {
		if attr(node, "id") == "intruder" {
			t.Fatal("JSON-LD broke out of script")
		}
		if node.Data == "script" && attr(node, "type") == "application/ld+json" {
			count++
			jsonldPosition = i
			var data map[string]any
			if err := json.Unmarshal([]byte(textContent(node)), &data); err != nil {
				t.Fatal(err)
			}
			if data["name"] != "A < B & café 🐺" || data["url"] != "https://example.com/tools/demo/" || data["identifier"] != "</script><script id=intruder>bad()</script>" {
				t.Fatal(data)
			}
			if node.Parent.Data != "body" {
				t.Fatal("wrong JSON-LD placement")
			}
		}
		if node.Data == "script" && attr(node, "type") == "module" {
			executablePosition = i
		}
	}
	if count != 1 || jsonldPosition > executablePosition {
		t.Fatalf("blocks/order: %d %d %d", count, jsonldPosition, executablePosition)
	}
	if strings.Contains(page, "{{.title}}") {
		t.Fatal("unresolved JSON-LD template")
	}
	for _, issue := range report.Validation.Diagnostics {
		if issue.Rule == "template.unused" && issue.Path == "jsonld" {
			t.Fatal(issue)
		}
	}
}

func TestJSONLDFailuresPreserveOutput(t *testing.T) {
	for _, tc := range []struct {
		name, template, rule string
		jsonld               any
	}{
		{name: "bindings", rule: "jsonld.bindings", jsonld: map[string]any{"name": "{{.data.missing}}", "description": "{{.otherMissing}}"}},
		{name: "scalar", rule: "jsonld.bindings", jsonld: true},
		{name: "structure", rule: "jsonld.structure", jsonld: map[string]any{"@context": "https://schema.org", "@type": 42}},
		{name: "ownership", rule: "jsonld.block", template: `<script type="application/ld+json">{"@context":"https://schema.org","name":"Handwritten"}</script>`, jsonld: map[string]any{"@context": "https://schema.org", "@type": "WebPage", "name": "{{.title}}"}},
		{name: "undeclared-url", rule: "links.declared", jsonld: map[string]any{"@context": "https://schema.org", "url": "https://unregistered.example/"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			if _, err := Run(root, true); err != nil {
				t.Fatal(err)
			}
			before := read(t, root, "output/index.html")
			changeConfig(t, root, func(config map[string]any) { config["jsonld"] = tc.jsonld })
			if tc.template != "" {
				write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", tc.template+"</body>", 1))
			}
			report, err := Run(root, true)
			if err == nil || report.Validation == nil || report.Published {
				t.Fatalf("%+v %v", report, err)
			}
			found := false
			for _, issue := range report.Validation.Diagnostics {
				if issue.Rule == tc.rule && issue.Severity == check.Error {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s: %+v", tc.rule, report.Validation)
			}
			if tc.name == "bindings" && (report.Validation.Errors != 2 || report.Validation.Warnings != 0 || report.Validation.Stage != "template") {
				t.Fatal(report.Validation)
			}
			if read(t, root, "output/index.html") != before {
				t.Fatal("failed JSON-LD replaced output")
			}
		})
	}
}

func TestJSONLDUnknownContextAndDisabledBlock(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(config map[string]any) {
		config["jsonld"] = map[string]any{"@context": "https://custom.example/context", "@type": "Custom", "name": "Name"}
	})
	report, err := Run(root, true)
	if err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	found := false
	for _, issue := range report.Validation.Diagnostics {
		if issue.Rule == "jsonld.context" && issue.Severity == check.Warning {
			found = true
		}
	}
	if !found {
		t.Fatal("missing offline context coverage advisory")
	}
	changeConfig(t, root, func(config map[string]any) { config["jsonld"] = nil })
	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, root, "output/index.html"), "application/ld+json") {
		t.Fatal("disabled block still emitted")
	}
}

func TestJSONLDEntityIdentifiersFollowURLPolicy(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(config map[string]any) {
		config["urls"] = map[string]any{"entity": "#entity", "legacyEntity": "http://example.com/entity"}
		config["jsonld"] = map[string]any{"@context": "https://schema.org", "@graph": []any{
			map[string]any{"@id": "{{.urls.entity}}", "@type": "WebPage", "name": "Name", "about": map[string]any{"@id": "{{.urls.legacyEntity}}"}},
			map[string]any{"@id": "{{.urls.legacyEntity}}", "@type": "Thing", "name": "Legacy"},
		}}
	})
	report, err := Run(root, true)
	if err != nil {
		t.Fatalf("%v %+v", err, report.Validation)
	}
	for _, issue := range report.Validation.Diagnostics {
		if issue.Rule == "links.behavior" {
			t.Fatalf("JSON-LD entity ID mistaken for fetched resource/HTML anchor: %+v", issue)
		}
	}
	changeConfig(t, root, func(config map[string]any) {
		config["jsonld"].(map[string]any)["@graph"].([]any)[0].(map[string]any)["@id"] = "#undeclared"
	})
	report, err = Run(root, true)
	if err == nil {
		t.Fatal("undeclared fragment ID bypassed named URL policy")
	}
	found := false
	for _, issue := range report.Validation.Diagnostics {
		if issue.Rule == "links.declared" {
			found = true
		}
	}
	if !found {
		t.Fatal(report.Validation)
	}
}
