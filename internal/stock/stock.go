// Package stock knows the defaults text-to-page generators fall back to:
// accent colors, house palettes and typefaces. Visitors read them as
// generated, so the compiler reports them wherever a page or its concept
// would use them without a reason of its own.
package stock

import (
	"strings"

	"github.com/Anonyfox/lfm/internal/color"
)

// Named is a stock color with the name shown in findings.
type Named struct {
	Name  string
	Color color.Color
}

func hex(name, value string) Named {
	c, ok := color.ParseHex(value)
	if !ok {
		panic("stock: bad color " + value)
	}
	return Named{name + " " + value, c}
}

// Accents are the indigo, violet and purple defaults of utility CSS kits, in
// both the hex (v3) and OKLCH (v4) generations.
var Accents = func() []Named {
	list := []Named{
		hex("Tailwind indigo-400", "#818cf8"), hex("Tailwind indigo-500", "#6366f1"), hex("Tailwind indigo-600", "#4f46e5"), hex("Tailwind indigo-700", "#4338ca"),
		hex("Tailwind violet-400", "#a78bfa"), hex("Tailwind violet-500", "#8b5cf6"), hex("Tailwind violet-600", "#7c3aed"), hex("Tailwind violet-700", "#6d28d9"),
		hex("Tailwind purple-400", "#c084fc"), hex("Tailwind purple-500", "#a855f7"), hex("Tailwind purple-600", "#9333ea"), hex("Tailwind purple-700", "#7e22ce"),
	}
	for _, v := range []struct{ name, value string }{
		{"Tailwind v4 indigo-400", "oklch(67.3% 0.182 276.935)"}, {"Tailwind v4 indigo-500", "oklch(58.5% 0.233 277.117)"}, {"Tailwind v4 indigo-600", "oklch(51.1% 0.262 276.966)"}, {"Tailwind v4 indigo-700", "oklch(45.7% 0.24 277.023)"},
		{"Tailwind v4 violet-400", "oklch(70.2% 0.183 293.541)"}, {"Tailwind v4 violet-500", "oklch(60.6% 0.25 292.717)"}, {"Tailwind v4 violet-600", "oklch(54.1% 0.281 293.009)"}, {"Tailwind v4 violet-700", "oklch(49.1% 0.27 292.581)"},
		{"Tailwind v4 purple-400", "oklch(71.4% 0.203 305.504)"}, {"Tailwind v4 purple-500", "oklch(62.7% 0.265 303.9)"}, {"Tailwind v4 purple-600", "oklch(55.8% 0.288 302.321)"}, {"Tailwind v4 purple-700", "oklch(49.6% 0.265 301.924)"},
	} {
		c := color.Parse(v.value)
		if len(c) != 1 {
			panic("stock: bad color " + v.value)
		}
		list = append(list, Named{v.name + " " + v.value, c[0]})
	}
	return list
}()

// Creams and Terracottas together form the house palette of 2025/26 assistant
// interfaces: cream ground, terracotta accent.
var (
	Creams      = []Named{hex("cream", "#f4f1ea"), hex("cream", "#faf9f5"), hex("cream", "#f7f1e4"), hex("cream", "#f0eee6")}
	Terracottas = []Named{hex("terracotta", "#d97757"), hex("terracotta", "#c96442"), hex("terracotta", "#d9622b"), hex("terracotta", "#cc785c")}
)

// WindowControls are the red, yellow and green dots of a mocked macOS window,
// in that order: fake product chrome.
var WindowControls = [][]Named{
	{hex("red", "#ff5f56"), hex("red", "#ff5f57")},
	{hex("yellow", "#ffbd2e"), hex("yellow", "#febc2e")},
	{hex("green", "#27c93f"), hex("green", "#28c840")},
}

// Nearest returns the first stock color closer than limit (OKLab distance).
func Nearest(c color.Color, set []Named, limit float64) (Named, bool) {
	for _, n := range set {
		if c.Distance(n.Color) < limit {
			return n, true
		}
	}
	return Named{}, false
}

// Purpleish reports a saturated violet to magenta: the hue band of generated
// gradients and accents.
func Purpleish(c color.Color) bool {
	h := c.Hue()
	return c.Chroma() >= 0.12 && h >= 270 && h <= 330 && c.L > 0.3 && c.L < 0.85
}

var (
	genericSans = set("inter", "roboto", "arial", "poppins", "geist", "system-ui", "-apple-system", "blinkmacsystemfont", "sans-serif", "manrope", "dm sans", "plus jakarta sans", "open sans", "montserrat", "lato")
	display     = set("space grotesk", "instrument serif", "fraunces", "syne", "geist", "clash display", "cabinet grotesk", "satoshi", "general sans")
	mono        = set("monospace", "ui-monospace", "menlo", "monaco", "consolas", "sf mono", "jetbrains mono", "ibm plex mono", "space mono", "courier", "courier new", "roboto mono", "fira code", "fira mono", "dm mono", "geist mono", "chivo mono", "source code pro", "berkeley mono", "commit mono", "martian mono")
)

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// Family normalizes a font-family name: lowercase, without quotes.
func Family(name string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(name), `"'`))
}

// GenericSans reports the sans families generators use for everything.
func GenericSans(family string) bool { return genericSans[Family(family)] }

// Display reports the display faces generators reach for to look designed.
func Display(family string) bool { return display[Family(family)] }

// Mono reports monospace families.
func Mono(family string) bool { return mono[Family(family)] }
