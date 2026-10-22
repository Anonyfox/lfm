package theme

import (
	"encoding/xml"
	"io"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/concept"
)

// variant resolves one of the test variants.
func variant(t *testing.T, name string) Theme {
	t.Helper()
	l, found := look(t, Variants[name])
	if len(found) > 0 {
		t.Fatalf("%s: %v", name, found)
	}
	return l.Resolve()
}

func TestVariantsReadOnWhiteAndTint(t *testing.T) {
	tables := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(Variants)) {
		m := variant(t, name)
		if p := m.ColorProblems(); len(p) > 0 {
			t.Errorf("%s: %v", name, p)
		}
		if m.Text == "" || m.Heading == "" || m.Base <= 0 || m.Leading <= 1 || m.Ratio <= 1 || m.Ornament == "" {
			t.Errorf("%s is incomplete: %+v", name, m)
		}
		tables[m.Table] = true
	}
	// Together the variants use every mechanism, so the tests that build each reach all of them.
	if len(tables) != 3 {
		t.Errorf("tables %v", tables)
	}
}

func TestBaseHoldsNoFaces(t *testing.T) {
	if Base.Text != "" || Base.Heading != "" || Base.Accent != Base.Ink {
		t.Fatalf("base %+v", Base)
	}
}

func TestContrastKnownValues(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want float64
	}{
		{"#000000", "#FFFFFF", 21},
		{"#FFFFFF", "#000000", 21},
		{"#777777", "#777777", 1},
		{"777777", "#ffffff", 4.478},
		{"#FF0000", "#FFFFFF", 3.998},
	} {
		if got := Contrast(tc.a, tc.b); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("Contrast(%s, %s) = %.3f, want %.3f", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestContrastTreatsMalformedColorAsBlack(t *testing.T) {
	if got := Contrast("#GGGGGG", "#FFFFFF"); math.Abs(got-21) > 1e-6 {
		t.Fatalf("contrast %v", got)
	}
}

func TestTintEndpoints(t *testing.T) {
	for _, tc := range []struct {
		in    string
		share float64
		want  string
	}{
		{"#1f4fd1", 0, "#1F4FD1"},
		{"#1F4FD1", 1, "#FFFFFF"},
		{"#000000", 0.5, "#808080"},
		{"336699", 0.9, "#EBF0F5"},
		{"#abc", 0.5, "#FFFFFF"},
	} {
		if got := Tint(tc.in, tc.share); got != tc.want {
			t.Errorf("Tint(%s, %v) = %s, want %s", tc.in, tc.share, got, tc.want)
		}
	}
}

func TestHexAndHalf(t *testing.T) {
	if got := Hex("#1f4fd1"); got != "1F4FD1" {
		t.Errorf("Hex %q", got)
	}
	if got := Hex("ABCDEF"); got != "ABCDEF" {
		t.Errorf("Hex without # %q", got)
	}
	for in, want := range map[float64]float64{10: 10, 10.2: 10, 10.25: 10.5, 10.74: 10.5, 10.76: 11, 13.333: 13.5} {
		if got := Half(in); got != want {
			t.Errorf("Half(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestColorProblemsNameEachWeakColor(t *testing.T) {
	m := Base
	m.Ink, m.Muted, m.Accent, m.Tint = "#999999", "#BBBBBB", "#FFCC00", "#999999"
	p := m.ColorProblems()
	if len(p) != 4 {
		t.Fatalf("problems %v", p)
	}
	for i, want := range []string{"ink #999999", "muted color #BBBBBB", "accent #FFCC00", "tint #999999"} {
		if !strings.Contains(p[i], want) {
			t.Errorf("problem %d %q lacks %q", i, p[i], want)
		}
	}
}

// look parses a Look section from a concept-style file.
func look(t *testing.T, body string) (Look, []string) {
	t.Helper()
	d, problems := concept.ParseFormat("# Documents: Test\n\n## Look\n\n"+body+"\n", "# Documents: ", "<name>", []concept.SectionSpec{{Name: "Look"}})
	if len(problems) > 0 {
		t.Fatalf("concept problems %+v", problems)
	}
	var found []string
	l := ParseLook(d.Sections["Look"], func(line int, message, hint string) {
		if line < 3 {
			t.Errorf("finding at line %d, before the Look section", line)
		}
		found = append(found, message)
	})
	return l, found
}

func TestParseLookReadsEveryDecision(t *testing.T) {
	l, found := look(t, "- Text font: Merriweather\n- Heading font: Montserrat\n- Accent: #1F4FD1\nParagraphs: Indented\nTables: grid\nHeadings: regular\nHeading color: accent\nText size: 11pt\nLine height: 1.5\nHeading scale: 1.2\nOrnament: ❦")
	if len(found) > 0 {
		t.Fatal(found)
	}
	want := Look{TextFont: "Merriweather", HeadFont: "Montserrat", Accent: "#1F4FD1", Paragraphs: "indented", Tables: "grid", Headings: "regular", Color: "accent", TextSize: 11, LineHeight: 1.5, HeadingScale: 1.2, Ornament: "❦"}
	if l != want {
		t.Fatalf("look %+v", l)
	}
	got := l.Resolve()
	if got.Text != "Merriweather" || got.Heading != "Montserrat" || !got.Indent || got.Table != "grid" || got.HeadingBold || !got.HeadingAccent || got.Base != 11 || got.Leading != 1.5 || got.Ratio != 1.2 || got.Ornament != "❦" || got.Tint != Tint("#1F4FD1", 0.9) {
		t.Fatalf("resolved %+v", got)
	}
}

func TestParseLookNeedsTheConceptsFaces(t *testing.T) {
	for _, body := range []string{"- Accent: #123456", "- Text font: \n- Accent: #123456"} {
		if _, found := look(t, body); len(found) != 1 || !strings.Contains(found[0], `no "Text font:"`) {
			t.Fatalf("%q: found %v", body, found)
		}
	}
	l, found := look(t, "- Text font: Lora")
	if len(found) > 0 || l.Resolve().Heading != "Lora" || l.Resolve().Accent != Base.Ink {
		t.Fatalf("one family, no accent: %+v %v", l.Resolve(), found)
	}
}

func TestParseLookTurnsAThemeIntoDecisions(t *testing.T) {
	_, found := look(t, "- Theme: warm\n- Text font: Lora")
	if len(found) != 1 || !strings.Contains(found[0], `"Theme:" is gone`) {
		t.Fatalf("found %v", found)
	}
}

func TestParseLookRejectsWhatIsNotADecision(t *testing.T) {
	for line, want := range map[string]string{
		"Accent: blue":        "not a #rrggbb color",
		"Accent: #12345":      "not a #rrggbb color",
		"Paragraphs: wavy":    "not one of spaced, indented",
		"Tables: zebra":       "not one of booktabs, hairline, grid",
		"Headings: italic":    "not one of bold, regular",
		"Heading color: blue": "not one of ink, accent",
		"Text size: 30":       "not between 8 and 14",
		"Line height: tall":   "not between 1.15 and 1.9",
		"Heading scale: 2":    "not between 1.05 and 1.6",
		"Ornament: * * *":     "not one glyph",
		"Text font: A\u0000B": "not a font family name",
	} {
		body := "- Text font: Lora\n- " + line
		if strings.HasPrefix(line, "Text font:") {
			body = "- " + line
		}
		l, found := look(t, body)
		if len(found) != 1 || !strings.Contains(found[0], want) {
			t.Errorf("%s: look %+v, found %v", line, l, found)
		}
	}
}

func TestParseLookWithoutSection(t *testing.T) {
	called := false
	if l := ParseLook(nil, func(int, string, string) { called = true }); l != (Look{}) || called {
		t.Fatalf("look %+v, reported %v", l, called)
	}
}

func TestDrawingMLIsWellFormedAndNamesFonts(t *testing.T) {
	for _, name := range slices.Sorted(maps.Keys(Variants)) {
		checkDrawingML(t, variant(t, name))
	}
	m := Base
	m.Text, m.Heading = `Fira & "Sans"`, "A<B"
	checkDrawingML(t, m)
}

func checkDrawingML(t *testing.T, m Theme) {
	t.Helper()
	x := m.DrawingML()
	dec := xml.NewDecoder(strings.NewReader(x))
	var major, minor, ink string
	var accents int
	var in string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: %v\n%s", m.Text, err, x)
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attr := func(name string) string {
			for _, a := range el.Attr {
				if a.Name.Local == name {
					return a.Value
				}
			}
			return ""
		}
		switch el.Name.Local {
		case "majorFont", "minorFont", "dk1":
			in = el.Name.Local
		case "latin":
			if in == "majorFont" {
				major = attr("typeface")
			} else if in == "minorFont" {
				minor = attr("typeface")
			}
		case "srgbClr":
			if in == "dk1" {
				ink, in = attr("val"), ""
			}
		}
		if strings.HasPrefix(el.Name.Local, "accent") {
			accents++
		}
		if el.Name.Space != "http://schemas.openxmlformats.org/drawingml/2006/main" {
			t.Errorf("%s: element %s outside the DrawingML namespace", m.Text, el.Name.Local)
		}
	}
	if major != m.Heading || minor != m.Text {
		t.Errorf("%s: fonts %q / %q, want %q / %q", m.Text, major, minor, m.Heading, m.Text)
	}
	if ink != Hex(m.Ink) {
		t.Errorf("%s: dk1 %q, want %q", m.Text, ink, Hex(m.Ink))
	}
	if accents != 6 {
		t.Errorf("%s: %d accents", m.Text, accents)
	}
	if !strings.Contains(x, `<a:accent1><a:srgbClr val="`+Hex(m.Accent)+`"/>`) {
		t.Errorf("%s: accent1 is not the accent", m.Text)
	}
}
