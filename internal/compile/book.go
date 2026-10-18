package compile

import (
	"bytes"
	"maps"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/term"
)

// checkBook checks deliverables/book against lfm.json, the offers and the research.
func checkBook(p project.Project, loaded project.Loaded, commerce commercecheck.Facts, researched research.Report) (*book.Report, []diagnostic.Diagnostic) {
	c := loaded.Config
	var cfg book.Config
	configured := c.Deliverables.Book != nil
	if configured {
		b := c.Deliverables.Book
		cfg = book.Config{Author: b.Author, Language: b.Language, Path: b.Path, Offer: b.Offer, Formats: b.Formats,
			Publisher: b.Publisher, Imprint: b.Imprint, Paper: b.Paper, ISBNEbook: b.ISBN.Ebook, ISBNPaperback: b.ISBN.Paperback}
	}
	if cfg.Language == "" {
		cfg.Language = c.Language
	}
	offers := map[string]*book.Price{}
	for _, offer := range commerce.Offers {
		var price *book.Price
		if m := offer.Price; m != nil && m.Scale > 0 {
			amount := new(big.Rat).SetFrac64(m.AmountMinor, m.Scale)
			currency := strings.ToUpper(m.Currency)
			price = &book.Price{Text: amount.FloatString(2) + " " + currency, Amount: amount, Currency: currency}
		}
		offers[strings.TrimPrefix(offer.Path, "data.commerce.offers.")] = price
	}
	r := book.Check(p, cfg, configured, offers, c.URL, researched.Voices, researched.Competitors)
	sources := jsonsrc.Index(loaded.Sources)
	for i, d := range r.Diagnostics {
		if d.Path == "" {
			continue
		}
		if loc, ok := sources.Nearest(d.Path); ok {
			r.Diagnostics[i].Location = loc
		}
	}
	return &r, r.Diagnostics
}

