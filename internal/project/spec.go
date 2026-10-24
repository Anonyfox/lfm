package project

import (
	"fmt"
	"slices"
	"strings"
)

// Kind is the JSON shape a field holds.
type Kind string

// Field kinds. A map holds keys of the author's choosing whose values the
// field's "*" child describes; free holds anything, unchecked.
const (
	Object Kind = "object"
	String Kind = "string"
	Int    Kind = "integer"
	List   Kind = "list"
	Map    Kind = "map"
	Free   Kind = "any"
)

// Field is one key of lfm.json: its shape, its meaning and how it is
// checked. Spec is the single source of the file's format: loading checks
// against it, config errors quote it, `lfm help config` prints it and
// docs/config.md is generated from it.
type Field struct {
	Path     string   // dotted JSON path; "*" stands for a key of a map
	Kind     Kind     // for a list, the kind of its items is String
	Values   []string // the allowed values of a string or of a list's items
	Default  string   // what lfm uses when the field is absent, as prose
	Required bool     // must be present whenever its parent object is
	Doc      string   // one line: what it is for
	Example  string   // a JSON value to copy
	Checked  string   // the rule that checks it beyond its shape, if any
	Reserved bool     // a documented subtree inside free data, checked by its rule
}

// Spec lists every field of lfm.json, parents before children.
var Spec = []Field{
	{Path: "schemaVersion", Kind: Int, Required: true, Values: []string{"1"}, Doc: "The format of this file; always 1.", Example: `1`},
	{Path: "url", Kind: String, Required: true, Doc: "The absolute HTTPS URL of the folder the page is deployed to, ending in /: its canonical URL; download paths hang off it.", Example: `"https://example.com/my-page/"`},
	{Path: "language", Kind: String, Required: true, Doc: "The page's language as a BCP 47 tag; deliverables default to it.", Example: `"en"`},
	{Path: "title", Kind: String, Required: true, Doc: "The page title, shown in the browser tab, search results and share previews.", Example: `"Keep your sourdough alive"`},
	{Path: "description", Kind: String, Required: true, Doc: "One sentence for search results and share previews: who it is for and what they get.", Example: `"The jar check and the rescue feeding for a starter that smells wrong."`},
	{Path: "socialImage", Kind: Object, Doc: "The share preview image."},
	{Path: "socialImage.src", Kind: String, Doc: "Project path of a 1200×630 PNG/JPEG, or an HTML card rendered by compile at 1200×630 CSS pixels. Relative card assets are bundled; Chrome is required for the first render.", Example: `"assets/images/share.jpg"`},
	{Path: "socialImage.alt", Kind: String, Doc: "What the image shows, for people who cannot see it.", Example: `"A jar of starter on a kitchen shelf"`},
	{Path: "social", Kind: Object, Doc: "Optional social account attribution."},
	{Path: "social.x", Kind: String, Doc: "The page's X (Twitter) handle: 1–15 ASCII letters, digits or underscores, with or without @; normalized to @handle and available as {{.social.x}}. Leave out or empty when unused. The compiler fills missing twitter:site and twitter:creator tags; authored tags are kept.", Example: `"@example"`},
	{Path: "seller", Kind: Object, Doc: "Who sells: printed on the book's copyright page and in every other deliverable's guide, files and download page."},
	{Path: "seller.name", Kind: String, Doc: "The seller or publisher as buyers see it.", Example: `"Brot & Glas Verlag"`},
	{Path: "seller.imprint", Kind: String, Doc: "The lines of the legal notice (Impressum): address, register, VAT ID; printed on the book's copyright page.", Example: `"Brot & Glas Verlag, Musterstraße 1, 12345 Berlin"`},
	{Path: "urls", Kind: Map, Doc: "Named complete URLs: checkout links, shop listings, contacts. Render them as {{.urls.<name>}}; every rendered link is checked against them.", Example: `{"checkout": "https://buy.stripe.com/…"}`},
	{Path: "urls.*", Kind: String, Doc: "One complete URL.", Example: `"https://www.etsy.com/listing/123456/my-planner"`},
	{Path: "links", Kind: Free, Doc: "Optional free-form labels or key references for links."},
	{Path: "jsonld", Kind: Free, Doc: "A structured data object or array the compiler emits as JSON-LD; strings may reference config, such as \"{{.title}}\"."},
	{Path: "data", Kind: Free, Doc: "Every product value the page uses: offers, prices, copy, FAQ. Each data/<name>.json adds .data.<name>."},
	{Path: "data.commerce", Kind: Object, Reserved: true, Doc: "The commerce facts lfm checks: the offers and their checkout links."},
	{Path: "data.commerce.offers", Kind: Map, Reserved: true, Checked: "commerce.config", Doc: "The offers the page sells, by key; deliverables name theirs in offer."},
	{Path: "data.commerce.offers.*", Kind: Object, Reserved: true, Doc: "One offer.", Example: `{"sku": "planner-2027", "name": "Planner 2027", "checkoutUrlKey": "checkout", "price": {"amountMinor": 990, "currency": "EUR"}}`},
	{Path: "data.commerce.offers.*.sku", Kind: String, Reserved: true, Doc: "A stable product code, unique across offers.", Example: `"planner-2027"`},
	{Path: "data.commerce.offers.*.name", Kind: String, Reserved: true, Doc: "The offer's name as the checkout shows it.", Example: `"Planner 2027"`},
	{Path: "data.commerce.offers.*.checkoutUrlKey", Kind: String, Reserved: true, Doc: "The key in urls of where the buy button goes: a Stripe Payment Link, or a shop listing for a deliverable sold in shops.", Example: `"checkout"`},
	{Path: "data.commerce.offers.*.price", Kind: Object, Reserved: true, Doc: "The price as the checkout charges it.", Example: `{"amountMinor": 990, "currency": "EUR"}`},
	{Path: "data.commerce.offers.*.price.amountMinor", Kind: Int, Reserved: true, Doc: "The amount in minor units: 990 is 9.90.", Example: `990`},
	{Path: "data.commerce.offers.*.price.currency", Kind: String, Reserved: true, Doc: "A three-letter currency code.", Example: `"EUR"`},
	{Path: "data.commerce.offers.*.stripe", Kind: Free, Reserved: true, Checked: "commerce.fields", Doc: "Optional Stripe IDs (productId, priceId, paymentLinkId) and the customFields snapshot of the Payment Link."},
	{Path: "build", Kind: Object, Doc: "How lfm builds the page."},
	{Path: "build.assets", Kind: Object, Doc: "Whether resources are inlined into the page or emitted as files."},
	{Path: "build.assets.default", Kind: String, Values: []string{"auto", "inline", "file"}, Default: "auto: lfm decides per resource", Doc: "The packing policy for every resource.", Example: `"auto"`},
	{Path: "build.assets.overrides", Kind: Map, Doc: "Packing for single source paths; always wins over default.", Example: `{"assets/images/hero.webp": "file"}`},
	{Path: "build.assets.overrides.*", Kind: String, Values: []string{"inline", "file"}, Doc: "How this source path is packed.", Example: `"file"`},
	{Path: "build.browserData", Kind: List, Doc: "Dot paths copied into <script type=\"application/json\" id=\"product-data\"> for scripts, keeping their nesting.", Example: `["data.offer"]`},
	{Path: "build.validation", Kind: Object, Doc: "Rule levels and size budgets."},
	{Path: "build.validation.rules", Kind: Map, Doc: "Per-rule levels; required rules stay errors. `lfm rules` lists them.", Example: `{"design.mono": "off"}`},
	{Path: "build.validation.rules.*", Kind: String, Values: []string{"off", "warn", "error"}, Doc: "The level of this rule.", Example: `"warn"`},
	{Path: "build.validation.budgets", Kind: Object, Doc: "This page's advisory sizes, which perf.budgets warns past; raise one where the page has a reason to be larger instead of turning the rule off.", Checked: "perf.budgets", Example: `{"cssKiB": 80}`},
	{Path: "build.validation.budgets.cssKiB", Kind: Int, Default: "20", Doc: "Total CSS in KiB, inline and in files.", Checked: "perf.budgets", Example: `80`},
	{Path: "build.validation.budgets.jsKiB", Kind: Int, Default: "50", Doc: "Total JavaScript in KiB, inline and in files.", Checked: "perf.budgets", Example: `120`},
	{Path: "build.validation.budgets.elements", Kind: Int, Default: "1500", Doc: "Elements in the page's DOM.", Checked: "perf.budgets", Example: `3000`},
	{Path: "build.validation.budgets.depth", Kind: Int, Default: "32", Doc: "How deep the DOM nests.", Checked: "perf.budgets", Example: `40`},
	{Path: "deliverables", Kind: Object, Doc: "What buyers receive besides the page: a book, a printable, a spreadsheet, a set of editable documents, slide decks, and two or more of them together as a bundle."},

	{Path: "deliverables.book", Kind: Object, Doc: "A book in deliverables/book, written with `lfm skills book`."},
	{Path: "deliverables.book.author", Kind: String, Required: true, Doc: "The name on the cover.", Example: `"Lena Brot"`},
	{Path: "deliverables.book.sell", Kind: String, Values: []string{"page", "shops", "bundle"}, Default: "page", Doc: sellDoc, Example: `"page"`},
	{Path: "deliverables.book.path", Kind: String, Checked: "book.config", Doc: pathDoc, Example: `"k3x9m2q7w8e4r5t6y1u0"`},
	{Path: "deliverables.book.offer", Kind: String, Checked: "book.config", Doc: offerDoc, Example: `"book"`},
	{Path: "deliverables.book.language", Kind: String, Default: "the page's language", Doc: "The book's language, when it differs from the page's.", Example: `"de"`},
	{Path: "deliverables.book.formats", Kind: List, Values: []string{"epub", "pdf"}, Default: `["epub", "pdf"]`, Doc: "The files the download page offers.", Example: `["epub", "pdf"]`},
	{Path: "deliverables.book.paper", Kind: String, Values: []string{"white", "cream"}, Default: "white", Doc: "The paperback's paper; it sets the spine width.", Example: `"cream"`},
	{Path: "deliverables.book.isbn", Kind: Object, Doc: "ISBNs per edition; each is optional, ebooks need none in most stores."},
	{Path: "deliverables.book.isbn.ebook", Kind: String, Checked: "book.config", Doc: "The ebook's ISBN-13, printed on the copyright page.", Example: `"978-3-16-148410-0"`},
	{Path: "deliverables.book.isbn.paperback", Kind: String, Checked: "book.config", Doc: "The paperback's ISBN-13, printed on the copyright page and as the barcode on the back cover.", Example: `"978-3-16-148411-7"`},

	{Path: "deliverables.printable", Kind: Object, Doc: "A printable in deliverables/printable (planner, game, puzzle pack), built with `lfm skills printable`."},
	{Path: "deliverables.printable.sell", Kind: String, Values: []string{"page", "shops", "bundle"}, Default: "page", Doc: sellDoc, Example: `"shops"`},
	{Path: "deliverables.printable.path", Kind: String, Checked: "printable.config", Doc: pathDoc, Example: `"p7m2x9q4w8e3r6t5y1u0"`},
	{Path: "deliverables.printable.offer", Kind: String, Checked: "printable.config", Doc: offerDoc, Example: `"planner"`},
	{Path: "deliverables.printable.language", Kind: String, Default: "the page's language", Doc: "The printable's language, when it differs from the page's.", Example: `"de"`},
	{Path: "deliverables.printable.version", Kind: String, Doc: "The version printed in the guide, so buyers see they have the latest.", Example: `"2027.1"`},

	{Path: "deliverables.spreadsheet", Kind: Object, Doc: "A spreadsheet template in deliverables/spreadsheet (budget, bookkeeping, tracker), built with `lfm skills spreadsheet`."},
	{Path: "deliverables.spreadsheet.sell", Kind: String, Values: []string{"page", "shops", "bundle"}, Default: "page", Doc: sellDoc, Example: `"shops"`},
	{Path: "deliverables.spreadsheet.path", Kind: String, Checked: "spreadsheet.config", Doc: pathDoc, Example: `"s7m2x9q4w8e3r6t5y1u0"`},
	{Path: "deliverables.spreadsheet.offer", Kind: String, Checked: "spreadsheet.config", Doc: offerDoc, Example: `"budget"`},
	{Path: "deliverables.spreadsheet.language", Kind: String, Default: "the page's language", Doc: "The language of the start guide and download page, when it differs from the page's.", Example: `"de"`},
	{Path: "deliverables.spreadsheet.version", Kind: String, Doc: "The version printed in the start guide.", Example: `"2027.1"`},
	{Path: "deliverables.spreadsheet.sheets", Kind: Map, Checked: "spreadsheet.sheets", Doc: "The Google Sheets master of every edition (and of its example as <key>-example): upload the .xlsx to Google Drive, open it with Google Sheets, share it as anyone with the link can view, paste its address here. Buyers get a copy link to it.", Example: `{"en": "https://docs.google.com/spreadsheets/d/…/edit"}`},
	{Path: "deliverables.spreadsheet.sheets.*", Kind: String, Doc: "The address of one Google Sheets master.", Example: `"https://docs.google.com/spreadsheets/d/1AbC…/edit"`},
	{Path: "deliverables.document", Kind: Object, Doc: "A set of editable documents in deliverables/document (letters, invoices, minutes, programs, binders), built with `lfm skills document`."},
	{Path: "deliverables.document.sell", Kind: String, Values: []string{"page", "shops", "bundle"}, Default: "page", Doc: sellDoc, Example: `"shops"`},
	{Path: "deliverables.document.path", Kind: String, Checked: "document.config", Doc: pathDoc, Example: `"k3n8q2w7x5r9t4m6p1"`},
	{Path: "deliverables.document.offer", Kind: String, Checked: "document.config", Doc: offerDoc, Example: `"kit"`},
	{Path: "deliverables.document.language", Kind: String, Default: "the page's language", Doc: "The language of the download page and the shop facts, when it differs from the page's.", Example: `"de"`},
	{Path: "deliverables.document.version", Kind: String, Doc: "The version printed in the start guide.", Example: `"2027.1"`},
	{Path: "deliverables.document.papers", Kind: List, Values: []string{"a4", "letter"}, Default: "US Letter and A4 for American English, A4 for every other language", Doc: "The paper sizes every language is built in; each size is its own edition and ZIP.", Example: `["a4", "letter"]`},
	{Path: "deliverables.document.docs", Kind: Map, Checked: "document.config", Doc: "Optional Google Docs masters by \"<language>/<document>\": upload the .docx to Google Drive, open it with Google Docs, share it as anyone with the link can view, paste its address. The start guide and download page then carry a copy link.", Example: `{"en/invoice": "https://docs.google.com/document/d/…/edit"}`},
	{Path: "deliverables.document.docs.*", Kind: String, Doc: "The address of one Google Docs master.", Example: `"https://docs.google.com/document/d/1AbC…/edit"`},
	{Path: "deliverables.deck", Kind: Object, Doc: "Slide decks in deliverables/deck (training briefings, workshops, quiz games) with speaker notes, built with `lfm skills deck`."},
	{Path: "deliverables.deck.sell", Kind: String, Values: []string{"page", "shops", "bundle"}, Default: "page", Doc: sellDoc, Example: `"shops"`},
	{Path: "deliverables.deck.path", Kind: String, Checked: "deck.config", Doc: pathDoc, Example: `"k3n8q2w7x5r9t4m6p1"`},
	{Path: "deliverables.deck.offer", Kind: String, Checked: "deck.config", Doc: offerDoc, Example: `"briefing"`},
	{Path: "deliverables.deck.language", Kind: String, Default: "the page's language", Doc: "The language of the download page and the shop facts, when it differs from the page's.", Example: `"de"`},
	{Path: "deliverables.deck.version", Kind: String, Doc: "The version printed in the start guide.", Example: `"2027.1"`},
	{Path: "deliverables.deck.papers", Kind: List, Values: []string{"a4", "letter"}, Default: "US Letter and A4 for American English, A4 for every other language", Doc: "The paper sizes the speaker notes and quiz sheets print on; every language ships one ZIP with each.", Example: `["a4"]`},
	{Path: "deliverables.deck.slides", Kind: Map, Checked: "deck.config", Doc: "Optional Google Slides masters by \"<language>/<deck>\": upload the .pptx to Google Drive, open it with Google Slides, share it as anyone with the link can view, paste its address. The start guide and download page then carry a copy link.", Example: `{"en/briefing": "https://docs.google.com/presentation/d/…/edit"}`},
	{Path: "deliverables.deck.slides.*", Kind: String, Doc: "The address of one Google Slides master.", Example: `"https://docs.google.com/presentation/d/1AbC…/edit"`},
	{Path: "deliverables.bundle", Kind: Object, Doc: "Two or more deliverables sold together, such as a deck with its handout documents or a book with its printable: its own name, listing in deliverables/bundle/listing.md, and download page or shop listing with every file of each."},
	{Path: "deliverables.bundle.units", Kind: List, Values: []string{"book", "printable", "spreadsheet", "document", "deck"}, Default: "every deliverable of the project", Checked: "bundle.config", Doc: "The deliverables the bundle holds, in the order its download page and listing show them. A deliverable whose sell is bundle sells only here.", Example: `["deck", "document"]`},
	{Path: "deliverables.bundle.name", Kind: String, Required: true, Doc: "The bundle's name as buyers see it.", Example: `"Starter Kit: book and workbook"`},
	{Path: "deliverables.bundle.sell", Kind: String, Values: []string{"page", "shops"}, Default: "page", Doc: "How buyers get the bundle: page = your checkout redirects to an unlisted download page with every file; shops = the shops deliver.", Example: `"page"`},
	{Path: "deliverables.bundle.path", Kind: String, Checked: "bundle.config", Doc: pathDoc, Example: `"b7m2x9q4w8e3r6t5y1u0"`},
	{Path: "deliverables.bundle.offer", Kind: String, Checked: "bundle.config", Doc: offerDoc, Example: `"kit"`},
	{Path: "deliverables.bundle.language", Kind: String, Default: "the page's language", Doc: "The bundle's language, when it differs from the page's.", Example: `"de"`},
}

