package compile

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
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
func deliverSpreadsheet(p project.Project, r *spreadsheet.Report, page []byte, isRelease bool, result *Result) (shipped, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	pageSale := r.Summary.Sell == deliver.SellPage && deliver.ValidPath(r.Config.Path) && r.Summary.URL != ""
	if r.Summary.Sell == deliver.SellPage && !pageSale {
		return out, nil
	}
	if pageSale {
		if leak := leaks(page, r.Config.Path, spreadsheet.RuleDelivery, "spreadsheet"); leak != nil {
			return out, leak
		}
	}
	var issues []diagnostic.Diagnostic
	warn := func(rule, message, hint string) {
		issues = append(issues, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: spreadsheet.WorkbookFile, Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	root := "spreadsheet"
	if pageSale {
		root = r.Config.Path
	}
	if result.Previews == nil {
		result.Previews = map[string][]byte{}
	}
	if !r.Buildable {
		if pageSale {
			downloadPage(result, r.Config.Path, spreadsheet.DownloadPage(*r, nil), out)
		}
		return out, issues
	}
	// The previews, the documents to print and the scan for earlier prints
	// do not depend on each other.
	var pr *press
	var starts map[string][]byte
	var plans []spreadsheet.ImagePlan
	previews := make([][]byte, len(r.Workbook.Editions))
	var prep sync.WaitGroup
	prep.Go(func() { pr = newPress(p, isRelease) })
	prep.Go(func() { starts = r.StartDocs() })
	if pictured(r.Listing.Present, r.Summary.Sell) {
		prep.Go(func() { plans = r.Images() })
	}
	for i, e := range r.Workbook.Editions {
		prep.Go(func() { previews[i] = r.Preview(e.Key) })
	}
	prep.Wait()
	for i, e := range r.Workbook.Editions {
		result.Previews[root+"/preview/"+e.Key+".html"] = previews[i]
	}
	defer pr.close()
	built, err := r.Build(func(key string) []byte { return pr.lookup(key) })
	if err != nil {
		return out, append(issues, diagnostic.Diagnostic{Rule: spreadsheet.RuleWorkbook, Severity: diagnostic.Error, Location: diagnostic.Location{File: spreadsheet.WorkbookFile, Line: 1, Column: 1}, Message: "The workbook could not be written: " + err.Error(), Hint: "Fix what the error names, then compile again."})
	}
	for _, f := range built {
		out.add(deliver.File{Name: f.Name, Label: label(f), Note: r.FileNote(f.Name)}, f.Data)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	printed := map[string][]byte{}
	for name, doc := range starts {
		wg.Go(func() {
			pdf, err := pr.job(printKey("spreadsheet start "+name, doc, nil), func(printer *release.Printer) ([]byte, error) {
				return printer.Print(doc, nil, false)
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				warn(spreadsheet.RuleChannels, "The start PDF "+name+" could not be printed: "+err.Error(), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release'.")
			} else if pdf != nil {
				printed[name] = pdf
			}
		})
	}
	var images []spreadsheet.Image
	if pictured(r.Listing.Present, r.Summary.Sell) {
		wg.Go(func() { images = spreadsheetImages(pr, plans) })
	}
	var compat []string
	var compatErr error
	if isRelease {
		wg.Go(func() {
			office, err := spreadsheet.Office()
			if err != nil {
				compatErr = err
				return
			}
			compat, compatErr = r.Compat(office, built)
		})
	}
	wg.Wait()
	names := make([]string, 0, len(printed))
	for name := range printed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out.add(deliver.File{Name: name, Label: "PDF", Note: r.FileNote(name)}, printed[name])
	}
	r.Summary.Pending = pr.pending
	r.Summary.Images = len(images)
	if isRelease {
		switch {
		case office.Missing(compatErr):
			// A second opinion only: lfm's engine computed every formula.
			r.Summary.Compat = "lfm computed every formula"
		case compatErr != nil:
			warn(spreadsheet.RuleCompat, "The files were not recalculated in LibreOffice: "+compatErr.Error(), "Run the release again; reinstall LibreOffice if it keeps failing.")
		case len(compat) > 0:
			for _, d := range compat {
				warn(spreadsheet.RuleCompat, d, "A formula one app computes differently breaks the template for its buyers; rewrite it with the portable functions.")
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
				warn(spreadsheet.RuleCompat, "Excel did not check the files: "+err.Error(), "Close Excel's open dialogs and run the release again.")
			case len(diffs) == 0:
				r.Summary.Compat += fmt.Sprintf(" · Excel agrees on %s", term.Plural(len(built), "file"))
			default:
				for _, d := range diffs {
					warn(spreadsheet.RuleCompat, d, "A file Excel refuses or computes differently breaks the template for most buyers; fix it, then release again.")
				}
				r.Summary.Compat += " · " + term.Plural(len(diffs), "difference") + " in Excel"
			}
		}
	}
	out.Images = images
	for _, f := range out.Files {
		r.Summary.Files = append(r.Summary.Files, f.Name)
	}
	if ownStores(r.Listing.Present, r.Summary.Sell) {
		dist, channels, found := spreadsheet.Distribution(*r, out.Files, out.Data, images, r.Summary.Pending, time.Now().Format("2006-01-02"))
		result.distribute("spreadsheet", dist)
		r.Summary.Channels = channels
		issues = append(issues, found...)
	} else {
		result.distribute("spreadsheet", map[string][]byte{})
	}
	if pageSale {
		downloadPage(result, r.Config.Path, spreadsheet.DownloadPage(*r, out.Files), out)
	}
	return out, issues
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
func spreadsheetImages(pr *press, plans []spreadsheet.ImagePlan) []spreadsheet.Image {
	planned := make([]listingPlan, len(plans))
	for i, plan := range plans {
		planned[i] = listingPlan{Name: plan.Name, Doc: plan.Doc, Key: printKey("spreadsheet image "+plan.Name, plan.Doc, nil)}
	}
	// The plans render the workbook's own HTML; no printer step comes first.
	return listingImages(pr, planned, func(*release.Printer) ([]listingPlan, map[string][]byte, error) { return planned, nil, nil })
}
