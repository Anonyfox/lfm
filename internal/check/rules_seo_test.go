package check

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const completeSocial = `<head><meta property="og:title" content="Useful result"><meta property="og:type" content="website"><meta property="og:url" content="https://example.com/"><meta property="og:image" content="https://example.com/share.webp"></head>`

func hasMessage(issues []Diagnostic, level Severity, message string) bool {
	for _, issue := range issues {
		if issue.Severity == level && strings.Contains(issue.Message, message) {
			return true
		}
	}
	return false
}

func TestMetadataPlacementLanguageAndSVG(t *testing.T) {
	valid := `<html lang="zh-Hant-TW"><head><title>Calculator</title><meta name="description" content="Calculate a result"><link rel="canonical" href="https://example.com/"></head><body><svg><title>Diagram</title></svg><template><title>Future</title><meta name=description content=Later></template></body></html>`
	if issues := checkRule(t, "seo.metadata", valid); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, tc := range []struct {
		html, message string
		level         Severity
	}{
		{`<html lang=en><head></head><body><title>Late</title></body></html>`, "title is outside head", Error},
		{`<body><meta name=description content=Late></body>`, "description is outside head", Warning},
		{`<html lang=en_US></html>`, "BCP 47", Warning},
		{`<html lang="English"></html>`, "BCP 47", Warning},
		{`<html lang=" de "></html>`, "BCP 47", Warning},
		{`<link rel=canonical href="http://localhost/page">`, "Canonical URL must", Error},
	} {
		if issues := checkRule(t, "seo.metadata", tc.html); !hasMessage(issues, tc.level, tc.message) {
			t.Fatal(tc, issues)
		}
	}
	for _, tag := range []string{"de-DE", "en", "es-419", "sr-Latn", "und", "x-acme"} {
		if issues := checkRule(t, "seo.metadata", strings.Replace(valid, "zh-Hant-TW", tag, 1)); len(issues) != 0 {
			t.Fatal(tag, issues)
		}
	}
}

func TestSocialMetadataContract(t *testing.T) {
	for _, tc := range []struct {
		html, message string
		level         Severity
	}{
		{"<head></head>", "og:title, og:type, og:url, og:image", Warning},
		{strings.Replace(completeSocial, `property="og:title"`, `name="og:title"`, 1), "wrong metadata attribute", Warning},
		{strings.Replace(completeSocial, "</head>", `<meta property="og:title" content="Other"></head>`, 1), "Repeated scalar", Warning},
		{strings.Replace(completeSocial, "</head>", `<meta name="twitter:description" content=""></head>`, 1), "twitter:description is empty", Warning},
		{strings.Replace(completeSocial, `https://example.com/share.webp`, `data:image/png;base64,YQ==`, 1), "og:image must", Error},
		{strings.Replace(completeSocial, `https://example.com/share.webp`, `/share.webp`, 1), "og:image must", Error},
		{strings.Replace(completeSocial, `https://example.com/share.webp`, `https://127.0.0.1/share.webp`, 1), "og:image must", Error},
		{completeSocial + `<body><meta name="twitter:title" content="Late"></body>`, "twitter:title is outside head", Warning},
	} {
		if issues := checkRule(t, "seo.social", tc.html); !hasMessage(issues, tc.level, tc.message) {
			t.Fatal(tc, issues)
		}
	}
	// Multiple images, media-scoped descriptors, custom types and deliberate
	// listing wording/preview differences are supported without duplicate noise.
	valid := strings.Replace(completeSocial, "</head>", `<meta property="og:image:alt" content="First"><meta property="og:image:width" content="1200"><meta property="og:image" content="https://example.com/second.webp"><meta property="og:image:alt" content="Second"><meta property="og:image:width" content="800"><meta name="twitter:title" content="Different headline"><meta name="twitter:image" content="https://example.com/third.webp"></head>`, 1)
	if issues := checkRule(t, "seo.social", valid); len(issues) != 0 {
		t.Fatal(issues)
	}
	if issues := checkRule(t, "seo.social", strings.Replace(completeSocial, `property="og:image"`, `property="og:image:url"`, 1)); len(issues) != 0 {
		t.Fatal(issues)
	}
	if issues := checkRule(t, "seo.social", completeSocial+`<body><template><meta property=og:url content=bad></template></body>`); len(issues) != 0 {
		t.Fatal(issues)
	}
}

func TestSocialCanonicalIdentity(t *testing.T) {
	for _, tc := range []struct {
		target, canonical, social string
		warn                      bool
	}{
		{"https://example.com/", "", "https://EXAMPLE.com:443/", false},
		{"https://example.com/", "", "https://example.com/other/", true},
		{"https://example.com/copy/", "https://example.com/original/", "https://example.com/original/", false},
		{"https://example.com/", "https://example.com/", "https://example.com/?campaign=1", false},
	} {
		raw := strings.Replace(completeSocial, `content="https://example.com/"`, `content="`+tc.social+`"`, 1)
		if tc.canonical != "" {
			raw = strings.Replace(raw, "</head>", `<link rel=canonical href="`+tc.canonical+`"></head>`, 1)
		}
		d, _ := Parse([]byte(raw), Options{URL: tc.target})
		issues := socialRule(d)
		if got := hasMessage(issues, Warning, "differs from page canonical"); got != tc.warn {
			t.Fatal(tc, issues)
		}
	}
}

