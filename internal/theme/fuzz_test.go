package theme

import (
	"regexp"
	"testing"

	"github.com/Anonyfox/lfm/internal/concept"
)

var themeColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Property: any Look section parses, findings land inside it, and the
// theme it resolves to writes well-formed DrawingML naming its fonts.
func FuzzParseLook(f *testing.F) {
	f.Add("- Theme: Warm\n- Accent: #1F4FD1\nText font: Merriweather\n- Heading font: Montserrat")
	f.Add("- Theme: baroque\n- Accent: blue")
	f.Add("Text font: Fira & \"Sans\"\nHeading font: A<B")
	f.Add("Text font: Lora\nParagraphs: indented\nTables: grid\nHeadings: regular\nHeading color: accent\nText size: 11pt\nLine height: 1.5\nHeading scale: 1.2\nOrnament: ❦")
	f.Add("Text size: -1\nLine height: NaN\nHeading scale: 1e9\nOrnament: \nTables: ")
	f.Fuzz(func(t *testing.T, body string) {
		d, _ := concept.ParseFormat("# Documents: Test\n\n## Look\n\n"+body+"\n", "# Documents: ", "<name>", []concept.SectionSpec{{Name: "Look"}})
		s := d.Sections["Look"]
		l := ParseLook(s, func(line int, message, hint string) {
			last := s.Line
			for _, l := range s.Lines {
				last = max(last, l.N)
			}
			if line < s.Line || line > last {
				t.Fatalf("finding on line %d, outside the Look section at %d to %d", line, s.Line, last)
			}
		})
		m := l.Resolve()
		if !themeColor.MatchString(m.Accent) || !themeColor.MatchString(m.Tint) {
			t.Fatalf("accent %q, tint %q", m.Accent, m.Tint)
		}
		checkDrawingML(t, m)
	})
}

// Property: a tint of any text is a color, and contrast is a ratio from 1
// to 21.
func FuzzTintContrast(f *testing.F) {
	f.Add("#1F4FD1", "#FFFFFF", 0.35)
	f.Add("zzzzzz", "", 1.0)
	f.Add("#ééé", "#12345", 0.0)
	f.Fuzz(func(t *testing.T, a, b string, share float64) {
		if share >= 0 && share <= 1 {
			if c := Tint(a, share); !themeColor.MatchString(c) {
				t.Fatalf("Tint(%q, %v) = %q", a, share, c)
			}
		}
		if c := Contrast(a, b); !(c >= 1 && c <= 21.0001) || c != Contrast(b, a) {
			t.Fatalf("Contrast(%q, %q) = %v", a, b, c)
		}
	})
}
