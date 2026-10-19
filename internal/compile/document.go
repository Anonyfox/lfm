package compile

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/bundle"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/term"
)

// checkDocument checks deliverables/document against lfm.json and the offers.
func checkDocument(p project.Project, loaded project.Loaded, commerce commercecheck.Facts) (*document.Report, []diagnostic.Diagnostic) {
	c := loaded.Config
	var cfg document.Config
	configured := c.Deliverables.Document != nil
	if configured {
		d := c.Deliverables.Document
		cfg = document.Config{Path: d.Path, Offer: d.Offer, Language: d.Language, Sell: d.Sell, Version: d.Version, Papers: d.Papers, Docs: d.Docs}
	}
	cfg.Seller = c.Seller.Name
	if cfg.Language == "" {
		cfg.Language = c.Language
	}
	r := document.Check(p, cfg, configured, offerPrices(commerce), c.URL)
	var sources *jsonsrc.Sources
	for i, d := range r.Diagnostics {
		if d.Location.File == "lfm.json" {
			if sources == nil {
				index := jsonsrc.Index(loaded.Sources)
				sources = &index
			}
			if loc, ok := sources.Nearest("deliverables.document"); ok {
				r.Diagnostics[i].Location = loc
			}
		}
	}
	return &r, r.Diagnostics
}

