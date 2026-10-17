package check

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/fontcheck"
)

func TestFontFacesLoadingChecks(t *testing.T) {
	for _, tc := range []struct{ name, css, want string }{
		{"swap", `@font-face{font-family:X;src:url(./a.woff2) format(woff2);font-display:swap}`, ""},
		{"optional-variable-subset", `@font-face{font-family:X;src:url(./a.woff2) format(woff2) tech(variations);font-display:optional;font-weight:100 900;unicode-range:U+0-ff}`, ""},
		{"auto", `@font-face{font-family:X;src:url(./a.woff2)}`, "auto (implicit)"},
		{"block", `@font-face{font-family:X;src:url(./a.woff2);font-display:block}`, "invisible"},
		{"missing", `@font-face{font-display:swap}`, "missing font-family"},
		{"bad-family", `@font-face{font-family:system-ui;src:local(X)}`, "invalid font-family"},
		{"local", `@font-face{font-family:X;src:local(X)}`, ""},
		{"format", `@font-face{font-family:X;src:url(./a.woff2) format(woff);font-display:fallback}`, "disagrees"},
		{"source-fallback", `@font-face{font-family:X;src:url(./a.ttf) format(opentype),url(./a.woff2) format(woff2);font-display:swap}`, ""},
		{"legacy-only", `@font-face{font-family:X;src:url(./a.ttf);font-display:swap}`, "no WOFF2"},
		{"bad-range", `@font-face{font-family:X;src:url(./a.woff2);font-display:swap;unicode-range:U+ffffff}`, "invalid unicode-range"},
		{"template", ``, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `<style>` + tc.css + `</style><template><style>@font-face{}</style></template>`
			d, err := Parse([]byte(raw), Options{FontAssets: map[string]fontcheck.Info{"./a.woff2": {Source: "assets/a.woff2", Format: "woff2", Bytes: 1000}, "./a.ttf": {Source: "assets/a.ttf", Format: "truetype", Bytes: 1000}}})
			if err != nil {
				t.Fatal(err)
			}
			issues := fontsRule(d)
			if tc.want == "" {
				if len(issues) > 0 {
					t.Fatal(issues)
				}
			} else if len(issues) != 1 || !strings.Contains(issues[0].Message, tc.want) {
				t.Fatal(issues)
			}
		})
	}
}

func TestFontPreloadAndSidecarCSS(t *testing.T) {
	raw := []byte(`<link rel="stylesheet" href="./bundle.css"><link rel="preload" as="font" href="./a.woff2" crossorigin="use-credentials" type="font/woff">`)
	d, _ := Parse(raw, Options{Stylesheets: map[string][]byte{"./bundle.css": []byte("\n@media print{@font-face{font-family:X;src:local(X),url(./a.woff2);unicode-range:U+0-FF}}")}, FontAssets: map[string]fontcheck.Info{"./a.woff2": {Source: "a.woff2", Format: "woff2"}}})
	issues := fontsRule(d)
	if len(issues) != 2 {
		t.Fatal(issues)
	}
	for _, text := range []string{"use-credentials", "type=", "bypasses"} {
		if !strings.Contains(issues[1].Message, text) {
			t.Fatal(issues)
		}
	}
	if issues[0].Location.File != "bundle.css" || issues[0].Location.Line != 2 {
		t.Fatal(issues[0])
	}
	// An opaque third-party stylesheet may declare the font; don't call it unused.
	d, _ = Parse([]byte(`<link rel="stylesheet" href="https://example.com/fonts.css"><link rel="preload" as="font" href="https://example.com/a.woff2" crossorigin>`), Options{})
	if got := fontsRule(d); len(got) > 0 {
		t.Fatal(got)
	}
}

func TestFontFamilyDefaultAcrossStylesheets(t *testing.T) {
	d, _ := Parse([]byte(`<style>@font-face{font-family:"Demo Serif";src:url(./a.woff2)}</style><style>@font-feature-values Demo Serif{font-display:optional}</style>`), Options{FontAssets: map[string]fontcheck.Info{"./a.woff2": {Source: "a.woff2", Format: "woff2"}}})
	if got := fontsRule(d); len(got) != 0 {
		t.Fatal("family default mislabeled implicit auto", got)
	}
}

func TestFontBudgetsDedupAndRuleOverrides(t *testing.T) {
	opts := Options{FontAssets: map[string]fontcheck.Info{"./a": {Source: "a.woff2", Bytes: fontByteBudget + 1, Format: "woff2"}, "./a#face": {Source: "a.woff2", Bytes: fontByteBudget + 1, Format: "woff2"}}}
	d, _ := Parse([]byte(`<style>@font-face{font-family:X;src:url(./a);font-display:swap}</style>`), opts)
	if issues := fontAssetsRule(d); len(issues) != 1 || issues[0].Path != "a.woff2" {
		t.Fatal(issues)
	}
	for _, level := range []Severity{Off, Warning, Error} {
		d, _ := Parse([]byte(`<style>@font-face{font-family:X;src:url(./a)}</style>`), opts)
		r := Run(d, []Rule{{"perf.fonts", "faces", fontsRule}, {"perf.font-assets", "assets", fontAssetsRule}}, map[string]Severity{"perf.fonts": level, "perf.font-assets": Off})
		if level == Off && len(r.Diagnostics) != 0 || level == Warning && r.Warnings != 1 || level == Error && r.Errors != 1 {
			t.Fatal(r)
		}
	}
	// The exact budget boundary is quiet.
	opts.FontAssets = map[string]fontcheck.Info{"./a": {Source: "a", Bytes: fontByteBudget}}
	d, _ = Parse([]byte(`<p>test</p>`), opts)
	if got := fontAssetsRule(d); len(got) > 0 {
		t.Fatal(got)
	}
}
