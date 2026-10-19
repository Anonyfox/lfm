// Package theme is the look lfm gives every editable file of a product:
// a handful of presets a concept's Look picks and may recolor or refont,
// the color math its checks need, and the DrawingML theme part Word and
// PowerPoint read, so a set's documents and decks match.
package theme

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/concept"
)

// Theme is the look of every editable file of a product: a handful of tokens a
// concept's Look picks and the compiler turns into Word styles, so the
// documents of a set match and nothing is formatted by hand.
type Theme struct {
	Name          string
	Text, Heading string  // font families; both from Google Fonts
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

// Presets are the themes a concept names in Look: Theme. Each is a
// coherent set; the concept may change its accent and fonts.
var Presets = map[string]Theme{
	"classic": {Name: "classic", Text: "EB Garamond", Heading: "EB Garamond", Base: 11.5, Leading: 1.35, Ratio: 1.25, Indent: true,
		Table: "booktabs", Ornament: "❧", Ink: "#1F1D1A", Muted: "#5E5850", Accent: "#7A2E2E", Tint: "#F4ECE6", Stroke: "#C9C1B6"},
	"modern": {Name: "modern", Text: "Inter", Heading: "Inter", Base: 10, Leading: 1.45, Ratio: 1.2, HeadingBold: true,
		Table: "hairline", Ornament: "·", Ink: "#16181D", Muted: "#565D69", Accent: "#1F4FD1", Tint: "#EAF0FD", Stroke: "#D3D8E0"},
	"editorial": {Name: "editorial", Text: "Source Serif 4", Heading: "Source Sans 3", Base: 10.5, Leading: 1.42, Ratio: 1.333, HeadingBold: true,
		Table: "booktabs", Ornament: "◆", Ink: "#1E1F22", Muted: "#5B5F66", Accent: "#0F5257", Tint: "#E6EFEF", Stroke: "#C9CCD1"},
	"minimal": {Name: "minimal", Text: "IBM Plex Sans", Heading: "IBM Plex Sans", Base: 10, Leading: 1.5, Ratio: 1.125, HeadingBold: true,
		Table: "hairline", Ornament: "—", Ink: "#1B1B1B", Muted: "#5F5F5F", Accent: "#1B1B1B", Tint: "#F2F2F2", Stroke: "#D6D6D6"},
	"warm": {Name: "warm", Text: "Lora", Heading: "Lora", Base: 11, Leading: 1.45, Ratio: 1.25, HeadingAccent: true,
		Table: "booktabs", Ornament: "❦", Ink: "#2B2420", Muted: "#6B5E55", Accent: "#9C4A2C", Tint: "#F6EDE6", Stroke: "#D9CBBF"},
	"technical": {Name: "technical", Text: "Source Sans 3", Heading: "Source Sans 3", Base: 10, Leading: 1.4, Ratio: 1.2, HeadingBold: true,
		Table: "grid", Ornament: "■", Ink: "#1C2128", Muted: "#56606E", Accent: "#2F4A6D", Tint: "#EBEFF4", Stroke: "#C5CDD8"},
}

// Names lists the presets in a stable order.
func Names() []string {
	names := make([]string, 0, len(Presets))
	for n := range Presets {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// Contrast is the WCAG 2.1 contrast ratio of two #rrggbb colors.
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0
	}
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(0) + 0.7152*ch(2) + 0.0722*ch(4)
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

// DrawingML is the theme part of a .docx or .pptx: the palette and the two
// font families, so Word's Design tab or PowerPoint's Variants recolor or
// refont the whole file at once. Apps without themes read the explicit
// values every style keeps.
func (m Theme) DrawingML() string {
	c := Hex
	accents := []string{m.Accent, Tint(m.Accent, 0.35), m.Muted, "#9C6644", "#457B9D", "#81B29A"}
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

// Look is a concept's Look section: the preset and what it changes.
type Look struct {
	Theme, Accent, TextFont, HeadFont string
}

var hexColor6 = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ParseLook reads a concept's Look section; what is wrong goes to add.
func ParseLook(s *concept.Section, add func(line int, message, hint string)) Look {
	var l Look
	if s == nil {
		return l
	}
	l.Theme, _ = s.Field("Theme")
	l.Theme = strings.ToLower(l.Theme)
	if _, ok := Presets[l.Theme]; !ok {
		add(s.Line, fmt.Sprintf("Look has no known \"Theme:\" (got %q).", l.Theme), "Pick one of "+strings.Join(Names(), ", ")+"; the theme sets fonts, sizes, spacing and tables for every file of the set.")
		l.Theme = ""
	}
	if v, line := s.Field("Accent"); line > 0 {
		l.Accent = v
		if !hexColor6.MatchString(l.Accent) {
			add(s.Line, fmt.Sprintf("Look's \"Accent:\" %q is not a #rrggbb color.", l.Accent), "Give the accent as #rrggbb, or leave it out for the theme's.")
			l.Accent = ""
		}
	}
	l.TextFont, _ = s.Field("Text font")
	l.HeadFont, _ = s.Field("Heading font")
	return l
}

// Resolve is the Look's theme with its accent and fonts; editorial when
// the Look names none.
func (l Look) Resolve() Theme {
	t := Presets[l.Theme]
	if t.Name == "" {
		t = Presets["editorial"]
	}
	if l.Accent != "" {
		t.Accent = l.Accent
		t.Tint = Tint(l.Accent, 0.9)
	}
	if l.TextFont != "" {
		t.Text = l.TextFont
	}
	if l.HeadFont != "" {
		t.Heading = l.HeadFont
	} else if l.TextFont != "" && Presets[l.Theme].Heading == Presets[l.Theme].Text {
		t.Heading = l.TextFont
	}
	return t
}
