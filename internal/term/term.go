// Package term renders plain, readable terminal output: optional color,
// aligned columns, wrapped paragraphs and human units. Color is used only
// when the writer is a terminal and the environment does not opt out.
package term

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Style renders text attributes as ANSI escapes, or not at all.
type Style struct{ Color bool }

// For picks colors for w: on for terminals, off for pipes and files, NO_COLOR
// and TERM=dumb; FORCE_COLOR turns it on regardless.
func For(w io.Writer) Style {
	if os.Getenv("FORCE_COLOR") != "" {
		return Style{Color: true}
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return Style{}
	}
	f, ok := w.(*os.File)
	if !ok {
		return Style{}
	}
	info, err := f.Stat()
	return Style{Color: err == nil && info.Mode()&os.ModeCharDevice != 0}
}

func (s Style) wrap(code, text string) string {
	if !s.Color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) Bold(text string) string   { return s.wrap("1", text) }
func (s Style) Dim(text string) string    { return s.wrap("2", text) }
func (s Style) Red(text string) string    { return s.wrap("31", text) }
func (s Style) Green(text string) string  { return s.wrap("32", text) }
func (s Style) Yellow(text string) string { return s.wrap("33", text) }
func (s Style) Cyan(text string) string   { return s.wrap("36", text) }

// Width is the column count text is wrapped to.
const Width = 96

// Wrap breaks text into lines of at most width runes, not counting indent,
// which prefixes every continuation line. Words longer than a line stay whole.
func Wrap(text string, width int, indent string) string {
	var out strings.Builder
	line := 0
	for i, word := range strings.Fields(text) {
		n := utf8.RuneCountInString(word)
		if i > 0 {
			if line+1+n > width {
				out.WriteString("\n" + indent)
				line = 0
			} else {
				out.WriteByte(' ')
				line++
			}
		}
		out.WriteString(word)
		line += n
	}
	return out.String()
}

// Pad right-pads text to width visible runes.
func Pad(text string, width int) string {
	if n := utf8.RuneCountInString(text); n < width {
		return text + strings.Repeat(" ", width-n)
	}
	return text
}

// Bytes formats a byte count in binary units.
func Bytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.2f MiB", float64(n)/(1024*1024))
	}
}

// Duration formats a compile-scale duration.
func Duration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "<1 ms"
	case d < 10*time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
}

// Plural returns "1 file", "2 files".
func Plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
