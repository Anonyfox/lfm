package release

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/chromedp/chromedp"
)

// printer starts one Chrome for a test, or skips the test without one.
func printer(t *testing.T) *Printer {
	t.Helper()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	p, err := NewPrinter(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

var mediaBox = regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([0-9.]+)\s+([0-9.]+)\s*\]`)

// pageDoc is a document of fixed-size .page sections of w×h CSS pixels.
func pageDoc(w, h int, style string, pages ...string) []byte {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><style>@page{size:` + strconv.Itoa(w) + `px ` + strconv.Itoa(h) + `px;margin:0}body{margin:0}
section.page{box-sizing:border-box;width:` + strconv.Itoa(w) + `px;height:` + strconv.Itoa(h) + `px;overflow:hidden;position:relative;padding:40px;font:16px/1.4 sans-serif;break-after:page}` + style + `</style></head><body>`)
	for _, p := range pages {
		b.WriteString(`<section class="page">` + p + `</section>`)
	}
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

func TestPrinter(t *testing.T) {
	t.Parallel()
	p := printer(t)
	font, err := os.ReadFile("../fontcheck/testdata/Go-Regular.woff2")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("shared tab preserves prints, layout and rasters", func(t *testing.T) {
		fresh, closeFresh := chromedp.NewContext(p.browser, chromedp.WithNewBrowserContext())
		defer closeFresh()
		if _, err := chromedp.Run(fresh, chromedp.Evaluate[bool]("true")); err != nil {
			t.Fatal(err)
		}
		standalone := &Printer{browser: fresh}
		assets := map[string][]byte{"go.woff2": font}
		dates := regexp.MustCompile(`/((?:Creation|Mod)Date)\s*\(D:[^)]*\)`)
		for _, content := range []string{"<h1>Clean</h1><p>Body</p><table><tr><th>Header</th></tr><tr><td>Value</td></tr></table>", "<h1>Overflow</h1><p>" + strings.Repeat("Too much text. ", 100) + "</p>"} {
			doc := pageDoc(400, 300, `@font-face{font-family:Go;src:url(go.woff2)}p{font-family:Go}h1{width:50vw}`, content)
			doc = bytes.Replace(doc, []byte("<head>"), []byte("<head><title>Print equivalence</title>"), 1)
			for _, outline := range []bool{false, true} {
				want, err := standalone.Print(doc, assets, outline)
				if err != nil {
					t.Fatal(err)
				}
				// A capture leaves a different density, media and viewport in the shared tab.
				if _, err := p.Picture([]byte("<p>Previous job</p>"), nil, 150, 100, 2, true, 0); err != nil {
					t.Fatal(err)
				}
				got, err := p.Print(doc, assets, outline)
				want, got = structIDs(dates.ReplaceAll(want, nil)), structIDs(dates.ReplaceAll(got, nil))
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("shared print with outline %v differs: %v\n%s", outline, err, pdfDifference(want, got))
				}
			}
			wantLayout, err := standalone.PageLayout(doc, assets, 400, 300, 24)
			if err != nil {
				t.Fatal(err)
			}
			gotLayout, err := p.PageLayout(doc, assets, 400, 300, 24)
			if err != nil || !slices.Equal(gotLayout, wantLayout) {
				t.Fatalf("shared layout %q; standalone %q: %v", gotLayout, wantLayout, err)
			}
			for _, scale := range []float64{2, 1} {
				want, err := standalone.PageImages(doc, assets, 400, 300, scale, []int{1})
				if err != nil {
					t.Fatal(err)
				}
				got, err := p.PageImages(doc, assets, 400, 300, scale, []int{1})
				if err != nil || len(got) != 1 || !bytes.Equal(got[0], want[0]) {
					t.Fatalf("shared raster at density %g differs: %v", scale, err)
				}
			}
		}
	})

	t.Run("Print prints A4 pages with the fonts from assets", func(t *testing.T) {
		doc := []byte(`<!doctype html><html><head><style>@font-face{font-family:"Go Test";src:url(go.woff2) format("woff2")}
@page{size:A4;margin:20mm}body{font:12pt "Go Test",serif}h1{break-before:page}h1:first-child{break-before:auto}</style></head>
<body><h1>One</h1><p>First page.</p><h1>Two</h1><p>Second page.</p></body></html>`)
		pdf, err := p.Print(doc, map[string][]byte{"go.woff2": font}, true)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(pdf, []byte("%PDF-")) || pdfx.PageCount(pdf) != 2 {
			t.Fatalf("%d pages", pdfx.PageCount(pdf))
		}
		boxes := mediaBox.FindAllSubmatch(pdf, -1)
		if len(boxes) == 0 {
			t.Fatal("no MediaBox")
		}
		for _, m := range boxes {
			w, _ := strconv.ParseFloat(string(m[1]), 64)
			h, _ := strconv.ParseFloat(string(m[2]), 64)
			if w < 594 || w > 597 || h < 841 || h > 844 {
				t.Fatalf("page is %.2f × %.2f pt, not A4", w, h)
			}
		}
		if !slices.Equal(pdfx.OutlinePages(pdf), []int{1, 2}) {
			t.Fatalf("outline %v", pdfx.OutlinePages(pdf))
		}
		embedded := regexp.MustCompile(`/BaseFont\s*/[A-Z]{6}\+Go`)
		if !embedded.Match(pdf) {
			t.Fatal("the web font from assets is not embedded")
		}
		without, err := p.Print(doc, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if embedded.Match(without) {
			t.Fatal("the Go font is embedded although no asset served it")
		}
	})

	for _, limit := range []int{0, 1 << 20} {
		t.Run("Picture and batch size, fitting and isolation "+strconv.Itoa(limit), func(t *testing.T) {
			doc := []byte(`<!doctype html><html><body style="margin:0;background:#c00"><div data-fit="200 100 1" style="width:400px;height:200px;background:#00c"></div></body></html>`)
			other := []byte(`<!doctype html><style>body{margin:0;background:#0c0}div{width:50vw;height:100vh;background:#cc0}</style><div></div><script>document.body.style.background="#000"</script>`)
			source := []byte(`<!doctype html><style>@font-face{font-family:Source;src:url(go.woff2)}body{margin:0;padding:12px;background:white;font:14px Source}header{position:absolute;top:0}footer{position:absolute;bottom:0}</style><header>Running header</header><p>Source page with embedded font.</p><footer>Page 1</footer>`)
			docs := [][]byte{doc, other, doc, source}
			singles := make([][]byte, len(docs))
			for i, doc := range docs {
				var err error
				singles[i], err = p.Picture(doc, map[string][]byte{"go.woff2": font}, 300, 150, 2, true, limit)
				if err != nil {
					t.Fatal(err)
				}
			}
			err := p.Pictures(docs, map[string][]byte{"go.woff2": font}, 300, 150, 2, true, limit, func(i int, raw []byte) {
				if !bytes.Equal(raw, singles[i]) {
					t.Fatal("batch changed a standalone capture")
				}
				img, _, err := image.Decode(bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds() != image.Rect(0, 0, 600, 300) {
					t.Fatal(img.Bounds())
				}
				if limit > 0 && len(raw) > limit {
					t.Fatal("JPEG exceeds limit")
				}
				if i == 0 || i == 2 {
					if r, _, bl, _ := img.At(100, 50).RGBA(); bl>>8 < 150 || r>>8 > 50 {
						t.Fatal("fitted box missing")
					}
					if r, _, bl, _ := img.At(500, 50).RGBA(); r>>8 < 150 || bl>>8 > 50 {
						t.Fatal("box did not fit")
					}
				}
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := p.Picture([]byte("<!doctype html><p>Size limit</p>"), nil, 300, 150, 2, false, 1); err == nil {
		t.Fatal("impossible JPEG limit succeeded")
	}

	t.Run("PageLayout reports overflow and margin intrusion", func(t *testing.T) {
		doc := pageDoc(400, 300, `.edge{position:absolute;left:4px;top:120px;margin:0}.bleed{position:absolute;inset:0;background:#eee}`,
			`<p>A clean page inside its margins.</p>`,
			`<p>`+strings.Repeat("Too much text for one page. ", 80)+`</p>`,
			`<p class="edge">Edge text</p>`,
			`<div class="bleed" data-bleed><p class="edge">Bleeds on purpose</p></div><p>Body</p>`,
			`<img src="dot.png" style="position:absolute;right:0;top:100px;width:30px;height:30px">`,
			`<div style="position:absolute;inset:2px;border:1px solid #000"></div>`,
		)
		dot := pngOf(t, 4, 4, func(x, y int) color.Color { return color.Black })
		got, err := p.PageLayout(doc, map[string][]byte{"dot.png": dot}, 400, 300, 24)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"Page 2 overflows: content runs past the page and is cut off.",
			`Page 3: text "Edge text" in p.edge reaches into the 6.35 mm margin printers cut or skip.`,
			"Page 5: image dot.png reaches into the 6.35 mm margin printers cut or skip.",
			"Page 6: a border on div reaches into the 6.35 mm margin printers cut or skip.",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		clean, err := p.PageLayout(pageDoc(400, 300, "", "<h1>Title</h1><p>Fine.</p>", "<p>Also fine.</p>"), nil, 400, 300, 24)
		if err != nil || len(clean) != 0 {
			t.Fatalf("clean pages: %q %v", clean, err)
		}
	})

	t.Run("Overflows names exactly the boxes that overflow", func(t *testing.T) {
		doc := []byte(`<!doctype html><html><head><style>body{margin:0;font:16px/1.25 sans-serif}[data-fit]{height:40px;width:200px;overflow:hidden}</style></head><body>
<div data-fit="title">Short</div>
<div data-fit="body">` + strings.Repeat("A line that wraps. ", 30) + `</div>
<div data-fit="body">` + strings.Repeat("Again too much. ", 30) + `</div>
<div data-fit="flagged" data-over="1">Fits, but the compiler flagged it</div>
<div data-fit="note">One line</div></body></html>`)
		got, err := p.Overflows(doc, nil, 800, 600)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{"body", "flagged"}) {
			t.Fatalf("%q", got)
		}
	})

	t.Run("PageImages renders one PNG per requested page", func(t *testing.T) {
		doc := pageDoc(200, 100, "", "<p>One</p>", "<p>Two</p>", "<p>Three</p>")
		shots, err := p.PageImages(doc, nil, 200, 100, 2, []int{3, 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(shots) != 2 {
			t.Fatalf("%d images", len(shots))
		}
		for i, raw := range shots {
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 200 {
				t.Fatalf("image %d is %v", i, b)
			}
		}
		if bytes.Equal(shots[0], shots[1]) {
			t.Fatal("page 3 and page 1 render the same")
		}
	})

	t.Run("BookLayout reports wide elements and tiny text", func(t *testing.T) {
		doc := []byte(`<!doctype html><html><head><style>body{margin:0;font:16px serif}.tiny{font-size:6px}</style></head><body>
<p>Prose.</p><table style="width:700px"><tr><td>Wide table</td></tr></table><p class="tiny">Fine print</p></body></html>`)
		got, err := p.BookLayout(doc, nil, 4.5)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(got, "\n")
		for _, want := range []string{`At phone width, table "Wide table" is 700 px wide in a 390 px column.`, `In print, table "Wide table" is 700 px wide in a 432 px column.`, "At phone width, 1 text passages render below 9 px.", "In print, 1 text passages render below 9 px."} {
			if !strings.Contains(joined, want) {
				t.Errorf("missing %q in\n%s", want, joined)
			}
		}
		clean, err := p.BookLayout([]byte(`<!doctype html><p>Plain prose.</p>`), nil, 4.5)
		if err != nil || len(clean) != 0 {
			t.Fatalf("clean book: %q %v", clean, err)
		}
	})

}

func TestClosedPrinterFailsFast(t *testing.T) {
	t.Parallel()
	p := printer(t)
	p.Close()
	doc := []byte(`<!doctype html><p>x`)
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"Print", func() error { _, err := p.Print(doc, nil, false); return err }},
		{"Picture", func() error { _, err := p.Picture(doc, nil, 100, 100, 1, true, 0); return err }},
		{"PageLayout", func() error { _, err := p.PageLayout(doc, nil, 100, 100, 10); return err }},
		{"Overflows", func() error { _, err := p.Overflows(doc, nil, 100, 100); return err }},
		{"PageImages", func() error { _, err := p.PageImages(doc, nil, 100, 100, 1, []int{1}); return err }},
		{"BookLayout", func() error { _, err := p.BookLayout(doc, nil, 4); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := false, tc.run(); err == nil {
				t.Error("closed printer succeeded")
			}
		})
	}
}

func TestPageImagesFailsFastOnAMissingPage(t *testing.T) {
	t.Parallel()
	p := printer(t)
	doc := pageDoc(200, 100, "", "<p>One</p>")
	start := time.Now()
	if _, err := p.PageImages(doc, nil, 200, 100, 1, []int{9}); err == nil {
		t.Fatal("a page that does not exist rendered")
	}
	// Waiting for the page took the tab's whole time; loading it on a busy
	// machine takes seconds.
	if time.Since(start) > tabTime/2 {
		t.Fatalf("a missing page took %s to fail", time.Since(start))
	}
}

func TestImagePlacementFindsTheFirstScreen(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	page := []byte(`<!doctype html><html><head><meta name="viewport" content="width=device-width"><style>body{margin:0}img{display:block;width:200px;height:100px}</style></head><body>
<img src="hero.png"><div style="height:3000px"></div><img src="late.png"><img src="hidden.png" style="display:none"></body></html>`)
	dot := pngOf(t, 2, 2, func(x, y int) color.Color { return color.White })
	got, err := ImagePlacement(context.Background(), page, map[string][]byte{"hero.png": dot, "late.png": dot, "hidden.png": dot})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Src != "hero.png" || !got[0].FirstScreen || got[0].PhoneArea != 20000 {
		t.Fatalf("%+v", got)
	}
	if got[1].FirstScreen || got[1].PhoneArea != 0 || got[2].FirstScreen {
		t.Fatalf("%+v", got)
	}
}

func TestBookLayoutTallImage(t *testing.T) {
	p := printer(t)
	var image bytes.Buffer
	if err := png.Encode(&image, newTestTallImage()); err != nil {
		t.Fatal(err)
	}
	assets := map[string][]byte{"tall.png": image.Bytes()}
	doc := []byte(`<!doctype html><style>@page{size:5in 8in;margin:0.75in 0.6in 0.85in}img{width:4in;height:auto}</style><p><img src="tall.png"></p>`)
	findings, err := p.BookLayout(doc, assets, 3.8)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(findings, "\n"), "tall.png is split across") {
		t.Fatal(findings)
	}
	doc = []byte(`<!doctype html><style>@page{size:5in 8in;margin:0.75in 0.6in 0.85in}img{max-width:100%;max-height:6.25in;width:auto;height:auto;break-inside:avoid}p{break-inside:avoid}</style><p><img src="tall.png"></p>`)
	findings, err = p.BookLayout(doc, assets, 3.8)
	if err != nil || len(findings) > 0 {
		t.Fatal(err, findings)
	}
}

func newTestTallImage() image.Image { return image.NewRGBA(image.Rect(0, 0, 100, 175)) }

// pdfDifference names where two PDFs first part, so a mismatch on a CI
// runner can be diagnosed from the log alone.
func pdfDifference(want, got []byte) string {
	i := 0
	for i < len(want) && i < len(got) && want[i] == got[i] {
		i++
	}
	window := func(b []byte) string {
		from, to := max(0, i-160), min(len(b), i+160)
		return strconv.Quote(string(b[from:to]))
	}
	return fmt.Sprintf("lengths %d (standalone) vs %d (shared); first difference at byte %d\nstandalone: %s\nshared:     %s", len(want), len(got), i, window(want), window(got))
}

// structIDs renumbers tagged-PDF structure IDs, "(node00000014)" in /ID and
// in references such as table /Headers, in order of first use. Chrome takes them from renderer-wide DOM node counters,
// so a tab that rendered other jobs first numbers the same tree differently
// (seen on macOS). The structure and every reference must still match.
func structIDs(pdf []byte) []byte {
	seen := map[string]int{}
	return regexp.MustCompile(`\(node[0-9]+\)`).ReplaceAllFunc(pdf, func(id []byte) []byte {
		n, ok := seen[string(id)]
		if !ok {
			n = len(seen)
			seen[string(id)] = n
		}
		return fmt.Appendf(nil, "(node%08d)", n)
	})
}
