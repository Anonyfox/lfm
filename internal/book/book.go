// Package book checks and builds the book deliverable of an lfm project: a
// researched concept in deliverables/book/concept.md, chapters written one at a
// time from it, and the finished EPUB and PDF that buyers download from a page
// at an unlisted path.
//
// The book follows the page's method. Its concept cites the project's research
// by ID, its chapters are checked for structure, length and the stock phrases of
// generated prose, and it is only built once every chapter exists. Until then
// the download page tells buyers that their copy is being written.
package book

import (
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// Layout of the deliverable inside a project.
const (
	Dir         = "deliverables/book"
	ConceptFile = Dir + "/concept.md"
	ChaptersDir = "chapters"
	ImagesDir   = "images"
	StyleFile   = "book.css"
)

// Minimums the concept is held to.
const (
	MinChapters   = 3
	MinObjections = 3
	MinVoices     = 8 // distinct voices cited
	MinPeers      = 3 // distinct competing books or guides cited
	MinJobWords   = 4
	MinWords      = 300
	MaxWords      = 20000
	WordTolerance = 0.35 // a chapter may run this far over or under its target
	CoverWidth    = 1600
	CoverHeight   = 2560
)

// Rule IDs.
const (
	RuleConfig   = "book.config"
	RuleConcept  = "book.concept"
	RuleEvidence = "book.evidence"
	RuleOpen     = "book.open"
	RuleChapter  = "book.chapter"
	RuleCopy     = "book.copy"
	RuleValue    = "book.value"
	RuleMachine  = "book.machine"
	RuleImages   = "book.images"
	RuleDelivery = "book.delivery"
	RuleBuild    = "book.build"
	RuleLayout   = "book.layout"
	RuleFonts    = "book.fonts"
	RulePDF      = "book.pdf"
	RuleListing  = "book.listing"
	RuleChannels = "book.channels"
)

// Rules describes every book rule for the catalog.
func Rules() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{ID: RuleConfig, Required: true, Summary: "A book in deliverables/book has its author and download path in lfm.json",
			Help: "Errors when deliverables/book exists but lfm.json has no deliverables.book with an author and a download path of at least 16 lowercase letters, digits and hyphens, when the offer it names is not in data.commerce.offers, when formats lists anything but epub and pdf, when an ISBN is not a valid ISBN-13 or both editions share one, or when paper is neither white nor cream. The path is the only lock on the files: anyone who knows it can download them. The finding prints a fresh random path to copy. The path is needed only when the book sells through the page (sell: page, the default); sell: shops or bundle publishes no download page."},
		{ID: RuleConcept, Summary: "deliverables/book/concept.md has every section and a complete, consistent plan",
			Help: "Warns for missing or misordered sections, a reader without \"Not for:\", a promise without \"Won't cover:\" or without \"Before:\" and \"After:\", fewer than three objections that each cite a voice and name the chapter answering them, missing Title, Subtitle or Opening hooks, a voice without Use and Avoid phrases, fewer than three chapters, chapter rows with an empty or duplicate title or job, a word target outside 300 to 20000, a file outside chapters/ or used twice, no cover in Imagery, and a Look without a valid Trim, Body, Headings and Accent. The concept is the whole brief for every chapter; a gap here becomes a gap in the book."},
		{ID: RuleEvidence, Summary: "The book concept cites real voices and competing books from research/",
			Help: "Warns when the concept cites IDs that do not exist in research/, cites fewer than 8 distinct voices or 3 distinct peers, or when the reader and the promise cite no voice. A book planned from the author's assumptions answers questions nobody asked. Add a research batch about the book's subject and its competing books, then cite what readers actually wrote."},
		{ID: RuleOpen, Summary: "Open questions in the book concept and listing that the owner has to answer",
			Help: "Lists every \"Open:\" line in the book concept and in listing.md. Answer it in that file, then remove the line."},
		{ID: RuleChapter, Summary: "Each written chapter matches its plan: title, headings, length, images and links",
			Help: "Warns when a chapter does not start with \"# \" and its planned title, has more than one top heading, skips heading levels, has an empty section, runs more than 35% over or under its word target, references a missing image or one without alternative text, links a file that is not a chapter, links plain http, contains HTML (the book renders Markdown only) or filler such as TODO or lorem ipsum. Each chapter is checked against its row in the concept, so the plan and the text never drift apart."},
		{ID: RuleCopy, Summary: "Chapters read like a person wrote them",
			Help: "Warns for the stock phrases and constructions of generated prose: the copy.tells list and the words models over-use, sentences that correct a claim nobody made, announcing the chapter, closing on a recap or encouragement, bold labels, pet words, hedged promises, em-dash density, uniform sentence, paragraph and section lengths, one-line punches, abstract nouns, readability outside the band of mass-market nonfiction, stock example names, and phrases the concept's Voice avoids. Once the book is complete, it also warns for Use phrases the book never says. Quotes inside blockquotes are not checked. Rewrite the passage around the concrete thing it stands for."},
		{ID: RuleValue, Summary: "Each chapter gives the reader something to use and to check",
			Help: "Warns when a chapter opens on a generalization with no name, number or quote, makes the reader wait more than 800 words for the first step, list, amount or instruction or lets 1200 words pass without one, links no source a reader can check, attributes claims to unnamed experts, falls below a floor of names, numbers and quotes, or repeats another chapter. Readers' one-star reviews of practical books name exactly this: one idea stretched, the same point restated, generic advice without steps. Fix the chapter by adding the case, the source or the step, never by padding."},
		{ID: RuleMachine, Summary: "Chapters would not be flagged by AI-text detectors",
			Help: "Warns when lfm's built-in estimate, from at least 150 words of a chapter, says AI-text detectors would likely flag it as generated, and names the measured habits and the passage that reads most generated. The estimate is set for about 1% false alarms on human writing in eight languages. It does not block the next chapter: revise flagged chapters in the whole-book pass, by rewriting the named passage in the book's own voice, never by adding errors to fool a detector."},
		{ID: RuleImages, Summary: "The cover and the planned images exist and fit their job",
			Help: "Warns when an image the concept plans is missing, when the cover is smaller than 1600×2560 pixels or not in a 1:1.6 portrait ratio, or when a planned image is never used in a chapter. Store images in deliverables/book/images as JPEG, PNG or WebP."},
		{ID: RuleDelivery, Required: true, Summary: "The download path stays unlisted",
			Help: "Errors when the page itself contains the book's download path: anyone could then download the book without paying. Link the download page only from Stripe's after-payment redirect and the invoice email."},
		{ID: RuleBuild, Required: true, Summary: "The EPUB and the download page build",
			Help: "Errors when a chapter cannot be converted into valid XHTML, a referenced font or image cannot be read, or the EPUB package cannot be written. Fix the named file."},
		{ID: RuleFonts, Summary: "The typefaces the concept names are embedded",
			Help: "Warns when the first face of the concept's Body or Headings stack is neither declared with @font-face in deliverables/book/book.css nor a system face every reader and PDF viewer has, nor a generic family. The PDF would silently print in a fallback face and e-readers would ignore the choice. Add the licensed font files under deliverables/book/fonts and an @font-face rule for each style used."},
		{ID: RuleLayout, Summary: "The book's pages hold together on a phone and in print",
			Help: "Release only: warns when an image, table or code block runs wider than the text column at phone width or on the printed page, or when book text renders below 9 pixels. Readers cannot scroll sideways in most ebook readers. Let wide elements wrap, scale or split."},
		{ID: RuleListing, Summary: "deliverables/book/listing.md holds store texts every channel accepts",
			Help: "Once every chapter exists, warns when listing.md is missing or a store would refuse it: a description over 4000 characters as HTML or under 200, links, prices or rank claims in a description, more than 7 keywords or one that repeats the title, more than 13 tags or one over 20 characters, BISAC or Thema codes that do not parse, an ebook priced above 80% of the paperback, an AI section without the use, tools and statement, and in German books a store price that differs from the page's. lfm renders the file into every channel's format under distribution/."},
		{ID: RuleChannels, Summary: "The store packages in distribution/ meet each channel's limits",
			Help: "Warns when a channel would refuse a file lfm built: a paperback below every printer's minimum of 24 pages, a front cover image below 300 dpi at the trim, an EPUB or PDF over Etsy's 20 MB, a trim KDP does not print, transparency in a print file, and a print that failed. The README in distribution/ lists the same per channel with what is ready and what the owner still has to do. Channels that refuse AI-generated books are listed, not built."},
		{ID: RulePDF, Summary: "The PDF is built when it is promised",
			Help: "Warns when formats includes pdf but no Chrome or Chromium was found to print it, or printing failed. The download page then lists only the EPUB. Install Chrome or set LFM_CHROME, or remove pdf from formats."},
	}
}

