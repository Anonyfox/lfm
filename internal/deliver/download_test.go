package deliver

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

func TestNewPathIsLongRandomAndUnambiguous(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		p := NewPath()
		if len(p) != 24 || !ValidPath(p) {
			t.Fatalf("path %q", p)
		}
		if strings.ContainsAny(p, "lo01-") {
			t.Fatalf("path %q holds a character that reads like another", p)
		}
		if seen[p] {
			t.Fatalf("path %q drawn twice", p)
		}
		seen[p] = true
	}
}

func TestValidPathNeedsSixteenSafeCharacters(t *testing.T) {
	for p, want := range map[string]bool{
		"abcdefghijklmnop":   true,
		"abcdefghijklmno":    false,
		"my-book-downloads1": true,
		"my-book-downloads-": false,
		"my_book_downloads1": false,
		"my.book.downloads1": false,
		"myböökdownloads123": false,
		"0123456789abcdefgh": true,
	} {
		if got := ValidPath(p); got != want {
			t.Errorf("ValidPath(%q) = %v", p, got)
		}
	}
}

func TestSlugKeepsFileNamesShortAndClean(t *testing.T) {
	for title, want := range map[string]string{
		"  --Hello,   World--  ": "hello-world",
		"Chapter 12: Ærø":        "chapter-12-aero",
		"Smørrebrød & Þing":      "smorrebrod-thing",
		"日本語":                    "notitle",
	} {
		if got := Slug(title, "notitle"); got != want {
			t.Errorf("Slug(%q) = %q, want %q", title, got, want)
		}
	}
	long := Slug(strings.Repeat("word ", 30), "x")
	if len(long) > 60 || strings.HasSuffix(long, "-") || !strings.HasPrefix(long, "word-word") {
		t.Fatalf("long slug %q", long)
	}
}

func TestPageListsCopyLinksBeforeFiles(t *testing.T) {
	w := Words("en")
	page := string(Page(Download{
		Title: "Budget Sheet", Heading: fmt.Sprintf(w.Heading, "Budget Sheet"), Subtitle: "Monthly budget",
		Links:  []Link{{Label: "Make your copy in Google Sheets", URL: "https://docs.google.com/spreadsheets/d/abc/copy", Note: "Needs a Google account"}},
		Files:  []File{{Name: "budget.xlsx", Label: "Excel", Size: 1234, Note: "For Excel and Numbers"}, {Name: "guide.pdf", Label: "Guide", Size: 3 << 20}},
		Thanks: w.Thanks, Keep: w.Keep, Pending: w.Pending,
	}))
	link, first, second := strings.Index(page, "docs.google.com/spreadsheets/d/abc/copy"), strings.Index(page, `href="budget.xlsx" download`), strings.Index(page, `href="guide.pdf" download`)
	if link < 0 || first < link || second < first {
		t.Fatalf("links %d, files %d %d:\n%s", link, first, second, page)
	}
	for _, want := range []string{`<html lang="en">`, "<title>Download: Budget Sheet</title>", "<h1>Budget Sheet</h1>", `<p class="sub">Monthly budget</p>`,
		`rel="noreferrer">Make your copy in Google Sheets</a><span class="note">Needs a Google account</span>`, "Excel (1.2 KiB)", "Guide (3.00 MiB)",
		"For Excel and Numbers", w.Thanks, w.Keep, "background: #222222"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, w.Pending) {
		t.Error("a page with files says they are being prepared")
	}
}

func TestPageSpeaksTheBuyersLanguage(t *testing.T) {
	w := Words("de")
	page := string(Page(Download{Language: "de", Title: "Planer", Heading: fmt.Sprintf(w.Heading, "Planer"), Thanks: w.Thanks, Pending: w.Pending, Keep: w.Keep}))
	if !strings.Contains(page, `<html lang="de">`) || !strings.Contains(page, "Deine Dateien werden gerade vorbereitet") {
		t.Fatalf("German pending page:\n%s", page)
	}
	if strings.Contains(page, "<ul>") || strings.Contains(page, w.Keep) {
		t.Fatal("a pending page lists files")
	}
}

