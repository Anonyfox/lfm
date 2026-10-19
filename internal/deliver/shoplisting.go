package deliver

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/concept"
)

// Store limits, from the strictest channel (Etsy).
const (
	MaxTitle       = 140
	MaxTags        = 13
	MaxTag         = 20
	MinDescription = 200
)

// ShopListingSections are the sections of a shop listing.md, in order.
var ShopListingSections = []concept.SectionSpec{
	{Name: "Title"}, {Name: "Description"}, {Name: "Tags"}, {Name: "Price"}, {Name: "AI"},
}

// ShopListing is a parsed listing.md of a product sold in digital shops:
// a printable or a bundle.
type ShopListing struct {
	Present     bool
	Title       string
	Description string // Markdown
	Tags        []string
	Price       *Price
	AIText      string
	AIImages    string
	AITools     string
	AIStatement string
}

// AIUsed reports whether the product declares any AI use.
func (l ShopListing) AIUsed() bool {
	return (l.AIText != "" && l.AIText != "none") || (l.AIImages != "" && l.AIImages != "none")
}

var aiLevels = map[string]bool{"none": true, "assisted": true, "generated": true}

// CheckShopListing parses a shop listing.md and reports where a shop would
// refuse it or where its price differs from the page's offer.
func CheckShopListing(text string, offer *Price, add func(line int, message, hint string)) ShopListing {
	l := ShopListing{Present: true}
	d, problems := concept.ParseFormat(text, "# Listing: ", "<product name>", ShopListingSections)
	for _, p := range problems {
		add(p.Line, p.Message, p.Hint)
	}
	line := func(name string) int {
		if s := d.Sections[name]; s != nil {
			return s.Line
		}
		return 1
	}
	if s := d.Sections["Title"]; s != nil {
		l.Title = firstLine(s)
		switch n := utf8.RuneCountInString(l.Title); {
		case n == 0:
			add(s.Line, "Title is empty.", "Write the store title: what it is, for whom, the sizes; the words buyers search first.")
		case n > MaxTitle:
			add(s.Line, fmt.Sprintf("Title has %d characters; Etsy takes %d.", n, MaxTitle), "Cut it to the product, the buyer and the formats.")
		}
	}
	l.Description = RawSection(text, d, "Description")
	if n := utf8.RuneCountInString(l.Description); d.Sections["Description"] != nil && n < MinDescription {
		add(line("Description"), fmt.Sprintf("Description has %d characters; stores want at least %d.", n, MinDescription), "Say what is included, the sizes, how it works, what buyers need, and that nothing is shipped.")
	}
	if m := LinkText.FindString(l.Description); m != "" {
		add(line("Description"), fmt.Sprintf("Description contains %q; stores refuse links and contact details there.", m), "Remove it; Etsy also forbids sending buyers off Etsy.")
	}
	if s := d.Sections["Tags"]; s != nil {
		for _, it := range s.Items() {
			tag := strings.TrimSpace(it.Text)
			if n := utf8.RuneCountInString(tag); n > MaxTag {
				add(it.N, fmt.Sprintf("Tag %q has %d characters; Etsy takes %d.", tag, n, MaxTag), "Shorten it to the phrase buyers type.")
			}
			l.Tags = append(l.Tags, tag)
		}
		if len(l.Tags) > MaxTags {
			add(s.Line, fmt.Sprintf("%d tags; Etsy takes %d.", len(l.Tags), MaxTags), "Keep the phrases buyers search most.")
		}
	}
	if s := d.Sections["Price"]; s != nil {
		raw := firstLine(s)
		if !strings.HasPrefix(raw, "Open:") {
			if p, ok := ParsePrice(raw); ok {
				l.Price = &p
				if offer != nil && offer.Currency == p.Currency && offer.Amount.Cmp(p.Amount) != 0 {
					add(s.Line, fmt.Sprintf("The store price %s differs from the page's offer %s.", p.Text, offer.Text), "Sell at one price everywhere; in Germany it is the law for many products and fair for all.")
				}
			} else {
				add(s.Line, fmt.Sprintf("%q is not a price.", raw), "Write it as 12.00 EUR, or \"Open: price\" until the owner decides.")
			}
		}
	}
	if s := d.Sections["AI"]; s != nil {
		l.AIText, _ = s.Field("Text")
		l.AIImages, _ = s.Field("Images")
		l.AITools, _ = s.Field("Tools")
		l.AIStatement, _ = s.Field("Statement")
		for _, v := range []string{l.AIText, l.AIImages} {
			if !aiLevels[v] {
				add(s.Line, fmt.Sprintf("AI level %q is not none, assisted or generated.", v), "Declare Text and Images honestly: Etsy and Creative Market ask, and TPT refuses generated work.")
				break
			}
		}
		if l.AIUsed() && (l.AITools == "" || l.AIStatement == "") {
			add(s.Line, "AI use is declared without Tools and Statement.", "Name the tools and write the one-sentence disclosure stores show.")
		}
	}
	return l
}

// firstLine is the first line of a section, without a list marker.
func firstLine(s *concept.Section) string {
	if s == nil || len(s.Lines) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.TrimLeft(s.Lines[0].Text, "-* "))
}
