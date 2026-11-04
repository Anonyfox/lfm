package compile

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
	"github.com/Anonyfox/lfm/internal/term"
)

// checkSpreadsheet checks deliverables/spreadsheet against lfm.json and the offers.
func checkSpreadsheet(p project.Project, loaded project.Loaded, commerce commercecheck.Facts) (*spreadsheet.Report, []diagnostic.Diagnostic) {
	c := loaded.Config
	var cfg spreadsheet.Config
	configured := c.Deliverables.Spreadsheet != nil
	if configured {
		s := c.Deliverables.Spreadsheet
		cfg = spreadsheet.Config{Path: s.Path, Offer: s.Offer, Language: s.Language, Sell: s.Sell, Version: s.Version, Sheets: s.Sheets}
	}
	cfg.Seller = c.Seller.Name
	if cfg.Language == "" {
		cfg.Language = c.Language
	}
	r := spreadsheet.Check(p, cfg, configured, offerPrices(commerce), c.URL)
	var sources *jsonsrc.Sources
	for i, d := range r.Diagnostics {
		if d.Location.File == "lfm.json" {
			if sources == nil {
				index := jsonsrc.Index(loaded.Sources)
				sources = &index
			}
			if loc, ok := sources.Nearest("deliverables.spreadsheet"); ok {
				r.Diagnostics[i].Location = loc
			}
		}
	}
	return &r, r.Diagnostics
}