func TestPageEscapesEverythingItShows(t *testing.T) {
	page := string(Page(Download{
		Language: `en"><script>`, Title: "<b>Title</b>", Heading: "</title><script>x()</script>", Subtitle: "Tom & Jerry", Accent: "red;}body{display:none}</style><script>",
		Links: []Link{{Label: "<i>copy</i>", URL: `https://x.test/?a=1&b="2"`, Note: "<note>"}},
		Files: []File{{Name: `a"b<c>.pdf`, Label: "<script>alert(1)</script>", Note: "it's"}},
	}))
	for _, bad := range []string{"<script>", "<b>Title", "<i>copy", "<note>", `"b<c>`, "display:none}"} {
		if strings.Contains(page, bad) {
			t.Errorf("page contains %q unescaped", bad)
		}
	}
	for _, want := range []string{"&lt;b&gt;Title&lt;/b&gt;", "Tom &amp; Jerry", `href="a&#34;b&lt;c&gt;.pdf"`, `href="https://x.test/?a=1&amp;b=&#34;2&#34;"`, "background: redbodydisplay:none/stylescript;", "it&#39;s"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func TestCSSValueStripsDeclarationEnds(t *testing.T) {
	if got := CSSValue("#a1b2c3; } <x>{"); got != "#a1b2c3  x" {
		t.Fatalf("%q", got)
	}
	if got := CSSValue("rgb(1, 2, 3)"); got != "rgb(1, 2, 3)" {
		t.Fatalf("%q", got)
	}
}

func TestWordsFallBackToEnglish(t *testing.T) {
	en := Words("en")
	if en.Thanks == "" || en.Pending == "" || en.Keep == "" || !strings.Contains(en.Heading, "%s") {
		t.Fatalf("%+v", en)
	}
	for _, lang := range []string{"", "pt", "xx-YY", "english"} {
		if Words(lang) != en {
			t.Errorf("Words(%q) is not English", lang)
		}
	}
	for lang, want := range map[string]string{"de": "Danke", "de-AT": "Danke", "DE": "Danke", "fr-CA": "Merci", "es": "Gracias"} {
		if !strings.HasPrefix(Words(lang).Thanks, want) {
			t.Errorf("Words(%q).Thanks = %q", lang, Words(lang).Thanks)
		}
	}
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for x := range 8 {
		img.Set(x, 3, color.RGBA{200, 10, 10, 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestJPEGStampRoundTrip(t *testing.T) {
	jpg := testJPEG(t)
	if JPEGStamp(jpg) != "" {
		t.Fatal("an unstamped JPEG has a stamp")
	}
	key := "sha256:" + strings.Repeat("ab", 32)
	stamped := StampJPEG(jpg, key)
	if got := JPEGStamp(stamped); got != key {
		t.Fatalf("stamp %q, want %q", got, key)
	}
	img, err := jpeg.Decode(bytes.NewReader(stamped))
	if err != nil || img.Bounds().Dx() != 8 || img.Bounds().Dy() != 6 {
		t.Fatalf("the stamped JPEG no longer decodes: %v", err)
	}
	if !bytes.Equal(stamped[len(stamped)-len(jpg)+2:], jpg[2:]) {
		t.Fatal("stamping changed the image data")
	}
	if JPEGStamp(StampJPEG(jpg, "")) != "" {
		t.Fatal("an empty key")
	}
}

func TestJPEGStampIgnoresOtherBytes(t *testing.T) {
	notJPEG := []byte("\x89PNG\r\n\x1a\n")
	if got := StampJPEG(notJPEG, "k"); !bytes.Equal(got, notJPEG) {
		t.Fatal("a PNG was stamped")
	}
	if StampJPEG(nil, "k") != nil {
		t.Fatal("nil was stamped")
	}
	for name, b := range map[string][]byte{
		"empty":         nil,
		"short":         {0xff, 0xd8, 0xff},
		"foreign":       {0xff, 0xd8, 0xff, 0xfe, 0x00, 0x05, 'a', 'b', 'c'},
		"truncated":     {0xff, 0xd8, 0xff, 0xfe, 0x00, 0x40, 'l', 'f', 'm'},
		"bad length":    {0xff, 0xd8, 0xff, 0xfe, 0x00, 0x01, 'x'},
		"other segment": {0xff, 0xd8, 0xff, 0xe0, 0x00, 0x05, 'l', 'f', 'm'},
	} {
		if got := JPEGStamp(b); got != "" {
			t.Errorf("%s: stamp %q", name, got)
		}
	}
}

func TestCopyLinkMakesTheBuyersCopy(t *testing.T) {
	id := "1AbC_dEf-GhIjKlMnOpQrStUv"
	for _, kind := range []string{"document", "presentation", "spreadsheets"} {
		url := "https://docs.google.com/" + kind + "/d/" + id + "/edit?usp=sharing#gid=0"
		want := "https://docs.google.com/" + kind + "/d/" + id + "/copy"
		if got := CopyLink("  "+url+"\n", kind); got != want {
			t.Errorf("CopyLink(%s) = %q, want %q", kind, got, want)
		}
	}
	for _, tc := range []struct{ url, kind string }{
		{"https://docs.google.com/spreadsheets/d/" + id + "/edit", "document"},
		{"https://docs.google.com/document/d/" + id, "Document"},
		{"https://docs.google.com/document/d/short/edit", "document"},
		{"http://docs.google.com/document/d/" + id, "document"},
		{"https://docs.google.com.evil.test/document/d/" + id, "document"},
		{"https://drive.google.com/file/d/" + id + "/view", "document"},
		{"https://evil.test/?https://docs.google.com/document/d/" + id, "document"},
		{"not a url", "document"},
		{"", ""},
	} {
		if got := CopyLink(tc.url, tc.kind); got != "" {
			t.Errorf("CopyLink(%q, %q) = %q", tc.url, tc.kind, got)
		}
	}
}

func TestFileNameIsSafeOnEverySystem(t *testing.T) {
	for title, want := range map[string]string{
		"Budget 2026":               "Budget 2026.pdf",
		`a/b\c:d*e?f"g<h>i|j`:       "abcdefghij.pdf",
		"  Größe: Übersicht \t\n ":  "Größe Übersicht.pdf",
		"Tab\there\x00":             "Tabhere.pdf",
		"":                          "Document.pdf",
		` /\:*?"<>| `:               "Document.pdf",
		"Brandschutz – Notizen (2)": "Brandschutz – Notizen (2).pdf",
	} {
		if got := FileName(title, "pdf"); got != want {
			t.Errorf("FileName(%q) = %q, want %q", title, got, want)
		}
	}
}
