package release

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Printer prints documents (books, printables) in one Chrome, a tab per job, so a release
// pays for one browser start however many PDFs it prints. It is safe for
// concurrent use.
type Printer struct {
	browser context.Context
	server  *httptest.Server
	stop    func()
	jobs    sync.Map // "/<n>/" -> http.Handler
	next    atomic.Int64
	slots   chan struct{} // open tabs; a release prints many files side by side
}

// tabs is how many tabs print at once: enough to keep the cores busy, few
// enough that Chrome's memory stays bounded however many files a set has.
var tabs = min(max(2, runtime.NumCPU()), 8)

// tabTime is how long one print or render may take once it has a tab.
const tabTime = time.Minute

// NewPrinter starts Chrome. Close it when the release is done.
func NewPrinter(ctx context.Context) (*Printer, error) {
	path, err := BrowserPath()
	if err != nil {
		return nil, err
	}
	p := &Printer{slots: make(chan struct{}, tabs)}
	p.server, err = localServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		job, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if h, ok := p.jobs.Load(job); ok {
			http.StripPrefix("/"+job, h.(http.Handler)).ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	}), net.Listen)
	if err != nil {
		return nil, err
	}
	// No deadline on the browser itself: a large release prints for longer
	// than any fixed limit; each tab has its own.
	ctx, cancel := context.WithCancel(ctx)
	opts := BrowserOptions(path)
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
// It waits for a free slot first; the tab's minute starts once it has one.
func (p *Printer) tab(doc []byte, assets map[string][]byte) (context.Context, string, func()) {
	p.slots <- struct{}{}
	job := fmt.Sprint(p.next.Add(1))
	p.jobs.Store(job, Handler(doc, assets))
	tab, cancelTab := chromedp.NewContext(p.browser)
	tab, cancelTimeout := context.WithTimeout(tab, tabTime)
	return tab, p.server.URL + "/" + job + "/", func() { cancelTimeout(); cancelTab(); p.jobs.Delete(job); <-p.slots }
}

