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
	tokens := strings.Replace(clean, `h1{font-family:"Recursive",sans-serif;`, `:root{--display:"Recursive",sans-serif}h1{font-family:var(--display);`, 1)
	if got := fidelity(t, facts, tokens); len(got) != 0 {
		t.Fatalf("a face set through a custom property is used: %+v", got)
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
	three := *facts
	three.Imagery = 3
	if got := fidelity(t, &three, clean); len(got) != 1 || !strings.Contains(got[0].Message, "plans 3 images, but the page shows 1") {
		t.Fatalf("counted imagery: %+v", got)
	}
	reused := strings.Replace(clean, `<img src=a.png alt="">`, `<img src=a.png alt=""><img src=a.png alt=""><div style="background:url('b.webp')"></div><svg viewBox="0 0 10 10" width="200"></svg>`, 1)
	if got := fidelity(t, &three, reused); len(got) != 0 {
		t.Fatalf("a.png once, b.webp and a drawing are three: %+v", got)
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

func TestMonoDeclaredInConceptIsADecision(t *testing.T) {
	var mono Rule
	for _, r := range DefaultRules() {
		if r.ID == "design.mono" {
			mono = r
		}
	}
	body := `<style>.l{font-family:"Martian Mono",monospace}</style></head><body>` + strings.Repeat(`<span class=l>Hours of sun</span>`, 9) + `</body></html>`
	run := func(facts *ConceptFacts) []Diagnostic {
		d, err := Parse([]byte("<!doctype html><html><head><title>t</title>"+body), Options{Concept: facts})
		if err != nil {
			t.Fatal(err)
		}
		return mono.Check(d)
	}
	if len(run(nil)) == 0 {
		t.Fatal("undeclared mono labels are the console default")
	}
	if got := run(&ConceptFacts{Families: []string{"Martian Mono, monospace"}}); len(got) != 0 {
		t.Fatalf("a declared mono face is the concept's choice: %+v", got)
	}
}

func TestConceptCopyPhrases(t *testing.T) {
	facts := &ConceptFacts{Use: []string{"zombie day", "rota"}, Avoid: []string{"self-care", "wellness"}}
	page := `</head><body><h1>Survive the zombie day</h1><p>Plan the rota.</p><blockquote>My self-care is sleep</blockquote><p>Unwellness is a word we avoid.</p></body></html>`
	if got := fidelity(t, facts, page); len(got) != 0 {
		t.Fatalf("used phrases present, avoided ones only quoted or inside other words: %+v", got)
	}
	bad := strings.Replace(page, "Plan the rota.", "A Wellness plan.", 1)
	bad = strings.Replace(bad, "zombie day", "night", 1)
	got := fidelity(t, facts, bad)
	var messages []string
	for _, d := range got {
		messages = append(messages, d.Message)
	}
	joined := strings.Join(messages, "\n")
	for _, want := range []string{`avoid "wellness"; the page uses it`, `use "zombie day"; the page never does`, `use "rota"; the page never does`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
}

func TestWordsInOrder(t *testing.T) {
	for text, want := range map[string]bool{
		"Seit Tag 3 geht er nicht mehr auf.":        true,
		"geht er seit Tag 3 nicht mehr auf":         true,
		"Er geht auf, aber nicht mehr.":             false,
		"Der Teig geht nicht. Mehr Zeit, dann auf.": true,
	} {
		if got := wordsInOrder(text, "geht nicht mehr auf", 5); got != want {
			t.Errorf("%q: %v", text, got)
		}
	}
}
