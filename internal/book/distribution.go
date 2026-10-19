package book

import (
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/deliver"

	"github.com/Anonyfox/lfm/internal/pdfx"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/term"
)

// Printed holds what Chrome printed for the store packages.
type Printed struct {
	PDF           []byte            // the downloadable PDF, with bookmarks and links
	Interior      []byte            // the paperback interior
	InteriorPages int               // its page count, even
	Inside        float64           // its inside margin in inches
	Wraps         map[string][]byte // full cover wraps by spine formula: "kdp", "lulu"
	Spines        map[string]float64
	Problems      []string // what could not be printed, and why
	Pending       bool     // the book changed since the last release printed it
	LastCover     []byte   // the store cover of the last publish, reused when its source is unchanged
}

// A channel is a store or distributor and what it takes. The rules are those
// published in October 2026; the README tells the owner to confirm them.
type channel struct {
	key, name, url   string
	ebook, pdf       bool
	paperback        bool
	wrap             string // spine formula lfm can lay out; "" means the channel's own template
	minPages         int
	refusesGenerated string // why, when the channel refuses AI-generated books
	description      string // "html", "plain" or "markdown"
	notes            func(c *channelContext) []string
}

type channelContext struct {
	r        Report
	b        Built
	p        Printed
	slug     string
	epubSize int
}

var channels = []channel{
	{key: "kdp", name: "Amazon KDP", url: "https://kdp.amazon.com", ebook: true, paperback: true, wrap: "kdp", minPages: 24, description: "html", notes: kdpNotes},
	{key: "apple", name: "Apple Books", url: "https://authors.apple.com", ebook: true, description: "plain", notes: appleNotes},
	{key: "google", name: "Google Play Books", url: "https://play.google.com/books/publish", ebook: true, pdf: true, description: "plain", notes: googleNotes},
	{key: "kobo", name: "Kobo Writing Life", url: "https://www.kobo.com/writinglife", ebook: true, description: "plain", notes: koboNotes},
	{key: "tolino", name: "tolino media", url: "https://www.tolino-media.de", ebook: true, description: "plain", notes: tolinoNotes},
	{key: "barnes-noble", name: "Barnes & Noble Press", url: "https://press.barnesandnoble.com", ebook: true, paperback: true, minPages: 24, description: "plain", notes: bnNotes},
	{key: "draft2digital", name: "Draft2Digital", url: "https://www.draft2digital.com", ebook: true, description: "plain",
		refusesGenerated: "Draft2Digital does not accept content generated entirely by AI, nor AI-generated nonfiction without credentials.", notes: d2dNotes},
	{key: "ingramspark", name: "IngramSpark", url: "https://www.ingramspark.com", ebook: true, paperback: true, minPages: 18, description: "plain",
		refusesGenerated: "IngramSpark's catalog guidelines prohibit content created by automated means, including AI.", notes: ingramNotes},
	{key: "lulu", name: "Lulu", url: "https://www.lulu.com", ebook: true, paperback: true, wrap: "lulu", minPages: 32, description: "plain", notes: luluNotes},
	{key: "etsy", name: "Etsy", url: "https://www.etsy.com/your/shops/me/tools/listings", ebook: true, pdf: true, description: "plain", notes: etsyNotes},
	{key: "gumroad", name: "Gumroad", url: "https://gumroad.com/products/new", ebook: true, pdf: true, description: "markdown", notes: shopNotes("Gumroad is the merchant of record: it collects and remits sales tax and VAT. It keeps 10% plus $0.50 per sale, 30% on sales through its Discover marketplace.")},
	{key: "payhip", name: "Payhip", url: "https://payhip.com", ebook: true, pdf: true, description: "markdown", notes: shopNotes("Payhip remits EU and UK VAT, but payments run through your own Stripe or PayPal, so you remain the seller. The free plan keeps 5% per sale. It can stamp the buyer's name into the PDF.")},
}

// KDPTrims are the paperback trims KDP prints, of those lfm offers.
var KDPTrims = map[string]bool{"5x8in": true, "5.5x8.5in": true, "6x9in": true, "A4": true, "letter": true}