// deliverSpreadsheet builds the .xlsx files on every compile (they take
// milliseconds), prints the start PDFs and renders the listing images in a
// release or reuses them, recalculates the files in LibreOffice in a
// release, and publishes the download page and shop packages.
func deliverSpreadsheet(p project.Project, r *spreadsheet.Report, page []byte, st *stock, result *Result) (shipped, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	s, stop := startSale(r.Summary.Sell, r.Config.Path, r.Summary.URL, page, spreadsheet.RuleDelivery, "spreadsheet")
	if s.skip || stop != nil {
		return out, stop
	}
	w := &warnings{}
	if !r.Buildable {
		if s.page {
			downloadPage(result, r.Config.Path, spreadsheet.DownloadPage(*r, nil), out)
		}
		return out, w.out
	}
	keys := make([]string, len(r.Workbook.Editions))
	for i, e := range r.Workbook.Editions {
		keys[i] = e.Key
	}
	pr := s.open(st, result, keys, func(i int) []byte { return r.Preview(keys[i]) }, nil)
	starts := r.StartDocs()
	var plans []spreadsheet.ImagePlan
	if pictured(r.Listing.Present, r.Summary.Sell) {
		plans = r.Images()
	}
	built, err := r.Build(func(key string) []byte { return pr.lookup(key) })
	if err != nil {
		return out, append(w.out, diagnostic.Diagnostic{Rule: spreadsheet.RuleWorkbook, Severity: diagnostic.Error, Location: diagnostic.Location{File: spreadsheet.WorkbookFile, Line: 1, Column: 1}, Message: "The workbook could not be written: " + err.Error(), Hint: "Fix what the error names, then compile again."})
	}
	for _, f := range built {
		out.add(deliver.File{Name: f.Name, Label: label(f), Note: r.FileNote(f.Name)}, f.Data)
	}

	names := slices.Sorted(maps.Keys(starts))
	var jobs []printJob
	for _, name := range names {
		doc := starts[name]
		jobs = append(jobs, printJob{key: printKey("spreadsheet start "+name, doc, nil), slot: name, file: spreadsheet.WorkbookFile, name: "start guide " + name, doc: func() []byte { return doc }})
	}
	printed, _ := printAll(pr, jobs, nil, spreadsheet.RuleChannels, w)
	var images []spreadsheet.Image
	if pictured(r.Listing.Present, r.Summary.Sell) {
		images = spreadsheetImages(pr, plans, r.Summary.Sell)
	}
	var compat []string
	var compatErr error
	if st.release {
		office, err := spreadsheet.Office()
		if err != nil {
			compatErr = err
		} else {
			compat, compatErr = remembered(office, "spreadsheet", builtData(built, func(b spreadsheet.Built) []byte { return b.Data }), func() ([]string, error) {
				return r.Compat(pr.ctx, office, built)
			})
		}
	}

	for _, name := range names {
		if pdf := printed[name]; pdf != nil {
			out.add(deliver.File{Name: name, Label: "PDF", Note: r.FileNote(name)}, pdf)
		}
	}
	r.Summary.Pending = pr.pending
	r.Summary.Images = len(images)
	if st.release {
		switch {
		case office.Missing(compatErr):
			// A second opinion only: lfm's engine computed every formula.
			r.Summary.Compat = "lfm computed every formula"
		case compatErr != nil:
			w.add(spreadsheet.WorkbookFile, spreadsheet.RuleCompat, "The files were not recalculated in LibreOffice: "+compatErr.Error(), "Run the release again; reinstall LibreOffice if it keeps failing.")
		case len(compat) > 0:
			for _, d := range compat {
				w.add(spreadsheet.WorkbookFile, spreadsheet.RuleCompat, d, "A formula one app computes differently breaks the template for its buyers; rewrite it with the portable functions.")
			}
			r.Summary.Compat = term.Plural(len(compat), "difference") + " in LibreOffice"
		default:
			r.Summary.Compat = fmt.Sprintf("LibreOffice agrees on %s", term.Plural(len(built), "file"))
		}
		// LFM_OFFICE, a switch for lfm's own tests, has Excel on a Mac
		// recalculate every file too; it opens windows, so no user gets it.
		if os.Getenv("LFM_OFFICE") != "" && office.HasExcel() {
			diffs, err := r.Excel(built)
			switch {
			case err != nil:
				w.add(spreadsheet.WorkbookFile, spreadsheet.RuleCompat, "Excel did not check the files: "+err.Error(), "Close Excel's open dialogs and run the release again.")
			case len(diffs) == 0:
				r.Summary.Compat += fmt.Sprintf(" · Excel agrees on %s", term.Plural(len(built), "file"))
			default:
				for _, d := range diffs {
					w.add(spreadsheet.WorkbookFile, spreadsheet.RuleCompat, d, "A file Excel refuses or computes differently breaks the template for most buyers; fix it, then release again.")
				}
				r.Summary.Compat += " · " + term.Plural(len(diffs), "difference") + " in Excel"
			}
		}
	}
	out.Images = images
	r.Summary.Files = allFiles(out)
	if ownStores(r.Listing.Present, r.Summary.Sell) {
		dist, channels, found := spreadsheet.Distribution(*r, out.Files, out.Data, images, r.Summary.Pending, time.Now().Format("2006-01-02"))
		result.distribute("spreadsheet", dist)
		r.Summary.Channels = channels
		w.out = append(w.out, found...)
	} else {
		result.distribute("spreadsheet", map[string][]byte{})
	}
	if s.page {
		downloadPage(result, r.Config.Path, spreadsheet.DownloadPage(*r, out.Files), out)
	}
	return out, w.out
}

func label(f spreadsheet.Built) string {
	l := "Excel " + strings.ToUpper(f.Edition.Key)
	if f.Dataset == spreadsheet.Example {
		l += " example"
	}
	return l
}

// spreadsheetImages are the listing images: reused while their documents
// are unchanged, rendered by a release otherwise.
func spreadsheetImages(pr *press, plans []spreadsheet.ImagePlan, sell string) []spreadsheet.Image {
	planned := make([]listingPlan, len(plans))
	for i, plan := range plans {
		planned[i] = listingPlan{Name: plan.Name, Doc: plan.Doc, Key: printKey("spreadsheet image "+plan.Name, plan.Doc, nil)}
	}
	// The plans render the workbook's own HTML; no printer step comes first.
	return listingImages(pr, "spreadsheet", sell, planned, func(_ *release.Printer, missing []listingPlan) ([]listingPlan, map[string][]byte, error) {
		return missing, nil, nil
	})
}
