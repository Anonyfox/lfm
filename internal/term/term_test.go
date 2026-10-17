package term

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestStyleOnlyColorsWhenEnabled(t *testing.T) {
	if got := (Style{}).Red("x"); got != "x" {
		t.Fatalf("plain style colored: %q", got)
	}
	if got := (Style{Color: true}).Red("x"); got != "\x1b[31mx\x1b[0m" {
		t.Fatalf("color style: %q", got)
	}
	if got := (Style{Color: true}).Bold(""); got != "" {
		t.Fatalf("empty text gained escapes: %q", got)
	}
}

func TestForDisablesColorForNonTerminals(t *testing.T) {
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("NO_COLOR", "")
	if For(&bytes.Buffer{}).Color {
		t.Fatal("buffer treated as terminal")
	}
	t.Setenv("FORCE_COLOR", "1")
	if !For(&bytes.Buffer{}).Color {
		t.Fatal("FORCE_COLOR ignored")
	}
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("NO_COLOR", "1")
	if For(&bytes.Buffer{}).Color {
		t.Fatal("NO_COLOR ignored")
	}
}

func TestWrap(t *testing.T) {
	got := Wrap("one two three four five", 9, "  ")
	if got != "one two\n  three\n  four five" {
		t.Fatalf("%q", got)
	}
	if got := Wrap("supercalifragilistic word", 5, ""); got != "supercalifragilistic\nword" {
		t.Fatalf("long word split: %q", got)
	}
	if got := Wrap("  spaced   out  ", 80, ""); got != "spaced out" {
		t.Fatalf("whitespace: %q", got)
	}
}

func FuzzWrapKeepsWordsAndWidth(f *testing.F) {
	f.Add("a quick brown fox", 7)
	f.Add("ünïcödé wörds hère", 4)
	f.Fuzz(func(t *testing.T, text string, width int) {
		if width < 1 || width > 200 || !utf8.ValidString(text) {
			return
		}
		got := Wrap(text, width, "")
		if strings.Join(strings.Fields(got), " ") != strings.Join(strings.Fields(text), " ") {
			t.Fatal("words changed")
		}
		for _, line := range strings.Split(got, "\n") {
			if utf8.RuneCountInString(line) > width && len(strings.Fields(line)) > 1 {
				t.Fatalf("line %q exceeds %d", line, width)
			}
		}
	})
}

func TestUnits(t *testing.T) {
	for n, want := range map[int]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 1536: "1.5 KiB", 3 << 20: "3.00 MiB"} {
		if got := Bytes(n); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", n, got, want)
		}
	}
	for d, want := range map[time.Duration]string{0: "<1 ms", 48 * time.Millisecond: "48 ms", 12 * time.Second: "12.0 s"} {
		if got := Duration(d); got != want {
			t.Errorf("Duration(%v) = %q, want %q", d, got, want)
		}
	}
	if Plural(1, "file") != "1 file" || Plural(2, "file") != "2 files" || Plural(0, "error") != "0 errors" {
		t.Fatal("plural")
	}
	if Pad("ab", 4) != "ab  " || Pad("abcdef", 4) != "abcdef" {
		t.Fatal("pad")
	}
}
