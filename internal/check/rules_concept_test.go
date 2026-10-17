package check

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/color"
)

func fidelity(t *testing.T, facts *ConceptFacts, body string) []Diagnostic {
	t.Helper()
	d, err := Parse([]byte("<!doctype html><html><head><title>t</title>"+body), Options{Concept: facts})
	if err != nil {
		t.Fatal(err)
	}
	return conceptFidelityRule(d)
}

func hexes(t *testing.T, values ...string) []color.Color {
	var out []color.Color
	for _, v := range values {
		c, ok := color.ParseHex(v)
		if !ok {
			t.Fatal(v)
		}
		out = append(out, c)
	}
	return out
}

func TestConceptFidelity(t *testing.T) {
	facts := &ConceptFacts{Colors: hexes(t, "#1b1f2a", "#f2efe6", "#e8a33d"), Families: []string{"Recursive, sans-serif", "system-ui"}, Imagery: 1}
	clean := `<style>body{background:#1b1f2a;color:#f2efe6;font:16px/1.4 system-ui,sans-serif}h1{font-family:"Recursive",sans-serif;color:#e8a33d}.card{box-shadow:0 2px 8px rgba(0,0,0,.4);border:1px solid rgba(242,239,230,.2)}</style></head><body><img src=a.png alt=""></body></html>`
	if got := fidelity(t, facts, clean); len(got) != 0 {
		t.Fatalf("clean page reported: %+v", got)
	}
	for name, tc := range map[string]struct{ body, want string }{
		"undeclared face":  {strings.Replace(clean, `"Recursive"`, `"Inter"`, 1), `"inter" is used`},
		"unused face":      {strings.Replace(clean, `font-family:"Recursive",sans-serif;`, "", 1), `"recursive" is declared in concept.md but never used`},
		"undeclared color": {strings.Replace(clean, "color:#e8a33d", "color:#6366f1", 1), "not in the concept palette: #6366f1 (h1)"},
		"no imagery":       {strings.Replace(clean, `<img src=a.png alt="">`, "", 1), "plans 1 images"},
		"inline color":     {strings.Replace(clean, `<img src=a.png alt="">`, `<img src=a.png alt="" style="outline:2px solid #00ff00">`, 1), "#00ff00"},
	} {
		t.Run(name, func(t *testing.T) {
			found := false
			for _, d := range fidelity(t, facts, tc.body) {
				found = found || strings.Contains(d.Message, tc.want)
			}
			if !found {
				t.Fatalf("no finding containing %q: %+v", tc.want, fidelity(t, facts, tc.body))
			}
		})
	}
	if got := fidelity(t, nil, clean); got != nil {
		t.Fatal("without a concept the rule is silent")
	}
}

func TestFamiliesParsing(t *testing.T) {
	for _, tc := range []struct {
		prop, value string
		want        string
	}{
		{"font-family", `"Recursive", system-ui, sans-serif`, "recursive"},
		{"font", "italic 700 1.2rem/1.1 'Big Shoulders Display', serif", "big shoulders display"},
		{"font", "inherit", ""},
		{"font-family", "var(--display), serif", ""},
	} {
		if got := strings.Join(families(tc.prop, tc.value), ","); got != tc.want {
			t.Errorf("families(%s: %s) = %q, want %q", tc.prop, tc.value, got, tc.want)
		}
	}
}