// Channels returns the keys of the channels lfm prepares, in order.
func Channels() []string {
	keys := make([]string, len(channels))
	for i, c := range channels {
		keys[i] = c.key
	}
	return keys
}

// takesPaperback reports whether a channel prints the paperback lfm built.
func (ch channel) takesPaperback(r Report, pages int) bool {
	return ch.paperback && pages >= ch.minPages && (ch.key != "kdp" || KDPTrims[r.Plan.Trim])
}

// refuses reports whether a channel refuses the book as declared in listing.md.
func (ch channel) refuses(r Report) bool {
	return ch.refusesGenerated != "" && r.Listing.AIText == "generated"
}

// Wraps returns the spine formulas of the cover wraps that a channel taking
// the paperback of this many pages lays out, so only those are printed.
func Wraps(r Report, pages int) []string {
	var keys []string
	for _, ch := range channels {
		if ch.wrap != "" && !ch.refuses(r) && ch.takesPaperback(r, pages) {
			keys = append(keys, ch.wrap)
		}
	}
	return keys
}

// WrapFile is the file name of the cover wrap laid out with a spine formula.
func WrapFile(key string) string { return "paperback-cover-" + key + ".pdf" }

// Distribution assembles the store packages: every file once at the top, a
// guide per channel in stores/ naming the files it takes and every field in
// its format, and a README over all of them. Channels that refuse the book are
// listed in the README only.
func Distribution(r Report, b Built, p Printed, day string) (map[string][]byte, []string, []diagnostic.Diagnostic) {
	files := map[string][]byte{}
	var built []string
	var issues []diagnostic.Diagnostic
	warn := func(file string, message, hint string) {
		line := 1
		if file == ConceptFile {
			line = max(1, r.Plan.LookLine)
		}
		issues = append(issues, at(file, line, RuleChannels, diagnostic.Warning, message, hint))
	}
	slug := deliver.Slug(r.Plan.Title, "book")
	c := &channelContext{r: r, b: b, p: p, slug: slug, epubSize: len(b.EPUB)}

	// what describes each file once; a guide lists the ones its channel takes.
	what := map[string]string{}
	put := func(name string, data []byte, description string) {
		files[name] = data
		what[name] = fmt.Sprintf("%s (%s)", description, term.Bytes(len(data)))
	}
	epub, pdf, cover, interior := slug+".epub", slug+".pdf", "cover.jpg", "paperback-interior.pdf"
	put(epub, b.EPUB, "the ebook, EPUB 3 with accessibility metadata")
	if r.Plan.Cover != "" {
		if data, err := StoreCover(r.Sources[r.Plan.Cover], p.LastCover); err != nil {
			warn(ConceptFile, "The store cover could not be written: "+err.Error(), "Save the cover as a JPEG or PNG of 1600×2560 pixels.")
		} else {
			put(cover, data, "the store cover, RGB JPEG")
		}
	}
	if p.PDF != nil {
		put(pdf, p.PDF, "the PDF with bookmarks")
	}
	for _, f := range []string{epub, pdf} {
		if len(files[f]) > 20<<20 {
			warn(ConceptFile, fmt.Sprintf("%s is %s, over Etsy's 20 MB per file.", f, term.Bytes(len(files[f]))), "Compress the book's images; most stores deliver large files slowly too.")
		}
	}
	paper := p.Interior != nil
	if paper {
		put(interior, p.Interior, fmt.Sprintf("the paperback interior, %s trim, %d pages, inside margin %.3g in", r.Plan.Trim, p.InteriorPages, p.Inside))
		for key, wrap := range p.Wraps {
			put(WrapFile(key), wrap, fmt.Sprintf("the full cover wrap with a %.3f in spine on %s paper", p.Spines[key], paperOf(r.Config)))
		}
		if p.InteriorPages < 24 {
			warn(ConceptFile, fmt.Sprintf("The paperback has %d pages; KDP and Barnes & Noble print from 24, Lulu from 32.", p.InteriorPages), "Sell it as an ebook only, or let the book grow; the stores skip the paperback.")
		}
		front := [2]float64{b.TrimInches[0] + Bleed, b.TrimInches[1] + 2*Bleed}
		if dpi := PrintDPI(b.CoverPixels, front); dpi > 0 && dpi < 300 {
			need := [2]int{int(front[0]*300 + 0.999), int(front[1]*300 + 0.999)}
			warn(ConceptFile, fmt.Sprintf("The printed front cover reaches %d dpi; print needs 300.", dpi),
				fmt.Sprintf("Add deliverables/book/images/cover-print.jpg of at least %d×%d pixels, the same design composed for the trim; lfm prints it instead of the ebook cover.", need[0], need[1]))
		}
		if pdfx.Transparent(p.Interior) {
			warn(Dir+"/"+StyleFile, "The paperback interior contains transparency; KDP and Lulu want it flattened.", "Avoid opacity in book.css and use images without an alpha channel (JPEG, or PNG without transparency).")
		}
		for key, wrap := range p.Wraps {
			if pdfx.Transparent(wrap) {
				warn(Dir+"/"+StyleFile, "The "+key+" cover wrap contains transparency; printers want it flattened.", "Avoid opacity in book.css and save the cover without an alpha channel.")
				break
			}
		}
		if !KDPTrims[r.Plan.Trim] {
			warn(ConceptFile, fmt.Sprintf("KDP does not print %s paperbacks.", r.Plan.Trim), "Pick 5x8in, 5.5x8.5in, 6x9in, A4 or letter in Look to sell the paperback on Amazon; IngramSpark and Lulu print it.")
		}
	}

	var table strings.Builder
	table.WriteString("| Channel | Guide | Ebook | PDF | Paperback |\n| --- | --- | --- | --- | --- |\n")
	var refused []string
	for _, ch := range channels {
		if ch.refuses(r) {
			refused = append(refused, fmt.Sprintf("- **%s**: %s", ch.name, ch.refusesGenerated))
			continue
		}
		var list []string
		take := func(name string) {
			if files[name] != nil {
				list = append(list, fmt.Sprintf("- [`%s`](../%s): %s", name, name, what[name]))
			}
		}
		take(epub)
		take(cover)
		hasPDF := ch.pdf && p.PDF != nil
		if hasPDF {
			take(pdf)
		}
		hasPaper := paper && ch.takesPaperback(r, p.InteriorPages)
		paperCell := "–"
		switch {
		case hasPaper && ch.wrap != "":
			take(interior)
			take(WrapFile(ch.wrap))
			paperCell = "ready"
		case hasPaper:
			take(interior)
			paperCell = "interior; cover on the channel's template"
		case ch.paperback && paper && p.InteriorPages < ch.minPages:
			paperCell = fmt.Sprintf("from %d pages", ch.minPages)
		case ch.paperback && paper:
			paperCell = "not this trim"
		}
		guide := "stores/" + ch.key + ".md"
		fmt.Fprintf(&table, "| %s | [`%s`](%s) | %s | %s | %s |\n", ch.name, guide, guide, "ready", map[bool]string{true: "ready", false: "–"}[hasPDF], paperCell)
		files[guide] = []byte(channelListing(ch, c, list, hasPaper, day))
		built = append(built, ch.key)
	}
	files["README.md"] = []byte(distributionReadme(c, table.String(), refused, day))
	return files, built, issues
}