// deliverDocument builds the .docx files on every compile, or reuses them
// while their inputs are unchanged. A release prints them in Chrome as
// Word lays them out, checks what every app will show, renders the listing images from the
// real first pages and prints the start guides; later compiles reuse
// those. Each edition ships as one ZIP.
func deliverDocument(p project.Project, r *document.Report, page []byte, isRelease bool, result *Result) (shipped, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	pageSale := r.Summary.Sell == deliver.SellPage && deliver.ValidPath(r.Config.Path) && r.Summary.URL != ""
	if r.Summary.Sell == deliver.SellPage && !pageSale {
		return out, nil
	}
	if pageSale {
		if leak := leaks(page, r.Config.Path, document.RuleDelivery, "document"); leak != nil {
			return out, leak
		}
	}
	var issues []diagnostic.Diagnostic
	warn := func(file, rule, message, hint string) {
		issues = append(issues, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: file, Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	root := "document"
	if pageSale {
		root = r.Config.Path
	}
	if result.Previews == nil {
		result.Previews = map[string][]byte{}
	}
	if !r.Buildable {
		if pageSale {
			downloadPage(result, r.Config.Path, document.DownloadPage(*r, nil), out)
		}
		return out, issues
	}

	var pr *press
	previews := make([][]byte, len(r.Editions))
	var prep sync.WaitGroup
	prep.Go(func() { pr = newPress(p, isRelease) })
	for i, e := range r.Editions {
		prep.Go(func() { previews[i] = r.Preview(e) })
	}
	prep.Wait()
	defer pr.close()
	for i, e := range r.Editions {
		result.Previews[root+"/preview/"+e.Key+".html"] = previews[i]
	}
	for name, data := range r.PreviewAssets() {
		result.Previews[root+"/preview/"+name] = data
	}

	// An unchanged set reuses its complete ZIPs as they are; nothing in
	// them needs to be read or built.
	jobs := r.Jobs()
	guideDocs := make([][]byte, len(r.Editions))
	guideKeys := make([]string, len(r.Editions))
	fonts := r.GuideAssets()
	fontKey := r.Digest()
	for i, e := range r.Editions {
		guideDocs[i] = r.Guide(e)
		guideKeys[i] = printKey("document guide "+e.Key, append(guideDocs[i], fontKey...), nil)
	}
	zipKeys := make([]string, len(r.Editions))
	reuse := !isRelease
	for i, e := range r.Editions {
		keys := []string{guideKeys[i]}
		for _, b := range jobs {
			if b.Edition == e {
				keys = append(keys, b.Key)
			}
		}
		zipKeys[i] = printKey("document zip "+e.Key, []byte(strings.Join(keys, ",")), nil)
		reuse = reuse && pr.has(zipKeys[i])
	}
	if reuse {
		var images []document.Image
		if pictured(r.Listing.Present, r.Summary.Sell) {
			images = documentImages(pr, r, jobs, nil)
		}
		return finishDocument(r, result, out, issues, pageSale, func(i int) []byte { return pr.lookup(zipKeys[i]) }, images, pr.pending)
	}

	built, err := r.Build(func(key string) []byte { return pr.lookup(key) })
	if err != nil {
		return out, append(issues, diagnostic.Diagnostic{Rule: document.RuleSource, Severity: diagnostic.Error, Location: diagnostic.Location{File: document.Dir, Line: 1, Column: 1}, Message: "The documents could not be written: " + err.Error(), Hint: "Fix what the error names, then compile again."})
	}

	// The PDFs are Chrome's prints of each document's print layout, which
	// follows the .docx: stamped and reused while the document and the
	// layout are unchanged, and held to the plan when printed.
	pdfs := map[string][]byte{}
	var mu sync.Mutex
	var found []document.Finding
	var printing sync.WaitGroup
	for _, b := range built {
		printing.Go(func() {
			key := printKey("document print "+document.PrintFormat, []byte(b.Key), nil)
			fresh := !pr.has(key)
			pdf, err := pr.job(key, func(printer *release.Printer) ([]byte, error) {
				return printer.Print(r.PrintDoc(b), fonts, false)
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				warn(b.Doc.File, document.RuleLayout, "The PDF of "+b.Name+" could not be printed: "+err.Error(), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release'.")
			case pdf != nil:
				pdfs[b.Key] = pdf
				if fresh {
					found = append(found, r.Inspect(b, pdf)...)
				}
			}
		})
	}
	printing.Wait()
	if isRelease {
		for _, f := range found {
			warn(f.File, document.RuleLayout, f.Message, "Shorten the text, split the document or change its planned pages; check the theme's fonts draw every character.")
		}
		checked := []string{fmt.Sprintf("%s print as planned", term.Plural(len(built)-len(found), "file"))}
		if len(found) > 0 {
			checked = []string{term.Plural(len(found), "layout finding")}
		}
		// LibreOffice and Word, when present, open the real files too; a
		// release needs neither.
		if soffice, err := office.Find(); err == nil {
			cross, err := r.Verify(soffice, built)
			switch {
			case err != nil:
				warn(document.Dir, document.RuleLayout, "LibreOffice failed: "+err.Error(), "Run 'lfm compile --release' again; reinstall LibreOffice if it keeps failing.")
			case len(cross) == 0:
				checked = append(checked, "LibreOffice agrees")
			default:
				for _, f := range cross {
					warn(f.File, document.RuleLayout, f.Message, "LibreOffice lays the file out differently; shorten the text or change the planned pages.")
				}
				checked = append(checked, term.Plural(len(cross), "finding")+" in LibreOffice")
			}
		}
		// LFM_OFFICE, a switch for lfm's own tests, has Word on a Mac open
		// every file too; it opens windows, so no user gets it.
		if os.Getenv("LFM_OFFICE") != "" && office.HasWord() {
			cross, err := r.Word(built)
			switch {
			case err != nil:
				warn(document.Dir, document.RuleLayout, "Word did not check the documents: "+err.Error(), "Close Word's open dialogs and run the release again.")
			case len(cross) == 0:
				checked = append(checked, "Word agrees")
			default:
				for _, f := range cross {
					warn(f.File, document.RuleLayout, f.Message, "Word refuses or lays out differently what the print shows; fix the document, then release again.")
				}
				checked = append(checked, term.Plural(len(cross), "finding")+" in Word")
			}
		}
		r.Summary.Layout = strings.Join(checked, " · ")
	}

	// The listing images show the real first pages, rendered in a release.
	var shots []document.Shot
	if isRelease && pictured(r.Listing.Present, r.Summary.Sell) {
		if printer, err := pr.printerFor(); err == nil {
			for _, b := range built {
				if b.Edition != r.Showcase() {
					continue
				}
				w, h := document.PagePixels(b.Edition)
				if png, err := printer.Picture(r.ShotDoc(b), fonts, w, h, 2); err == nil {
					shots = append(shots, document.Shot{Key: b.Key, Title: b.Doc.Title, PNG: png})
				}
			}
		}
	}

	guides := make([][]byte, len(r.Editions))
	var images []document.Image
	var wg sync.WaitGroup
	for i, e := range r.Editions {
		wg.Go(func() {
			pdf, err := pr.job(guideKeys[i], func(printer *release.Printer) ([]byte, error) {
				return printer.Print(guideDocs[i], fonts, false)
			})
			if err != nil {
				warn(document.Dir, document.RuleChannels, "The start guide "+r.GuideName(e)+" could not be printed: "+err.Error(), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release'.")
			}
			guides[i] = pdf
		})
	}
	if pictured(r.Listing.Present, r.Summary.Sell) {
		wg.Go(func() { images = documentImages(pr, r, built, shots) })
	}
	wg.Wait()

	zips := make([][]byte, len(r.Editions))
	for i, e := range r.Editions {
		// A complete ZIP, every PDF and the guide in it, carries the key of
		// what it holds and is reused while that is unchanged.
		key := zipKeys[i]
		if guides[i] == nil {
			key = ""
		}
		for _, b := range built {
			if b.Edition == e && pdfs[b.Key] == nil {
				key = ""
			}
		}
		if key != "" {
			zips[i] = pr.lookup(key)
		}
		if zips[i] == nil {
			zips[i], err = r.Zip(e, built, pdfs, guides[i], key)
			if err != nil {
				warn(document.Dir, document.RuleChannels, "The ZIP of "+e.Key+" could not be written: "+err.Error(), "Fix what the error names, then compile again.")
			}
		}
	}
	return finishDocument(r, result, out, issues, pageSale, func(i int) []byte { return zips[i] }, images, pr.pending)
}

// finishDocument publishes the editions' ZIPs, the shop packages and the
// download page.
func finishDocument(r *document.Report, result *Result, out shipped, issues []diagnostic.Diagnostic, pageSale bool, zip func(int) []byte, images []document.Image, pending bool) (shipped, []diagnostic.Diagnostic) {
	for i, e := range r.Editions {
		if data := zip(i); data != nil {
			out.add(deliver.File{Name: r.ZipName(e), Label: strings.ToUpper(e.Lang) + " · " + paperLabel(e.Paper), Note: r.FileNote(e)}, data)
		}
	}
	r.Summary.Pending = pending
	r.Summary.Images = len(images)
	for _, img := range images {
		out.Images = append(out.Images, bundle.Image{Name: img.Name, Data: img.Data})
	}
	for _, f := range out.Files {
		r.Summary.Files = append(r.Summary.Files, f.Name)
	}
	if ownStores(r.Listing.Present, r.Summary.Sell) {
		dist, channels, found := document.Distribution(*r, out.Files, out.Data, images, pending, time.Now().Format("2006-01-02"))
		result.distribute("document", dist)
		r.Summary.Channels = channels
		issues = append(issues, found...)
	} else {
		result.distribute("document", map[string][]byte{})
	}
	if pageSale {
		downloadPage(result, r.Config.Path, document.DownloadPage(*r, out.Files), out)
	}
	return out, issues
}

func paperLabel(paper string) string {
	if paper == "letter" {
		return "US Letter"
	}
	return strings.ToUpper(paper)
}

// documentImages are the listing images, keyed by the files they show:
// reused while those are unchanged, rendered by a release from the first
// pages LibreOffice printed.
func documentImages(pr *press, r *document.Report, built []document.Built, shots []document.Shot) []document.Image {
	var keys []string
	var placeholder []document.Shot // the plan with blank pages names and keys the images
	for _, b := range built {
		if b.Edition == r.Showcase() {
			keys = append(keys, b.Key)
			placeholder = append(placeholder, document.Shot{Key: b.Key, Title: b.Doc.Title})
		}
	}
	basis := []byte(strings.Join(keys, ",") + "|" + strings.Join(r.Facts(), "|"))
	names := r.Images(placeholder)
	// The images' own markup belongs to the key, so a new design renders again.
	for _, plan := range names {
		basis = append(basis, plan.Doc...)
	}
	var out []document.Image
	all := true
	for _, plan := range names {
		key := printKey("document image "+plan.Name, basis, nil)
		if jpg := pr.lookup(key); jpg != nil {
			out = append(out, document.Image{Name: plan.Name, What: plan.What, Data: jpg})
		} else {
			all = false
		}
	}
	if all || len(shots) == 0 {
		if !all {
			pr.mu.Lock()
			pr.pending = true
			pr.mu.Unlock()
		}
		return out
	}
	printer, err := pr.printerFor()
	if err != nil {
		return out
	}
	out = nil
	for _, plan := range r.Images(shots) {
		png, err := printer.Picture(plan.Doc, r.GuideAssets(), 1500, 1125, 2)
		if err != nil {
			continue
		}
		jpg, err := jpegUnder(png, 1<<20, printKey("document image "+plan.Name, basis, nil))
		if err != nil {
			continue
		}
		out = append(out, document.Image{Name: plan.Name, What: plan.What, Data: jpg})
	}
	return out
}
