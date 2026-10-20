package deck

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/theme"
)

// TestFitAgreesWithChrome grows the text of a body, two columns and a
// statement step by step on every theme and holds the fit check, which a
// compile runs without a browser, to Chrome's print of the same slides:
// it must flag every slide Chrome cuts off, and at most one step before.
func TestFitAgreesWithChrome(t *testing.T) {
	t.Parallel()
	printer, err := release.NewPrinter(context.Background())
	if err != nil || testing.Short() {
		t.Skip(err)
	}
	defer printer.Close()
	const steps = 14
	sentence := "Check the fryer thermostat before the first batch of the day"
	var md strings.Builder
	md.WriteString("---\ntitle: Fit\n---\n# Fit\n")
	kinds := []string{"body", "columns", "statement"}
	for _, kind := range kinds {
		for n := 1; n <= steps; n++ {
			fmt.Fprintf(&md, "\n---\n## %s %d\n\n", kind, n)
			switch kind {
			case "body":
				for i := 0; i < n; i++ {
					md.WriteString("- " + sentence + "\n")
				}
			case "columns":
				md.WriteString("::: columns\n")
				for i := 0; i < n; i++ {
					md.WriteString("- " + sentence + "\n")
				}
				md.WriteString("+++\n- One point\n:::\n")
			case "statement":
				md.WriteString("> " + strings.Repeat(sentence+". ", n) + "\n")
			}
		}
	}
	concept, err := os.ReadFile(filepath.Join("testdata", "briefing", "concept.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			look := strings.Replace(string(concept), "Theme: warm", "Theme: "+name+"\nText font: Lora\nHeading font: Lora", 1)
			r := Check(fixture(t, "briefing", map[string]string{Dir + "/concept.md": look, Dir + "/en/briefing.md": md.String(), Dir + "/de/briefing.md": ""}), testConfig, true, nil, "")
			built, _ := r.Build(func(string) []byte { return nil })
			k := r.design()
			for _, b := range built {
				if b.Edition.Lang != "en" {
					continue
				}
				w, h := ShotPixels()
				over, err := printer.Overflows(r.PrintDoc(b, Print{Kind: "slides"}), r.GuideAssets(), w, h)
				if err != nil {
					t.Fatal(err)
				}
				for i, kind := range kinds {
					// The first step each check flags, steps+1 for none.
					static, chrome := steps+1, steps+1
					for n := steps; n >= 1; n-- {
						s := b.Deck.Slides[1+i*steps+n-1]
						if need, have, _ := k.fit(s); need > have {
							static = n
						}
						if slices.Contains(over, strconv.Itoa(s.N)) {
							chrome = n
						}
					}
					t.Logf("%-9s fit flags from %2d, Chrome cuts off from %2d", kind, static, chrome)
					if static > chrome || static < chrome-1 {
						t.Errorf("%s: the fit check flags from step %d, Chrome cuts off from %d", kind, static, chrome)
					}
				}
			}
		})
	}
}
