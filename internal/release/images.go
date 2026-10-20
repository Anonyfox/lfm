package release

import (
	"context"
	"fmt"
	"net/http/httptest"
	"time"

	"github.com/chromedp/chromedp"
)

// Placement is where one <img> renders, in document order.
type Placement struct {
	Src         string  `json:"src"`
	FirstScreen bool    `json:"firstScreen"` // visible before scrolling at any reviewed size
	PhoneArea   float64 `json:"phoneArea"`   // visible area in the phone's first screen, CSS px²
}

// ImagePlacement renders the page with scripts blocked at the critical-CSS
// sizes and reports for every image whether it shows before scrolling. The
// compiler uses it to fill missing loading and fetch-priority hints.
func ImagePlacement(ctx context.Context, page []byte, assets map[string][]byte) ([]Placement, error) {
	path, err := BrowserPath()
	if err != nil {
		return nil, err
	}
	server := httptest.NewServer(Handler(page, assets))
	defer server.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(path), chromedp.Flag("disable-background-networking", true), chromedp.Flag("disable-component-update", true), chromedp.Flag("no-first-run", true))
	alloc, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	tab, cancelTab := chromedp.NewContext(alloc)
	defer cancelTab()
	type box struct {
		Src     string
		Visible bool
		Area    float64
	}
	measure := `[...document.images].map(i=>{const r=i.getBoundingClientRect();const w=Math.max(0,Math.min(r.right,innerWidth)-Math.max(r.left,0)),h=Math.max(0,Math.min(r.bottom,innerHeight)-Math.max(r.top,0));const cs=getComputedStyle(i);const shown=cs.visibility!=="hidden"&&cs.display!=="none";return {Src:i.getAttribute("src")||"",Visible:shown&&w*h>0,Area:shown?w*h:0}})`
	var out []Placement
	for i, v := range viewports {
		opts := []chromedp.EmulateViewportOption{}
		if i == 0 {
			opts = append(opts, chromedp.EmulateMobile)
		}
		if err := chromedp.Do(tab, chromedp.EmulateViewport(v.Width, v.Height, opts...), chromedp.Navigate(server.URL+"/")); err != nil {
			return nil, err
		}
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.fonts.ready.then(()=>true)`, chromedp.EvalAwaitPromise)); err != nil {
			return nil, err
		}
		boxes, err := chromedp.Run(tab, chromedp.Evaluate[[]box](measure))
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = make([]Placement, len(boxes))
		}
		if len(boxes) != len(out) {
			return nil, fmt.Errorf("image count changed between sizes")
		}
		for k, b := range boxes {
			out[k].Src = b.Src
			out[k].FirstScreen = out[k].FirstScreen || b.Visible
			if i == 0 {
				out[k].PhoneArea = b.Area
			}
		}
	}
	return out, nil
}
