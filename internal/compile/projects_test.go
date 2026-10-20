package compile

import "github.com/Anonyfox/lfm/internal/testproject"

// The projects the deliverable tests compile, shared with the CLI's tests.
const (
	bookPath      = testproject.BookPath
	printablePath = testproject.PrintablePath
	sheetPath     = testproject.SheetPath
	docPath       = testproject.DocPath
	deckPath      = testproject.DeckPath
	bundlePath    = testproject.BundlePath
)

var (
	configure          = testproject.Configure
	writePrintable     = testproject.WritePrintable
	bookProject        = testproject.Book
	printableProject   = testproject.Printable
	spreadsheetProject = testproject.Spreadsheet
	documentProject    = testproject.Document
	deckProject        = testproject.Deck
)
