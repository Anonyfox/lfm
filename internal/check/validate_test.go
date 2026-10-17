package check

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func checkRule(t *testing.T, id, input string) []Diagnostic {
	t.Helper()
	d, err := Parse([]byte(input), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range DefaultRules() {
		if rule.ID == id {
			return Run(d, []Rule{rule}, nil).Diagnostics
		}
	}
	t.Fatalf("unknown test rule %s", id)
	return nil
}

func TestSelectorsAndLocations(t *testing.T) {
	raw := []byte("<!doctype html>\n<html><head><title>T</title></head><body>\n<ul><li class=x>A</li><li>B</li></ul>\n<img id=hero>\n</body></html>")
	d, err := Parse(raw, Options{})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := d.Query("body > ul li:nth-child(2):not(.x)")
	if err != nil || len(nodes) != 1 || Text(nodes[0]) != "B" {
		t.Fatalf("selector: %v %v", nodes, err)
	}
	if _, err := d.Query("img["); err == nil {
		t.Fatal("invalid selector accepted")
	}
	issues := Run(d, []Rule{{"alt", "", imageAltRule}}, nil).Diagnostics
	if len(issues) != 1 || issues[0].Location.Line != 4 || issues[0].Location.Column != 1 || issues[0].Location.Offset != bytes.Index(raw, []byte("<img")) {
		t.Fatalf("locations: %+v", issues)
	}
	if !bytes.Equal(raw, d.Raw) {
		t.Fatal("analysis changed bytes")
	}
	for _, newline := range []string{"\n", "\r\n", "\r"} {
		lines := lineStarts([]byte("a" + newline + "b"))
		got := location("index.html", lines, 1+len(newline))
		if got.Line != 2 || got.Column != 1 {
			t.Fatalf("newline %q: %+v", newline, got)
		}
	}
}

func TestRegistryAndOverrides(t *testing.T) {
	seen := map[string]bool{}
	for _, rule := range DefaultRules() {
		if rule.ID == "" || rule.Description == "" || rule.Check == nil || seen[rule.ID] {
			t.Fatalf("invalid rule: %+v", rule)
		}
		seen[rule.ID] = true
	}
	for _, config := range []map[string]Severity{{"typo": Off}, {"html.ids": "warning"}, {"html.ids": ""}} {
		if ValidateOptions(config) == nil {
			t.Fatalf("accepted config: %v", config)
		}
	}
	if err := ValidateOptions(map[string]Severity{"html.ids": Off}); err != nil {
		t.Fatal(err)
	}
	d, _ := Parse([]byte("<p id=x></p><p id=x></p>"), Options{})
	rule := Rule{"html.ids", "", idsRule}
	if got := Run(d, []Rule{rule}, map[string]Severity{"html.ids": Off}); len(got.Diagnostics) != 0 {
		t.Fatal(got)
	}
	if got := Run(d, []Rule{rule}, map[string]Severity{"html.ids": Warning}); got.Errors != 0 || got.Warnings != 1 {
		t.Fatal(got)
	}
	first := Run(d, DefaultRules(), nil)
	if next := Run(d, DefaultRules(), nil); !reflect.DeepEqual(first, next) {
		t.Fatal("nondeterministic diagnostics")
	}
	encoded, err := json.Marshal(first)
	if err != nil || !json.Valid(encoded) {
		t.Fatal(err)
	}
}

func TestAnalyzeAndRenderedSource(t *testing.T) {
	valid := `<!doctype html><html><head><meta charset=utf-8><link rel=canonical href="https://example.com/"><title>T</title></head><body><h1>T</h1></body></html>`
	before := []byte(valid)
	report, err := Analyze(before, Options{})
	if err != nil || report.Errors != 0 {
		t.Fatalf("%+v %v", report, err)
	}
	if string(before) != valid {
		t.Fatal("mutated output")
	}
	report, err = Analyze(before, Options{RenderedSource: []byte(`<html><head><img src=x><title>T</title></head><body></body></html>`)})
	if err == nil || report.Errors == 0 {
		t.Fatal("source repair hidden")
	}
	found := false
	for _, issue := range report.Diagnostics {
		if issue.Rule == "html.syntax" && issue.Location.File == "rendered-template.html" {
			found = true
		}
	}
	if !found {
		t.Fatal(report)
	}
}

func FuzzAnalyze(f *testing.F) {
	for _, seed := range []string{"", `<!doctype html><p>hello`, `<svg><text>Hi</text></svg>`, `<template><button>OK</button></template>`, `<script>let a="</script>"</script>`, "\x00\xff",
		`<meta property=og:url content="http://[::1]/"><meta name=robots content="nosnippet,max-snippet:0">`,
		`<main><section data-nosnippet><h1>Answer</h1></section></main><a href="#a:~:text=Answer">Go</a><p hidden id=a>Answer</p>`,
		`<fieldset disabled><legend><input autofocus></legend><label><input><input></label></fieldset><div aria-hidden=true><button tabindex=2>Go</button></div>`,
		`<details><summary>One</summary><summary>Two</summary></details><div role=slider aria-valuenow=NaN aria-valuemin=2 aria-valuemax=1></div>`,
		`<a href="https://buy.stripe.com/test_ABC?buyer/email=private%40example.com">Buy</a>`,
		`<script type="application/ld+json">{"@context":"https://schema.org","@type":"Product","offers":{"@type":"Offer","price":-1,"priceCurrency":"EUR"},"potentialAction":{"@type":"BuyAction","target":"https://buy.stripe.com/ABC"}}</script>`,
		`<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@id":"#offer","@type":"Offer"},{"@id":"#offer","price":49,"priceCurrency":"EUR"}]}</script>`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 20_000 {
			t.Skip()
		}
		raw := []byte(input)
		original := append([]byte(nil), raw...)
		_, _ = Analyze(raw, Options{})
		if !bytes.Equal(raw, original) {
			t.Fatal("mutated input")
		}
	})
}

