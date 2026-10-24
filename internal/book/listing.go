package book

import (
	"bytes"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/yuin/goldmark"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// ListingFile holds the store texts of the book: descriptions, keywords,
// categories, prices and the AI disclosure, written once and rendered into the
// format and limits of every channel.
const ListingFile = Dir + "/listing.md"

// Store limits the listing is held to; the strictest channel sets each one.
const (
	MaxDescription   = 4000 // KDP, counted as the HTML it receives
	MinDescription   = 200
	MaxShort         = 300
	MaxBackCover     = 1200
	MaxKeywords      = 7 // KDP
	MaxKeywordLength = 50
	MaxTags          = 13 // Etsy
	MaxTagLength     = 20
	MaxCategories    = 3
	MaxTitleLength   = 200 // KDP, title and subtitle together
)

// ListingSections are the sections of listing.md in their required order.
var ListingSections = []concept.SectionSpec{
	{Name: "Description"},
	{Name: "Short description"},
	{Name: "Back cover"},
	{Name: "Author"},
	{Name: "Keywords"},
	{Name: "Tags"},
	{Name: "Categories"},
	{Name: "Prices"},
	{Name: "AI"},
}

// AI use levels, as KDP defines them: generated means an AI tool created the
// content, even if it was edited afterwards; assisted means a person created it
// and AI helped to edit, check or brainstorm.
var aiLevels = map[string]bool{"none": true, "assisted": true, "generated": true}

// Price is one list price of an edition.
type Price = deliver.Price

// Listing is the parsed listing.md.
type Listing struct {
	Present     bool
	Description string // Markdown
	Short       string
	BackCover   string
	Author      string
	Keywords    []string
	Tags        []string
	BISAC       []string // "CODE label"
	Thema       []string
	KDP         []string
	Ebook       []Price
	Paperback   []Price
	AIText      string
	AIImages    string
	AITools     string
	AIStatement string
}

// AIGenerated reports whether any part of the book is AI-generated.
func (l Listing) AIGenerated() bool { return l.AIText == "generated" || l.AIImages == "generated" }

// AIUsed reports whether the book declares any AI use.
func (l Listing) AIUsed() bool {
	return (l.AIText != "" && l.AIText != "none") || (l.AIImages != "" && l.AIImages != "none")
}

// PriceIn returns the price of an edition in a currency.
func PriceIn(prices []Price, currency string) *Price {
	for i := range prices {
		if prices[i].Currency == currency {
			return &prices[i]
		}
	}
	return nil
}

var (
	bisacCode   = regexp.MustCompile(`^[A-Z]{3}[0-9]{6}$`)
	themaCode   = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.+-]{0,9}$`)
	priceText   = regexp.MustCompile(`^([0-9]+(?:[.,][0-9]{1,2})?)\s*([A-Z]{3})$`)
	moneyText   = regexp.MustCompile(`(?i)[€$£]\s?[0-9]|[0-9](?:[.,][0-9]{2})?\s?(?:€|eur\b|usd\b|euro\b|dollars?\b|£|gbp\b)`)
	reviewText  = regexp.MustCompile(`(?i)\b(?:five[- ]star|5[- ]star|bestsell(?:er|ing)|#1 (?:new )?(?:best|amazon)|leave a review|rezension hinterlassen|bestseller)\b`)
	listingDesc = goldmark.New(goldmark.WithRendererOptions(gmhtml.WithXHTML()))
)

// checkListing parses listing.md and warns where a store would refuse or
// truncate it.
func checkListing(text string, plan Plan, offer *Price, language string, report func(rule string, line int, message, hint string)) Listing {
	l := Listing{Present: true}
	add := func(line int, message, hint string) { report(RuleListing, line, message, hint) }
	d, problems := concept.ParseFormat(text, "# Listing: ", "<title>", ListingSections)
	for _, p := range problems {
		add(p.Line, p.Message, p.Hint)
	}
	line := func(name string) int {
		if s := d.Sections[name]; s != nil {
			return s.Line
		}
		return 1
	}
	if d.Title != "" && plan.Title != "" && d.Title != plan.Title {
		add(1, fmt.Sprintf("The listing is titled %q, the concept %q.", d.Title, plan.Title), "Stores need the title exactly as it appears on the cover; use the concept's.")
	}
	if n := len([]rune(plan.Title + ": " + plan.Subtitle)); n > MaxTitleLength {
		add(1, fmt.Sprintf("Title and subtitle are %d characters; KDP takes fewer than %d.", n, MaxTitleLength), "Shorten the subtitle in the concept's Hooks.")
	}

	l.Description = deliver.RawSection(text, d, "Description")
	l.Short = concept.StripCitations(deliver.RawSection(text, d, "Short description"))
	l.BackCover = deliver.RawSection(text, d, "Back cover")
	l.Author = deliver.RawSection(text, d, "Author")
	plain := PlainText(l.Description)
	switch n := len([]rune(KDPDescription(l.Description))); {
	case strings.TrimSpace(plain) == "":
		add(line("Description"), "The Description is empty.", "Write the store description: who the book is for, what they can do after it, what is inside.")
	case n > MaxDescription:
		add(line("Description"), fmt.Sprintf("The Description is %d characters as HTML; KDP takes %d.", n, MaxDescription), "Cut it to the promise, the contents and who it is for.")
	case len([]rune(plain)) < MinDescription:
		add(line("Description"), fmt.Sprintf("The Description is %d characters; write at least %d.", len([]rune(plain)), MinDescription), "Say who the book is for, what they can do after it and what is inside.")
	}
	for _, f := range []struct{ name, text string }{{"Description", l.Description}, {"Short description", l.Short}, {"Back cover", l.BackCover}} {
		if m := deliver.LinkText.FindString(f.text); m != "" {
			add(line(f.name), fmt.Sprintf("%s contains %q; stores reject links and contact details in descriptions.", f.name, m), "Remove it; the page and the book carry the links.")
		}
		if m := moneyText.FindString(f.text); m != "" {
			add(line(f.name), fmt.Sprintf("%s mentions a price (%q); stores set and show prices themselves.", f.name, m), "Remove the price from the text.")
		}
		if m := reviewText.FindString(f.text); m != "" {
			add(line(f.name), fmt.Sprintf("%s contains %q; stores reject rank claims and review requests.", f.name, m), "Describe the book itself.")
		}
		for _, h := range check.FindTells(paragraphs(PlainText(f.text))) {
			add(line(f.name), fmt.Sprintf("%s: %q is a stock phrase of generated copy.", f.name, h.Phrase), "Say the concrete thing the reader gets.")
		}
	}
	if n := len([]rune(l.Short)); n == 0 || n > MaxShort {
		add(line("Short description"), fmt.Sprintf("The Short description is %d characters; write one or two sentences up to %d.", n, MaxShort), "It opens the Etsy and shop listings and the EPUB's description.")
	}
	if n := len([]rune(PlainText(l.BackCover))); n == 0 || n > MaxBackCover {
		add(line("Back cover"), fmt.Sprintf("The Back cover text is %d characters; write up to %d.", n, MaxBackCover), "The paperback's back carries it beside the barcode; a few short paragraphs fit.")
	}
	if strings.TrimSpace(l.Author) == "" {
		add(line("Author"), "The Author section is empty.", "Write two to four sentences on why the author can write this book; facts from the owner only.")
	}

	title := strings.ToLower(plan.Title + " " + plan.Subtitle)
	for _, it := range d.Sections["Keywords"].Items() {
		k := strings.TrimSpace(it.Text)
		l.Keywords = append(l.Keywords, k)
		switch {
		case len([]rune(k)) > MaxKeywordLength:
			add(it.N, fmt.Sprintf("Keyword %q is %d characters; KDP takes %d.", k, len([]rune(k)), MaxKeywordLength), "Shorten it to the phrase readers search.")
		case strings.Contains(title, strings.ToLower(k)):
			add(it.N, fmt.Sprintf("Keyword %q repeats the title.", k), "Title words are searched already; use the slot for another phrase readers search.")
		}
	}
	if n := len(l.Keywords); n == 0 || n > MaxKeywords {
		add(line("Keywords"), fmt.Sprintf("Keywords lists %d phrases; list 1 to %d.", n, MaxKeywords), "Use the phrases readers search, from the research voices.")
	}
	for _, it := range d.Sections["Tags"].Items() {
		t := strings.TrimSpace(it.Text)
		l.Tags = append(l.Tags, t)
		if len([]rune(t)) > MaxTagLength {
			add(it.N, fmt.Sprintf("Tag %q is %d characters; Etsy takes %d.", t, len([]rune(t)), MaxTagLength), "Shorten it.")
		}
	}
	if n := len(l.Tags); n == 0 || n > MaxTags {
		add(line("Tags"), fmt.Sprintf("Tags lists %d tags; list 1 to %d.", n, MaxTags), "Etsy and shops search by them; use all 13 when you can.")
	}

	for _, it := range d.Sections["Categories"].Items() {
		kind, value, ok := strings.Cut(it.Text, ":")
		value = strings.TrimSpace(value)
		code, _, _ := strings.Cut(value, " ")
		switch strings.TrimSpace(kind) {
		case "BISAC":
			if !bisacCode.MatchString(code) {
				add(it.N, fmt.Sprintf("%q is not a BISAC code.", code), "Use the code from the BISAC subject list, e.g. CKB009000 Cooking / Methods / Baking.")
			}
			l.BISAC = append(l.BISAC, value)
		case "Thema":
			if !themaCode.MatchString(code) {
				add(it.N, fmt.Sprintf("%q is not a Thema code.", code), "Use the code from the Thema subject list, e.g. WBVD.")
			}
			l.Thema = append(l.Thema, value)
		case "KDP":
			l.KDP = append(l.KDP, value)
		default:
			ok = false
		}
		if !ok {
			add(it.N, "Categories lines start with BISAC:, Thema: or KDP:.", "Write - BISAC: <code> <label>, - Thema: <code> <label> or - KDP: <category path>.")
		}
	}
	for _, c := range []struct {
		name string
		list []string
	}{{"BISAC", l.BISAC}, {"Thema", l.Thema}, {"KDP", l.KDP}} {
		if n := len(c.list); n == 0 || n > MaxCategories {
			add(line("Categories"), fmt.Sprintf("Categories lists %d %s entries; list 1 to %d.", n, c.name, MaxCategories), "Apple, Google, Kobo and the distributors use BISAC, European shops Thema, KDP its own categories.")
		}
	}

	if s := d.Sections["Prices"]; s != nil {
		for _, it := range s.Items() {
			if strings.HasPrefix(it.Text, "Open:") {
				continue
			}
			kind, value, _ := strings.Cut(it.Text, ":")
			var prices []Price
			for _, part := range strings.Split(value, ",") {
				part = strings.TrimSpace(part)
				m := priceText.FindStringSubmatch(part)
				if m == nil {
					add(it.N, fmt.Sprintf("%q is not a price.", part), "Write prices as 7.99 EUR, 8.99 USD.")
					continue
				}
				amount, _ := new(big.Rat).SetString(strings.ReplaceAll(m[1], ",", "."))
				prices = append(prices, Price{Text: part, Amount: amount, Currency: m[2]})
			}
			switch strings.TrimSpace(kind) {
			case "Ebook":
				l.Ebook = prices
			case "Paperback":
				l.Paperback = prices
			default:
				add(it.N, "Prices lines start with Ebook: or Paperback:.", "Write - Ebook: 7.99 EUR, 8.99 USD and, for a print edition, - Paperback: 16.99 EUR.")
			}
		}
		for _, e := range l.Ebook {
			if p := PriceIn(l.Paperback, e.Currency); p != nil {
				if limit := new(big.Rat).Mul(p.Amount, big.NewRat(4, 5)); e.Amount.Cmp(limit) > 0 {
					add(s.Line, fmt.Sprintf("The ebook (%s) costs more than 80%% of the paperback (%s); KDP then pays 35%% instead of 70%%.", e.Text, p.Text), "Price the ebook at least 20% below the paperback, or accept the lower royalty.")
				}
			}
		}
		if e := PriceIn(l.Ebook, "EUR"); e != nil && offer != nil && offer.Currency == "EUR" && e.Amount.Cmp(offer.Amount) != 0 && strings.HasPrefix(strings.ToLower(language), "de") {
			add(s.Line, fmt.Sprintf("The ebook lists at %s but the page sells it for %s; the German fixed book price allows one end price on every channel.", e.Text, offer.Text), "Set one price for the page and every store, or confirm with the owner that the Buchpreisbindung does not apply.")
		}
		if len(l.Ebook) == 0 && !strings.Contains(s.Text(), "Open:") {
			add(s.Line, "Prices has no Ebook: line.", "The owner sets the prices; write - Open: prices per edition until they do.")
		}
	}

	if s := d.Sections["AI"]; s != nil {
		l.AIText, _ = s.Field("Text")
		l.AIImages, _ = s.Field("Images")
		l.AITools, _ = s.Field("Tools")
		l.AIStatement, _ = s.Field("Statement")
		for _, f := range []struct{ name, value string }{{"Text", l.AIText}, {"Images", l.AIImages}} {
			if !aiLevels[f.value] {
				add(s.Line, fmt.Sprintf("AI has no valid \"%s:\" (none, assisted or generated; got %q).", f.name, f.value), "Generated means an AI tool wrote or drew it, even if edited afterwards; assisted means a person made it and AI helped to edit or check. Stores ask, and several refuse generated books.")
			}
		}
		if l.AIUsed() {
			if l.AITools == "" {
				add(s.Line, "AI has no \"Tools:\" line.", "Name the AI tools used; tolino prints them in the imprint and Apple names them in the description.")
			}
			if l.AIStatement == "" {
				add(s.Line, "AI has no \"Statement:\" line.", "Write one sentence in the book's language for the copyright page saying what AI did and with which tools.")
			}
			for _, tool := range strings.Split(l.AITools, ",") {
				tool, _, _ = strings.Cut(tool, " (")
				if tool = strings.TrimSpace(tool); tool != "" && l.AIStatement != "" && !strings.Contains(strings.ToLower(l.AIStatement), strings.ToLower(tool)) {
					add(s.Line, fmt.Sprintf("The AI statement does not name %s.", tool), "tolino requires every tool named in the imprint.")
				}
			}
		}
	}
	for _, sec := range d.Order {
		for _, ln := range sec.Lines {
			if _, question, ok := strings.Cut(ln.Text, "Open:"); ok && (strings.TrimSpace(question) != "" || (sec.Name == "AI" && strings.Contains(ln.Text, "Statement:"))) {
				hint := "Get the answer from the owner, then replace the line with the decision."
				if strings.Contains(ln.Text, "Statement:") {
					hint += " The AI Statement would be printed in the book."
				}
				report(RuleOpen, ln.N, "Open question in the listing's "+sec.Name+": "+preview(question), hint)
			}
		}
	}
	return l
}

func paragraphs(text string) []string {
	var out []string
	for _, p := range strings.Split(text, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var (
	kdpTags      = strings.NewReplacer("<strong>", "<b>", "</strong>", "</b>", "<h1>", "<h4>", "</h1>", "</h4>", "<h2>", "<h4>", "</h2>", "</h4>", "<h3>", "<h4>", "</h3>", "</h4>", "<br />", "<br>")
	anchorTag    = regexp.MustCompile(`</?a\b[^>]*>`)
	unknownTag   = regexp.MustCompile(`</?(?:code|pre|blockquote|hr|img|del|table|thead|tbody|tr|td|th)\b[^>]*>`)
	emphasisMark = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__|\*([^*]+)\*|_([^_]+)_`)
	headingMark  = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	listMark     = regexp.MustCompile(`(?m)^[ \t]*(?:[-*+]|\d+\.)[ \t]+`)
	linkMark     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// KDPDescription renders Markdown into the HTML KDP accepts: p, br, b, i, em,
// h4 to h6, ul, ol and li.
func KDPDescription(md string) string {
	var out bytes.Buffer
	if err := listingDesc.Convert([]byte(md), &out); err != nil {
		return ""
	}
	html := kdpTags.Replace(out.String())
	html = anchorTag.ReplaceAllString(html, "")
	html = unknownTag.ReplaceAllString(html, "")
	return strings.TrimSpace(strings.ReplaceAll(html, "\n", ""))
}

// PlainText renders Markdown as plain text for stores that take no markup:
// paragraphs separated by blank lines, list items with bullets.
func PlainText(md string) string {
	s := linkMark.ReplaceAllString(md, "$1")
	s = emphasisMark.ReplaceAllStringFunc(s, func(m string) string {
		sub := emphasisMark.FindStringSubmatch(m)
		return sub[1] + sub[2] + sub[3] + sub[4]
	})
	s = headingMark.ReplaceAllString(s, "")
	s = listMark.ReplaceAllString(s, "• ")
	return strings.TrimSpace(s)
}
