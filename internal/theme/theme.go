// Package theme is the look lfm gives every editable file of a product:
// the faces, colors and typesetting a concept's Look decides, on a quiet
// base for what it leaves open, the color math its checks need, and the
// DrawingML theme part Word and PowerPoint read, so a set's documents and
// decks match.
package theme

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/color"
	"github.com/Anonyfox/lfm/internal/concept"
)

// Theme is the look of every editable file of a product: the tokens a
// concept's Look decides and the compiler turns into Word and PowerPoint
// styles, so the documents of a set match and nothing is formatted by hand.
type Theme struct {
	Text, Heading string  // font families the concept names
	Base          float64 // body size in pt
	Leading       float64 // body line height as a multiple of the size
	Ratio         float64 // heading scale
	Indent        bool    // paragraphs separate by a first-line indent instead of space
	HeadingBold   bool
	HeadingAccent bool   // headings take the accent; otherwise the ink
	Table         string // booktabs, hairline or grid
	Ornament      string // the section break glyph
	Ink, Muted    string
	Accent, Tint  string
	Stroke        string // hairlines and table rules
}

// Base is the typesetting a Look starts from. It holds no faces and no
// accent of its own: a concept names its families, and a set without an
// accent stays in its ink.
var Base = Theme{Base: 10.5, Leading: 1.42, Ratio: 1.25, HeadingBold: true, Table: "booktabs", Ornament: "·",
	Ink: "#1E1F22", Muted: "#5B5F66", Accent: "#1E1F22", Tint: Tint("#1E1F22", 0.92), Stroke: "#C9CCD1"}

// Families are the font families a theme sets, each once, text first.
func Families(t Theme) []string {
	switch {
	case t.Text == "":
		return nil
	case t.Heading == "" || t.Heading == t.Text:
		return []string{t.Text}
	}
	return []string{t.Text, t.Heading}
}

// Contrast is the WCAG 2.1 contrast ratio of two #rrggbb colors.
func Contrast(a, b string) float64 {
	ca, _ := color.ParseHex(a)
	cb, _ := color.ParseHex(b)
	return color.Contrast(ca, cb)
}

// Tint mixes a color with white: share 0 is the color, 1 white.
func Tint(hex string, share float64) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return "#FFFFFF"
	}
	out := "#"
	for i := 0; i < 6; i += 2 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		out += fmt.Sprintf("%02X", int(math.Round(float64(v)+(255-float64(v))*share)))
	}
	return out
}

// Shade mixes a color with black: share 0 is the color, 1 black.
func Shade(hex string, share float64) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return "#000000"
	}
	out := "#"
	for i := 0; i < 6; i += 2 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		out += fmt.Sprintf("%02X", int(math.Round(float64(v)*(1-share))))
	}
	return out
}

// DrawingML is the theme part of a .docx or .pptx: the palette and the two
// font families, so Word's Design tab or PowerPoint's Variants recolor or
// refont the whole file at once. Apps without themes read the explicit
// values every style keeps.
func (m Theme) DrawingML() string {
	c := Hex
	// Word and PowerPoint offer six accents; all come from the set's own colors.
	accents := []string{m.Accent, Tint(m.Accent, 0.35), m.Muted, Shade(m.Accent, 0.35), Tint(m.Muted, 0.4), m.Ink}
	var acc strings.Builder
	for i, a := range accents {
		fmt.Fprintf(&acc, `<a:accent%d><a:srgbClr val="%s"/></a:accent%d>`, i+1, c(a), i+1)
	}
	line := func(w int) string {
		return fmt.Sprintf(`<a:ln w="%d" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/><a:miter lim="800000"/></a:ln>`, w)
	}
	solid := `<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`
	return `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="lfm"><a:themeElements>` +
		fmt.Sprintf(`<a:clrScheme name="lfm"><a:dk1><a:srgbClr val="%s"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1><a:dk2><a:srgbClr val="%s"/></a:dk2><a:lt2><a:srgbClr val="%s"/></a:lt2>%s<a:hlink><a:srgbClr val="%s"/></a:hlink><a:folHlink><a:srgbClr val="%s"/></a:folHlink></a:clrScheme>`,
			c(m.Ink), c(m.Muted), c(m.Tint), acc.String(), c(m.Accent), c(m.Muted)) +
		fmt.Sprintf(`<a:fontScheme name="lfm"><a:majorFont><a:latin typeface="%s"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont><a:minorFont><a:latin typeface="%s"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont></a:fontScheme>`,
			html.EscapeString(m.Heading), html.EscapeString(m.Text)) +
		`<a:fmtScheme name="lfm"><a:fillStyleLst>` + solid + solid + solid + `</a:fillStyleLst><a:lnStyleLst>` + line(6350) + line(12700) + line(19050) +
		`</a:lnStyleLst><a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst><a:bgFillStyleLst>` +
		solid + solid + solid + `</a:bgFillStyleLst></a:fmtScheme></a:themeElements><a:objectDefaults/><a:extraClrSchemeLst/></a:theme>`
}