func TestPublicURLsRejectOnlyKnownLocalDestinations(t *testing.T) {
	for _, raw := range []string{"http://localhost/", "http://app.localhost./", "http://app.local/", "https://demo.test/", "https://demo.invalid/", "http://10.0.0.1/", "http://127.1/", "http://192.168.1.1/", "http://169.254.1.1/", "http://[::1]/", "http://[fd00::1]/", "http://[::ffff:192.168.1.1]/", "http://0.0.0.0/"} {
		if publicURL(raw) {
			t.Fatal("accepted local/private URL", raw)
		}
	}
	for _, raw := range []string{"https://example.com/", "https://bücher.de/über", "https://8.8.8.8/", "https://[2606:4700:4700::1111]/", "https://example.com/?q=one%20two"} {
		if !publicURL(raw) {
			t.Fatal("rejected public URL syntax", raw)
		}
	}
}

func TestHeadingHierarchyAndVisibility(t *testing.T) {
	for _, raw := range []string{`<h1>Task</h1><h2>Part</h2><h3>Detail</h3><h2>Next</h2>`, `<h1>One</h1><h1>Two</h1>`, `<h1><img alt="Main task"></h1>`, `<h1>Task</h1><template><h5></h5></template><section hidden><h6>Inactive</h6></section>`} {
		if issues := checkRule(t, "seo.headings", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	if issues := checkRule(t, "seo.headings", `<h1 aria-labelledby=label></h1><span id=label hidden>Accessible heading</span><details><summary>More</summary><h6>Closed detail</h6></details>`); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, tc := range []struct{ raw, message string }{
		{`<h1 hidden>Hidden</h1>`, "No visible main"},
		{`<div aria-hidden=true><h1>Hidden</h1></div>`, "No visible main"},
		{`<h1>Task</h1><h3>Detail</h3>`, "hierarchy jumps"},
		{`<h1> </h1>`, "no readable content"},
		{`<h1>Task</h1><h2><template>Inert</template></h2>`, "no readable content"},
	} {
		if issues := checkRule(t, "seo.headings", tc.raw); !hasMessage(issues, Warning, tc.message) {
			t.Fatal(tc, issues)
		}
	}
}

func TestSnippetRestrictions(t *testing.T) {
	for _, value := range []string{"nosnippet", "INDEX, NOSNIPPET", "max-snippet:0", "max-snippet: 50", "max-snippet:+10, max-image-preview:large"} {
		if issues := checkRule(t, "seo.snippets", `<meta name=Googlebot content="`+value+`">`); len(issues) != 1 || issues[0].Hint == "" {
			t.Fatal(value, issues)
		}
	}
	for _, raw := range []string{`<meta name=robots content="index,follow,max-snippet:-1">`, `<meta name=robots content="max-snippet:garbage">`, `<meta name=robots content="unavailable_after: 25 Jun 2030 15:00:00 PST">`, `<meta name=unknown-bot content=nosnippet>`, `<template><meta name=robots content=nosnippet></template>`} {
		if issues := checkRule(t, "seo.snippets", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, tc := range []struct {
		content string
		count   int
	}{{"none", 2}, {"noindex, nofollow", 2}, {"nofollow", 1}, {"index,follow", 0}} {
		if issues := checkRule(t, "seo.indexability", `<meta name=robots content="`+tc.content+`">`); len(issues) != tc.count {
			t.Fatal(tc, issues)
		}
	}
}

func TestInitialTextAndMainLandmarks(t *testing.T) {
	for _, raw := range []string{`<main><h1>Calculator</h1><p>Cost = guests × price.</p><canvas></canvas></main>`, `<div role=main><p>Answer</p></div>`, `<main>Answer</main><main hidden>Alternate</main>`, `<main><details><summary>Question?</summary><p>Answer</p></details></main>`, `<main>Answer <div data-nosnippet>Small excluded passage</div></main>`, `<main data-nosnippet>Unsupported attribute does not suppress text.</main>`} {
		if issues := checkRule(t, "seo.content", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, tc := range []struct{ raw, message string }{
		{`<p>Answer</p>`, "No visible main"},
		{`<main>One</main><main>Two</main>`, "Multiple visible main"},
		{`<main><canvas aria-label=Result></canvas><script>let answer=42;</script><template><p>Future answer</p></template><img alt=Result></main>`, "no initially readable"},
		{`<main><p hidden>Hidden answer</p><noscript>Fallback only</noscript><textarea>Input</textarea></main>`, "no initially readable"},
		{`<main><details><summary></summary><p>Closed answer</p></details></main>`, "no initially readable"},
		{`<main><dialog><p>Closed dialog</p></dialog></main>`, "no initially readable"},
		{`<details><summary>Expand</summary><main>Answer after click</main></details>`, "No visible main"},
		{`<main><section data-nosnippet>Entire answer</section></main>`, "All initially readable"},
		{`<section data-nosnippet><main>Entire answer</main></section>`, "All initially readable"},
	} {
		if issues := checkRule(t, "seo.content", tc.raw); !hasMessage(issues, Warning, tc.message) {
			t.Fatal(tc, issues)
		}
	}
}

func TestFragmentDiscoveryAndReferenceEdgeCases(t *testing.T) {
	for _, raw := range []string{`<a href="#part">Go</a><p id=part hidden>Answer</p>`, `<a href="./#part">Go</a><section hidden><p id=part>Answer</p></section>`, `<area href="https://example.com/#part"><p id=part hidden>Answer</p>`, `<a href="#part:~:text=Answer">Go</a><p id=part hidden>Answer</p>`, `<a href="#top">Go</a><p id=top hidden>Answer</p>`} {
		d, _ := Parse([]byte(raw), Options{URL: "https://example.com/"})
		issues := fragmentDiscoveryRule(d)
		if len(issues) != 1 || len(issues[0].Related) != 1 || issues[0].Hint == "" {
			t.Fatal(raw, issues)
		}
	}
	for _, raw := range []string{`<a href="#part">Go</a><p id=part hidden=until-found>Answer</p>`, `<a href="#part">Go</a><details><summary>Question</summary><p id=part>Answer</p></details>`, `<a href="?state=other#part">Other document state</a><p id=part hidden>Answer</p>`, `<a href="https://other.example/#part">Other page</a><p id=part hidden>Answer</p>`, `<a href="#:~:text=Answer">Text fragment</a>`, `<template><a href="#part">Go</a></template><p id=part hidden>Answer</p>`} {
		d, _ := Parse([]byte(raw), Options{URL: "https://example.com/"})
		if issues := fragmentDiscoveryRule(d); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, raw := range []string{`<a name=old></a><a href="#old">Legacy</a>`, `<a href="#TOP">Top</a>`, `<a href="#:~:text=Hello">Text fragment</a>`, `<p id="part:~:">Answer</p><a href="#part%3A~%3A">Encoded ID</a>`, `<p id=part>Answer</p><a href="#part:~:text=Answer">ID and text</a>`} {
		if issues := checkRule(t, "html.references", raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, raw := range []string{`<a href="./#:~:text=Hello">Text fragment</a>`, `<p id=part>Answer</p><a href="./#part:~:text=Answer">Both</a>`, `<template><p id=part>Inert target</p></template><a href="./#part">Go</a>`} {
		d, _ := Parse([]byte(raw), Options{URL: "https://example.com/"})
		issues := linksBehaviorRule(d)
		want := 0
		if strings.Contains(raw, "Inert target") {
			want = 1
		}
		if len(issues) != want {
			t.Fatal(raw, issues)
		}
	}
	d, _ := Parse([]byte(`<template><a href="./#future">Dormant navigation</a></template>`), Options{URL: "https://example.com/"})
	if issues := linksBehaviorRule(d); len(issues) != 0 {
		t.Fatal("inert fragment navigation", issues)
	}
}

func TestSEOFindingsPreserveBytesAndLocations(t *testing.T) {
	raw := []byte("<html lang=en><head>\n<meta property=og:image content=bad>\n</head><body><main>Answer</main></body></html>")
	d, _ := Parse(raw, Options{})
	before := bytes.Clone(raw)
	first := Run(d, DefaultRules(), nil)
	if !reflect.DeepEqual(first, Run(d, DefaultRules(), nil)) || !bytes.Equal(raw, before) {
		t.Fatal("analysis mutated input or diagnostics")
	}
	for _, issue := range first.Diagnostics {
		if issue.Rule == "seo.social" && issue.Severity == Error && (issue.Location.Line != 2 || issue.Location.Column != 1 || issue.Hint == "") {
			t.Fatal(issue)
		}
	}
}

func BenchmarkAnalyzeLinkedSections(b *testing.B) {
	var body strings.Builder
	for i := 0; i < 700; i++ {
		fmt.Fprintf(&body, `<section id="part%d"><h2>Section %d</h2><p>Static answer.</p><a href="https://example.com/#part%d">Read section</a></section>`, i, i, i)
	}
	raw := []byte(`<!doctype html><html lang=en>` + completeSocial + `<body><main><h1>Guide</h1>` + body.String() + `</main></body></html>`)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		_, _ = Analyze(raw, Options{URL: "https://example.com/"})
	}
}