// deliverBook adds the download page and, once every chapter is written, the
// book's files to the output, under the unlisted path.
func deliverBook(p project.Project, r *book.Report, page []byte, isRelease bool, result *Result) []diagnostic.Diagnostic {
	if r.Config.Path == "" || !book.ValidPath(r.Config.Path) || r.Summary.URL == "" {
		return nil
	}
	var issues []diagnostic.Diagnostic
	at := func(sev diagnostic.Severity, rule, message, hint string) {
		issues = append(issues, diagnostic.Diagnostic{Rule: rule, Severity: sev, Location: diagnostic.Location{File: book.ConceptFile, Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	if bytes.Contains(page, []byte(r.Config.Path)) {
		issues = append(issues, diagnostic.Diagnostic{Rule: book.RuleDelivery, Severity: diagnostic.Error, Location: diagnostic.Location{File: "index.html", Line: 1, Column: 1},
			Message: "The page contains the book's download path; anyone could download it without paying.",
			Hint:    "Remove every link to the download page from the page. Buyers reach it through Stripe's after-payment redirect and the invoice email only."})
		return issues
	}
	var files []book.File
	if r.Buildable {
		built, err := book.Build(*r, func(rel string) ([]byte, error) { return p.Read(book.Dir + "/" + rel) }, newest(p, r))
		if err != nil {
			at(diagnostic.Error, book.RuleBuild, "The book could not be built: "+err.Error(), "Fix the named file and compile again.")
			return issues
		}
		slug := book.Slug(r.Plan.Title)
		if r.Config.Wants("epub") {
			files = append(files, book.File{Name: slug + ".epub", Label: "EPUB", Note: book.Words(r.Config.Language).EPUBNote, Size: len(built.EPUB)})
			result.Artifact[r.Config.Path+"/"+slug+".epub"] = built.EPUB
		}
		pr := newPress(p, r.Config.Path, isRelease)
		defer pr.close()
		var pdf []byte
		var pdfErr, layoutErr error
		var layout []string
		var wg sync.WaitGroup
		if r.Config.Wants("pdf") || r.Listing.Present {
			wg.Go(func() { pdf, pdfErr = pr.screen(built) })
		}
		var printed book.Printed
		if r.Listing.Present {
			wg.Go(func() { printed = pr.paperback(r, built) })
		}
		if isRelease {
			wg.Go(func() {
				if printer, err := pr.printerFor(); err != nil {
					layoutErr = err
				} else {
					layout, layoutErr = printer.BookLayout(built.Print, built.PrintAssets, built.TrimInches[0]-1.4)
				}
			})
		}
		wg.Wait()
		r.Summary.Pending = pr.pending
		if r.Config.Wants("pdf") && pdf != nil {
			files = append(files, book.File{Name: slug + ".pdf", Label: "PDF", Note: book.Words(r.Config.Language).PDFNote, Size: len(pdf)})
			result.Artifact[r.Config.Path+"/"+slug+".pdf"] = pdf
		}
		// The PDF is a deliverable when the formats name it, and a store file
		// otherwise; a browser that cannot start is reported once for both.
		printRule := book.RuleChannels
		if r.Config.Wants("pdf") {
			printRule = book.RulePDF
		}
		switch {
		case pr.startErr != nil:
			at(diagnostic.Warning, printRule, "The PDF and the paperback were not printed: "+pr.startErr.Error(), "Install Chrome or Chromium, or set LFM_CHROME; or remove pdf from deliverables.book.formats.")
		default:
			if pdfErr != nil {
				at(diagnostic.Warning, printRule, "The PDF could not be printed: "+pdfErr.Error(), "Check the book's fonts and images, then compile again.")
			}
			for _, problem := range printed.Problems {
				at(diagnostic.Warning, book.RuleChannels, problem, "Check the book's fonts, images and book.css, then run 'lfm compile --release' again.")
			}
		}
		if !r.Listing.Present {
			result.Distribution = map[string][]byte{} // packages of a removed listing go too
		}
		if r.Listing.Present {
			printed.PDF = pdf
			printed.Pending = pr.pending
			printed.LastCover, _ = os.ReadFile(filepath.Join(p.Root, project.DistributionDir, "cover.jpg"))
			files, channels, found := book.Distribution(*r, built, printed, time.Now().Format("2006-01-02"))
			result.Distribution = files
			r.Summary.Channels = channels
			issues = append(issues, found...)
		}
		if isRelease {
			switch {
			case layoutErr != nil:
				at(diagnostic.Warning, book.RuleLayout, "The book layout check could not run: "+layoutErr.Error(), "Install Chrome or Chromium, or set LFM_CHROME.")
			default:
				for _, f := range layout {
					at(diagnostic.Warning, book.RuleLayout, f, "Let the element shrink or wrap: images scale with max-width, tables split, code blocks wrap.")
				}
				r.Summary.Layout = "fits at phone width and in print"
				if len(layout) > 0 {
					r.Summary.Layout = term.Plural(len(layout), "layout finding")
				}
			}
		}
	}
	download := book.DownloadPage(r.Plan, r.Config, files)
	result.Artifact[r.Config.Path+"/index.html"] = download
	result.Files = append(result.Files, File{r.Config.Path + "/index.html", len(download)})
	for _, f := range files {
		r.Summary.Files = append(r.Summary.Files, f.Name)
		result.Files = append(result.Files, File{r.Config.Path + "/" + f.Name, f.Size})
	}
	return issues
}

// newest is the latest change among the book's sources, the EPUB's modified
// date. The sources were resolved and read already; their times are taken by
// name.
func newest(p project.Project, r *book.Report) time.Time {
	var t time.Time
	for _, rel := range append(slices.Collect(maps.Keys(r.Sources)), "concept.md") {
		if info, err := os.Stat(filepath.Join(p.Root, filepath.FromSlash(book.Dir+"/"+rel))); err == nil && info.ModTime().After(t) {
			t = info.ModTime()
		}
	}
	if t.IsZero() {
		t = time.Unix(0, 0)
	}
	return t
}
