package deliver

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/concept"
)

func TestSellDefaultsToThePage(t *testing.T) {
	for in, want := range map[string]string{"": SellPage, SellPage: SellPage, SellShops: SellShops, SellBundle: SellBundle} {
		if got := Sell(in); got != want {
			t.Errorf("Sell(%q) = %q", in, got)
		}
	}
}

func TestShopNamesShopListings(t *testing.T) {
	for url, want := range map[string]string{
		"https://www.etsy.com/listing/123/planner":       "Etsy",
		"https://etsy.me/3abc":                           "Etsy",
		"https://anonyfox.gumroad.com/l/planner":         "Gumroad",
		"https://payhip.com/b/AbCd":                      "Payhip",
		"https://ko-fi.com/s/abc":                        "Ko-fi",
		"https://www.amazon.de/dp/B0ABC":                 "Amazon",
		"https://smile.amazon.co.uk/dp/B0ABC":            "Amazon",
		"https://amzn.to/xyz":                            "Amazon",
		"https://books.apple.com/de/book/id1":            "Apple Books",
		"https://play.google.com/store/books/details":    "Google Play Books",
		"https://www.kobo.com/de/de/ebook/x":             "Kobo",
		"https://www.thalia.de/shop/home/artikeldetails": "tolino",
		"https://WWW.HUGENDUBEL.DE/de/ebook":             "tolino",
		"https://www.barnesandnoble.com/w/x":             "Barnes & Noble",
		"https://www.lulu.com/shop/x":                    "Lulu",
		"https://store.lemonsqueezy.com/checkout/x":      "Lemon Squeezy",
	} {
		if got := Shop(url); got != want {
			t.Errorf("Shop(%q) = %q, want %q", url, got, want)
		}
	}
	for _, url := range []string{
		"https://buy.stripe.com/test_abc",
		"https://notetsy.com/listing",
		"https://etsy.com.evil.test/listing",
		"https://amazonaws.com/bucket",
		"https://example.com/?r=https://etsy.com",
		"etsy.com/listing/1",
		"://bad",
		"",
	} {
		if got := Shop(url); got != "" {
			t.Errorf("Shop(%q) = %q, want none", url, got)
		}
	}
}

func TestRawSectionKeepsParagraphs(t *testing.T) {
	text := "# Listing: X\r\n\r\n## Title\r\n\r\nT\r\n\r\n## Description\r\n\r\nFirst paragraph.\r\n\r\n- item\r\n\r\nLast.\r\n\r\n## Tags\r\n\r\n- a\r\n"
	d, _ := concept.ParseFormat(text, "# Listing: ", "<x>", ShopListingSections)
	if got := RawSection(text, d, "Description"); got != "First paragraph.\n\n- item\n\nLast." {
		t.Fatalf("%q", got)
	}
	if got := RawSection(text, d, "Tags"); got != "- a" {
		t.Fatalf("last section %q", got)
	}
	if got := RawSection(text, d, "AI"); got != "" {
		t.Fatalf("missing section %q", got)
	}
}

func TestLinkTextFindsAddresses(t *testing.T) {
	for text, want := range map[string]string{
		"see HTTP://x.test":       "HTTP://",
		"at www.shop-1.de today":  "www.shop-1.",
		"mail a.b+c@mail.example": "a.b+c@mail.example",
		"A4 and 8.5x11 inch":      "",
		"no@tld":                  "",
	} {
		if got := LinkText.FindString(text); !strings.EqualFold(got, want) {
			t.Errorf("LinkText in %q = %q, want %q", text, got, want)
		}
	}
}