// Moved maps keys of earlier versions to where they live now, so an old
// lfm.json is told the new place instead of only "unknown field".
var Moved = map[string]string{
	"deliverables.book.publisher": "seller.name",
	"deliverables.book.imprint":   "seller.imprint",
	"deliverables.printable.shop": "seller.name",
}

// orList joins values as "a, b or c".
func orList(values []string) string {
	if len(values) < 2 {
		return strings.Join(values, "")
	}
	return strings.Join(values[:len(values)-1], ", ") + " or " + values[len(values)-1]
}

const (
	sellDoc  = "How buyers get the files: page = your checkout redirects to an unlisted download page (needs path); shops = Etsy, Gumroad, Amazon and others deliver, the page's offer links to the listing; bundle = only inside deliverables.bundle."
	pathDoc  = "The unlisted download folder, at least 16 lowercase letters, digits or hyphens; needed when sell is page. It is the only lock on the files; a compile without it prints a fresh one."
	offerDoc = "The key of the offer in data.commerce.offers that sells it; its price holds the listing's price, and its link must match sell."
)

// Lookup returns the field at a concrete path, matching "*" against map keys.
func Lookup(path string) (Field, bool) {
	parts := strings.Split(path, ".")
	for _, f := range Spec {
		pattern := strings.Split(f.Path, ".")
		if len(pattern) != len(parts) {
			continue
		}
		match := true
		for i, p := range pattern {
			if p != "*" && p != parts[i] {
				match = false
				break
			}
		}
		if match {
			return f, true
		}
	}
	return Field{}, false
}

