// Package spreadsheet is the spreadsheet deliverable: budget, bookkeeping and
// tracker templates described in workbook.json, built as .xlsx files per
// edition, computed and checked by lfm's own formula engine on every compile
// and, when installed, by LibreOffice on a release, and delivered with a Google Sheets copy
// link the owner creates once.
package spreadsheet

import (
	"os"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

// The layout under deliverables/spreadsheet.
const (
	Dir          = "deliverables/spreadsheet"
	ConceptFile  = Dir + "/concept.md"
	WorkbookName = "workbook.json"
	WorkbookFile = Dir + "/" + WorkbookName
	ListingFile  = Dir + "/listing.md"
)

// Rule IDs.
const (
	RuleConfig   = "spreadsheet.config"
	RuleConcept  = "spreadsheet.concept"
	RuleWorkbook = "spreadsheet.workbook"
	RuleFormulas = "spreadsheet.formulas"
	RuleCompat   = "spreadsheet.compat"
	RuleSheets   = "spreadsheet.sheets"
	RuleDelivery = "spreadsheet.delivery"
	RuleListing  = "spreadsheet.listing"
	RuleChannels = "spreadsheet.channels"
)

// Rules describes every spreadsheet rule for the catalog.
func Rules() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{ID: RuleConfig, Required: true, Summary: "A spreadsheet in deliverables/spreadsheet has its download path in lfm.json",
			Help: "Errors when deliverables/spreadsheet exists but lfm.json has no deliverables.spreadsheet, when it sells through the page without a download path of at least 16 lowercase letters, digits and hyphens, or when its offer is not in data.commerce.offers. The path is the only lock on the files."},
		{ID: RuleConcept, Summary: "deliverables/spreadsheet/concept.md plans the buyer, promise, tabs and look",
			Help: "Warns for missing sections, a buyer without \"Not for:\", a Tabs table that does not list every sheet of workbook.json by key with its job, and a Look without an Accent. The concept is the brief the sheets are built against."},
		{ID: RuleWorkbook, Required: true, Summary: "workbook.json describes editions, sheets, columns and datasets lfm can build",
			Help: "Errors for JSON lfm cannot read, unknown fields, sheets without a key, name or valid kind, sheet names Excel refuses (over 31 characters, : \\ / ? * [ ]) or that repeat, tables without rows or columns, columns or cells with an unknown type, enums without values, dataset values of the wrong type, and texts missing an edition's language. A workbook that does not build ships nothing."},
		{ID: RuleFormulas, Required: true, Summary: "Every formula parses, uses portable functions and computes without errors",
			Help: "Computes every formula with lfm's engine for the blank template and every dataset, and errors for formulas that do not parse, use functions Excel, Google Sheets and LibreOffice do not all compute the same way (XLOOKUP, FILTER, LET, QUERY, TEXT with date codes), name columns or cells that do not exist, refer to themselves, return #DIV/0!, #N/A, #VALUE!, #REF! or #NUM!, or differ from the values the workbook expects. Buyers judge a template by its first wrong total."},
		{ID: RuleCompat, Summary: "LibreOffice computes the file as lfm does",
			Help: "With --release and LibreOffice installed, which no release needs, recalculates every built file in LibreOffice and warns where a cell's value differs from lfm's or where LibreOffice reports an error. A formula one app computes differently breaks the template for its buyers."},
		{ID: RuleSheets, Summary: "Every edition has its Google Sheets master linked",
			Help: "Warns while deliverables.spreadsheet.sheets has no link for an edition or its example, or a link is not a Google Sheets document. Buyers get a copy link to the master; creating it takes the owner's Google account once: upload the .xlsx to Google Drive, open it with Google Sheets, share it as anyone with the link can view, and paste its address."},
		{ID: RuleDelivery, Required: true, Summary: "The download path stays unlisted",
			Help: "Errors when the compiled page contains the spreadsheet's download path. Buyers reach it through the checkout's after-payment redirect and the invoice email only."},
		{ID: RuleListing, Summary: "deliverables/spreadsheet/listing.md holds shop texts every channel accepts",
			Help: "Warns where a shop would refuse the listing: a title over 140 characters, more than 13 tags or one over 20 characters, a description under 200 characters or with links, a price that differs from the page's offer, or an AI section without the use, tools and statement. lfm renders it into every shop's guide with the facts of the workbook."},
		{ID: RuleChannels, Summary: "The shop packages meet each shop's limits",
			Help: "Warns when a shop would refuse what lfm built: a file over Etsy's 20 MB or more files than Etsy's five once zipped. A refused upload costs the sales the template was made for."},
	}
}

// Config is deliverables.spreadsheet in lfm.json.
type Config struct {
	Path, Offer, Language, Sell, Version string
	Sheets                               map[string]string // edition key (or key-example) -> Google Sheets master
	Seller                               string
}

// Present reports whether the project has deliverables/spreadsheet.
func Present(p project.Project) bool {
	dir, err := p.Path(Dir)
	if err != nil {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
