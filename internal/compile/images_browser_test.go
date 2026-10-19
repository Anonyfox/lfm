package compile

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
)

func TestBrowserInferredImageSizesPreserveLoadedLayout(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := release.BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	body := `<div class="frame"><img id="plain" src="assets/images/photo.png" alt=""><img id="width" src="assets/images/photo.png" width="6" alt=""><img id="responsive" class="responsive" src="assets/images/photo.png" alt=""><img id="deliberate" src="assets/images/photo.png" width="20" height="20" alt=""><img id="CSS-only" style="width:50%" src="assets/images/photo.png" alt=""><div style="display:flex;width:6px"><img id="flex" src="assets/images/photo.png" alt=""></div><img id="rotated-jpeg" src="rotated.jpg" alt=""><img id="rotated-png" src="rotated.png" alt=""><img id="SVG" src="fixed.svg" alt=""></div>`
	root := imageProject(t, body, `body{margin:16px}.frame{width:80%}.responsive{width:100%;height:auto}`)
	// Valid JPEG/PNG files with EXIF orientation 6, not mocked dimensions.
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewNRGBA(image.Rect(0, 0, 12, 8)), nil)
	exif := append([]byte("Exif\x00\x00"), tiff...)
	app := append([]byte{0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)
	rotated := append(bytes.Clone(jpg.Bytes()[:2]), app...)
	rotated = append(rotated, jpg.Bytes()[2:]...)
	if err := os.WriteFile(filepath.Join(root, "rotated.jpg"), rotated, 0644); err != nil {
		t.Fatal(err)
	}
	original := imagePNG(t, 12, 8)
	chunk := make([]byte, len(tiff)+12)
	binary.BigEndian.PutUint32(chunk, uint32(len(tiff)))
	copy(chunk[4:], "eXIf")
	copy(chunk[8:], tiff)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	rotated = append(bytes.Clone(original[:len(original)-12]), chunk...)
	rotated = append(rotated, original[len(original)-12:]...)
	if err := os.WriteFile(filepath.Join(root, "rotated.png"), rotated, 0644); err != nil {
		t.Fatal(err)
	}
	write(t, root, "fixed.svg", `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60" viewBox="0 0 2 1"><rect fill="#cc0011" width="2" height="1"/></svg>`)
	report, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, report.Validation)
	}
	after := []byte(read(t, root, "output/index.html"))
	doc, err := html.Parse(bytes.NewReader(after))
	if err != nil {
		t.Fatal(err)
	}
	index := 0
	htmlx.Walk(doc, func(n *html.Node) {
		if n.Data != "img" || n.Namespace != "" {
			return
		}
		for _, key := range report.Images[index].Added {
			htmlx.DelAttr(n, key)
		}
		index++
	})
	var before bytes.Buffer
	html.Render(&before, doc)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:")
		if r.URL.Path == "/after" {
			w.Write(after)
		} else {
			w.Write(before.Bytes())
		}
	}))
	defer server.Close()
	deadline, cancel := context.WithTimeout(context.Background(), browserTestTimeout)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path))
	allocator, stop := chromedp.NewExecAllocator(deadline, opts...)
	defer stop()
	tab, closeTab := chromedp.NewContext(allocator)
	defer closeTab()
	for _, width := range []int64{390, 1280} {
		if _, err := chromedp.Run(tab, chromedp.EmulateViewport(width, 900)); err != nil {
			t.Fatal(err)
		}
		var observed string
		var pixels []byte
		for _, route := range []string{"/before", "/after"} {
			if _, err := chromedp.Run(tab, chromedp.Navigate(server.URL+route)); err != nil {
				t.Fatal(err)
			}
			if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(async()=>{await document.fonts.ready;await Promise.all(Array.from(document.images,i=>i.decode()));await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return true})()`, chromedp.EvalAwaitPromise)); err != nil {
				t.Fatal(err)
			}
			got, err := chromedp.Run(tab, chromedp.Evaluate[string](`JSON.stringify(Array.from(document.images,i=>{const r=i.getBoundingClientRect();return [i.id,r.x,r.y,r.width,r.height,i.naturalWidth,i.naturalHeight,getComputedStyle(i).objectFit]}))`))
			if err != nil {
				t.Fatal(err)
			}
			shot, err := chromedp.Run(tab, chromedp.CaptureScreenshot())
			if err != nil {
				t.Fatal(err)
			}
			if route == "/before" {
				observed, pixels = got, shot
			} else {
				if got != observed {
					t.Fatalf("loaded image geometry changed at %dpx:\n%s\n%s", width, observed, got)
				}
				if !bytes.Equal(pixels, shot) {
					t.Fatalf("loaded pixels changed at %dpx", width)
				}
			}
		}
	}
}
