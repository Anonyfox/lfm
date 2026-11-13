package check

import (
	"strings"
	"testing"
)

func TestFooterRuleBytesCountsOnlyFooterRules(t *testing.T) {
	a, b := ".fox-footer{color:red}", ".fox-footer a,.fox-footer__credit a{x:y}"
	raw := "main{margin:0}" + a + "@media(min-width:40em){" + b + "main,.fox-footer{padding:0}}.fox-footer~main{x:y}.fox-footerish{x:y}"
	if got, want := footerRuleBytes([]byte(raw)), len(a)+len(b); got != want {
		t.Fatalf("footer bytes %d, want %d", got, want)
	}
}

func TestBuiltinFooterCSSDoesNotConsumeAuthoredBudget(t *testing.T) {
	authored := "main{content:'" + strings.Repeat("a", 19*1024) + "'}"
	footerCSS := ".fox-footer{content:'" + strings.Repeat("f", 3*1024) + "'}"
	footer := `<footer class="fox-footer"><p class="fox-footer__credit">Made with</p></footer>`
	page := func(css, body string) string { return `<style>` + css + `</style><main>x</main>` + body }

	if got := checkRule(t, "perf.budgets", page(authored+footerCSS, footer)); len(got) != 0 {
		t.Fatalf("built-in footer CSS was charged to the page: %v", got)
	}
	// Without the built-in footer element the same bytes are authored CSS.
	if got := checkRule(t, "perf.budgets", page(authored+footerCSS, "")); len(got) != 1 {
		t.Fatalf("footer-styled CSS without the built-in footer must count: %v", got)
	}
	// Rules that reach outside the footer stay counted.
	escape := ".fox-footer~main{content:'" + strings.Repeat("e", 3*1024) + "'}"
	if got := checkRule(t, "perf.budgets", page(authored+escape, footer)); len(got) != 1 {
		t.Fatalf("sibling-combinator rule must count: %v", got)
	}
	// Over budget on authored CSS alone: still warns and names the excluded bytes.
	over := "main{content:'" + strings.Repeat("a", 21*1024) + "'}"
	got := checkRule(t, "perf.budgets", page(over+footerCSS, footer))
	if len(got) != 1 || !strings.Contains(got[0].Message, "built-in footer (3.0 KiB, not counted)") || !strings.Contains(got[0].Message, "is 21.0 KiB") {
		t.Fatalf("authored overage: %v", got)
	}
}

func TestBuiltinFooterCSSInLinkedSheetCountsOnce(t *testing.T) {
	authored := "main{content:'" + strings.Repeat("a", 19*1024) + "'}"
	sheet := []byte(authored + ".fox-footer{content:'" + strings.Repeat("f", 3*1024) + "'}")
	raw := `<link rel=stylesheet href=s.css><noscript><link rel=stylesheet href=s.css></noscript><main>x</main><footer class="fox-footer"></footer>`
	run := func(footer bool) []Diagnostic {
		input := raw
		if !footer {
			input = strings.Replace(raw, `<footer class="fox-footer"></footer>`, "", 1)
		}
		d, err := Parse([]byte(input), Options{ExternalCSSBytes: len(sheet), Stylesheets: map[string][]byte{"s.css": sheet}})
		if err != nil {
			t.Fatal(err)
		}
		for _, rule := range DefaultRules() {
			if rule.ID == "perf.budgets" {
				return Run(d, []Rule{rule}, nil).Diagnostics
			}
		}
		t.Fatal("no perf.budgets")
		return nil
	}
	if got := run(true); len(got) != 0 {
		t.Fatalf("linked footer CSS charged: %v", got)
	}
	if got := run(false); len(got) != 1 {
		t.Fatalf("no built-in footer, linked CSS must count: %v", got)
	}
}
