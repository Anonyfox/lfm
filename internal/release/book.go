package release

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Printer prints book documents in one Chrome, a tab per job, so a release
// pays for one browser start however many PDFs it prints. It is safe for
// concurrent use.
type Printer struct {
	browser context.Context
	server  *httptest.Server
	stop    func()
	jobs    sync.Map // "/<n>/" -> http.Handler
	next    atomic.Int64
}

// NewPrinter starts Chrome. Close it when the release is done.
func NewPrinter() (*Printer, error) {
	path, err := BrowserPath()
	if err != nil {
		return nil, err
	}
	p := &Printer{}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		job, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if h, ok := p.jobs.Load(job); ok {
			http.StripPrefix("/"+job, h.(http.Handler)).ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path), chromedp.Flag("disable-background-networking", true), chromedp.Flag("disable-component-update", true), chromedp.Flag("no-first-run", true))
	alloc, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	browser, cancelBrowser := chromedp.NewContext(alloc)
	p.stop = func() { cancelBrowser(); cancelAlloc(); cancel(); p.server.Close() }
	if err := chromedp.Do(browser); err != nil { // starts the browser once
		p.stop()
		return nil, fmt.Errorf("start Chrome: %w", err)
	}
	p.browser = browser
	return p, nil
}

// Close stops Chrome.
func (p *Printer) Close() { p.stop() }

// tab opens a new tab serving doc and its flat assets and returns its address.
func (p *Printer) tab(doc []byte, assets map[string][]byte) (context.Context, string, func()) {
	job := fmt.Sprint(p.next.Add(1))
	p.jobs.Store(job, Handler(doc, assets))
	tab, cancelTab := chromedp.NewContext(p.browser)
	tab, cancelTimeout := context.WithTimeout(tab, 60*time.Second)
	return tab, p.server.URL + "/" + job + "/", func() { cancelTimeout(); cancelTab(); p.jobs.Delete(job) }
}

// Print prints a book document to PDF at the page size its @page rule
// declares, with bookmarks from its headings when outline is set. Fonts and
// images are awaited first so no fallback face is baked in.
func (p *Printer) Print(doc []byte, assets map[string][]byte, outline bool) ([]byte, error) {
	tab, url, done := p.tab(doc, assets)
	defer done()
	if err := chromedp.Do(tab, chromedp.Navigate(url)); err != nil {
		return nil, fmt.Errorf("load the book for printing: %w", err)
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.fonts.ready.then(()=>Promise.all([...document.images].map(i=>i.decode().catch(()=>{})))).then(()=>true)`, chromedp.EvalAwaitPromise)); err != nil {
		return nil, fmt.Errorf("wait for fonts and images: %w", err)
	}
	yes := true
	pdf, err := chromedp.Call(tab, page.PrintToPDF, page.PrintToPDFParams{PreferCSSPageSize: &yes, PrintBackground: &yes, GenerateDocumentOutline: &outline, GenerateTaggedPDF: &yes})
	if err != nil {
		return nil, fmt.Errorf("print the PDF: %w", err)
	}
	return pdf.Data, nil
}

// BookLayout loads a book document at phone width and at the width of the
// printed text column and reports what does not fit: elements wider than the
// column and text below 9 pixels.
func (p *Printer) BookLayout(doc []byte, assets map[string][]byte, columnInches float64) ([]string, error) {
	tab, url, done := p.tab(doc, assets)
	defer done()
	type report struct {
		Wide  []string
		Tiny  int
		Width int
	}
	measure := `(()=>{const W=document.documentElement.clientWidth;const wide=[];
for(const el of document.querySelectorAll('img,table,pre,figure,svg,video')){const r=el.getBoundingClientRect();if(r.width>0&&r.right>W+1){const t=el.tagName.toLowerCase();const label=t==='img'?'image '+(el.getAttribute('src')||''):t==='pre'?'code block "'+el.textContent.trim().slice(0,30)+'"':t==='table'?'table "'+el.textContent.trim().slice(0,30)+'"':t;wide.push(label+' is '+Math.round(r.width)+' px wide')}}
let tiny=0;const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(walker.nextNode()){const n=walker.currentNode;if(!n.textContent.trim())continue;const s=getComputedStyle(n.parentElement);if(parseFloat(s.fontSize)<9&&s.display!=='none'&&s.visibility!=='hidden')tiny++}
return {Wide:wide.slice(0,6),Tiny:tiny,Width:W}})()`
	var issues []string
	sizes := []struct {
		name  string
		width int64
		media string
	}{
		{"at phone width", 390, "screen"},
		{"in print", int64(columnInches * 96), "print"},
	}
	for _, s := range sizes {
		if err := chromedp.Do(tab, chromedp.EmulateViewport(s.width, 900)); err != nil {
			return nil, err
		}
		if _, err := chromedp.Call(tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Media: s.media}); err != nil {
			return nil, err
		}
		if err := chromedp.Do(tab, chromedp.Navigate(url)); err != nil {
			return nil, err
		}
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.fonts.ready.then(()=>true)`, chromedp.EvalAwaitPromise)); err != nil {
			return nil, err
		}
		r, err := chromedp.Run(tab, chromedp.Evaluate[report](measure))
		if err != nil {
			return nil, err
		}
		for _, w := range r.Wide {
			issues = append(issues, fmt.Sprintf("%s, %s in a %d px column.", capitalize(s.name), w, r.Width))
		}
		if r.Tiny > 0 {
			issues = append(issues, fmt.Sprintf("%s, %d text passages render below 9 px.", capitalize(s.name), r.Tiny))
		}
	}
	return issues, nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}
