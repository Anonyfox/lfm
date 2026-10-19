package deliver

import (
	"bytes"
	"testing"

	"golang.org/x/net/html"
)

func TestPaths(t *testing.T) {
	for range 50 {
		if p := NewPath(); !ValidPath(p) {
			t.Fatalf("generated path %q is invalid", p)
		}
	}
	for _, bad := range []string{"", "book", "abc", "UPPERCASEUPPERCASE12", "has space in it 1234", "-leading-hyphen-1234", "double--hyphen-12345"} {
		if ValidPath(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSlugSpellsOutLetters(t *testing.T) {
	for title, want := range map[string]string{"The Weekend Starter: Keep It Alive!": "the-weekend-starter-keep-it-alive", "Größer leben": "groesser-leben", "Café crème": "cafe-creme", "Łódź": "lodz", "???": "file"} {
		if got := Slug(title, "file"); got != want {
			t.Errorf("Slug(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestPageNeverTravels(t *testing.T) {
	for _, files := range [][]File{nil, {{Name: "a.pdf", Label: "PDF", Size: 1234}}} {
		page := Page(Download{Title: "T", Heading: "Download: T", Files: files, Pending: "being prepared", Thanks: "Thanks", Keep: "Keep"})
		if _, err := html.Parse(bytes.NewReader(page)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(page, []byte(`content="noindex, nofollow"`)) || !bytes.Contains(page, []byte(`no-referrer`)) {
			t.Fatal("download page is indexable or leaks its address")
		}
		if (files == nil) != bytes.Contains(page, []byte("being prepared")) {
			t.Fatal("pending state")
		}
	}
}
