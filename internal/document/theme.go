package document

import (
	"fmt"
	"math"

	"github.com/Anonyfox/lfm/internal/theme"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// Theme, Face and family are the shared look and fonts.
type (
	Theme  = theme.Theme
	Face   = typeface.Face
	family = typeface.Family
)

// Themes are the presets a concept names in Look: Theme.
var Themes = theme.Presets

// ThemeNames lists the presets in a stable order.
func ThemeNames() []string { return theme.Names() }

// Paper is a page size in millimetres.
type Paper struct {
	Key           string
	Width, Height float64
}

// Papers are the sizes documents are built in.
var Papers = map[string]Paper{
	"a4":     {"a4", 210, 297},
	"letter": {"letter", 215.9, 279.4},
}

// metrics are a theme laid out on a paper with real fonts: every size,
// line height and gap in points, the margins in millimetres. All styles
// and both renderers read them, so a .docx and its preview agree.
type metrics struct {
	Theme
	Paper                    Paper
	Small, H3, H2, H1, Title float64 // sizes
	Line, HeadLine           float64 // body and heading leading in pt
	Unit                     float64 // the spacing unit: a quarter of the body leading
	Margin, Top, Bottom      float64 // mm
	TextWidth                float64 // mm
	Measure                  float64 // characters per body line
	text, heading            *family
	// The lines Chrome prints: the design leading, or the font's own line
	// where Word for Windows sets it taller (it reads winAscent and
	// winDescent; Word for Mac and LibreOffice read the typographic
	// metrics). A document that fits Chrome's print fits every app.
	PrintLine, PrintHeadFactor float64
}

var (
	half = theme.Half
	tint = theme.Tint
)

func layoutTheme(t Theme, paper Paper, text, heading *family) metrics {
	m := metrics{Theme: t, Paper: paper, text: text, heading: heading}
	m.Small = math.Max(8, half(t.Base/t.Ratio))
	m.H3 = half(t.Base * math.Pow(t.Ratio, 0.5))
	m.H2 = half(t.Base * t.Ratio)
	m.H1 = half(t.Base * t.Ratio * t.Ratio)
	m.Title = half(t.Base * math.Pow(t.Ratio, 4))
	if m.Title < m.H1+6 {
		m.Title = half(m.H1 * 1.6)
	}
	m.Line = math.Round(t.Base*t.Leading*2) / 2
	m.HeadLine = 1.15
	m.Unit = m.Line / 4
	// The side margins follow from a comfortable measure of about 72
	// characters, between 19 mm (home printers) and 40 mm, so small text
	// on wide paper still reads in lines a reader can follow.
	avg := 0.5
	if text != nil && text.Regular() != nil && text.Regular().AvgChar > 0 {
		avg = text.Regular().AvgChar
	}
	charMM := avg * t.Base * 25.4 / 72
	m.Margin = math.Min(40, math.Max(19, (paper.Width-72*charMM)/2))
	m.Margin = math.Round(m.Margin*2) / 2
	m.Top, m.Bottom = 22, 24
	m.TextWidth = paper.Width - 2*m.Margin
	m.Measure = m.TextWidth / charMM
	natural := func(f *family) float64 {
		if f != nil && f.Regular() != nil && f.Regular().Natural > 0 {
			return f.Regular().Natural
		}
		return 1.2
	}
	m.PrintLine = math.Max(m.Line, t.Base*natural(text))
	m.PrintHeadFactor = math.Max(m.HeadLine, natural(heading))
	return m
}

// themeProblems are the theme's design defects: colors that do not read,
// a scale or size outside what documents bear.
func themeProblems(m metrics) []string {
	out := m.Theme.ColorProblems()
	if m.Measure < 45 || m.Measure > 90 {
		out = append(out, fmt.Sprintf("Body lines hold about %.0f characters on %s; readable text keeps 45 to 90.", m.Measure, m.Paper.Key))
	}
	return out
}
