// Package document is the editable document deliverable: letters, invoices,
// minutes, programs, binders, contracts and any other document a buyer
// fills in Word, Google Docs, Pages or LibreOffice, and slide decks with
// speaker notes for PowerPoint, Keynote and Google Slides. One Markdown file per
// document and language and a theme from the concept compile to .docx files
// whose look lives in real styles, per language and paper size; a release
// prints them in Chrome as Word lays them out and checks what every app will show.
package document

import (
	"os"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

// The layout under deliverables/document.
const (
	Dir         = "deliverables/document"
	ConceptFile = Dir + "/concept.md"
	FontsDir    = Dir + "/fonts"
	ImagesDir   = Dir + "/images"
	ListingFile = Dir + "/listing.md"
)

// Rule IDs.
const (
	RuleConfig   = "document.config"
	RuleConcept  = "document.concept"
	RuleFonts    = "document.fonts"
	RuleSource   = "document.source"
	RuleDesign   = "document.design"
	RuleContent  = "document.content"
	RuleLayout   = "document.layout"
	RuleDelivery = "document.delivery"
	RuleListing  = "document.listing"
	RuleChannels = "document.channels"
)

// Rules describes every document rule for the catalog.
func Rules() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{ID: RuleConfig, Required: true, Summary: "Documents in deliverables/document have their download path in lfm.json",
			Help: "Errors when deliverables/document exists but lfm.json has no deliverables.document, when it sells through the page without a download path of at least 16 lowercase letters, digits and hyphens, or when its offer is not in data.commerce.offers. The path is the only lock on the files."},
		{ID: RuleConcept, Summary: "deliverables/document/concept.md plans the buyer, promise, documents and look",
			Help: "Warns for missing sections, a buyer without \"Not for:\", a Documents table that does not list every document by file name with its job, and a Look without a known Theme. The concept is the brief every document is written against, and its Look sets one style for the whole set."},
		{ID: RuleFonts, Required: true, Summary: "The theme's fonts are in deliverables/document/fonts, static, embeddable and licensed",
			Help: "Errors when a font family the theme uses has no regular face in deliverables/document/fonts, a face is a variable font or forbids embedding, or the folder has no licence file. Word and LibreOffice show the fonts embedded in each .docx, Google Docs knows Google Fonts families by name, and Pages users install the files lfm ships, so every font must be a free Google Fonts family in static TrueType files with its OFL licence. Warns when the bold or italic face is missing and when a document uses a character the font does not draw, which apps fill in from another font."},
		{ID: RuleSource, Required: true, Summary: "Every document parses and every language has every planned document",
			Help: "Errors for front matter lfm cannot read, a missing title, unknown directives, centered text outside ceremonial documents, unclosed ::: blocks, headings deeper than ###, raw HTML, code blocks, images that are missing or not PNG or JPEG, and a language folder that lacks a document of the concept. In a deck (kind: slides) it also errors for empty slides, forms, signatures and ornaments on slides, a second # or ## heading on a slide, columns other than two or beside more content, a picture beside more content, and header, footer or cover settings. A document that does not build ships nothing."},
		{ID: RuleDesign, Summary: "The theme reads well on every paper and the documents keep to it",
			Help: "Errors when the ink, muted color or accent fall below WCAG contrast, or a theme's body lines hold fewer than 45 or more than 90 characters on a paper. Warns for tables with more than seven columns, number columns aligned left, and images printed below 150 dpi. On slides it measures every line with the theme's fonts and warns where a title wraps past two lines or the text needs more room than its box, where a slide shows more than 60 words, lists more than seven points or has no title, for tables over six columns or eight rows, pictures under 144 pixels per inch on the slide, and fewer than half a minute a slide. The compiler sets sizes, spacing and margins from the theme; these are the choices it cannot make."},
		{ID: RuleContent, Summary: "Sample content reads as real and contains nothing generated",
			Help: "Warns for lorem ipsum, John Doe, Acme and other filler, emoji, long runs of capitals, game-show brand names, and documents without fields (==…==) for the buyer to replace. A deck with planned minutes warns for slides without speaker notes and notes that carry less than a fifth of its time at 130 words a minute; a translated deck warns when its slides differ from the first language's; a quiz offers two to six answers. Buyers judge a template by its sample content: it should read like the real thing, with every field to change highlighted."},
		{ID: RuleLayout, Summary: "Every document prints on its planned pages, paper and fonts",
			Help: "With --release, prints every document's print layout, which follows its .docx line for line, in Chrome and warns when a document runs over or under the pages its front matter declares, prints on another paper size, or uses a font other than the theme's, which means a character the font does not draw. The print takes the taller of the design leading and the font's own line, as Word for Windows sets it, so a document that fits there fits in Word, Pages and LibreOffice. A deck prints one 16:9 page a slide, laid out as its .pptx places every box, and its speaker notes on the edition's paper; a slide whose text its box cuts off is a finding. With LibreOffice installed, which no release needs, it also opens every .docx and .pptx and holds its print to the same plan."},
		{ID: RuleDelivery, Required: true, Summary: "The download path stays unlisted",
			Help: "Errors when the compiled page contains the documents' download path. Buyers reach it through the checkout's after-payment redirect and the invoice email only."},
		{ID: RuleListing, Summary: "deliverables/document/listing.md holds shop texts every channel accepts",
			Help: "Warns where a shop would refuse the listing: a title over 140 characters, more than 13 tags or one over 20 characters, a description under 200 characters or with links, a price that differs from the page's offer, or an AI section without the use, tools and statement. lfm renders it into every shop's guide with the facts of the set."},
		{ID: RuleChannels, Summary: "The shop packages meet each shop's limits",
			Help: "Warns when a shop would refuse what lfm built: a file over Etsy's 20 MB or more files than Etsy's five. A refused upload costs the sales the set was made for."},
	}
}

// Config is deliverables.document in lfm.json.
type Config struct {
	Path, Offer, Language, Sell, Version string
	Papers                               []string          // paper sizes for every language; default by language
	Docs                                 map[string]string // "lang/key" -> Google Docs master, for copy links
	Seller                               string
}

// Present reports whether the project has deliverables/document.
func Present(p project.Project) bool {
	dir, err := p.Path(Dir)
	if err != nil {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