// Hex is a #rrggbb color as OOXML writes it: RRGGBB.
func Hex(c string) string { return strings.ToUpper(strings.TrimPrefix(c, "#")) }

// Half rounds a size to half points, as Word and PowerPoint store sizes.
func Half(pt float64) float64 { return math.Round(pt*2) / 2 }

// ColorProblems are the colors of a theme that do not read on white or
// on its tint.
func (m Theme) ColorProblems() []string {
	var out []string
	if c := Contrast(m.Ink, "#FFFFFF"); c < 7 {
		out = append(out, fmt.Sprintf("The ink %s has a contrast of %.1f:1 on white; body text needs 7:1.", m.Ink, c))
	}
	if c := Contrast(m.Muted, "#FFFFFF"); c < 4.5 {
		out = append(out, fmt.Sprintf("The muted color %s has a contrast of %.1f:1 on white; small text needs 4.5:1.", m.Muted, c))
	}
	if c := Contrast(m.Accent, "#FFFFFF"); c < 4.5 {
		out = append(out, fmt.Sprintf("The accent %s has a contrast of %.1f:1 on white; headings and links in it need 4.5:1. Darken it.", m.Accent, c))
	}
	if c := Contrast(m.Ink, m.Tint); c < 7 {
		out = append(out, fmt.Sprintf("Text on the tint %s has a contrast of %.1f:1; boxes and table headers need 7:1. Lighten the tint.", m.Tint, c))
	}
	return out
}

// Look is a concept's Look section: the faces, the accent and whatever
// typesetting the concept decides; what it leaves out comes from Base.
type Look struct {
	TextFont, HeadFont, Accent          string
	Paragraphs, Tables, Headings, Color string // spaced|indented, booktabs|hairline|grid, bold|regular, ink|accent
	TextSize, LineHeight, HeadingScale  float64
	Ornament                            string
}

var hexColor6 = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// lookChoices are the Look lines that pick a typesetting mechanism, with
// the values each takes; the first is Base's.
var lookChoices = []struct {
	label  string
	values []string
	set    func(*Look, string)
}{
	{"Paragraphs", []string{"spaced", "indented"}, func(l *Look, v string) { l.Paragraphs = v }},
	{"Tables", []string{"booktabs", "hairline", "grid"}, func(l *Look, v string) { l.Tables = v }},
	{"Headings", []string{"bold", "regular"}, func(l *Look, v string) { l.Headings = v }},
	{"Heading color", []string{"ink", "accent"}, func(l *Look, v string) { l.Color = v }},
}

// lookSizes are the Look lines that set a quantity, with its range.
var lookSizes = []struct {
	label, unit string
	low, high   float64
	set         func(*Look, float64)
}{
	{"Text size", "pt", 8, 14, func(l *Look, v float64) { l.TextSize = v }},
	{"Line height", "times the text size", 1.15, 1.9, func(l *Look, v float64) { l.LineHeight = v }},
	{"Heading scale", "times the size below", 1.05, 1.6, func(l *Look, v float64) { l.HeadingScale = v }},
}