// Children returns the fields directly under path, in Spec order.
func Children(path string) []Field {
	var out []Field
	prefix := path + "."
	if path == "" {
		prefix = ""
	}
	for _, f := range Spec {
		rest, ok := strings.CutPrefix(f.Path, prefix)
		if ok && rest != "" && !strings.Contains(rest, ".") {
			out = append(out, f)
		}
	}
	return out
}

// Name is the last segment of the field's path.
func (f Field) Name() string { return f.Path[strings.LastIndex(f.Path, ".")+1:] }

// Shape describes the field's type and values in a few words.
func (f Field) Shape() string {
	switch {
	case len(f.Values) > 0 && f.Kind == List:
		return "list of " + strings.Join(f.Values, " | ")
	case len(f.Values) > 0:
		return strings.Join(f.Values, " | ")
	case f.Kind == List:
		return "list of strings"
	}
	return string(f.Kind)
}

// Hint explains a field in the words of a config finding: what it is for,
// what it takes, and where to read the rest.
func (f Field) Hint() string {
	var b strings.Builder
	b.WriteString(f.Doc)
	if len(f.Values) > 0 {
		fmt.Fprintf(&b, " Allowed: %s.", strings.Join(f.Values, ", "))
	}
	if f.Example != "" && len(f.Values) == 0 {
		fmt.Fprintf(&b, " Example: %s.", f.Example)
	}
	parent := ""
	if i := strings.LastIndex(f.Path, "."); i > 0 {
		parent = " " + strings.ReplaceAll(f.Path[:i], ".*", "")
	}
	fmt.Fprintf(&b, " 'lfm help config%s' lists the options.", parent)
	return b.String()
}

// allowed reports whether value is one of the field's values.
func (f Field) allowed(value string) bool {
	return len(f.Values) == 0 || slices.Contains(f.Values, value)
}
