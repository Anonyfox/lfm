// Package deliver holds what every deliverable a page sells shares: the
// unlisted folder its files are served from, file names, and the download
// page buyers land on after paying.
package deliver

import (
	"crypto/rand"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"

	"github.com/Anonyfox/lfm/internal/term"
	"golang.org/x/text/unicode/norm"
)

// MinPathLength is the shortest unlisted download folder name: the path is
// the only lock on the files.
const MinPathLength = 16

var pathPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// ValidPath reports whether p can serve as the unlisted download folder.
func ValidPath(p string) bool {
	return len(p) >= MinPathLength && pathPattern.MatchString(p) && !strings.Contains(p, "--")
}

// NewPath returns a fresh random download path.
func NewPath() string {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// Slug is a file name for a title: lowercase ASCII, accents folded and the
// letters that do not decompose spelled out, so "Größer leben" is
// groesser-leben. fallback names a title without letters.
func Slug(title, fallback string) string {
	var b strings.Builder
	dash := false
	folded := strings.NewReplacer("ß", "ss", "ä", "ae", "ö", "oe", "ü", "ue", "æ", "ae", "œ", "oe", "ø", "o", "ł", "l", "đ", "d", "þ", "th").Replace(strings.ToLower(title))
	for _, r := range norm.NFD.String(folded) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return fallback
	}
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

// Link is an address on the download page, such as a Google Sheets copy link.
type Link struct{ Label, URL, Note string }

// File is one download on the page.
type File struct {
	Name, Label, Note string
	Size              int
}

// Download is the unlisted page buyers land on after paying.
type Download struct {
	Language, Title, Subtitle, Accent string
	Heading                           string // the page title, e.g. "Download: <title>"
	Links                             []Link // shown before the files
	Files                             []File
	Thanks                            string // above the files
	Pending                           string // instead of the files while they are not ready
	Keep                              string // below the files
}

// Page renders the download page: marked noindex and sending no referrer, so
// the address does not travel.
func Page(d Download) []byte {
	if d.Language == "" {
		d.Language = "en"
	}
	if d.Accent == "" {
		d.Accent = "#222222"
	}
	esc := html.EscapeString
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html>
<html lang="%s"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow"><meta name="referrer" content="no-referrer">
<title>%s</title>
<style>
body { margin: 0; font: 17px/1.55 system-ui, sans-serif; color: #1a1a1a; background: #fafaf8; }
main { max-width: 34rem; margin: 0 auto; padding: 3rem 1.25rem; }
h1 { font-size: 1.7rem; line-height: 1.2; margin: 0 0 0.3rem; }
.sub { margin: 0 0 2rem; color: #4a4a4a; }
ul { list-style: none; padding: 0; margin: 0 0 2rem; }
li { margin: 0 0 1rem; }
a.file { display: inline-block; padding: 0.7rem 1rem; color: #fff; background: %s; text-decoration: none; font-weight: 600; }
a.file:focus-visible { outline: 3px solid #1a1a1a; outline-offset: 2px; }
.note { display: block; margin-top: 0.3rem; font-size: 0.9rem; color: #4a4a4a; }
</style></head><body><main>
<h1>%s</h1>
`, esc(d.Language), esc(d.Heading), CSSValue(d.Accent), esc(d.Title))
	if d.Subtitle != "" {
		fmt.Fprintf(&b, "<p class=\"sub\">%s</p>\n", esc(d.Subtitle))
	}
	if len(d.Links) > 0 {
		b.WriteString("<ul>\n")
		for _, l := range d.Links {
			fmt.Fprintf(&b, "<li><a class=\"file\" href=\"%s\" rel=\"noreferrer\">%s</a><span class=\"note\">%s</span></li>\n", esc(l.URL), esc(l.Label), esc(l.Note))
		}
		b.WriteString("</ul>\n")
	}
	if len(d.Files) == 0 {
		fmt.Fprintf(&b, "<p>%s</p>\n", esc(d.Pending))
	} else {
		fmt.Fprintf(&b, "<p>%s</p>\n<ul>\n", esc(d.Thanks))
		for _, f := range d.Files {
			fmt.Fprintf(&b, "<li><a class=\"file\" href=\"%s\" download>%s (%s)</a><span class=\"note\">%s</span></li>\n", esc(f.Name), esc(f.Label), term.Bytes(f.Size), esc(f.Note))
		}
		fmt.Fprintf(&b, "</ul>\n<p>%s</p>\n", esc(d.Keep))
	}
	b.WriteString("</main></body></html>\n")
	return []byte(b.String())
}

// CSSValue strips what could end a CSS declaration from a configured value.
func CSSValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ';' || r == '{' || r == '}' || r == '<' || r == '>' {
			return -1
		}
		return r
	}, s)
}

// StampJPEG records key in a JPEG comment right after the start marker, the
// way a stamped PDF records its inputs, so a later compile can reuse the
// image while its inputs are unchanged.
func StampJPEG(jpg []byte, key string) []byte {
	comment := "lfm-build " + key
	if len(jpg) < 2 || jpg[0] != 0xff || jpg[1] != 0xd8 || len(comment)+2 > 0xffff {
		return jpg
	}
	segment := []byte{0xff, 0xfe, byte((len(comment) + 2) >> 8), byte(len(comment) + 2)}
	out := append(append(append([]byte{0xff, 0xd8}, segment...), comment...), jpg[2:]...)
	return out
}

// JPEGStamp returns the key StampJPEG recorded, or "".
func JPEGStamp(jpg []byte) string {
	if len(jpg) < 6 || jpg[0] != 0xff || jpg[1] != 0xd8 || jpg[2] != 0xff || jpg[3] != 0xfe {
		return ""
	}
	n := int(jpg[4])<<8 | int(jpg[5])
	if n < 2 || 4+n > len(jpg) {
		return ""
	}
	comment, ok := strings.CutPrefix(string(jpg[6:4+n]), "lfm-build ")
	if !ok {
		return ""
	}
	return comment
}

var googleLink = regexp.MustCompile(`^https://docs\.google\.com/(document|presentation|spreadsheets)/d/([A-Za-z0-9_-]{20,})`)

// CopyLink turns the address of a shared Google Docs, Slides or Sheets
// master of kind (document, presentation, spreadsheets) into the link
// that makes the buyer's own copy; "" for any other address.
func CopyLink(url, kind string) string {
	m := googleLink.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil || m[1] != kind {
		return ""
	}
	return "https://docs.google.com/" + kind + "/d/" + m[2] + "/copy"
}

// FileName is a file name buyers see: a title, safe on every system.
func FileName(title, ext string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return -1
		}
		return r
	}, title)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		clean = "Document"
	}
	return clean + "." + ext
}

// Image is a listing image a deliverable ships: its file name and JPEG.
type Image struct {
	Name string // 01-cover.jpg
	Data []byte
}
