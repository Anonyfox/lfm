package document

import (
	"regexp"
	"strings"
)

// themeXML is the Word theme of a document.
func themeXML(m metrics) string { return m.Theme.DrawingML() }

var (
	colorAttr = regexp.MustCompile(`w:color w:val="([0-9A-F]{6})"`)
	borderTag = regexp.MustCompile(`(<w:(?:top|left|bottom|right|insideH|insideV)\b[^>]*?)w:color="([0-9A-F]{6})"`)
	fillAttr  = regexp.MustCompile(`w:fill="([0-9A-F]{6})"`)
	fontsTag  = regexp.MustCompile(`<w:rFonts w:ascii="([^"]*)" w:hAnsi="[^"]*"`)
)

// themed links a part's colors and fonts to the theme, keeping every
// explicit value: the accent is accent1, the ink text1, the muted color
// text2, the tint accent1 at a tenth, the heading family the major font and
// the text family the minor one.
func themed(xml string, m metrics) string {
	names := map[string]string{hexColor(m.Ink): "text1", hexColor(m.Muted): "text2", hexColor(m.Accent): "accent1"}
	xml = colorAttr.ReplaceAllStringFunc(xml, func(s string) string {
		hex := colorAttr.FindStringSubmatch(s)[1]
		if n, ok := names[hex]; ok {
			return s + ` w:themeColor="` + n + `"`
		}
		return s
	})
	xml = borderTag.ReplaceAllStringFunc(xml, func(s string) string {
		g := borderTag.FindStringSubmatch(s)
		if n, ok := names[g[2]]; ok {
			return s + ` w:themeColor="` + n + `"`
		}
		return s
	})
	if hex := hexColor(m.Tint); hex == hexColor(tint(m.Accent, 0.9)) {
		xml = fillAttr.ReplaceAllStringFunc(xml, func(s string) string {
			if fillAttr.FindStringSubmatch(s)[1] == hex {
				return s + ` w:themeFill="accent1" w:themeFillTint="1A"`
			}
			return s
		})
	}
	return fontsTag.ReplaceAllStringFunc(xml, func(s string) string {
		switch fontsTag.FindStringSubmatch(s)[1] {
		case m.Heading:
			if m.Heading == m.Text {
				return s + ` w:asciiTheme="minorHAnsi" w:hAnsiTheme="minorHAnsi"`
			}
			return s + ` w:asciiTheme="majorHAnsi" w:hAnsiTheme="majorHAnsi"`
		case m.Text:
			return s + ` w:asciiTheme="minorHAnsi" w:hAnsiTheme="minorHAnsi"`
		}
		return s
	})
}

// separators are a language's decimal symbol and list separator, which
// Word's fields and formulas use.
func separators(lang string) (decimal, list string) {
	base, _, _ := strings.Cut(strings.ToLower(lang), "-")
	switch base {
	case "de", "fr", "es", "it", "pt", "nl", "da", "nb", "sv", "fi", "pl", "cs", "ru", "tr":
		return ",", ";"
	}
	return ".", ","
}
