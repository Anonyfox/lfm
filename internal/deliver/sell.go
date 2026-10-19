package deliver

import (
	"net/url"
	"strings"
)

// How a deliverable or a bundle is sold, as its "sell" in lfm.json says.
const (
	// SellPage sells through the page's checkout: after paying, buyers land
	// on an unlisted download page lfm publishes. The default.
	SellPage = "page"
	// SellShops sells through shops that deliver the files themselves; the
	// page's buy buttons link to the shop listings and no download page exists.
	SellShops = "shops"
	// SellBundle sells a deliverable only inside the bundle.
	SellBundle = "bundle"
)

// Sell returns the sell mode with its default.
func Sell(mode string) string {
	if mode == "" {
		return SellPage
	}
	return mode
}

// shops are the hosts of shop listings a page may link to, by name.
var shops = []struct{ name, host string }{
	{"Etsy", "etsy.com"}, {"Etsy", "etsy.me"}, {"Gumroad", "gumroad.com"}, {"Payhip", "payhip.com"},
	{"Ko-fi", "ko-fi.com"}, {"Amazon", "amazon."}, {"Amazon", "amzn.to"}, {"Apple Books", "books.apple.com"},
	{"Google Play Books", "play.google.com"}, {"Kobo", "kobo.com"}, {"tolino", "thalia.de"}, {"tolino", "hugendubel.de"},
	{"Barnes & Noble", "barnesandnoble.com"}, {"Lulu", "lulu.com"}, {"Lemon Squeezy", "lemonsqueezy.com"},
}

// Shop names the shop a checkout URL belongs to, or "" for any other host,
// such as a Stripe Payment Link.
func Shop(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	for _, s := range shops {
		if strings.HasSuffix(s.host, ".") {
			if strings.HasPrefix(host, s.host) || strings.Contains(host, "."+s.host) {
				return s.name
			}
			continue
		}
		if host == s.host || strings.HasSuffix(host, "."+s.host) {
			return s.name
		}
	}
	return ""
}
