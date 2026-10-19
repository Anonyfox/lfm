package compile

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/term"
)

// checkPrintable checks deliverables/printable against lfm.json and the offers.
func checkPrintable(p project.Project, loaded project.Loaded, commerce commercecheck.Facts) (*printable.Report, []diagnostic.Diagnostic) {
	c := loaded.Config
	var cfg printable.Config
	configured := c.Deliverables.Printable != nil
	if configured {
		pc := c.Deliverables.Printable
		cfg = printable.Config{Path: pc.Path, Offer: pc.Offer, Language: pc.Language, Shop: c.Seller.Name, Version: pc.Version, Sell: pc.Sell}
	}
	if cfg.Language == "" {
		cfg.Language = c.Language
	}
	r := printable.Check(p, cfg, configured, offerPrices(commerce), c.URL)
	sources := jsonsrc.Index(loaded.Sources)
	for i, d := range r.Diagnostics {
		if d.Location.File == "lfm.json" {
			if loc, ok := sources.Nearest("deliverables.printable"); ok {
				r.Diagnostics[i].Location = loc
			}
		}
	}
	return &r, r.Diagnostics
}

// mmPixels converts millimetres to CSS pixels.
func mmPixels(mm float64) int64 { return int64(mm*96/25.4 + 0.5) }

