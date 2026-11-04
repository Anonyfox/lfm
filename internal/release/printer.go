package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Printer renders sequential print jobs and picture batches in one Chrome.
// Each job or batch has a fresh browser context; runtime can reuse Chrome.
type Printer struct {
	browser context.Context
	stop    func()
}

// tabTime is how long one print or render may take once it has a tab.
const tabTime = time.Minute

// NewPrinter starts Chrome. Close it when the release is done.
func NewPrinter(ctx context.Context) (*Printer, error) {
	path, err := BrowserPath()
	if err != nil {
		return nil, err
	}
	browser, stop, err := openBrowser(ctx, BrowserOptions(path), tabTime)
	if err != nil {
		return nil, err
	}
	return &Printer{browser: browser, stop: stop}, nil
}

// Close stops Chrome.
func (p *Printer) Close() { p.stop() }

// Print prints a document to PDF at the page size its @page rule
// declares, with bookmarks from its headings when outline is set. Fonts and
// images are awaited first so no fallback face is baked in.
func (p *Printer) Print(doc []byte, assets map[string][]byte, outline bool) ([]byte, error) {
	tab, done, err := p.load(doc, assets, 0, 0, 0, "")
	if err != nil {
		return nil, err
	}
	defer done()
	yes := true
	pdf, err := chromedp.Call(tab, page.PrintToPDF, page.PrintToPDFParams{PreferCSSPageSize: &yes, PrintBackground: &yes, GenerateDocumentOutline: &outline, GenerateTaggedPDF: &yes})
	if err != nil {
		return nil, fmt.Errorf("print the PDF: %w", err)
	}
	return pdf.Data, nil
}

