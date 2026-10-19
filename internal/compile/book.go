package compile

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/deliver"

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
			Publisher: c.Seller.Name, Imprint: c.Seller.Imprint, Paper: b.Paper, ISBNEbook: b.ISBN.Ebook, ISBNPaperback: b.ISBN.Paperback, Sell: b.Sell}
	}
	if cfg.Language == "" {
		cfg.Language = c.Language
	}
	offers := offerPrices(commerce)
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

// shipped is what a deliverable built for buyers: its downloads with their
// bytes, which its own download page and a bundle's offer.
type shipped struct {
	Files []deliver.File
	Data  map[string][]byte
}

func (s *shipped) add(f deliver.File, data []byte) {
	f.Size = len(data)
	s.Files = append(s.Files, f)
	s.Data[f.Name] = data
}

// leaks reports an error when the page contains a download path.
func leaks(page []byte, path, rule, what string) []diagnostic.Diagnostic {
	if path == "" || !bytes.Contains(page, []byte(path)) {
		return nil
	}
	return []diagnostic.Diagnostic{{Rule: rule, Severity: diagnostic.Error, Location: diagnostic.Location{File: "index.html", Line: 1, Column: 1},
		Message: "The page contains the " + what + "'s download path; anyone could download it without paying.",
		Hint:    "Remove every link to the download page from the page. Buyers reach it through the checkout's after-payment redirect and the invoice email only."}}
}

// downloadPage publishes a download page and its files under path.
func downloadPage(result *Result, path string, page []byte, s shipped) {
	result.Artifact[path+"/index.html"] = page
	result.Files = append(result.Files, File{path + "/index.html", len(page)})
	for _, f := range s.Files {
		result.Artifact[path+"/"+f.Name] = s.Data[f.Name]
		result.Files = append(result.Files, File{path + "/" + f.Name, f.Size})
	}
}

// deliverBook builds the book once every chapter is written, publishes its
// download page when it sells through the page, and assembles its store
// packages when it has a listing and sells on its own. It returns the files
// for a bundle.
func deliverBook(p project.Project, r *book.Report, page []byte, isRelease bool, result *Result) (shipped, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	sell := r.Summary.Sell
	pageSale := sell == deliver.SellPage && deliver.ValidPath(r.Config.Path) && r.Summary.URL != ""
	if sell == deliver.SellPage && !pageSale {
		return out, nil // the config finding says what is missing
	}
	if pageSale {
		if leak := leaks(page, r.Config.Path, book.RuleDelivery, "book"); leak != nil {
			return out, leak
		}
	}
	ownStores := sell != deliver.SellBundle && r.Listing.Present
	var issues []diagnostic.Diagnostic
	at := func(sev diagnostic.Severity, rule, message, hint string) {
		issues = append(issues, diagnostic.Diagnostic{Rule: rule, Severity: sev, Location: diagnostic.Location{File: book.ConceptFile, Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	if r.Buildable {
		built, err := book.Build(*r, func(rel string) ([]byte, error) { return p.Read(book.Dir + "/" + rel) }, newest(p, r))
		if err != nil {
			at(diagnostic.Error, book.RuleBuild, "The book could not be built: "+err.Error(), "Fix the named file and compile again.")
			return out, issues
		}
		slug := deliver.Slug(r.Plan.Title, "book")
		if r.Config.Wants("epub") {
			out.add(deliver.File{Name: slug + ".epub", Label: "EPUB", Note: book.Words(r.Config.Language).EPUBNote}, built.EPUB)
		}
		pr := newPress(p, isRelease)
		defer pr.close()
		var pdf []byte
		var pdfErr, layoutErr error
		var layout []string
		var wg sync.WaitGroup
		if r.Config.Wants("pdf") || ownStores {
			wg.Go(func() { pdf, pdfErr = pr.screen(built) })
		}
		var printed book.Printed
		if ownStores {
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
			out.add(deliver.File{Name: slug + ".pdf", Label: "PDF", Note: book.Words(r.Config.Language).PDFNote}, pdf)
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
		if ownStores {
			printed.PDF = pdf
			printed.Pending = pr.pending
			printed.LastCover, _ = os.ReadFile(filepath.Join(p.Root, project.DistributionDir, "book", "cover.jpg"))
			files, channels, found := book.Distribution(*r, built, printed, time.Now().Format("2006-01-02"))
			result.distribute("book", files)
			r.Summary.Channels = channels
			issues = append(issues, found...)
		} else {
			result.distribute("book", map[string][]byte{}) // no package of its own
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
	for _, f := range out.Files {
		r.Summary.Files = append(r.Summary.Files, f.Name)
	}
	if pageSale {
		downloadPage(result, r.Config.Path, book.DownloadPage(r.Plan, r.Config, out.Files), out)
	}
	return out, issues
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