func paperOf(cfg Config) string {
	if cfg.Paper == "" {
		return "white"
	}
	return cfg.Paper
}

func channelListing(ch channel, c *channelContext, list []string, paper bool, day string) string {
	r, l := c.r, c.r.Listing
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n\nUpload at %s. Built by lfm compile on %s from deliverables/book; edit the book and compile again rather than this file.\n\n## Files\n\nUpload these from `distribution/`:\n\n%s\n\n", ch.name, r.Plan.Title, ch.url, day, strings.Join(list, "\n"))
	b.WriteString("## Fields\n\n")
	field := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "- **%s:** %s\n", name, value)
		}
	}
	title := r.Plan.Title
	if ch.key == "etsy" {
		title = etsyTitle(r.Plan.Title, r.Plan.Subtitle, r.Config.Language)
		field("Title", fmt.Sprintf("%s (%d of 140 characters)", title, len([]rune(title))))
	} else {
		field("Title", title)
		field("Subtitle", r.Plan.Subtitle)
	}
	field("Author", r.Config.Author)
	field("Language", r.Config.Language)
	field("Publisher", r.Config.Publisher)
	if ch.ebook && r.Config.ISBNEbook != "" && ch.key != "etsy" && ch.key != "gumroad" && ch.key != "payhip" {
		field("ISBN (ebook)", r.Config.ISBNEbook)
	}
	if paper {
		isbn := r.Config.ISBNPaperback
		if isbn == "" {
			isbn = "none set: " + map[bool]string{true: "take the channel's free ISBN, which only works on this channel, or buy one (in Germany from MVB)", false: "this channel requires your own ISBN for the paperback"}[ch.key != "ingramspark"]
		}
		field("ISBN (paperback)", isbn)
	}
	prices := priceLines(ch, c)
	if paper && len(l.Paperback) == 0 {
		prices = append(prices, "Paperback: no price in listing.md yet; the owner sets it")
	}
	if len(prices) > 0 {
		b.WriteString("- **Prices:**\n")
		for _, p := range prices {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
	}

	switch ch.description {
	case "html":
		desc := KDPDescription(l.Description)
		fmt.Fprintf(&b, "\n## Description\n\nPaste as HTML (%d of %d characters):\n\n```html\n%s\n```\n", len([]rune(desc)), MaxDescription, desc)
	case "markdown":
		fmt.Fprintf(&b, "\n## Summary\n\n%s\n\n## Description\n\n%s\n", l.Short, aiNote(ch, l, l.Description))
	default:
		fmt.Fprintf(&b, "\n## Description\n\nPaste as plain text:\n\n```text\n%s\n```\n", aiNote(ch, l, PlainText(l.Description)))
	}
	switch ch.key {
	case "kdp":
		fmt.Fprintf(&b, "\n## Keywords\n\nOne per box:\n\n%s\n\n## Categories\n\n%s\n", bullets(l.Keywords, "`"), bullets(l.KDP, ""))
	case "etsy", "gumroad", "payhip":
		fmt.Fprintf(&b, "\n## Tags\n\n%s\n", bullets(l.Tags, "`"))
	default:
		fmt.Fprintf(&b, "\n## Categories\n\nBISAC:\n\n%s\n", bullets(l.BISAC, ""))
		if len(l.Thema) > 0 && (ch.key == "tolino" || ch.key == "kobo") {
			fmt.Fprintf(&b, "\nThema:\n\n%s\n", bullets(l.Thema, ""))
		}
		if len(l.Keywords) > 0 {
			fmt.Fprintf(&b, "\nKeywords: %s\n", strings.Join(l.Keywords, ", "))
		}
	}
	if l.Author != "" && ch.key != "etsy" {
		fmt.Fprintf(&b, "\n## Author bio\n\n```text\n%s\n```\n", PlainText(l.Author))
	}
	if notes := ch.notes(c); len(notes) > 0 {
		b.WriteString("\n## Before you publish\n\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return b.String()
}

func bullets(items []string, quote string) string {
	if len(items) == 0 {
		return "- (none in listing.md)"
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = "- " + quote + it + quote
	}
	return strings.Join(out, "\n")
}

// aiNote adds the disclosure a channel wants inside the description.
func aiNote(ch channel, l Listing, desc string) string {
	if !l.AIUsed() {
		return desc
	}
	switch {
	case ch.key == "apple" && l.AIText == "generated", ch.key == "etsy":
		return desc + "\n\n" + l.AIStatement
	}
	return desc
}

func etsyTitle(title, subtitle, language string) string {
	kind := "Ebook (EPUB + PDF)"
	if strings.HasPrefix(strings.ToLower(language), "de") {
		kind = "E-Book (EPUB + PDF)"
	}
	for _, t := range []string{title + ": " + subtitle + " | " + kind, title + " | " + kind, title} {
		if len([]rune(t)) <= 140 && !strings.HasSuffix(t, ": ") {
			return t
		}
	}
	return string([]rune(title)[:140])
}

// band describes where a price falls in a channel's royalty bands.
func band(p *Price, low, high string, inside, outside string) string {
	lo, _ := new(big.Rat).SetString(low)
	hi, _ := new(big.Rat).SetString(high)
	if p.Amount.Cmp(lo) >= 0 && p.Amount.Cmp(hi) <= 0 {
		return fmt.Sprintf("%s: %s", p.Text, inside)
	}
	return fmt.Sprintf("%s: %s (the band is %s to %s %s)", p.Text, outside, low, high, p.Currency)
}

func priceLines(ch channel, c *channelContext) []string {
	l := c.r.Listing
	var out []string
	switch ch.key {
	case "kdp":
		bands := map[string][2]string{"USD": {"2.99", "12.99"}, "EUR": {"2.69", "12.99"}, "GBP": {"1.77", "12.99"}}
		for _, p := range l.Ebook {
			if bd, ok := bands[p.Currency]; ok {
				out = append(out, "Ebook "+band(&p, bd[0], bd[1], "in the 70% royalty band", "35% royalty"))
			} else {
				out = append(out, "Ebook "+p.Text)
			}
		}
		for _, p := range l.Paperback {
			threshold := map[string]string{"USD": "9.99", "EUR": "9.99", "GBP": "7.99"}[p.Currency]
			if threshold == "" {
				out = append(out, "Paperback "+p.Text)
				continue
			}
			out = append(out, "Paperback "+band(&p, threshold, "100000", "60% royalty, minus printing", "50% royalty, minus printing"))
		}
	case "tolino":
		if p := PriceIn(l.Ebook, "EUR"); p != nil {
			out = append(out, "Ebook "+band(p, "3.99", "24.98", "87% in tolino shops, 70% in others", "67% in tolino shops, 40% in others"))
		}
	case "kobo":
		for _, p := range l.Ebook {
			if p.Currency == "USD" {
				out = append(out, "Ebook "+band(&p, "2.99", "100000", "70% royalty", "45% royalty"))
			} else {
				out = append(out, "Ebook "+p.Text)
			}
		}
	case "barnes-noble":
		for _, p := range l.Ebook {
			out = append(out, "Ebook "+p.Text)
		}
		for _, p := range l.Paperback {
			if p.Currency == "USD" {
				out = append(out, "Paperback "+band(&p, "14.99", "100000", "meets the print minimum", "below the print minimum, which B&N enforces"))
			} else {
				out = append(out, "Paperback "+p.Text)
			}
		}
	default:
		for _, p := range l.Ebook {
			out = append(out, "Ebook "+p.Text)
		}
		if ch.paperback {
			for _, p := range l.Paperback {
				out = append(out, "Paperback "+p.Text)
			}
		}
	}
	return out
}

func kdpNotes(c *channelContext) []string {
	l := c.r.Listing
	mb := float64(c.epubSize) / (1 << 20)
	notes := []string{
		fmt.Sprintf("Delivery: at the 70%% royalty KDP deducts about $%.2f or €%.2f per sale for the %.1f MB file.", max(0.01, mb*0.15), max(0.01, mb*0.12), mb),
		"KDP Select (Kindle Unlimited) makes the ebook exclusive to Amazon for 90 days, including your own page: the download page would have to stop selling it. Stay out of Select to sell everywhere.",
		"Paperback: upload the interior and the cover PDF, choose no bleed and the paper the cover was laid out for, then order a proof copy before you publish.",
	}
	switch {
	case l.AIGenerated():
		notes = append(notes, "AI: answer Yes to AI-generated content and name what was generated ("+aiParts(l)+"); Amazon does not show it to readers.")
	case l.AIUsed():
		notes = append(notes, "AI: assisted use (editing, checking) needs no disclosure; answer No to AI-generated content.")
	}
	if c.r.Config.ISBNPaperback != "" {
		notes = append(notes, "The cover carries its own barcode: tell KDP the cover already includes one.")
	}
	return notes
}

func aiParts(l Listing) string {
	var parts []string
	if l.AIText == "generated" {
		parts = append(parts, "text")
	}
	if l.AIImages == "generated" {
		parts = append(parts, "images")
	}
	return strings.Join(parts, " and ") + " with " + l.AITools
}

func appleNotes(c *channelContext) []string {
	l := c.r.Listing
	notes := []string{"Apple validates every upload with the latest EPUBCheck; lfm checks the package itself, so run EPUBCheck once if Java is at hand."}
	if l.AIText == "generated" {
		notes = append(notes, "AI: add the contributor role \"AI Generated by\" naming "+l.AITools+"; the description above already states the method, as Apple requires.")
	}
	return notes
}

func googleNotes(c *channelContext) []string {
	pictures := ""
	if len(c.r.Plan.Images) > 1 {
		pictures = ", visual"
	}
	return []string{
		"Google ignores the accessibility metadata inside the EPUB. In Book settings, Accessibility, enter: access modes textual" + pictures + "; sufficient textual; features structural navigation, table of contents, reading order, display transformability" + map[bool]string{true: ", alternative text", false: ""}[pictures != ""] + "; hazards none; conformance EPUB Accessibility 1.1, WCAG 2.2 AA.",
		"Upload the EPUB and the PDF together: readers then get both.",
	}
}

func koboNotes(c *channelContext) []string {
	return []string{"Kobo rejects many AI-heavy and low-effort submissions; the book's research and concept are your answer if they ask.", "Kobo Plus is optional and not exclusive."}
}

func tolinoNotes(c *channelContext) []string {
	notes := []string{"tolino reaches Thalia, Hugendubel, Weltbild and the libraries' Onleihe: the main German ebook market beside Amazon."}
	if c.r.Listing.AIUsed() {
		notes = append(notes, "AI: tolino requires the AI use named in the imprint; the copyright page carries the listing's statement.")
	}
	return notes
}

func bnNotes(c *channelContext) []string {
	return []string{"Paperback cover: download Barnes & Noble's cover template for the page count and paper, then place cover.jpg on the front and the back cover text below on the back.", "Barnes & Noble's free ISBN only works on Barnes & Noble.", backCover(c)}
}

func d2dNotes(c *channelContext) []string {
	return []string{"Draft2Digital forwards the ebook to Apple, Kobo, Barnes & Noble, tolino, libraries (OverDrive, BorrowBox, Hoopla) and more for about 10% of the price. Do not also upload directly to a store it delivers to.", "A one-time $20 activation fee applies to new accounts."}
}

func ingramNotes(c *channelContext) []string {
	return []string{
		"IngramSpark asks for PDF/X-1a or PDF/X-3 print files; lfm's PDF embeds every font but is not PDF/X. Convert it with a preflight tool, or upload it and check the eproof closely.",
		"Cover: download the cover template from IngramSpark's generator for the page count, place cover.jpg on the front and the back cover text below on the back; it requires a barcode, which the generator adds for your ISBN.",
		backCover(c),
		"Revisions after approval cost a fee; proof first.",
	}
}

func backCover(c *channelContext) string {
	return "Back cover text:\n\n  ```text\n  " + strings.ReplaceAll(PlainText(c.r.Listing.BackCover), "\n", "\n  ") + "\n  ```"
}

func luluNotes(c *channelContext) []string {
	return []string{"Lulu marks PDF ebooks as not accessible; sell the EPUB as the ebook.", "Order a print proof before enabling global distribution."}
}

func etsyNotes(c *channelContext) []string {
	notes := []string{"Etsy takes up to 5 files of at most 20 MB each; add cover.jpg as the first listing photo, not as a file."}
	if c.r.Listing.AIUsed() {
		notes = append(notes, "AI: Etsy requires AI use disclosed in the description; the statement closes the description above.")
	}
	return notes
}

func shopNotes(text string) func(c *channelContext) []string {
	return func(c *channelContext) []string {
		return []string{text, "Selling here and on your own page means two checkouts for the same book: keep the price the same everywhere."}
	}
}

func distributionReadme(c *channelContext, table string, refused []string, day string) string {
	r, l := c.r, c.r.Listing
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: store packages\n\nBuilt by `lfm compile` on %s from `deliverables/book`. The book's files are here once; each guide in `stores/` names the files its channel takes and every field in that channel's format, ready to paste. This folder is never deployed; keep it out of git.\n\n%s\n", r.Plan.Title, day, table)
	if len(refused) > 0 {
		fmt.Fprintf(&b, "Not prepared, because listing.md declares AI-generated text:\n\n%s\n", strings.Join(refused, "\n"))
	}
	b.WriteString("\n## Decide once, before any store\n\n")
	decisions := []string{
		"**Wide or exclusive.** KDP Select pays for Kindle Unlimited reads but forbids selling the ebook anywhere else, your own page included. Every other channel is non-exclusive.",
		"**ISBNs.** Ebooks need none on these stores. A paperback needs one per channel or your own: the free ISBNs of KDP, Barnes & Noble and Draft2Digital only work there. Selling the paperback on several channels takes your own (in Germany from MVB), set in `deliverables.book.isbn`.",
		"**Tax.** Every US store asks for a tax interview (W-8BEN outside the US) before it pays; claim your country's treaty rate. Amazon, Apple, Google, Kobo, Barnes & Noble, Etsy, Gumroad and Lulu collect VAT themselves.",
		"**Rules change.** These packages follow the channels' rules as published in October 2026. Check a channel's help pages when something here disagrees with its upload form.",
	}
	if l.AIUsed() {
		decisions = append(decisions, fmt.Sprintf("**AI use.** listing.md declares text %s and images %s with %s. Stores define generated as created by AI even when edited afterwards; keep the declaration true, it travels into every listing.", l.AIText, l.AIImages, l.AITools))
	}
	if strings.HasPrefix(strings.ToLower(r.Config.Language), "de") {
		decisions = append(decisions,
			"**Buchpreisbindung.** A German-language ebook sold to German buyers has one end price on every channel, your own page included; a sale needs a published price change.",
			"**Pflichtablieferung.** Deliver the EPUB (and the paperback) to the Deutsche Nationalbibliothek after publication.",
			"**Impressum.** The copyright page prints `seller.name` and `seller.imprint` from lfm.json: give the publisher's name and address there.")
	}
	if r.Config.Publisher == "" {
		decisions = append(decisions, "**Publisher.** `seller.name` is empty; stores then show the author or \"Independently published\".")
	}
	for _, d := range decisions {
		fmt.Fprintf(&b, "- %s\n", d)
	}
	if c.p.Interior != nil {
		fmt.Fprintf(&b, "\n## Paperback\n\n%s trim, %d pages on %s paper, inside margin %.3g in, chapters start on right-hand pages, no bleed.", r.Plan.Trim, c.p.InteriorPages, paperOf(r.Config), c.p.Inside)
		for _, key := range slices.Sorted(maps.Keys(c.p.Wraps)) {
			fmt.Fprintf(&b, " `%s` is laid out for %s's spine of %.3f in.", WrapFile(key), map[string]string{"kdp": "KDP", "lulu": "Lulu"}[key], c.p.Spines[key])
		}
		b.WriteString(" Printers without a wrap here lay out the cover on their own template from `cover.jpg` and the back cover text.\n")
	}
	if c.p.Pending {
		c.p.Problems = append(c.p.Problems, "The book changed since the last release: `lfm compile --release` prints the PDF, the paperback and its covers again. Ordinary compiles reuse them while the book is unchanged.")
	}
	if len(c.p.Problems) > 0 {
		b.WriteString("\n## Not built\n\n")
		for _, p := range c.p.Problems {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	return b.String()
}
