package skills

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/stock"
)

// Any concrete look in guidance becomes what a generator makes: a font, a
// color, a style word or an example value is copied, not considered.
// Guidance names criteria, quantities and slots; formats, commands, paths
// and rule IDs, where conformity is the point, may show examples.

// commonFamilies are families generators reach for beyond lfm's own stock
// lists, and the ones lfm's skills once offered.
var commonFamilies = []string{"lora", "literata", "georgia", "garamond", "eb garamond", "source serif", "source sans",
	"ibm plex", "ibm plex sans", "merriweather", "playfair", "playfair display", "helvetica", "verdana", "times new roman",
	"noto sans", "noto serif", "nunito", "raleway", "work sans", "oswald", "libre baskerville", "crimson"}

// registers are words that rate or label a style instead of deciding it.
var registers = []string{"modern", "premium", "clean", "minimal", "minimalist", "elegant", "sleek", "luxury", "luxurious",
	"bold", "playful", "warm", "cozy", "friendly", "professional", "sophisticated", "vibrant", "fresh", "edgy", "classic",
	"timeless", "editorial", "technical", "corporate", "trendy", "stylish", "chic", "rustic", "whimsical", "quirky", "calm",
	"airy", "quiet"}

// allowed are the uses of those words that name no style, each with why.
var allowed = []struct{ word, phrase, why string }{
	{"warm", "never cream, beige, ivory or other warm off-whites", "a prohibited colour range, not a recommended style"},
	{"warm", "warm off-whites and paper, parchment or notebook decoration fail", "a prohibition, not a style recommendation"},
	{"clean", "are clean", "a check with zero findings"},
	{"clean", "is clean", "a check with zero findings"},
	{"clean", "compiles clean", "a compile with zero findings"},
	{"bold", "bold or regular", "the values of the Look line Headings"},
	{"bold", "regular, bold, italic and bold italic", "the static faces a font family ships"},
	{"bold", "-bold.ttf", "a font file name slot"},
	{"bold", "**bold**", "Markdown's strong emphasis"},
	{"bold", "manual bold headings", "formatting by hand instead of a heading"},
	{"bold", "in bold is a total", "Markdown strong emphasis marks a table's total row"},
	{"bold", "bold, lists", "the Markdown a store description allows"},
	{"fresh", "fresh path", "a newly generated download path"},
	{"fresh", "fresh one", "a newly generated download path"},
	{"fresh", "fresh eyes", "a reread after a pause"},
	{"professional", "a professional answering", "a research role of who wrote a voice"},
}

// allowedHints are example values in messages and hints that are
// identifiers or facts a buyer needs, not a look to copy, each with why.
var allowedHints = []struct{ phrase, why string }{
	{`such as EUR or JPY`, "ISO currency codes, the identifier Stripe expects"},
	{`e.g. CKB009000`, "the shape of a BISAC subject code"},
	{`e.g. WBVD`, "the shape of a Thema subject code"},
	{`such as "Ink: color, ink-saver"`, "the line format of a printable variant"},
	{`such as GoodNotes or Notability`, "the note apps a tablet printable opens in, told to its buyers"},
	{`such as "trusted by"`, "the proof claims the copy rule detects, so an author recognizes a finding"},
	{`such as "Learn more"`, "the generic labels the link rule detects, so an author recognizes a finding"},
	{`such as TODO or lorem ipsum`, "the placeholder text the chapter rule detects, so an author recognizes a finding"},
}

var (
	colorValue   = regexp.MustCompile(`(?:^|[^\w&])#[0-9a-fA-F]{3,8}(?:[^\w-]|$)|\b(?:rgb|rgba|hsl|hsla|oklch|oklab|lab|lch)\(`)
	exampleValue = regexp.MustCompile(`\b(?:e\.g\.|for example|such as)[ :,]+(?:["“]|[A-Z][\w-]*)`)
)

// plants are the planted defaults in a text; used records the allowed
// phrases a text relies on.
func plants(text string, used map[int]bool) []string {
	var out []string
	families := append(stock.Families(), commonFamilies...)
	for n, line := range strings.Split(text, "\n") {
		low := strings.ToLower(line)
		at := func(kind, what string) {
			out = append(out, kind+" "+strconv.Quote(what)+" in line "+strconv.Itoa(n+1)+": "+strings.TrimSpace(line))
		}
		for _, f := range families {
			if regexp.MustCompile(`(?:^|[^\w-])`+regexp.QuoteMeta(f)+`(?:[^\w-]|$)`).MatchString(low) && !strings.HasPrefix(f, "-") && f != "sans-serif" && f != "monospace" {
				at("font", f)
			}
		}
		if m := colorValue.FindString(line); m != "" {
			at("color", strings.TrimSpace(m))
		}
		if m := exampleValue.FindString(line); m != "" {
			at("example value", m)
		}
	words:
		for _, w := range registers {
			if !regexp.MustCompile(`\b` + w + `\b`).MatchString(low) {
				continue
			}
			for i, a := range allowed {
				if (a.word == w || strings.HasPrefix(a.word, w+" ")) && strings.Contains(low, a.phrase) {
					used[i] = true
					continue words
				}
			}
			at("style word", w)
		}
	}
	return out
}

// TestGuidancePlantsNoDefaults holds every skill, the files lfm init writes
// and the root SKILL.md to criteria and slots.
func TestGuidancePlantsNoDefaults(t *testing.T) {
	texts := map[string]string{}
	all, err := All(NewData("dev"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		texts["skill "+s.Topic] = s.Text
	}
	for _, root := range []string{"../../SKILL.md", "../../site/assets/agents/SKILL.md"} {
		raw, err := os.ReadFile(root)
		if err != nil {
			t.Fatal(err)
		}
		texts[root] = string(raw)
	}
	err = filepath.WalkDir("../scaffold/files", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		raw, err := os.ReadFile(path)
		texts[path] = string(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	used := map[int]bool{}
	for name, text := range texts {
		for _, p := range plants(text, used) {
			t.Errorf("%s: %s", name, p)
		}
	}
	for i, a := range allowed {
		if !used[i] {
			t.Errorf("allowed phrase %q (%s) is used nowhere; drop it", a.phrase, a.why)
		}
	}
}

// TestHintsSuggestNoSubstitutes holds every message and hint lfm prints to
// the mechanism and the measured gap: a finding never proposes a font, a
// color or an example value to take instead.
func TestHintsSuggestNoSubstitutes(t *testing.T) {
	usedHints := map[int]bool{}
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// stock holds the defaults lfm detects, and the tests' own fixtures,
		// the theme variants among them, name fonts and colors.
		if d.IsDir() && (d.Name() == "testdata" || d.Name() == "stock" || d.Name() == "testproject") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "theme/variants.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			// Prose a person reads: several words, not code, markup or data.
			if err != nil || len(strings.Fields(s)) < 5 || strings.ContainsAny(s, "<>{}") && !strings.Contains(s, "<family") {
				return true
			}
		finding:
			for _, p := range plants(s, map[int]bool{}) {
				if strings.HasPrefix(p, "style word") {
					continue
				}
				for i, a := range allowedHints {
					if strings.Contains(s, a.phrase) && strings.HasPrefix(p, "example value") {
						usedHints[i] = true
						continue finding
					}
				}
				t.Errorf("%s: %s", path, p)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range allowedHints {
		if !usedHints[i] {
			t.Errorf("allowed hint %q (%s) is used nowhere; drop it", a.phrase, a.why)
		}
	}
}
