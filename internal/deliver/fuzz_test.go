package deliver

import (
	"regexp"
	"strings"
	"testing"
)

// Property: a listing.md of any shape parses without panicking, every
// finding on a line of the text, and its plain text has no Markdown bold.
func FuzzShopListing(f *testing.F) {
	f.Add(cleanParts().text(), "7.99 EUR")
	f.Add("# Listing: X\n\n## Description\n## Title\n", "1 USD")
	f.Add("## Price\n\nOpen: 5 EUR\n## AI\n- Text: assisted\n", "")
	f.Fuzz(func(t *testing.T, text, offer string) {
		var o *Price
		if p, ok := ParsePrice(offer); ok {
			o = &p
		}
		lines := strings.Count(text, "\n") + 1
		l := CheckShopListing(text, o, func(line int, message, hint string) {
			if line < 1 || line > lines+1 {
				t.Fatalf("finding on line %d of %d: %s", line, lines, message)
			}
		})
		if strings.Contains(PlainText(l.Description), "**") {
			t.Fatalf("plain text keeps bold: %q", PlainText(l.Description))
		}
	})
}

var slugShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Property: a slug is the fallback or lowercase ASCII words joined by
// single dashes, at most 60 bytes.
func FuzzSlug(f *testing.F) {
	for _, s := range []string{"Größer leben", "", "---", "Ærø Œuvre ŁÓDŹ", strings.Repeat("ab ", 40), "日本語"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, title string) {
		s := Slug(title, "fallback")
		if s != "fallback" && (!slugShape.MatchString(s) || len(s) > 60) {
			t.Fatalf("Slug(%q) = %q", title, s)
		}
	})
}

// Property: a stamped JPEG reads its key back and keeps every byte after
// the start marker; reading a stamp never panics.
func FuzzJPEGStamp(f *testing.F) {
	f.Add([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 2}, "0123456789abcdef")
	f.Add([]byte{0xff, 0xd8, 0xff, 0xfe, 0, 12, 'l', 'f', 'm'}, "")
	f.Add([]byte{0xff, 0xd8}, strings.Repeat("k", 70000))                      // longer than a segment holds: the length wrapped
	f.Add([]byte{0xff, 0xd8}, strings.Repeat("k", 0xffff-2-len("lfm-build "))) // the longest key a segment holds
	f.Add([]byte{0xff, 0xd8}, strings.Repeat("k", 0xffff-1-len("lfm-build "))) // one more
	f.Fuzz(func(t *testing.T, jpg []byte, key string) {
		JPEGStamp(jpg)
		if len(jpg) < 2 || jpg[0] != 0xff || jpg[1] != 0xd8 {
			return
		}
		stamped := StampJPEG(jpg, key)
		// A segment's length counts its two length bytes and stops at 0xffff.
		if len("lfm-build ")+len(key)+2 > 0xffff {
			if string(stamped) != string(jpg) {
				t.Fatal("a key longer than a comment segment was written")
			}
			return
		}
		if got := JPEGStamp(stamped); got != key {
			t.Fatalf("stamped %q, read %q", key, got)
		}
		if !strings.HasSuffix(string(stamped), string(jpg[2:])) {
			t.Fatal("stamping changed the image")
		}
	})
}
