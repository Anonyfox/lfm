package check

import (
	"strings"
	"testing"
)

func TestSEO(t *testing.T) {
	valid := `<html lang=en><head><title>T</title><meta name=description content=D><link rel=canonical href="https://example.com/"></head><body><h1>H</h1></body></html>`
	if issues := checkRule(t, "seo.metadata", valid); len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, input := range []string{`<title></title>`, `<title>A</title><title>B</title>`, `<link rel=canonical href=/relative>`, `<link rel=canonical href="https://example.com/#fragment">`} {
		if len(checkRule(t, "seo.metadata", input)) == 0 {
			t.Fatal(input)
		}
	}
	if len(checkRule(t, "seo.indexability", `<meta name=Robots content="follow, NOINDEX">`)) != 1 {
		t.Fatal("missed noindex")
	}
	if len(checkRule(t, "seo.indexability", `<meta name=robots content=index>`)) != 0 {
		t.Fatal("false noindex")
	}
	issues := checkRule(t, "seo.social", `<meta property=og:image content="data:image/png;base64,YQ==">`)
	if !hasMessage(issues, Error, "og:image must be") {
		t.Fatal("embedded social image")
	}
	if len(checkRule(t, "seo.social", completeSocial)) != 0 {
		t.Fatal("public image")
	}
	if len(checkRule(t, "seo.headings", `<h1>A</h1><h1>B</h1>`)) != 0 {
		t.Fatal("multiple h1 is not invalid HTML")
	}
}

func TestAccessibility(t *testing.T) {
	for _, input := range []string{`<img alt="">`, `<img alt=Diagram>`, `<area alt=X href=x>`} {
		if len(checkRule(t, "a11y.images", input)) != 0 {
			t.Fatal(input)
		}
	}
	if len(checkRule(t, "a11y.images", `<img>`)) != 1 {
		t.Fatal("missing alt")
	}
	for _, input := range []string{
		`<label>Name<input></label>`, `<label for=x>Name</label><input id=x>`,
		`<input aria-label=Name>`, `<p id=name>Name</p><input aria-labelledby=name>`,
		`<input type=hidden>`, `<input type=submit>`, `<button>Go</button>`, `<a href=x><img alt=Buy></a>`,
		`<button aria-label=Close><svg></svg></button>`, `<input title=Name>`,
		`<span id=label hidden>Hidden label</span><input aria-labelledby=label>`,
	} {
		if issues := checkRule(t, "a11y.controls", input); len(issues) != 0 {
			t.Fatalf("%s: %+v", input, issues)
		}
	}
	for _, input := range []string{`<input placeholder=Name>`, `<button><svg></svg></button>`, `<a href=x><img alt=""></a>`, `<input type=button value="">`} {
		if len(checkRule(t, "a11y.controls", input)) != 1 {
			t.Fatal(input)
		}
	}
}

func TestSizeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		size  int
		level Severity
	}{{HTMLWarning - 1, ""}, {HTMLWarning, Warning}, {HTMLLimit, Warning}, {HTMLLimit + 1, Error}} {
		d, err := Parse([]byte("<p>"+strings.Repeat("x", tc.size-7)+"</p>"), Options{})
		if err != nil {
			t.Fatal(err)
		}
		issues := Run(d, []Rule{testRule("perf.html-size", sizeRule)}, nil).Diagnostics
		if tc.level == "" {
			if len(issues) != 0 {
				t.Fatal(issues)
			}
			continue
		}
		if len(issues) != 1 || issues[0].Severity != tc.level || !strings.Contains(issues[0].Hint, "gzip/Brotli") {
			t.Fatalf("%d: %+v", tc.size, issues)
		}
	}
}

func TestPerformanceRules(t *testing.T) {
	if len(checkRule(t, "perf.images", `<img src=x width=10 height=20>`)) != 0 {
		t.Fatal("sized image")
	}
	if len(checkRule(t, "perf.images", `<img src=x width=0 height=20>`)) != 1 {
		t.Fatal("zero dimensions")
	}
	if len(checkRule(t, "perf.images", `<img src=x width=10 height=20 loading=lazy fetchpriority=high>`)) != 1 {
		t.Fatal("contradictory priority")
	}
	if len(checkRule(t, "perf.images", `<img src="data:image/png;base64,`+strings.Repeat("a", LargeEmbed)+`" width=10 height=20 loading=lazy>`)) != 1 {
		t.Fatal("lazy inline image")
	}
	if len(checkRule(t, "perf.images", `<img src="data:image/png;base64,YQ==" width=10 height=20 loading=lazy>`)) != 0 {
		t.Fatal("tiny decorative embeds should not create warning noise")
	}
	if len(checkRule(t, "perf.inline-assets", `<img srcset="data:image/png;base64,YQ== 1x, x.png 2x">`)) != 1 {
		t.Fatal("inline srcset")
	}
	if len(checkRule(t, "perf.viewport", `<meta name=viewport content="width=device-width, initial-scale=1">`)) != 0 {
		t.Fatal("valid viewport")
	}
	if len(checkRule(t, "perf.viewport", `<meta name=viewport content="width=device-width, user-scalable = no">`)) != 1 {
		t.Fatal("disabled zoom")
	}
	if len(checkRule(t, "perf.budgets", `<style>`+strings.Repeat(" ", 20*1024+1)+`</style>`)) != 1 {
		t.Fatal("CSS budget")
	}
}
