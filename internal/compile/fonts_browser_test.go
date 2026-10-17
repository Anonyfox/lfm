package compile

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/release"
	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
)

func TestBrowserFontPackingAndPreloadReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	path, err := release.BrowserPath()
	if err != nil {
		t.Skip(err)
	}
	root := fontWorkbench(t, "", "")
	font, err := os.ReadFile("../fontcheck/testdata/Go-Regular.woff2")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "demo.woff2", string(font))
	// Keep the browser fixture focused on typography and loading.
	source := helperHTML("", `<link rel="preload" as="font" href="demo.woff2"><style>@font-face{font-family:Demo;src:local("Lykos absent font"),url(./demo.woff2) format(woff2);font-display:swap;font-weight:400;unicode-range:U+0-FF;size-adjust:105%}body{margin:16px;font:20px Demo,serif}h1{font-weight:400}</style>`)
	source = strings.Replace(source, `<img src="assets/images/picture.svg" srcset="data:image/png;base64,aGVsbG8= 1x, assets/images/picture.svg 2x" alt="diagram">`, "", 1)
	write(t, root, "index.html.tmpl", source)
	r, err := Run(root, true)
	if err != nil {
		t.Fatal(err)
	}
	after := []byte(read(t, root, "output/index.html"))
	var output string
	for _, a := range r.Packing {
		if a.Source == "demo.woff2" {
			output = a.Output
		}
	}
	// Compare the packed artifact against the same font embedded: the font
	// descriptors and DOM are identical, only the automatic delivery differs.
	doc, _ := html.Parse(bytes.NewReader(after))
	walkHTML(doc, func(n *html.Node) {
		if n.Data == "link" && attr(n, "as") == "font" {
			n.Parent.RemoveChild(n)
		}
		if n.Data == "style" {
			setText(n, strings.ReplaceAll(textContent(n), fileURL(output), "data:font/woff2;base64,"+base64.StdEncoding.EncodeToString(font)))
		}
	})
	var before bytes.Buffer
	html.Render(&before, doc)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		switch q.URL.Path {
		case "/after":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(after)
		case "/before":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(before.Bytes())
		case "/" + output:
			requests.Add(1)
			w.Header().Set("Content-Type", "font/woff2")
			w.Write(font)
		default:
			http.NotFound(w, q)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path))
	allocator, stop := chromedp.NewExecAllocator(ctx, opts...)
	defer stop()
	tab, closeTab := chromedp.NewContext(allocator)
	defer closeTab()
	for _, width := range []int64{390, 1280} {
		if _, err := chromedp.Run(tab, chromedp.EmulateViewport(width, 900)); err != nil {
			t.Fatal(err)
		}
		var layout string
		var pixels []byte
		for _, route := range []string{"/before", "/after"} {
			requests.Store(0)
			if _, err := chromedp.Run(tab, chromedp.Navigate(server.URL+route)); err != nil {
				t.Fatal(err)
			}
			if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(async()=>{await document.fonts.ready; await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return true})()`, chromedp.EvalAwaitPromise)); err != nil {
				t.Fatal(err)
			}
			loaded, err := chromedp.Run(tab, chromedp.Evaluate[bool](`Array.from(document.fonts).some(f=>f.family==='Demo'&&f.status==='loaded')`))
			if err != nil || !loaded {
				t.Fatal("font failed to load", err)
			}
			got, err := chromedp.Run(tab, chromedp.Evaluate[string](`JSON.stringify(Array.from(document.querySelectorAll('h1,p,details'),n=>{const r=n.getBoundingClientRect();return [n.textContent,r.x,r.y,r.width,r.height,getComputedStyle(n).fontFamily]}))`))
			if err != nil {
				t.Fatal(err)
			}
			shot, err := chromedp.Run(tab, chromedp.CaptureScreenshot())
			if err != nil {
				t.Fatal(err)
			}
			if route == "/before" {
				layout, pixels = got, shot
			} else {
				if got != layout || !bytes.Equal(pixels, shot) {
					t.Fatalf("font rendering changed at %dpx", width)
				}
				if requests.Load() != 1 {
					t.Fatalf("font fetched %d times; preload not reused", requests.Load())
				}
			}
		}
	}
}