// Print prints a document to PDF at the page size its @page rule
// declares, with bookmarks from its headings when outline is set. Fonts and
// images are awaited first so no fallback face is baked in.
func (p *Printer) Print(doc []byte, assets map[string][]byte, outline bool) ([]byte, error) {
	tab, url, done := p.tab(doc, assets)
	defer done()
	if err := chromedp.Do(tab, chromedp.Navigate(url)); err != nil {
		return nil, fmt.Errorf("load the document for printing: %w", err)
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

// load opens doc in a new tab at a viewport of width×height CSS pixels and
// scale device pixels per CSS pixel, with print or screen media, and waits
// for fonts and images.
func (p *Printer) load(doc []byte, assets map[string][]byte, width, height int64, scale float64, media string) (context.Context, func(), error) {
	tab, url, done := p.tab(doc, assets)
	fail := func(err error) (context.Context, func(), error) { done(); return nil, nil, err }
	if _, err := chromedp.Call(tab, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: height, DeviceScaleFactor: scale}); err != nil {
		return fail(err)
	}
	if _, err := chromedp.Call(tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Media: media}); err != nil {
		return fail(err)
	}
	if err := chromedp.Do(tab, chromedp.Navigate(url)); err != nil {
		return fail(fmt.Errorf("load the document: %w", err))
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.fonts.ready.then(()=>Promise.all([...document.images].map(i=>i.decode().catch(()=>{})))).then(()=>true)`, chromedp.EvalAwaitPromise)); err != nil {
		return fail(fmt.Errorf("wait for fonts and images: %w", err))
	}
	return tab, done, nil
}

// PageLayout loads a document of fixed-size .page sections in print media
// and reports, per page, content that overflows the page or reaches into the
// safe margin of safe CSS pixels: text, images, SVG and canvas outside the
// inset, unless inside an element marked data-bleed.
func (p *Printer) PageLayout(doc []byte, assets map[string][]byte, width, height int64, safe float64) ([]string, error) {
	tab, done, err := p.load(doc, assets, width, height, 1, "print")
	if err != nil {
		return nil, err
	}
	defer done()
	script := fmt.Sprintf(`(()=>{const S=%g,out=[];let n=0;
for(const pg of document.querySelectorAll('body > section.page')){n++;const P=pg.getBoundingClientRect();
if(pg.scrollHeight>pg.clientHeight+1||pg.scrollWidth>pg.clientWidth+1){out.push('Page '+n+' overflows: content runs past the page and is cut off.');continue}
const inside=r=>r.left>=P.left+S-0.5&&r.top>=P.top+S-0.5&&r.right<=P.right-S+0.5&&r.bottom<=P.bottom-S+0.5;
const name=el=>{const t=el.tagName.toLowerCase();return t==='img'?'image '+(el.getAttribute('src')||''):t};
let hit='';const w=document.createTreeWalker(pg,NodeFilter.SHOW_TEXT);
while(!hit&&w.nextNode()){const t=w.currentNode;if(!t.textContent.trim()||t.parentElement.closest('[data-bleed]'))continue;
const fs=parseFloat(getComputedStyle(t.parentElement).fontSize)||0;const g=document.createRange();g.selectNodeContents(t);for(const q of g.getClientRects()){const pad=Math.max(0,(q.height-fs)/2);const r={left:q.left,right:q.right,top:q.top+pad,bottom:q.bottom-pad,width:q.width};if(r.width>0&&!inside(r)){const el=t.parentElement;hit='text "'+t.textContent.trim().slice(0,30)+'" in '+el.tagName.toLowerCase()+(el.className?'.'+String(el.className).trim().split(/\s+/)[0]:'');break}}}
if(!hit)for(const el of pg.querySelectorAll('img,svg,canvas')){if(el.closest('[data-bleed]')||el.closest('svg')!==el&&el.tagName!=='IMG'&&el.tagName!=='CANVAS')continue;const r=el.getBoundingClientRect();if(r.width>0&&!inside(r)){hit=name(el);break}}
if(!hit)for(const el of [pg,...pg.querySelectorAll('*')]){if(el.closest('[data-bleed]'))continue;const c=getComputedStyle(el);const lines=['Top','Right','Bottom','Left'].some(k=>parseFloat(c['border'+k+'Width'])>0&&c['border'+k+'Style']!=='none'&&!/rgba\(.*,\s*0\)$/.test(c['border'+k+'Color']));if(!lines)continue;const r=el===pg?P:el.getBoundingClientRect();if(r.width>0&&!inside(r)){hit=(el===pg?'the page border':'a border on '+el.tagName.toLowerCase()+(el.className?' .'+String(el.className).split(' ')[0]:''));break}}
if(hit)out.push('Page '+n+': '+hit+' reaches into the 6.35 mm margin printers cut or skip.')}
return out})()`, safe)
	return chromedp.Run(tab, chromedp.Evaluate[[]string](script))
}

// Overflows loads a document in print media and names the data-fit value
// of every element whose content does not fit inside it, each once: text
// a box would cut off or spill.
func (p *Printer) Overflows(doc []byte, assets map[string][]byte, width, height int64) ([]string, error) {
	tab, done, err := p.load(doc, assets, width, height, 1, "print")
	if err != nil {
		return nil, err
	}
	defer done()
	return chromedp.Run(tab, chromedp.Evaluate[[]string](`(()=>{const out=[];for(const el of document.querySelectorAll('[data-fit]')){if((el.dataset.over||el.scrollHeight>el.clientHeight+1)&&!out.includes(el.dataset.fit))out.push(el.dataset.fit)}return out})()`))
}

// PageImages renders pages (1-based) of a document of .page sections as PNGs
// at scale device pixels per CSS pixel.
func (p *Printer) PageImages(doc []byte, assets map[string][]byte, width, height int64, scale float64, pages []int) ([][]byte, error) {
	tab, done, err := p.load(doc, assets, width, height, scale, "print")
	if err != nil {
		return nil, err
	}
	defer done()
	// A screenshot of a missing page waits for it until the tab times out.
	count, err := chromedp.Run(tab, chromedp.Evaluate[int](`document.querySelectorAll('body > section.page').length`))
	if err != nil {
		return nil, fmt.Errorf("count pages: %w", err)
	}
	var out [][]byte
	for _, n := range pages {
		if n < 1 || n > count {
			return nil, fmt.Errorf("render page %d: the document has %d pages", n, count)
		}
		png, err := chromedp.Run(tab, chromedp.Screenshot(chromedp.CSS(fmt.Sprintf("body > section.page:nth-of-type(%d)", n))))
		if err != nil {
			return nil, fmt.Errorf("render page %d: %w", n, err)
		}
		out = append(out, png)
	}
	return out, nil
}

// Picture renders a document at width×height CSS pixels and scale as a PNG of
// the viewport.
func (p *Printer) Picture(doc []byte, assets map[string][]byte, width, height int64, scale float64) ([]byte, error) {
	tab, done, err := p.load(doc, assets, width, height, scale, "screen")
	if err != nil {
		return nil, err
	}
	defer done()
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](fitScript)); err != nil {
		return nil, fmt.Errorf("fit the document: %w", err)
	}
	return chromedp.Run(tab, chromedp.CaptureScreenshot())
}

// fitScript scales every element marked data-fit="width height max" to
// fit its box, at most max; with data-fit-size its parent takes the scaled
// size, since transforms do not change layout. Pages run no scripts of
// their own, so the printer does this layout step.
const fitScript = `(()=>{for(const el of document.querySelectorAll('[data-fit]')){
const [w,h,m]=el.dataset.fit.split(' ').map(Number);
const s=Math.min(m,w/el.scrollWidth,h/el.scrollHeight);
el.style.transformOrigin='top left';el.style.transform='scale('+s+')';
if('fitSize' in el.dataset){el.parentElement.style.width=el.scrollWidth*s+'px';el.parentElement.style.height=el.scrollHeight*s+'px';el.parentElement.style.overflow='hidden'}}
return true})()`