// ParseLook reads a concept's Look section; what is wrong goes to add.
func ParseLook(s *concept.Section, add func(line int, message, hint string)) Look {
	var l Look
	if s == nil {
		return l
	}
	if _, line := s.Field("Theme"); line > 0 {
		add(line, "Look's \"Theme:\" is gone: a concept decides its faces and typesetting itself.",
			"Name the families in Text font: and Heading font:, the accent in Accent:, and where the base does not fit the product, Paragraphs:, Tables:, Headings:, Heading color:, Text size:, Line height:, Heading scale: or Ornament:.")
	}
	font := func(label string) string {
		v, _ := s.Field(label)
		if !utf8.ValidString(v) || strings.IndexFunc(v, unicode.IsControl) >= 0 {
			add(s.Line, fmt.Sprintf("Look's \"%s:\" %q is not a font family name.", label, v), "Name the family exactly as its font files call themselves.")
			return ""
		}
		return v
	}
	l.TextFont, l.HeadFont = font("Text font"), font("Heading font")
	if v, _ := s.Field("Text font"); strings.TrimSpace(v) == "" {
		add(s.Line, "Look names no \"Text font:\".", "Name the family the concept chose for running text, exactly as its font files call themselves; Heading font: names a second family when headings use one.")
	}
	if v, line := s.Field("Accent"); line > 0 {
		l.Accent = v
		if !hexColor6.MatchString(l.Accent) {
			add(line, fmt.Sprintf("Look's \"Accent:\" %q is not a #rrggbb color.", l.Accent), "Give the accent the concept chose as #rrggbb, or leave it out for a set in its ink.")
			l.Accent = ""
		}
	}
	for _, c := range lookChoices {
		v, line := s.Field(c.label)
		if line == 0 {
			continue
		}
		if v = strings.ToLower(strings.TrimSpace(v)); !slices.Contains(c.values, v) {
			add(line, fmt.Sprintf("Look's \"%s:\" %q is not one of %s.", c.label, v, strings.Join(c.values, ", ")), "Leave it out for "+c.values[0]+", or name one of the others.")
			continue
		}
		c.set(&l, v)
	}
	for _, z := range lookSizes {
		v, line := s.Field(z.label)
		if line == 0 {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "pt")), 64)
		if err != nil || n < z.low || n > z.high {
			add(line, fmt.Sprintf("Look's \"%s:\" %q is not between %g and %g %s.", z.label, v, z.low, z.high, z.unit), "Give a number in that range, or leave it out for the base.")
			continue
		}
		z.set(&l, n)
	}
	if v, line := s.Field("Ornament"); line > 0 {
		if g := strings.TrimSpace(v); g == "" || utf8.RuneCountInString(g) > 2 || strings.ContainsAny(g, " \t") {
			add(line, fmt.Sprintf("Look's \"Ornament:\" %q is not one glyph.", v), "Give the one character a section break shows, or leave it out for the base.")
		} else {
			l.Ornament = g
		}
	}
	return l
}

// Resolve is the Look on the base: the concept's faces and accent, and its
// typesetting where it decides any.
func (l Look) Resolve() Theme {
	t := Base
	t.Text, t.Heading = l.TextFont, l.HeadFont
	if t.Heading == "" {
		t.Heading = t.Text
	}
	if l.Accent != "" {
		t.Accent = l.Accent
		t.Tint = Tint(l.Accent, 0.9)
	}
	t.Indent = l.Paragraphs == "indented"
	if l.Tables != "" {
		t.Table = l.Tables
	}
	t.HeadingBold = l.Headings != "regular"
	t.HeadingAccent = l.Color == "accent"
	if l.TextSize > 0 {
		t.Base = l.TextSize
	}
	if l.LineHeight > 0 {
		t.Leading = l.LineHeight
	}
	if l.HeadingScale > 0 {
		t.Ratio = l.HeadingScale
	}
	if l.Ornament != "" {
		t.Ornament = l.Ornament
	}
	return t
}
