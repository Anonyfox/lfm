package compile

import (
	"context"
	"strings"
	"testing"
)

// Exercise the default and opt-out through the complete asset and URL pipeline.
func TestBuiltinFooter(t *testing.T) {
	for _, tc := range []struct {
		name         string
		footer       any
		own, warning bool
		want         []string
	}{
		{"default", nil, false, false, []string{`href=https://lfm.cash/`, `href=https://anonyfox.com/`, "crafted with", "fox-footer__heart"}},
		{"disabled", false, true, false, []string{"Project footer"}},
		{"duplicate", map[string]any{}, true, true, []string{"fox-footer__credit"}},
		{"configured", map[string]any{
			"wordmark": "Example", "tagline": "Tools for your work.", "accent": "var(--glow)", "type": "var(--mono)", "muted": "var(--hairline)",
			"groups": []any{map[string]any{"label": "Product", "links": []any{map[string]any{"urlKey": "news", "text": "Release notes", "lang": "de"}}}, map[string]any{"label": "Legal", "links": []any{map[string]any{"urlKey": "news", "text": "News archive"}}}},
			"badges": []any{map[string]any{"urlKey": "news", "srcKey": "badge", "alt": "Featured", "width": 175, "height": 55, "target": "_blank", "rel": "noopener"}},
		}, false, false, []string{"Example", "Tools for your work.", "Release notes", `lang=de`, `href=https://example.org/news/`, "Featured", "src=https://example.org/badge.svg", "width=175", "height=55", "target=_blank", "rel=noopener", "--footer-accent:var(--glow)", "--footer-columns:2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			changeConfig(t, root, func(c map[string]any) {
				delete(c, "footer")
				if tc.footer != nil {
					c["footer"] = tc.footer
				}
				if tc.name == "configured" {
					c["urls"] = map[string]any{"news": "https://example.org/news/", "badge": "https://example.org/badge.svg"}
				}
			})
			if tc.own {
				write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", "<footer>Project footer</footer></body>", 1))
			}
			result, err := Compile(context.Background(), root, Options{Publish: true})
			if err != nil {
				t.Fatalf("%v: %+v", err, result.Validation)
			}
			raw := read(t, root, "output/index.html")
			for _, want := range tc.want {
				if !strings.Contains(raw, want) {
					t.Errorf("missing %q", want)
				}
			}
			count := strings.Count(raw, `<footer class=fox-footer`)
			if tc.footer == false {
				if count != 0 {
					t.Fatal("disabled footer rendered")
				}
			} else if count != 1 {
				t.Fatalf("footer count %d", count)
			}
			warned := false
			for _, d := range result.Validation.Diagnostics {
				if d.Rule == "footer.duplicate" {
					warned = true
				}
				if d.Rule == "template.unused" && (strings.HasPrefix(d.Path, "footer") || strings.HasPrefix(d.Path, "urls")) {
					t.Errorf("rendered footer field reported unused: %+v", d)
				}
				if d.Rule == "links.literal" || d.Rule == "links.declared" {
					t.Errorf("built-in URL finding: %+v", d)
				}
			}
			if warned != tc.warning {
				t.Errorf("duplicate warning = %v", warned)
			}
			if tc.footer != false {
				fi, si := strings.Index(raw, `class=fox-footer`), strings.LastIndex(raw, "<script")
				if fi < 0 || (si >= 0 && fi > si) {
					t.Errorf("footer should precede trailing scripts")
				}
			}
		})
	}
}