// BookLayout loads a book document at phone width and at the width of the
// printed text column and reports what does not fit: elements wider than the
// column, clipped images, images split across printed pages and text below 9 pixels.
func (p *Printer) BookLayout(doc []byte, assets map[string][]byte, columnInches float64) ([]string, error) {
	type report struct {
		Wide   []string
		Tiny   int
		Width  int
		Images []string
	}
	measure := `(()=>{const W=document.documentElement.clientWidth;const wide=[];
for(const el of document.querySelectorAll('img,table,pre,figure,svg,video')){const r=el.getBoundingClientRect();if(r.width>0&&r.right>W+1){const t=el.tagName.toLowerCase();const label=t==='img'?'image '+(el.getAttribute('src')||''):t==='pre'?'code block "'+el.textContent.trim().slice(0,30)+'"':t==='table'?'table "'+el.textContent.trim().slice(0,30)+'"':t;wide.push(label+' is '+Math.round(r.width)+' px wide')}}
let tiny=0;const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(walker.nextNode()){const n=walker.currentNode;if(!n.textContent.trim())continue;const s=getComputedStyle(n.parentElement);if(parseFloat(s.fontSize)<9&&s.display!=='none'&&s.visibility!=='hidden')tiny++}
const images=[];
for(const img of document.images){if(img.closest('.cover'))continue;const r=img.getBoundingClientRect();let problem='';
 for(let el=img.parentElement;el&&!problem;el=el.parentElement){const s=getComputedStyle(el),p=el.getBoundingClientRect();if((['hidden','clip'].includes(s.overflowY)&&(r.top<p.top-1||r.bottom>p.bottom+1))||(['hidden','clip'].includes(s.overflowX)&&(r.left<p.left-1||r.right>p.right+1)))problem='is clipped';}
 if(problem)images.push('image '+img.getAttribute('src')+' '+problem);
}
return {Wide:wide.slice(0,6),Tiny:tiny,Width:W,Images:images}})()`
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
		tab, done, err := p.load(doc, assets, s.width, 900, 1, s.media)
		if err != nil {
			return nil, err
		}
		defer done()
		r, err := chromedp.Run(tab, chromedp.Evaluate[report](measure))
		if err != nil {
			return nil, err
		}
		for _, image := range r.Images {
			issues = append(issues, capitalize(s.name)+", "+image+".")
		}
		for _, w := range r.Wide {
			issues = append(issues, fmt.Sprintf("%s, %s in a %d px column.", capitalize(s.name), w, r.Width))
		}
		if r.Tiny > 0 {
			issues = append(issues, fmt.Sprintf("%s, %d text passages render below 9 px.", capitalize(s.name), r.Tiny))
		}
		if s.media == "print" {
			sources, err := chromedp.Run(tab, chromedp.Evaluate[[]string](`Array.from(document.images).filter(img=>!img.closest('.cover')).map((img,n)=>{img.alt='lfm-layout-image-'+n;return img.getAttribute('src')||''})`))
			if err != nil {
				return nil, err
			}
			yes := true
			pdf, err := chromedp.Call(tab, page.PrintToPDF, page.PrintToPDFParams{PreferCSSPageSize: &yes, GenerateTaggedPDF: &yes})
			if err != nil {
				return nil, fmt.Errorf("print the book for its layout check: %w", err)
			}
			pages := pdfx.FigurePages(pdf.Data)
			for i, source := range sources {
				if n := pages[fmt.Sprintf("lfm-layout-image-%d", i)]; n > 1 {
					issues = append(issues, fmt.Sprintf("In print, image %s is split across %d pages.", source, n))
				}
			}
		}
		done()
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
	server, err := localServer(Handler(doc, assets), net.Listen)
	if err != nil {
		return nil, nil, err
	}
	tab, cancelTab := chromedp.NewContext(p.browser, chromedp.WithNewBrowserContext())
	tab, cancelTimeout := context.WithTimeout(tab, tabTime)
	done := func() { cancelTimeout(); cancelTab(); server.Close() }
	fail := func(err error) (context.Context, func(), error) { done(); return nil, nil, err }
	if width > 0 {
		if _, err := chromedp.Call(tab, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: height, DeviceScaleFactor: scale}); err != nil {
			return fail(err)
		}
		if _, err := chromedp.Call(tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Media: media}); err != nil {
			return fail(err)
		}
	}
	if err := chromedp.Do(tab, chromedp.Navigate(server.URL+"/")); err != nil {
		return fail(fmt.Errorf("load the document: %w", err))
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](readyScript, chromedp.EvalAwaitPromise)); err != nil {
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

// Picture renders the viewport as PNG, or native JPEG when limit > 0.
// Fast PNG encoding preserves pixels; JPEG quality falls from 90 to 50 to fit limit.
func (p *Printer) Picture(doc []byte, assets map[string][]byte, width, height int64, scale float64, fast bool, limit int) ([]byte, error) {
	var out []byte
	err := p.Pictures([][]byte{doc}, assets, width, height, scale, fast, limit, func(_ int, data []byte) { out = data })
	return out, err
}

// Pictures renders independent documents sequentially in one tab. limit > 0
// selects native JPEG; otherwise it captures lossless PNG. consume runs before
// the next capture, so temporary rasters do not accumulate in memory.
func (p *Printer) Pictures(docs [][]byte, assets map[string][]byte, width, height int64, scale float64, fast bool, limit int, consume func(int, []byte)) error {
	if len(docs) == 0 {
		return nil
	}
	tab, done, err := p.load(docs[0], assets, width, height, scale, "screen")
	if err != nil {
		return err
	}
	defer done()
	for i, doc := range docs {
		if i > 0 {
			raw, _ := json.Marshal(string(doc))
			if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.open();document.write(`+string(raw)+`);document.close();true`)); err != nil {
				return err
			}
			if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](readyScript, chromedp.EvalAwaitPromise)); err != nil {
				return err
			}
		}
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](fitScript)); err != nil {
			return fmt.Errorf("fit the document: %w", err)
		}
		yes := true
		params := page.CaptureScreenshotParams{FromSurface: &yes, OptimizeForSpeed: &fast}
		if limit > 0 {
			params.Format = page.CaptureScreenshotFormatJpeg
		}
		for quality := int64(90); quality >= 50; quality -= 8 {
			if limit > 0 {
				params.Quality = &quality
			}
			shot, err := chromedp.Call(tab, page.CaptureScreenshot, params)
			if err != nil {
				return err
			}
			if limit <= 0 || len(shot.Data) <= limit {
				consume(i, shot.Data)
				break
			}
			if quality == 50 {
				return fmt.Errorf("JPEG exceeds %d bytes at quality 50", limit)
			}
		}
	}
	return nil
}

const readyScript = `document.fonts.ready.then(()=>Promise.all([...document.images].map(i=>i.decode().catch(()=>{})))).then(()=>true)`

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
