package compile

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
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
func deliverPrintable(p project.Project, r *printable.Report, page []byte, st *stock, result *Result) (shipped, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	sell := r.Summary.Sell
	s, stop := startSale(sell, r.Config.Path, r.Summary.URL, page, printable.RuleDelivery, "printable")
	if s.skip || stop != nil {
		return out, stop
	}
	keys := make([]string, len(r.Editions))
	for i, ed := range r.Editions {
		keys[i] = ed.Edition.Key
	}
	pr := s.open(st, result, keys, func(i int) []byte { return r.Editions[i].Doc }, r.Assets)
	var issues []diagnostic.Diagnostic
	warn := func(rule, message, hint string) {
		issues = append(issues, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: printable.TemplateFile, Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	if r.Buildable {
		printed := printable.Printed{PDFs: map[string][]byte{}}

		var layout []string               // findings in the order first seen
		layoutIn := map[string][]string{} // finding -> editions it occurs in
		unchecked := false                // an edition's layout check did not run
		for _, ed := range r.Editions {
			e := ed.Edition
			pdf, err := pr.job(printKey("printable "+e.Key, ed.Doc, r.Assets), func(printer *release.Printer) ([]byte, error) {
				return printer.Print(ed.Doc, r.Assets, e.Size.Digital)
			})
			var found []string
			var layoutErr error
			if st.validate && err == nil {
				if printer, perr := pr.printerFor(); perr == nil {
					found, layoutErr = printer.PageLayout(ed.Doc, r.Assets, mmPixels(e.Size.Width), mmPixels(e.Size.Height), float64(mmPixels(printable.Safe)))
				}
			}

			if layoutErr != nil {
				unchecked = true
				warn(printable.RuleLayout, fmt.Sprintf("The layout check of the %s edition could not run: %s", e.Label, layoutErr), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release --validate'.")
			}
			switch {
			case err != nil:
				unchecked = true
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
		}
		// Listing images serve the printable's own shops and the bundle's.
		if r.Listing.Present || sell == deliver.SellBundle {
			out.Images = printableImages(pr, r, &out)
		}

		for _, f := range layout {
			which := strings.Join(layoutIn[f], ", ")
			if len(layoutIn[f]) == len(r.Editions) {
				which = "Every edition"
			}
			warn(printable.RuleLayout, which+": "+f, "Every .page is padded by var(--safe); keep text, pictures and borders inside that padding (negative margins and absolute positions escape it). Put a frame on an inner element; mark a deliberate full-bleed background with data-bleed.")
		}
		printed.Images = out.Images
		printed.Pending = pr.pending
		r.Summary.Pending = pr.pending
		r.Summary.Images = len(out.Images)
		if st.validate && pr.startErr == nil && !unchecked {
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
	r.Summary.Files = allFiles(out)
	if s.page {
		downloadPage(result, r.Config.Path, printable.DownloadPage(*r, out.Files), out)
	}
	return out, issues
}

// listingImages returns the listing images: reused while the first edition
// and the image layouts are unchanged, rendered by a release otherwise.
func printableImages(pr *press, r *printable.Report, out *shipped) []printable.Image {
	covers := r.SizeCovers()
	inputs := []byte{}
	for _, ed := range covers {
		inputs = append(inputs, ed.Doc...)
	}
	var planned []listingPlan
	for _, plan := range r.Images() {
		planned = append(planned, listingPlan{Name: plan.Name, Doc: plan.Doc,
			Key: printKey("listing image "+plan.Name, append(append([]byte{}, plan.Doc...), inputs...), r.Assets)})
	}
	return listingImages(pr, "printable", r.Summary.Sell, planned, func(printer *release.Printer, missing []listingPlan) ([]listingPlan, map[string][]byte, error) {
		// The plans show real pages: the first edition's and each size's cover.
		var wanted []byte
		for _, plan := range missing {
			wanted = append(wanted, plan.Doc...)
		}
		shots := slices.DeleteFunc(r.Shots(), func(s printable.Shot) bool { return !bytes.Contains(wanted, []byte(`src="`+s.Name+`"`)) })
		pages := make([]int, len(shots))
		for i, s := range shots {
			pages[i] = s.Page
		}
		first := r.Editions[0]
		size := first.Edition.Size
		var rasters [][]byte
		var err error
		if len(pages) > 0 {
			rasters, err = printer.PageImages(first.Doc, r.Assets, mmPixels(size.Width), mmPixels(size.Height), printable.PageScale, pages)
		}
		if err != nil {
			return nil, nil, err
		}
		assets := map[string][]byte{}
		for i, s := range shots {
			assets[s.Name] = rasters[i]
			if s.Page == 1 {
				out.Cover = rasters[i]
			}
		}
		for _, ed := range covers {
			s := ed.Edition.Size
			if !bytes.Contains(wanted, []byte(`src="cover-`+s.Key+`.png"`)) {
				continue
			}
			cover, err := printer.PageImages(ed.Doc, r.Assets, mmPixels(s.Width), mmPixels(s.Height), 1, []int{1})
			if err != nil {
				return nil, nil, err
			}
			assets["cover-"+s.Key+".png"] = cover[0]
		}
		return missing, assets, nil
	})
}
