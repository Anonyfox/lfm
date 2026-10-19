// Package deck is the slide deck deliverable: decks a buyer presents from
// PowerPoint, Keynote or Google Slides, such as a yearly safety briefing, a
// workshop or a quiz night, sold for what they say as much as how they
// look. One Markdown file per deck and language and a theme from the
// concept compile to .pptx files whose look lives in a slide master and
// its layouts, with speaker notes, a notes PDF on paper and a quiz sheet;
// every compile measures each slide's text with the fonts, and a release
// prints the slides in Chrome as PowerPoint lays them out.
package deck

import (
	"os"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

// The layout under deliverables/deck.
const (
	Dir         = "deliverables/deck"
	ConceptFile = Dir + "/concept.md"
	FontsDir    = Dir + "/fonts"
	ImagesDir   = Dir + "/images"
	ListingFile = Dir + "/listing.md"
)

// Rule IDs.
const (
	RuleConfig   = "deck.config"
	RuleConcept  = "deck.concept"
	RuleFonts    = "deck.fonts"
	RuleSource   = "deck.source"
	RuleDesign   = "deck.design"
	RuleContent  = "deck.content"
	RuleLayout   = "deck.layout"
	RuleDelivery = "deck.delivery"
	RuleListing  = "deck.listing"
	RuleChannels = "deck.channels"
)

// Rules describes every deck rule for the catalog.
func Rules() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{ID: RuleConfig, Required: true, Summary: "Decks in deliverables/deck have their download path in lfm.json",
			Help: "Errors when deliverables/deck exists but lfm.json has no deliverables.deck, when it sells through the page without a download path of at least 16 lowercase letters, digits and hyphens, or when its offer is not in data.commerce.offers. Warns for a Google Slides master that is not a Google Slides link. The path is the only lock on the files."},
		{ID: RuleConcept, Summary: "deliverables/deck/concept.md plans the buyer, promise, decks and look",
			Help: "Warns for missing sections, a buyer without \"Not for:\", a Decks table that does not list every deck by file name with its job, and a Look without a known Theme. The concept is the brief every deck is written against."},
		{ID: RuleFonts, Required: true, Summary: "The theme's fonts are in deliverables/deck/fonts, static, embeddable and licensed",
			Help: "Errors when a font family the theme uses has no regular face in deliverables/deck/fonts, a face is a variable font or forbids embedding, or the folder has no licence file. PowerPoint shows the fonts each .pptx embeds, Google Slides knows Google Fonts families by name, and Keynote users install the files lfm ships, so every font must be a free Google Fonts family in static TrueType files with its OFL licence. Warns when a slide uses a character the font does not draw."},
		{ID: RuleSource, Required: true, Summary: "Every deck parses and every language has every planned deck",
			Help: "Errors for front matter lfm cannot read, a missing title, unknown directives, unclosed ::: blocks, raw HTML, code blocks, images that are missing or not PNG or JPEG, empty slides, forms, signatures and ornaments on slides, a second # or ## heading on a slide, columns other than two or beside more content, a picture beside more content, and a language folder that lacks a deck of the concept. A deck that does not build ships nothing."},
		{ID: RuleDesign, Summary: "Every slide's text fits its box and reads from the back of the room",
			Help: "Measures every line with the theme's fonts, as PowerPoint, Keynote and LibreOffice set them, and warns where a title wraps past two lines or a slide's text needs more room than its box. Warns where a slide shows more than 60 words, lists more than seven points or has no title, for tables over six columns or eight rows, pictures under 144 pixels per inch on the slide, fewer than half a minute a slide, and colors below WCAG contrast."},
		{ID: RuleContent, Summary: "The notes carry the talk, quizzes have an answer, sample content reads as real",
			Help: "A deck with planned minutes warns for slides without speaker notes and for notes that carry less than a fifth of its time at 130 words a minute. Warns for quiz questions without two to six answers, a translated deck whose slides differ from the first language's, lorem ipsum and other filler, emoji, shouting capitals and game-show brand names."},
		{ID: RuleLayout, Summary: "Every slide prints as planned in Chrome, as PowerPoint shows it",
			Help: "With --release, prints every deck in Chrome laid out as its .pptx places each box, one 16:9 page a slide, and its speaker notes and quiz sheet on paper; warns for a slide whose box cuts its text off, another page count or size, and a font other than the theme's. With LibreOffice installed, which no release needs, it also opens every .pptx and holds its print to the same plan."},
		{ID: RuleDelivery, Required: true, Summary: "The download path stays unlisted",
			Help: "Errors when the compiled page contains the decks' download path. Buyers reach it through the checkout's after-payment redirect and the invoice email only."},
		{ID: RuleListing, Summary: "deliverables/deck/listing.md holds shop texts every channel accepts",
			Help: "Warns where a shop would refuse the listing: a title over 140 characters, more than 13 tags or one over 20 characters, a description under 200 characters or with links, a price that differs from the page's offer, or an AI section without the use, tools and statement. lfm renders it into every shop's guide with the facts of the decks."},
		{ID: RuleChannels, Summary: "The shop packages meet each shop's limits",
			Help: "Warns when a shop would refuse what lfm built: a file over Etsy's 20 MB or more files than Etsy's five. A refused upload costs the sales the decks were made for."},
	}
}

// Config is deliverables.deck in lfm.json.
type Config struct {
	Path, Offer, Language, Sell, Version string
	Papers                               []string          // the notes' paper sizes; by language when empty
	Slides                               map[string]string // "lang/key" -> Google Slides master, for copy links
	Seller                               string
}

// Present reports whether the project has deliverables/deck.
func Present(p project.Project) bool {
	dir, err := p.Path(Dir)
	if err != nil {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
