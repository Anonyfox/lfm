package deliver

import (
	"math/big"
	"regexp"
	"strings"

	"github.com/Anonyfox/lfm/internal/concept"
)

// Price is one list price, e.g. of an edition or a product.
type Price struct {
	Text     string // as written, e.g. "7.99 EUR"
	Amount   *big.Rat
	Currency string
}

var priceText = regexp.MustCompile(`^([0-9]+(?:[.,][0-9]{1,2})?)\s*([A-Z]{3})$`)

// ParsePrice reads a price written as 7.99 EUR or 7,99 EUR.
func ParsePrice(s string) (Price, bool) {
	s = strings.TrimSpace(s)
	m := priceText.FindStringSubmatch(s)
	if m == nil {
		return Price{}, false
	}
	amount, ok := new(big.Rat).SetString(strings.Replace(m[1], ",", ".", 1))
	if !ok {
		return Price{}, false
	}
	return Price{Text: amount.FloatString(2) + " " + m[2], Amount: amount, Currency: m[2]}, true
}

// LinkText matches a web address or email address in store copy, which
// stores refuse in descriptions.
var LinkText = regexp.MustCompile(`(?i)\bhttps?://|\bwww\.[a-z0-9-]+\.|[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)

// RawSection returns the source lines of a section as written, blank lines
// included, so Markdown paragraphs survive.
func RawSection(text string, d concept.Document, name string) string {
	s := d.Sections[name]
	if s == nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	end := len(lines)
	for _, other := range d.Order {
		if other.Line > s.Line && other.Line-1 < end {
			end = other.Line - 1
		}
	}
	return strings.TrimSpace(strings.Join(lines[s.Line:end], "\n"))
}
