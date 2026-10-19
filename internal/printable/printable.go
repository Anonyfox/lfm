// Package printable is the printable deliverable: a planner, tracker, game or
// puzzle pack written as one HTML template of fixed-size pages, rendered for
// every paper size and variant the concept names, checked page by page, and
// printed to PDFs a buyer prints at home or imports into a note app.
package printable

import (
	"os"
	"slices"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

// The fixed layout under deliverables/printable.
const (
	Dir          = "deliverables/printable"
	ConceptFile  = Dir + "/concept.md"
	TemplateName = "printable.html.tmpl"
	TemplateFile = Dir + "/" + TemplateName
	StyleName    = "printable.css"
	ListingFile  = Dir + "/listing.md"
	DataDir      = "data"
)

// Rule IDs.
const (
	RuleConfig   = "printable.config"
	RuleConcept  = "printable.concept"
	RulePages    = "printable.pages"
	RuleBuild    = "printable.build"
	RuleLayout   = "printable.layout"
	RuleDelivery = "printable.delivery"
	RuleListing  = "printable.listing"
	RuleChannels = "printable.channels"
)

// Rules describes every printable rule for the catalog.
func Rules() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{ID: RuleConfig, Required: true, Summary: "A printable in deliverables/printable has its download path in lfm.json",
			Help: "Errors when deliverables/printable exists but lfm.json has no deliverables.printable with a download path of at least 16 lowercase letters, digits and hyphens, when the offer it names is not in data.commerce.offers, or when sell is not page, shops or bundle. Only a printable sold through the page needs the path. The path is the only lock on the files. The finding prints a fresh random path to copy."},
		{ID: RuleConcept, Summary: "deliverables/printable/concept.md plans the buyer, promise, contents, editions and look",
			Help: "Warns for missing or misordered sections, a buyer without \"Not for:\", a Contents table without parts, page counts or jobs, Editions without known paper sizes, and a Look without Body, Headings and Accent. The concept is the brief every page is built against; the compiler holds the pages to its contents."},
		{ID: RulePages, Summary: "Every edition renders to whole pages that match the contents and link correctly",
			Help: "Renders printable.html.tmpl for every edition and warns when an element outside a <section class=\"page\"> would print, when a page has no data-part or a part the concept does not plan, when a part's page count differs from the Contents table, when the first page is not the cover (data-role=\"cover\"), when no page tells buyers how to print or import (data-role=\"guide\"), when puzzles (data-role=\"puzzle\") have no answer key (data-role=\"key\"), when an id repeats or an internal link points nowhere, and when an image is missing or outside the folder. Buyers punish broken links and keys above anything else."},
		{ID: RuleBuild, Required: true, Summary: "The template renders for every edition",
			Help: "Errors when printable.html.tmpl does not parse or fails for an edition, including a puzzle generator that cannot make a solvable puzzle from its input. Nothing is printed or published from a template that does not render."},
		{ID: RuleLayout, Summary: "Every printed page holds its content inside the safe margin",
			Help: "With --release, prints every edition and warns when a page's content overflows its page, reaches into the 0.25 in (6.35 mm) margin printers cut or skip, or when the PDF's page count differs from the rendered pages: a blank or split page shifts every link after it. Home printers clip what reaches the edge, and buyers review what they print."},
		{ID: RuleDelivery, Required: true, Summary: "The download path stays unlisted",
			Help: "Errors when the compiled page contains the printable's download path. Buyers reach it through Stripe's after-payment redirect and the invoice email only."},
		{ID: RuleListing, Summary: "deliverables/printable/listing.md holds store texts every channel accepts",
			Help: "Warns when listing.md is missing once the pages are clean, or when a store would refuse it: a title over 140 characters, more than 13 tags or one over 20 characters, a description under 200 characters or with links, a price that differs from the page's offer, or an AI section without the use, tools and statement. lfm renders it into every shop's guide with the facts from the build."},
		{ID: RuleChannels, Summary: "The store packages in distribution/ meet each channel's limits",
			Help: "Warns when a channel would refuse what lfm built: a PDF over Etsy's 20 MB, more files than Etsy's five once zipped by size, links that send Etsy buyers off Etsy, or listing images that could not be rendered. A refused upload or a delisted product costs the sales the work was for."},
	}
}

// Config is deliverables.printable in lfm.json.
type Config struct {
	Path     string
	Offer    string
	Language string
	Shop     string // the seller's name on the cover and in the licence
	Version  string // printed in the guide, e.g. 2027.1
	Sell     string // page (default), shops or bundle
}

// Present reports whether the project has deliverables/printable.
func Present(p project.Project) bool {
	dir, err := p.Path(Dir)
	if err != nil {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// Roles a page can carry in data-role.
var Roles = []string{"cover", "guide", "contents", "puzzle", "key", "cut", "certificate", "extra"}

func knownRole(r string) bool { return slices.Contains(Roles, r) }
