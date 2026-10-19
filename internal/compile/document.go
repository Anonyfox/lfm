package compile

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/buildid"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/paper"
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
// Word lays them out, checks what every app will show, renders the listing
// images from the real first pages and prints the start guides; later
// compiles reuse those. Each edition ships as one ZIP.
func deliverDocument(p project.Project, r *document.Report, page []byte, st *stock, result *Result) (shipped, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	s, stop := startSale(r.Summary.Sell, r.Config.Path, r.Summary.URL, page, document.RuleDelivery, "document")
	if s.skip || stop != nil {
		return out, stop
	}
	w := &warnings{}
	if !r.Buildable {
		if s.page {
			downloadPage(result, r.Config.Path, document.DownloadPage(*r, nil), out)
		}
		return out, nil
	}
	keys := make([]string, len(r.Editions))
	for i, e := range r.Editions {
		keys[i] = e.Key
	}
	pr := s.open(st, result, keys, func(i int) []byte { return r.Preview(r.Editions[i]) }, r.PreviewAssets())

	// Every print is keyed by what Chrome prints, every file by its inputs
	// and the binary: an unchanged set reuses its complete ZIPs as they are,
	// and a newer lfm writes the files anew but keeps the PDFs.
	jobs := r.Jobs()
	fonts, fontKey := r.GuideAssets(), r.Digest()
	guideDocs := make([][]byte, len(r.Editions))
	guideKeys := make([]string, len(r.Editions))
	guideNames := make([]string, len(r.Editions))
	for i, e := range r.Editions {
		guideDocs[i] = r.Guide(e)
		guideKeys[i] = printKey("document guide "+e.Key, append(guideDocs[i], fontKey...), nil)
		guideNames[i] = r.GuideName(e)
	}
	printKeys := map[string]string{}
	for _, b := range jobs {
		printKeys[b.Key] = printKey("document print", append(r.PrintDoc(b), fontKey...), nil)
	}
	zipKeys := make([]string, len(r.Editions))
	reuse := !st.release
	for i, e := range r.Editions {
		parts := []string{buildid.Writer(), guideKeys[i]}
		for _, b := range jobs {
			if b.Edition == e {
				parts = append(parts, b.Key, printKeys[b.Key])
			}
		}
		zipKeys[i] = printKey("document zip "+e.Key, []byte(strings.Join(parts, ",")), nil)
		reuse = reuse && pr.has(zipKeys[i])
	}
	pictures := pictured(r.Listing.Present, r.Summary.Sell)
	if reuse {
		var images []document.Image
		if pictures {
			images = documentImages(pr, r, jobs, printKeys, nil)
		}
		return finishDocument(r, result, out, w.out, s.page, func(i int) []byte { return pr.lookup(zipKeys[i]) }, images, pr.pending)
	}

	built, err := r.Build(func(key string) []byte { return pr.lookup(key) })
	if err != nil {
		return out, []diagnostic.Diagnostic{{Rule: document.RuleSource, Severity: diagnostic.Error, Location: diagnostic.Location{File: document.Dir, Line: 1, Column: 1}, Message: "The documents could not be written: " + err.Error(), Hint: "Fix what the error names, then compile again."}}
	}
	prints := make([]printJob, len(built))
	for i, b := range built {
		prints[i] = printJob{key: printKeys[b.Key], slot: b.Key, file: b.Doc.File, name: b.Name,
			doc: func() []byte { return r.PrintDoc(b) },
			checks: func(pdf []byte, _ *release.Printer) []finding {
				return documentFindings(r.Inspect(b, pdf))
			}}
	}
	pdfs, found := printAll(pr, prints, fonts, document.RuleLayout, w)
	if st.release {
		for _, f := range found {
			w.add(f.File, document.RuleLayout, f.Message, "Shorten the text, split the document or change its planned pages; check the theme's fonts draw every character.")
		}
		checked := []string{fmt.Sprintf("%s print as planned", term.Plural(len(built)-len(found), "file"))}
		if len(found) > 0 {
			checked = []string{term.Plural(len(found), "layout finding")}
		}
		// LibreOffice, when present, opens the real files too; a release
		// needs none of it. LFM_OFFICE, a switch for lfm's own tests, has
		// Word on a Mac open every file too; it opens windows, so no user
		// gets it.
		apps := []opinion{libreOffice(func(soffice string) ([]finding, error) {
			f, err := r.Verify(soffice, built)
			return documentFindings(f), err
		}, "LibreOffice lays the file out differently; shorten the text or change the planned pages.")}
		if os.Getenv("LFM_OFFICE") != "" && office.HasWord() {
			apps = append(apps, opinion{app: "Word", run: func() ([]finding, error) {
				f, err := r.Word(built)
				return documentFindings(f), err
			}, failHint: "Close Word's open dialogs and run the release again.", differHint: "Word refuses or lays out differently what the print shows; fix the document, then release again."})
		}
		r.Summary.Layout = strings.Join(append(checked, secondOpinions(document.Dir, document.RuleLayout, apps, w)...), " · ")
	}

	// The listing images show the real first pages, rendered in a release.
	var shots []document.Shot
	if st.release && pictures {
		if printer, err := pr.printerFor(); err == nil {
			for _, b := range built {
				if b.Edition != r.Showcase() {
					continue
				}
				pw, ph := document.PagePixels(b.Edition)
				if png, err := printer.Picture(r.ShotDoc(b), fonts, pw, ph, 2); err == nil {
					shots = append(shots, document.Shot{Key: b.Key, Title: b.Doc.Title, PNG: png})
				}
			}
		}
	}
	var guides [][]byte
	var images []document.Image
	var wg sync.WaitGroup
	wg.Go(func() {
		guides = printGuides(pr, guideKeys, guideDocs, guideNames, fonts, document.Dir, document.RuleChannels, w)
	})
	if pictures {
		wg.Go(func() { images = documentImages(pr, r, built, printKeys, shots) })
	}
	wg.Wait()

	zips := make([][]byte, len(r.Editions))
	for i, e := range r.Editions {
		complete := guides[i] != nil
		for _, b := range built {
			if b.Edition == e && pdfs[b.Key] == nil {
				complete = false
			}
		}
		zips[i], err = editionZip(pr, zipKeys[i], complete, func(key string) ([]byte, error) { return r.Zip(e, built, pdfs, guides[i], key) })
		if err != nil {
			w.add(document.Dir, document.RuleChannels, "The ZIP of "+e.Key+" could not be written: "+err.Error(), "Fix what the error names, then compile again.")
		}
	}
	return finishDocument(r, result, out, w.out, s.page, func(i int) []byte { return zips[i] }, images, pr.pending)
}