// deliverPrintable prints every edition (or reuses its print), checks the
// printed pages, renders the listing images, publishes the download page when
// the printable sells through the page, and assembles its store packages when
// it sells on its own. It returns the files and images for a bundle.
func deliverPrintable(p project.Project, r *printable.Report, page []byte, isRelease bool, result *Result) (shipped, []printable.Image, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	sell := r.Summary.Sell
	pageSale := sell == deliver.SellPage && deliver.ValidPath(r.Config.Path) && r.Summary.URL != ""
	if sell == deliver.SellPage && !pageSale {
		return out, nil, nil // the config finding says what is missing
	}
	if pageSale {
		if leak := leaks(page, r.Config.Path, printable.RuleDelivery, "printable"); leak != nil {
			return out, nil, leak
		}
	}
	// lfm serve shows every edition as HTML at <path>/preview/<edition>.html,
	// or printable/preview/ when no download page exists.
	root := "printable"
	if pageSale {
		root = r.Config.Path
	}
	if result.Previews == nil {
		result.Previews = map[string][]byte{}
	}
	for _, ed := range r.Editions {
		result.Previews[root+"/preview/"+ed.Edition.Key+".html"] = ed.Doc
	}
	for name, data := range r.Assets {
		result.Previews[root+"/preview/"+name] = data
	}
	var issues []diagnostic.Diagnostic
	warn := func(rule, message, hint string) {
		issues = append(issues, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: printable.TemplateFile, Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	var images []printable.Image
	if r.Buildable {
		pr := newPress(p, isRelease)
		defer pr.close()
		printed := printable.Printed{PDFs: map[string][]byte{}}
		var mu sync.Mutex
		var wg sync.WaitGroup
		var layout []string               // findings in the order first seen
		layoutIn := map[string][]string{} // finding -> editions it occurs in
		for _, ed := range r.Editions {
			wg.Go(func() {
				e := ed.Edition
				pdf, err := pr.job(printKey("printable "+e.Key, ed.Doc, r.Assets), func(printer *release.Printer) ([]byte, error) {
					return printer.Print(ed.Doc, r.Assets, e.Size.Digital)
				})
				var found []string
				if isRelease && err == nil {
					if printer, perr := pr.printerFor(); perr == nil {
						found, _ = printer.PageLayout(ed.Doc, r.Assets, mmPixels(e.Size.Width), mmPixels(e.Size.Height), float64(mmPixels(printable.Safe)))
					}
				}
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err != nil:
					warn(printable.RuleLayout, fmt.Sprintf("The %s edition could not be printed: %s", e.Label, err), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release'.")
				case pdf != nil:
					printed.PDFs[e.Key] = pdf
					if n := pdfx.PageCount(pdf); n != len(ed.Pages) {
						warn(printable.RuleLayout, fmt.Sprintf("The %s PDF has %d pages, but %d page sections were rendered; links after the first difference land on the wrong page.", e.Label, n, len(ed.Pages)), "Keep every page section at the page's size: a section that grows splits into two pages.")
					}
				}
				for _, f := range found {
					if layoutIn[f] == nil {
						layout = append(layout, f)
					}
					layoutIn[f] = append(layoutIn[f], e.Label)
				}
			})
		}
		// Listing images serve the printable's own shops and the bundle's.
		if r.Listing.Present || sell == deliver.SellBundle {
			wg.Go(func() { images = listingImages(pr, r) })
		}
		wg.Wait()
		for _, f := range layout {
			which := strings.Join(layoutIn[f], ", ")
			if len(layoutIn[f]) == len(r.Editions) {
				which = "Every edition"
			}
			warn(printable.RuleLayout, which+": "+f, "Every .page is padded by var(--safe); keep text, pictures and borders inside that padding (negative margins and absolute positions escape it). Put a frame on an inner element; mark a deliberate full-bleed background with data-bleed.")
		}
		printed.Images = images
		printed.Pending = pr.pending
		r.Summary.Pending = pr.pending
		r.Summary.Images = len(images)
		if isRelease && pr.startErr == nil {
			r.Summary.Layout = "every page inside the safe margin"
			if len(layout) > 0 {
				r.Summary.Layout = term.Plural(len(layout), "layout finding")
			}
		}
		for _, ed := range r.Editions {
			if pdf := printed.PDFs[ed.Edition.Key]; pdf != nil {
				out.add(deliver.File{Name: r.FileName(ed.Edition), Label: ed.Edition.Label, Note: printable.FileNote(*r, ed.Edition)}, pdf)
			}
		}
		if sell != deliver.SellBundle && r.Listing.Present {
			dist, channels, found := printable.Distribution(*r, printed, time.Now().Format("2006-01-02"))
			result.distribute("printable", dist)
			r.Summary.Channels = channels
			issues = append(issues, found...)
		} else {
			result.distribute("printable", map[string][]byte{}) // no package of its own
		}
	}
	for _, f := range out.Files {
		r.Summary.Files = append(r.Summary.Files, f.Name)
	}
	if pageSale {
		downloadPage(result, r.Config.Path, printable.DownloadPage(*r, out.Files), out)
	}
	return out, images, issues
}

// listingImages returns the listing images: reused while the first edition
// and the image layouts are unchanged, rendered by a release otherwise.
func listingImages(pr *press, r *printable.Report) []printable.Image {
	plans := r.Images()
	first := r.Editions[0]
	keys := make([]string, len(plans))
	var out []printable.Image
	all := true
	covers := r.SizeCovers()
	inputs := []byte{}
	for _, ed := range covers {
		inputs = append(inputs, ed.Doc...)
	}
	for i, plan := range plans {
		keys[i] = printKey("listing image "+plan.Name, append(append([]byte{}, plan.Doc...), inputs...), r.Assets)
		if jpg := pr.lookup(keys[i]); jpg != nil {
			out = append(out, printable.Image{Name: plan.Name, What: plan.What, Data: jpg})
		} else {
			all = false
		}
	}
	if all {
		return out
	}
	if !pr.release {
		pr.mu.Lock()
		pr.pending = true
		pr.mu.Unlock()
		return out
	}
	printer, err := pr.printerFor()
	if err != nil {
		return out
	}
	shots := r.Shots()
	pages := make([]int, len(shots))
	for i, s := range shots {
		pages[i] = s.Page
	}
	size := first.Edition.Size
	rasters, err := printer.PageImages(first.Doc, r.Assets, mmPixels(size.Width), mmPixels(size.Height), printable.PageScale, pages)
	if err != nil {
		return out
	}
	assets := map[string][]byte{}
	for i, s := range shots {
		assets[s.Name] = rasters[i]
	}
	for _, ed := range covers {
		s := ed.Edition.Size
		cover, err := printer.PageImages(ed.Doc, r.Assets, mmPixels(s.Width), mmPixels(s.Height), 1, []int{1})
		if err != nil {
			return out
		}
		assets["cover-"+s.Key+".png"] = cover[0]
	}
	out = nil
	for i, plan := range plans {
		png, err := printer.Picture(plan.Doc, assets, printable.ImageWidth, printable.ImageHeight, printable.ImageScale)
		if err != nil {
			continue
		}
		jpg, err := jpegUnder(png, 1<<20, keys[i])
		if err != nil {
			continue
		}
		out = append(out, printable.Image{Name: plan.Name, What: plan.What, Data: jpg})
	}
	return out
}
