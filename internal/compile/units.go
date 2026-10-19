package compile

import (
	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
)

// unit is one deliverable of the project as a compile drives it: checked
// beside the others, summarized in the result, held to its offer,
// delivered after the page, gathered by the bundle.
type unit interface {
	// summarize puts the unit's summary into the result.
	summarize(*Result)
	// sale names its lfm.json field, how it sells, its offer and its rule.
	sale(project.Deliverables) (field, sell, offer, rule string, ok bool)
	// deliver builds, prints and publishes it; the files go to the bundle.
	deliver(p project.Project, page []byte, release bool, result *Result) []diagnostic.Diagnostic
	// opens is the file type LibreOffice checks for it, or "".
	opens() string
}

// checking is what a unit's check reads.
type checking struct {
	p          project.Project
	loaded     project.Loaded
	commerce   commercecheck.Facts
	researched research.Report
}

// kinds are the deliverables lfm builds, in the order the report lists
// them; each checks only its own folder.
var kinds = []struct {
	present func(project.Project) bool
	check   func(checking) (unit, []diagnostic.Diagnostic)
}{
	{book.Present, func(c checking) (unit, []diagnostic.Diagnostic) {
		r, found := checkBook(c.p, c.loaded, c.commerce, c.researched)
		return orNil(r, &bookUnit{r: r}), found
	}},
	{printable.Present, func(c checking) (unit, []diagnostic.Diagnostic) {
		r, found := checkPrintable(c.p, c.loaded, c.commerce)
		return orNil(r, &printableUnit{r: r}), found
	}},
	{spreadsheet.Present, func(c checking) (unit, []diagnostic.Diagnostic) {
		r, found := checkSpreadsheet(c.p, c.loaded, c.commerce)
		return orNil(r, &spreadsheetUnit{r: r}), found
	}},
	{document.Present, func(c checking) (unit, []diagnostic.Diagnostic) {
		r, found := checkDocument(c.p, c.loaded, c.commerce)
		return orNil(r, &documentUnit{r: r}), found
	}},
	{deck.Present, func(c checking) (unit, []diagnostic.Diagnostic) {
		r, found := checkDeck(c.p, c.loaded, c.commerce)
		return orNil(r, &deckUnit{r: r}), found
	}},
}

func orNil[R any](r *R, u unit) unit {
	if r == nil {
		return nil
	}
	return u
}

type bookUnit struct {
	r   *book.Report
	out shipped
}

func (u *bookUnit) summarize(res *Result) { res.Book = &u.r.Summary }
func (u *bookUnit) opens() string         { return "" }
func (u *bookUnit) sale(d project.Deliverables) (string, string, string, string, bool) {
	if d.Book == nil {
		return "", "", "", "", false
	}
	return "deliverables.book", d.Book.Sell, d.Book.Offer, book.RuleConfig, true
}
func (u *bookUnit) deliver(p project.Project, page []byte, release bool, res *Result) []diagnostic.Diagnostic {
	var found []diagnostic.Diagnostic
	u.out, found = deliverBook(p, u.r, page, release, res)
	return found
}

type printableUnit struct {
	r      *printable.Report
	out    shipped
	images []printable.Image
}

func (u *printableUnit) summarize(res *Result) { res.Printable = &u.r.Summary }
func (u *printableUnit) opens() string         { return "" }
func (u *printableUnit) sale(d project.Deliverables) (string, string, string, string, bool) {
	if d.Printable == nil {
		return "", "", "", "", false
	}
	return "deliverables.printable", d.Printable.Sell, d.Printable.Offer, printable.RuleConfig, true
}
func (u *printableUnit) deliver(p project.Project, page []byte, release bool, res *Result) []diagnostic.Diagnostic {
	var found []diagnostic.Diagnostic
	u.out, u.images, found = deliverPrintable(p, u.r, page, release, res)
	return found
}

type spreadsheetUnit struct {
	r   *spreadsheet.Report
	out shipped
}

func (u *spreadsheetUnit) summarize(res *Result) { res.Spreadsheet = &u.r.Summary }
func (u *spreadsheetUnit) opens() string         { return ".xlsx" }
func (u *spreadsheetUnit) sale(d project.Deliverables) (string, string, string, string, bool) {
	if d.Spreadsheet == nil {
		return "", "", "", "", false
	}
	return "deliverables.spreadsheet", d.Spreadsheet.Sell, d.Spreadsheet.Offer, spreadsheet.RuleConfig, true
}
func (u *spreadsheetUnit) deliver(p project.Project, page []byte, release bool, res *Result) []diagnostic.Diagnostic {
	var found []diagnostic.Diagnostic
	u.out, found = deliverSpreadsheet(p, u.r, page, release, res)
	return found
}

type documentUnit struct {
	r   *document.Report
	out shipped
}

func (u *documentUnit) summarize(res *Result) { res.Document = &u.r.Summary }
func (u *documentUnit) opens() string         { return ".docx" }
func (u *documentUnit) sale(d project.Deliverables) (string, string, string, string, bool) {
	if d.Document == nil {
		return "", "", "", "", false
	}
	return "deliverables.document", d.Document.Sell, d.Document.Offer, document.RuleConfig, true
}
func (u *documentUnit) deliver(p project.Project, page []byte, release bool, res *Result) []diagnostic.Diagnostic {
	var found []diagnostic.Diagnostic
	u.out, found = deliverDocument(p, u.r, page, release, res)
	return found
}

type deckUnit struct {
	r   *deck.Report
	out shipped
}

func (u *deckUnit) summarize(res *Result) { res.Deck = &u.r.Summary }
func (u *deckUnit) opens() string         { return ".pptx" }
func (u *deckUnit) sale(d project.Deliverables) (string, string, string, string, bool) {
	if d.Deck == nil {
		return "", "", "", "", false
	}
	return "deliverables.deck", d.Deck.Sell, d.Deck.Offer, deck.RuleConfig, true
}
func (u *deckUnit) deliver(p project.Project, page []byte, release bool, res *Result) []diagnostic.Diagnostic {
	var found []diagnostic.Diagnostic
	u.out, found = deliverDeck(p, u.r, page, release, res)
	return found
}

// gather collects the delivered units for the bundle.
func gather(units []unit) delivered {
	var d delivered
	for _, u := range units {
		switch u := u.(type) {
		case *bookUnit:
			d.book, d.books = u.r, u.out
		case *printableUnit:
			d.printable, d.prints, d.printImages = u.r, u.out, u.images
		case *spreadsheetUnit:
			d.spreadsheet, d.grids = u.r, u.out
		case *documentUnit:
			d.document, d.docs = u.r, u.out
		case *deckUnit:
			d.deck, d.decks = u.r, u.out
		}
	}
	return d
}
