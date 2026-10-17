package check

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/urlcheck"
)

func TestFinalURLSyntax(t *testing.T) {
	for _, input := range []string{
		`<a href="https://example.org:99999/">go</a>`,
		`<a href="htpps://example.org/">go</a>`,
		`<a href="https:/example.org/">go</a>`,
		`<form action="https//example.org/submit"></form>`,
		`<q cite="https://example..org/source">Q</q>`,
		`<img srcset="data:image/png;base64,aGVsbG8= 1x, https://example.org:99999/image 2x">`,
		`<script type=application/ld+json>{"url":"https://example.org:99999/"}</script>`,
	} {
		if len(checkRule(t, "links.syntax", input)) == 0 {
			t.Fatal("missed URL problem", input)
		}
	}
	for _, input := range []string{
		`<a href="https://bücher.de/über?x=1&amp;y=2">go</a>`,
		`<a href="mailto:a@example.org?subject=Hi%20there">mail</a><a href="tel:+4940123">call</a>`,
		`<script>const x="https://bad.example:99999";</script>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><use href="#icon"/></svg>`,
		`<script type=application/ld+json>{"@context":{"term":"https://schema.org/term"},"@type":"https://schema.org/Product"}</script>`,
	} {
		if issues := checkRule(t, "links.syntax", input); len(issues) > 0 {
			t.Fatal(input, issues)
		}
	}
}
func TestDeclaredURLsAndGeneratedAssets(t *testing.T) {
	registry := urlcheck.NewRegistry("https://example.org/product/", []urlcheck.Declaration{
		{Path: "url", Value: "https://example.org/product/"},
		{Path: "urls.help", Value: "../help/?a=1&b=2"},
		{Path: "urls.section", Value: "#section"},
	})
	raw := `<a href="https://example.org/help/?a=1&amp;b=2">help</a><a href="#section">section</a><a href="other/">unknown</a><img src="./asset.webp"><meta property="og:image" content="https://example.org/product/asset.webp"><svg><use href="#icon"/></svg>`
	doc, err := Parse([]byte(raw), Options{URL: "https://example.org/product/", Registry: registry, GeneratedAssets: map[string]bool{"asset.webp": true}})
	if err != nil {
		t.Fatal(err)
	}
	report := Run(doc, []Rule{testRule("links.declared", linksDeclaredRule)}, nil)
	if report.Errors != 1 || report.Diagnostics[0].Element != "a" {
		t.Fatal(report)
	}
	if !strings.HasPrefix(raw[report.Diagnostics[0].Location.Offset:], "other/") {
		t.Fatal(report.Diagnostics[0])
	}
}
func TestURLBehavior(t *testing.T) {
	cases := []struct {
		raw   string
		count int
	}{
		{`<a onclick="go()">go</a>`, 1},
		{`<span role="link" onclick="go()">go</span>`, 1},
		{`<button onclick="go()">Do</button>`, 0},
		{`<a id="anchor">named target</a>`, 0},
		{`<a href="tel:+49123">Call</a>`, 0},
		{`<img src="http://cdn.example/image">`, 1},
		{`<a href="http://other.example/">normal outbound</a>`, 0},
		{`<a href="https://other.example/file.pdf" download>file</a>`, 1},
		{`<a href="./file.pdf" download>file</a>`, 0},
		{`<a href="https://example.org/product/#missing">same page</a>`, 1},
		{`<p id="section"></p><a href="./#section">same page</a>`, 0},
		{`<a href="https://other.example/#missing">different document</a>`, 0},
		{`<a href="#">placeholder</a>`, 1},
	}
	for _, tc := range cases {
		doc, err := Parse([]byte(tc.raw), Options{URL: "https://example.org/product/"})
		if err != nil {
			t.Fatal(err)
		}
		issues := linksBehaviorRule(doc)
		if len(issues) != tc.count {
			t.Fatal(tc.raw, issues)
		}
	}
}
func TestPublicURLUsesBrowserSyntax(t *testing.T) {
	for _, raw := range []string{"https://example.org:99999/", "https:/example.org/", "https://example.org/#part", "https://name:secret@example.org/"} {
		if publicURL(raw) {
			t.Fatal("bad canonical/social URL accepted", raw)
		}
	}
	if !publicURL("https://bücher.de/über") {
		t.Fatal("Unicode IRI rejected")
	}
}
