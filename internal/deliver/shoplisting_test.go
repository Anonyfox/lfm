package deliver

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
)

// cleanDescription is a description stores take: long enough, no links.
var cleanDescription = "A **weekly planner** you print at home.\n\n" +
	"- Undated pages for any year\n- A4 and US Letter\n\n" +
	"Print as many copies as you like for your own use. Nothing is shipped; you download the PDF files right after buying and print them on any home printer."

type listingParts struct {
	title, description, price, ai string
	tags                          []string
}

func cleanParts() listingParts {
	return listingParts{
		title:       "Weekly Planner Printable, Undated, A4 and Letter",
		description: cleanDescription,
		price:       "7.99 EUR",
		ai:          "- Text: none\n- Images: none",
		tags:        []string{"weekly planner", "printable planner", "undated planner"},
	}
}

func (p listingParts) text() string {
	var b strings.Builder
	b.WriteString("# Listing: Weekly Planner\n\n## Title\n\n" + p.title + "\n\n## Description\n\n" + p.description + "\n\n## Tags\n\n")
	for _, t := range p.tags {
		b.WriteString("- " + t + "\n")
	}
	b.WriteString("\n## Price\n\n" + p.price + "\n\n## AI\n\n" + p.ai + "\n")
	return b.String()
}

type problem struct {
	line          int
	message, hint string
}

func check(t *testing.T, text string, offer *Price) (ShopListing, []problem) {
	t.Helper()
	var got []problem
	l := CheckShopListing(text, offer, func(line int, message, hint string) {
		got = append(got, problem{line, message, hint})
	})
	return l, got
}

func mustPrice(t *testing.T, s string) *Price {
	t.Helper()
	p, ok := ParsePrice(s)
	if !ok {
		t.Fatalf("ParsePrice(%q) failed", s)
	}
	return &p
}

func TestCleanShopListingParsesWithoutProblems(t *testing.T) {
	l, problems := check(t, cleanParts().text(), mustPrice(t, "7.99 EUR"))
	if len(problems) > 0 {
		t.Fatalf("clean listing refused: %+v", problems)
	}
	if !l.Present || l.Title != "Weekly Planner Printable, Undated, A4 and Letter" {
		t.Fatalf("title %q", l.Title)
	}
	if l.Description != cleanDescription {
		t.Fatalf("description lost its Markdown paragraphs:\n%q", l.Description)
	}
	if strings.Join(l.Tags, "|") != "weekly planner|printable planner|undated planner" {
		t.Fatalf("tags %q", l.Tags)
	}
	if l.Price == nil || l.Price.Text != "7.99 EUR" || l.Price.Currency != "EUR" {
		t.Fatalf("price %+v", l.Price)
	}
	if l.AIText != "none" || l.AIImages != "none" || l.AIUsed() {
		t.Fatalf("AI %+v", l)
	}
}

func TestShopListingRefusals(t *testing.T) {
	longTitle := strings.Repeat("Planner, ", 16) + "A4ü" // 147 characters, ü counted once
	manyTags := make([]string, 14)
	for i := range manyTags {
		manyTags[i] = fmt.Sprintf("tag %d", i)
	}
	tests := []struct {
		name  string
		edit  func(*listingParts)
		offer string
		want  string
	}{
		{"title over 140", func(p *listingParts) { p.title = longTitle }, "", "Title has 147 characters; Etsy takes 140."},
		{"empty title", func(p *listingParts) { p.title = "" }, "", "Title is empty."},
		{"more than 13 tags", func(p *listingParts) { p.tags = manyTags }, "", "14 tags; Etsy takes 13."},
		{"tag over 20", func(p *listingParts) { p.tags = []string{"printable weekly planner pages"} }, "", `Tag "printable weekly planner pages" has 30 characters; Etsy takes 20.`},
		{"short description", func(p *listingParts) { p.description = "Too short." }, "", "Description has 10 characters; stores want at least 200."},
		{"link in description", func(p *listingParts) { p.description = cleanDescription + " More at https://example.com." }, "", `Description contains "https://"; stores refuse links`},
		{"www address in description", func(p *listingParts) { p.description = cleanDescription + " See www.example.com." }, "", `Description contains "www.example."`},
		{"email in description", func(p *listingParts) { p.description = cleanDescription + " Write to me@example.com." }, "", `Description contains "me@example.com"`},
		{"price differs from offer", func(p *listingParts) { p.price = "9.99 EUR" }, "7.99 EUR", "The store price 9.99 EUR differs from the page's offer 7.99 EUR."},
		{"price unreadable", func(p *listingParts) { p.price = "cheap" }, "", `"cheap" is not a price.`},
		{"unknown AI level", func(p *listingParts) { p.ai = "- Text: some\n- Images: none\n- Tools: X\n- Statement: Y." }, "", `AI level "some" is not none, assisted or generated.`},
		{"missing AI level", func(p *listingParts) { p.ai = "- Text: none" }, "", `AI level "" is not none`},
		{"AI used without tools", func(p *listingParts) {
			p.ai = "- Text: assisted\n- Images: none\n- Statement: Text drafted with an assistant."
		}, "", "AI use is declared without Tools and Statement."},
		{"AI used without statement", func(p *listingParts) { p.ai = "- Text: none\n- Images: generated\n- Tools: Imagen" }, "", "AI use is declared without Tools and Statement."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := cleanParts()
			tc.edit(&p)
			var offer *Price
			if tc.offer != "" {
				offer = mustPrice(t, tc.offer)
			}
			_, problems := check(t, p.text(), offer)
			if len(problems) != 1 {
				t.Fatalf("want one problem, got %+v", problems)
			}
			if !strings.Contains(problems[0].message, tc.want) {
				t.Fatalf("message %q, want %q", problems[0].message, tc.want)
			}
			if problems[0].hint == "" || problems[0].line < 1 {
				t.Fatalf("problem without hint or line: %+v", problems[0])
			}
		})
	}
}