func documentFindings(in []document.Finding) []finding {
	out := make([]finding, len(in))
	for i, f := range in {
		out[i] = finding{f.File, f.Message}
	}
	return out
}

// finishDocument publishes the editions' ZIPs, the shop packages and the
// download page.
func finishDocument(r *document.Report, result *Result, out shipped, issues []diagnostic.Diagnostic, pageSale bool, zip func(int) []byte, images []document.Image, pending bool) (shipped, []diagnostic.Diagnostic) {
	for i, e := range r.Editions {
		if data := zip(i); data != nil {
			out.add(deliver.File{Name: r.ZipName(e), Label: strings.ToUpper(e.Lang) + " · " + paper.Label(e.Paper), Note: r.FileNote(e)}, data)
		}
	}
	r.Summary.Pending = pending
	r.Summary.Images = len(images)
	out.Images = images
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

// documentImages are the listing images: the plan with blank pages names
// and keys them, a release renders them from the first pages printed.
// They show the print layout, so they key on the prints: a newer lfm that
// writes the files anew keeps them.
func documentImages(pr *press, r *document.Report, built []document.Built, printKeys map[string]string, shots []document.Shot) []document.Image {
	var keys []string
	var placeholder []document.Shot
	for _, b := range built {
		if b.Edition == r.Showcase() {
			keys = append(keys, printKeys[b.Key])
			placeholder = append(placeholder, document.Shot{Key: b.Key, Title: b.Doc.Title})
		}
	}
	basis := []byte(strings.Join(keys, ",") + "|" + strings.Join(r.Facts(), "|"))
	names := r.Images(placeholder)
	for _, plan := range names {
		basis = append(basis, plan.Doc...)
	}
	planned := make([]listingPlan, len(names))
	for i, plan := range names {
		planned[i] = listingPlan{Name: plan.Name, Key: printKey("document image "+plan.Name, basis, nil)}
	}
	return listingImages(pr, planned, func(*release.Printer) ([]listingPlan, map[string][]byte, error) {
		if len(shots) == 0 {
			return nil, nil, nil
		}
		var plans []listingPlan
		for _, plan := range r.Images(shots) {
			plans = append(plans, listingPlan{Name: plan.Name, Doc: plan.Doc})
		}
		return plans, r.GuideAssets(), nil
	})
}
