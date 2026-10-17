package urlcheck

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

func TestURLSyntaxAndRepairs(t *testing.T) {
	tests := []struct {
		raw     string
		valid   bool
		message string
	}{
		{"https://example.org/path?a=1&b=2#part", true, ""},
		{"/other/", true, ""}, {"../other/", true, ""}, {"?x=1", true, ""}, {"#part", true, ""},
		{"https://bücher.de/über", true, ""}, {"https://[2001:db8::1]:443/", true, ""},
		{"HTTPS://EXAMPLE.ORG:443/a", true, ""},
		{"https:/example.org", false, "scheme://host"}, {"https:example.org", false, "scheme://host"}, {"https:///example.org", false, "extra slashes"},
		{"htpps://example.org", false, "misspelling"},
		{"https//example.org", false, "separator is malformed"},
		{"https://https://example.org", false, "repeats a protocol"},
		{"example.com", true, "hostname"},
		{"https://example..com", true, "DNS-label"}, {"htps://example.org", false, "misspelling"},
		{"https://example.org:70000/", false, "port"}, {"https://example.org:abc/", false, "port"},
		{"https://[::bad/", false, "IPv6"}, {"https://exa mple.org/", false, "host"},
		{"https://example.org/%ZZ", true, "repairs/reports"}, {"https://example.org/a b", true, "repairs/reports"},
		{"https:\\example.org", false, "scheme://host"}, {"https://example.org\\path", true, "repairs/reports"},
		{" https://example.org/ ", true, "surrounding whitespace"}, {"https://exa\tmple.org/", false, "control characters"},
		{"https://user:TOPSECRET@example.org/?key=SECRET", true, "user information"},
		{"//example.org/path", true, "Protocol-relative"}, {"www.example.org/page", true, "hostname"},
		{"brochure.pdf", true, ""}, {"./example.com/page", true, ""},
		{"mailto:hello@example.org?subject=Hello%20World&body=Thanks", true, ""},
		{"mailto://hello@example.org", true, "uses //"}, {"mailto:not-an-address", true, "plain email"}, {"mailto:", true, "no recipient"},
		{"tel:+4940123456", true, ""}, {"tel://wrong", true, "recognizable number"},
		{"myapp://item/1", true, "custom/application"}, {"webcal://example.org/calendar.ics", true, ""},
		{"javascript:alert(1)", false, "pseudo-URL"}, {"JaVaScRiPt:alert(1)", false, "pseudo-URL"},
		{"https://example.org/?a=1&amp;b=2", true, "HTML-escaped"},
		{"https://example.org/${id}", false, "placeholder"}, {"https://example.org/{{id}}", false, "placeholder"},
		{"https://localhost/", true, "loopback"}, {"https://example.org/a\u200b", true, "invisible Unicode"},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			result := New("https://base.example/product/").Inspect(tc.raw)
			if result.Valid() != tc.valid {
				t.Fatalf("valid=%v %+v", result.Valid(), result.Findings)
			}
			var text strings.Builder
			for _, d := range result.Findings {
				text.WriteString(d.Message)
				text.WriteString(d.Hint)
			}
			if tc.message != "" && !strings.Contains(text.String(), tc.message) {
				t.Fatalf("missing %s: %s", tc.message, text.String())
			}
			if tc.message == "" && len(result.Findings) > 0 {
				t.Fatalf("unexpected findings: %+v", result.Findings)
			}
			if strings.Contains(text.String(), "SECRET") {
				t.Fatal("URL credential/query value leaked")
			}
		})
	}
}
func TestRegistryMatchingAndDuplicates(t *testing.T) {
	r := NewRegistry("https://example.org/product/", []Declaration{
		{Path: "url", Value: "https://example.org/product/"},
		{Path: "urls.help", Value: "../help/?a=1&b=2"},
		{Path: "urls.phone", Value: "tel:+4940123"},
		{Path: "urls.helpAgain", Value: "https://EXAMPLE.org:443/help/?a=1&b=2"},
	})
	if len(r.Match("https://example.org/help/?a=1&b=2")) != 2 || len(r.Match("tel:+4940123")) != 1 || len(r.Match("../help/?b=2&a=1")) != 0 {
		t.Fatal("URL normalization changed query semantics")
	}
	issues := r.Check()
	if len(issues) != 1 || issues[0].Rule != "links.registry" || len(issues[0].Related) != 1 {
		t.Fatal(issues)
	}
	if len(Apply(issues, map[string]diagnostic.Severity{"links.registry": diagnostic.Off})) != 0 {
		t.Fatal("override failed")
	}
	raw, err := json.Marshal(r.Check())
	if err != nil || strings.Contains(string(raw), "tel:+") {
		t.Fatal(string(raw), err)
	}
}
func TestSchemeAndTrustedValues(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "vbscript:hello", "https://ex\tample.org"} {
		if Trustable(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"tel:+49123", "myapp://a", "#section", "mailto:hello@example.org"} {
		if !Trustable(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"1wrong:a", "./file:a", "/a:b", "a/b:c"} {
		if Scheme(raw) != "" {
			t.Fatal(raw, Scheme(raw))
		}
	}
}
func FuzzURLPolicy(f *testing.F) {
	for _, raw := range []string{"https://example.org/a?x=1&y=2", "https://[::1]:99999/", "mailto:a@example.org", "#a", "https:\\host", "data:image/svg+xml,<svg/>"} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 8192 {
			t.Skip()
		}
		result := New("https://example.org/base/").Inspect(raw)
		for _, finding := range result.Findings {
			if finding.Message == "" || finding.Hint == "" {
				t.Fatal("empty diagnostic")
			}
		}
	})
}

func TestRegistryChecksContextualEscapingChanges(t *testing.T) {
	for _, raw := range []string{" https://example.org/ ", "https://example.org\\path", " /next/ "} {
		registry := NewRegistry("https://base.example/product/", []Declaration{{Path: "urls.bad", Value: raw}})
		found := false
		for _, d := range registry.Check() {
			if d.Rule == "links.syntax" && d.Severity == diagnostic.Error && strings.Contains(d.Message, "Contextual URL escaping") {
				found = true
			}
		}
		if !found {
			t.Fatal("render-time destination drift unreported", raw, registry.Check())
		}
	}
}

func FuzzRenderedURLMatching(f *testing.F) {
	for _, seed := range []string{"https://bücher.de/über?x=1&x=2", "https://example.org/O'Reilly_(book)", "tel:+4940123", "myapp://go/1", "https://cdn.example/image,large.webp", "#section"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			t.Skip()
		}
		registry := NewRegistry("https://example.org/product/", []Declaration{{Path: "urls.test", Value: raw}})
		for _, issue := range registry.Check() {
			if issue.Severity == diagnostic.Error {
				t.Skip()
			}
		}
		for _, form := range renderedForms(raw) {
			if !Embedded(form) && len(registry.Match(form)) == 0 {
				t.Fatal("Go-rendered URL no longer matches declared owner")
			}
		}
	})
}