func TestShopListingProblemsPointAtTheirLine(t *testing.T) {
	p := cleanParts()
	p.tags = []string{"ok", "a tag far longer than twenty"}
	text := p.text()
	_, problems := check(t, text, nil)
	if len(problems) != 1 {
		t.Fatalf("%+v", problems)
	}
	lines := strings.Split(text, "\n")
	if got := lines[problems[0].line-1]; got != "- a tag far longer than twenty" {
		t.Fatalf("problem on line %d: %q", problems[0].line, got)
	}
}

func TestShopListingPriceRules(t *testing.T) {
	for _, tc := range []struct {
		price, offer string
		refused      bool
	}{
		{"Open: price", "7.99 EUR", false}, // the owner has not decided
		{"7,99 EUR", "7.99 EUR", false},    // a decimal comma is the same price
		{"12.00 EUR", "12 EUR", false},     // trailing zeros are the same amount
		{"- 7.99 EUR", "7.99 EUR", false},  // a list marker is dropped
		{"9.99 USD", "7.99 EUR", false},    // another currency is not compared
		{"12 EUR", "", false},              // no offer to compare with
		{"7.990 EUR", "7.99 EUR", true},    // three decimals are no price
		{"7.99 eur", "7.99 EUR", true},     // the currency is upper case
	} {
		p := cleanParts()
		p.price = tc.price
		var offer *Price
		if tc.offer != "" {
			offer = mustPrice(t, tc.offer)
		}
		l, problems := check(t, p.text(), offer)
		if want := map[bool]int{true: 1}[tc.refused]; len(problems) != want {
			t.Errorf("price %q against %q: %+v", tc.price, tc.offer, problems)
		}
		if strings.HasPrefix(tc.price, "Open:") && l.Price != nil {
			t.Errorf("an open price was read as %+v", l.Price)
		}
	}
}

func TestShopListingReportsFormatProblems(t *testing.T) {
	_, problems := check(t, "# Listing: X\n\n## Title\n\nA title\n", nil)
	var missing []string
	for _, p := range problems {
		if name, ok := strings.CutPrefix(p.message, "Missing section "); ok {
			missing = append(missing, strings.TrimSuffix(name, "."))
		}
	}
	if strings.Join(missing, ",") != "Description,Tags,Price,AI" {
		t.Fatalf("missing sections %q in %+v", missing, problems)
	}
	_, problems = check(t, strings.Replace(cleanParts().text(), "# Listing: Weekly Planner", "# Weekly Planner", 1), nil)
	if len(problems) != 1 || problems[0].line != 1 {
		t.Fatalf("a wrong title line: %+v", problems)
	}
}

func TestParsePrice(t *testing.T) {
	for in, want := range map[string]string{"7.99 EUR": "7.99 EUR", "7,99 EUR": "7.99 EUR", " 12 USD ": "12.00 USD", "0.5GBP": "0.50 GBP", "1000 EUR": "1000.00 EUR"} {
		p, ok := ParsePrice(in)
		if !ok || p.Text != want {
			t.Errorf("ParsePrice(%q) = %q, %v; want %q", in, p.Text, ok, want)
		}
	}
	p, _ := ParsePrice("7,99 EUR")
	if p.Amount.Cmp(big.NewRat(799, 100)) != 0 || p.Currency != "EUR" {
		t.Fatalf("%+v", p)
	}
	for _, bad := range []string{"", "EUR 7.99", "7.99", "7.999 EUR", "1.234,56 EUR", "-3 EUR", "7.99 euro", "free"} {
		if _, ok := ParsePrice(bad); ok {
			t.Errorf("ParsePrice(%q) accepted", bad)
		}
	}
}

func TestAIUsedCountsTextOrImages(t *testing.T) {
	for _, tc := range []struct {
		l    ShopListing
		want bool
	}{
		{ShopListing{}, false},
		{ShopListing{AIText: "none", AIImages: "none"}, false},
		{ShopListing{AIText: "assisted", AIImages: "none"}, true},
		{ShopListing{AIText: "none", AIImages: "generated"}, true},
	} {
		if got := tc.l.AIUsed(); got != tc.want {
			t.Errorf("%+v: AIUsed = %v", tc.l, got)
		}
	}
}

func TestPlainTextDropsMarkdown(t *testing.T) {
	got := PlainText("## What you get\n\nA **bold** and __strong__ planner.\n\n- one\n  * two\n### Sizes\nA4")
	want := "What you get\n\nA bold and strong planner.\n\n• one\n• two\nSizes\nA4"
	if got != want {
		t.Fatalf("PlainText:\n%q\nwant\n%q", got, want)
	}
}
