package compile

import (
	"context"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
)

func TestNamedURLsBuildAndEscaping(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{"checkout": "https://checkout.example/pay?product=one&ref=hero", "phone": "tel:+49401234", "app": "myapp://product/1", "email": "mailto:hello@example.org?subject=Hi%20there"}
	})
	extra := `<a href="{{.urls.checkout}}">Pay</a><a href="{{.urls.phone}}">Call</a><a href="{{.urls.app}}">App</a><a href="{{.urls.email}}">Mail</a>`
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", extra+"</body>", 1))
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	raw := read(t, root, "output/index.html")
	links := map[string]bool{}
	for _, n := range parsePage(t, raw) {
		if n.Data == "a" {
			links[htmlx.Attr(n, "href")] = true
		}
	}
	for _, href := range []string{"tel:+49401234", "https://checkout.example/pay?product=one&ref=hero", "myapp://product/1", "mailto:hello@example.org?subject=Hi%20there"} {
		if !links[href] {
			t.Fatal("URL altered or blocked", href)
		}
	}
	found := false
	for _, d := range report.Validation.Diagnostics {
		if d.Rule == "links.syntax" && strings.Contains(d.Message, "custom/application") {
			found = true
		}
	}
	if !found {
		t.Fatal("custom scheme needs an advisory")
	}
}
func TestURLFailuresPreserveOutput(t *testing.T) {
	cases := []struct {
		name, rule string
		change     func(*testing.T, string)
	}{
		{"literal even when declared", "links.literal", func(t *testing.T, root string) {
			changeConfig(t, root, func(c map[string]any) { c["urls"] = map[string]any{"checkout": "https://checkout.example/"} })
			write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<a href="https://checkout.example/">Pay</a></body>`, 1))
		}},
		{"dynamic undeclared", "links.declared", func(t *testing.T, root string) {
			changeConfig(t, root, func(c map[string]any) { c["data"].(map[string]any)["checkout"] = "https://checkout.example/" })
			write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<a href="{{.data.checkout}}">Pay</a></body>`, 1))
		}},
		{"protocol typo", "links.syntax", func(t *testing.T, root string) {
			changeConfig(t, root, func(c map[string]any) {
				c["urls"] = map[string]any{"checkout": "htpps://checkout.example/?secret=DO_NOT_PRINT"}
			})
			write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<a href="{{.urls.checkout}}">Pay</a></body>`, 1))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			if _, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
				t.Fatal(err)
			}
			before := read(t, root, "output/index.html")
			tc.change(t, root)
			report, err := Compile(context.Background(), root, Options{Publish: true})
			if err == nil || report.Published {
				t.Fatal(report, err)
			}
			found := false
			for _, d := range report.Validation.Diagnostics {
				if d.Rule == tc.rule {
					found = true
				}
				if strings.Contains(d.String(), "DO_NOT_PRINT") {
					t.Fatal("URL query leaked")
				}
			}
			if !found {
				t.Fatal(tc.rule, report.Validation)
			}
			if read(t, root, "output/index.html") != before {
				t.Fatal("failed URL gate replaced output")
			}
		})
	}
}
func TestURLRuleOverrides(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"links.literal": "warn", "links.declared": "warn"}}
	})
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<a href="https://other.example/">Other</a></body>`, 1))
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil || !report.Published {
		t.Fatal(report, err)
	}
	rules := map[string]bool{}
	for _, d := range report.Validation.Diagnostics {
		rules[d.Rule] = true
	}
	if !rules["links.literal"] || !rules["links.declared"] {
		t.Fatal(report.Validation)
	}
}
func TestMultipleNamedURLErrorsCollected(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{"checkout": "htpps://checkout.example/", "help": "https://example.org:99999/"}
	})
	report, err := Compile(context.Background(), root, Options{})
	if err == nil || report.Validation.Errors != 2 || report.Validation.Warnings != 0 {
		t.Fatal(report.Validation, err)
	}
	for _, d := range report.Validation.Diagnostics {
		if d.Rule != "links.syntax" || !strings.HasPrefix(d.Path, "urls.") || d.Location.File != "lfm.json" || d.Location.Offset == 0 {
			t.Fatal(d)
		}
	}
}

func TestContextNormalizedNamedURLs(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{
			"unicode":     "https://bücher.de/über?arr[]=1&arr[]=2",
			"punctuation": "https://example.org/O'Reilly_(book)",
			"srcsetComma": "https://cdn.example/image,large.webp",
			"signed":      "https://example.org/download?key=A%2Fb%2fC&x=1&x=2&signature=AbC+def",
		}
	})
	extra := `<a href="{{.urls.unicode}}">Unicode</a><a href="{{.urls.punctuation}}">Book</a><a href="{{.urls.signed}}">Signed</a><img srcset="{{.urls.srcsetComma}} 2x" alt="">`
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", extra+"</body>", 1))
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	raw := read(t, root, "output/index.html")
	if !strings.Contains(raw, `key=A%2Fb%2fC&amp;x=1&amp;x=2&amp;signature=AbC+def`) {
		t.Fatal("signed query changed")
	}
	for _, d := range report.Validation.Diagnostics {
		if d.Rule == "links.declared" {
			t.Fatal(d)
		}
	}
}

func TestUnicodeCanonicalAndURLValueTypeLocation(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) { c["url"] = "https://bücher.de/über/" })
	write(t, root, "index.html.tmpl", fixtureHTML)
	report, err := Compile(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	for _, d := range report.Validation.Diagnostics {
		if d.Rule == "seo.metadata" && strings.Contains(d.Message, "Canonical URL differs") {
			t.Fatal("normalization created false target mismatch", d)
		}
	}
	changeConfig(t, root, func(c map[string]any) { c["urls"] = map[string]any{"bad": 42} })
	report, err = Compile(context.Background(), root, Options{})
	if err == nil {
		t.Fatal("non-string URL accepted")
	}
	d := report.Validation.Diagnostics[0]
	source := read(t, root, "lfm.json")
	if d.Rule != "config.invalid" || d.Location.Offset <= strings.Index(source, `"bad"`) {
		t.Fatal("type error location lost", d)
	}
}

func TestLiteralURLReportsSyntaxAndExistingOwner(t *testing.T) {
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) { c["urls"] = map[string]any{"checkout": "https://checkout.example/"} })
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<a href="https://checkout.example/">Pay</a><a href="htpps://other.example/">Typo</a></body>`, 1))
	report, err := Compile(context.Background(), root, Options{})
	if err == nil || report.Validation.Errors != 3 {
		t.Fatal(report.Validation, err)
	}
	syntax, owner := false, false
	for _, d := range report.Validation.Diagnostics {
		if d.Rule == "links.syntax" {
			syntax = true
		}
		if d.Rule == "links.literal" && strings.Contains(d.Hint, "{{.urls.checkout}}") && len(d.Related) == 1 {
			owner = true
		}
	}
	if !syntax || !owner {
		t.Fatal(report.Validation)
	}
}