// Config is deliverables.book in lfm.json.
type Config struct {
	Author        string   `json:"author"`
	Language      string   `json:"language"`
	Path          string   `json:"path"`
	Offer         string   `json:"offer"`
	Formats       []string `json:"formats"`
	Publisher     string   `json:"publisher"` // seller.name
	Imprint       string   `json:"imprint"`   // seller.imprint
	Paper         string   `json:"paper"`
	ISBNEbook     string   `json:"isbnEbook"`
	ISBNPaperback string   `json:"isbnPaperback"`
	Sell          string   `json:"sell"` // page (default), shops or bundle
}

// ValidISBN reports whether s is an ISBN-13 with a correct check digit;
// hyphens and spaces are ignored.
func ValidISBN(s string) bool {
	digits := strings.NewReplacer("-", "", " ", "").Replace(s)
	if len(digits) != 13 || !(strings.HasPrefix(digits, "978") || strings.HasPrefix(digits, "979")) {
		return false
	}
	sum := 0
	for i, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
		d := int(r - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return sum%10 == 0
}

// Wants reports whether format is configured.
func (c Config) Wants(format string) bool {
	if len(c.Formats) == 0 {
		return format == "epub" || format == "pdf" // the default
	}
	return slices.Contains(c.Formats, format)
}