func BenchmarkAnalyzeSmallFixture(b *testing.B) {
	raw := []byte(`<!doctype html><html lang=en><head><meta charset=utf-8><title>Example</title><meta name=description content=Example><link rel=canonical href="https://example.com/"><meta name=viewport content="width=device-width, initial-scale=1"></head><body><h1>Example</h1><img src="data:image/png;base64,aGVsbG8=" width=10 height=10 alt=""><label>Name<input name=name></label><a href="https://example.com/checkout">Buy</a></body></html>`)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		_, _ = Analyze(raw, Options{})
	}
}

func TestEmbedAccounting(t *testing.T) {
	uri := "data:image/png;base64," + strings.Repeat("a", 40000)
	raw := []byte(`<style>.x{background:url("` + uri + `")}</style><img src="` + uri + `"><script>const a="data:font/woff2;base64,YWJj";</script>`)
	report, _ := Analyze(raw, Options{InlineSources: map[[32]byte][]string{sha256.Sum256([]byte(uri)): {"assets/images/hero.png"}}})
	embeds := report.Stats.Embeds
	if len(embeds) != 2 || embeds[0].Count != 2 || embeds[0].Bytes != len(uri) || embeds[0].TotalBytes != 2*len(uri) || !reflect.DeepEqual(embeds[0].Sources, []string{"assets/images/hero.png"}) {
		t.Fatalf("%+v", embeds)
	}
	if strings.Contains(causeHint(&Document{Stats: report.Stats}), uri) {
		t.Fatal("data URL leaked into diagnostics")
	}
	if !strings.Contains(causeHint(&Document{Stats: report.Stats}), `build.assets.overrides["assets/images/hero.png"]`) {
		t.Fatal("missing actionable override")
	}
}

func TestTextualEmbedAccountingExcludesURLFragment(t *testing.T) {
	uri := "data:image/svg+xml,%3Csvg%20id=%22paint%22%3E%23%25%26%3C/svg%3E"
	raw := []byte(`<img src="` + uri + `#crop"><style>.x{background:url(` + uri + `#paint)}</style><script>const x="` + uri + `";</script>`)
	embeds := InspectEmbeds(raw, map[[32]byte][]string{sha256.Sum256([]byte(uri)): {"assets/image.svg"}})
	if len(embeds) != 1 || embeds[0].Count != 3 || embeds[0].Bytes != len(uri) || embeds[0].TotalBytes != 3*len(uri) || !reflect.DeepEqual(embeds[0].Sources, []string{"assets/image.svg"}) {
		t.Fatalf("%+v", embeds)
	}
}
