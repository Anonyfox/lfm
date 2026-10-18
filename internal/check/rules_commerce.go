package check

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"golang.org/x/net/html"
)

func commerceRule(id string) func(*Document) []Diagnostic {
	return func(d *Document) []Diagnostic {
		var issues []Diagnostic
		for _, issue := range d.commerceFacts() {
			if issue.Rule == id {
				issues = append(issues, issue)
			}
		}
		return issues
	}
}

func (d *Document) commerceFacts() []Diagnostic {
	if d.commerceDone {
		return d.commerceIssues
	}
	d.commerceDone = true
	offers := d.Options.Commerce.Offers
	byURL := map[string][]int{}
	for i, offer := range offers {
		if key := commercecheck.Identity(offer.CheckoutURL); key != "" {
			byURL[key] = append(byURL[key], i)
		}
	}
	seen, usable := map[string]bool{}, map[string]bool{}
	for raw := range d.Options.Commerce.CheckedCheckout {
		seen[raw] = true
	}
	inspect := func(node *html.Node, raw string) {
		identity := commercecheck.Identity(raw)
		u, _ := url.Parse(raw)
		knownHost := u != nil && commercecheck.StripeHost(u.Hostname())
		if !knownHost && len(byURL[identity]) == 0 {
			return
		}
		if !seen[raw] {
			seen[raw] = true
			for _, finding := range commercecheck.Checkout(raw) {
				level := Warning
				if finding.Error {
					level = Error
				}
				issue := d.At(node, level, finding.Message, finding.Hint)
				issue.Rule = "commerce.checkout"
				d.commerceIssues = append(d.commerceIssues, issue)
			}
			if knownHost && u.Query().Has("prefilled_amount") {
				for _, index := range byURL[identity] {
					if offers[index].Price != nil {
						d.commerceAdd("commerce.checkout", node, Warning, "Amount prefill cannot change a fixed-price offer.", "Use that offer's fixed Payment Link; prefilled_amount applies only to customer-chosen amounts.", &offers[index])
						break
					}
				}
			}
		}
		if node == nil || node.Namespace != "" {
			return
		}
		if node.Data == "a" || node.Data == "area" {
			if !inactiveHTML(node) && !hidden(node) && !strings.EqualFold(Attr(node, "aria-disabled"), "true") {
				usable[identity] = true
			}
			if HasAttr(node, "download") {
				d.commerceAdd("commerce.checkout", node, Warning, "Checkout anchor requests a download.", "Use ordinary navigation to the hosted checkout; download is not a fulfilment mechanism.", nil)
			}
		} else if node.Data == "form" || node.Data == "button" || node.Data == "input" {
			d.commerceAdd("commerce.checkout", node, Warning, "Hosted Payment Link used as a form submission target.", "Expose an ordinary <a href> to checkout. Browser form data is not a supported way to prefill Stripe custom fields.", nil)
		}
	}
	for _, n := range d.Elements {
		if n.Namespace != "" || inTemplate(n) {
			continue
		}
		switch n.Data {
		case "a", "area":
			inspect(n, Attr(n, "href"))
		case "form":
			inspect(n, Attr(n, "action"))
		case "button", "input":
			if HasAttr(n, "formaction") {
				inspect(n, Attr(n, "formaction"))
			}
		}
	}
	// Reuse standard JSON-LD expansion, including aliases, @graph and @id links.
	// No second expansion, remote context fetch, or compact-key guessing.
	d.jsonldFacts()
	expansionUnavailable := len(d.jsonldExpanded) > 0
	for _, block := range d.jsonldExpanded {
		if block.data != nil {
			expansionUnavailable = false
		}
	}
	records := commerceRecords(d.jsonldExpanded)
	matcher := newCommerceMatcher(offers)
	matched := map[int]bool{}
	for _, record := range records {
		if schemaType(record.value, "Offer") {
			d.checkOfferRecord(record, matcher, matched)
		}
		if schemaType(record.value, "BuyAction") {
			for _, target := range actionTargets(record.value) {
				inspect(record.node, target)
			}
			d.checkBuyRecord(record, matcher)
		}
	}
	for i, offer := range offers {
		if offer.CheckoutURL != "" && !usable[commercecheck.Identity(offer.CheckoutURL)] {
			d.commerceAdd("commerce.discovery", nil, Warning, "Declared offer has no initially usable native checkout anchor.", "Render an <a href> using urls[checkoutUrlKey] outside hidden/inert/closed views; keep a direct purchase path alongside the interactive funnel.", &offer)
		}
		if offer.Price != nil && (offer.Price.Scale == 1 || offer.Price.Scale == 100) && offer.Price.AmountMinor > 0 && !d.priceVisible(*offer.Price) {
			d.commerceAdd("commerce.discovery", nil, Warning, fmt.Sprintf("The price of the fixed-price offer (%s) does not appear in the page's initial text.", formatMinor(*offer.Price)), "Show the price next to the offer and its checkout link in the HTML, with its currency; a price only in scripts, images or structured data is invisible to readers, crawlers and assistants.", &offer)
		}
		if offer.Price != nil && !matched[i] && !expansionUnavailable {
			d.commerceAdd("commerce.discovery", nil, Warning, "Fixed-price offer has no identifiable Schema.org Offer in initial JSON-LD.", "Describe the offer in the compiler-owned block; use the same sku on its Offer/Product or an unambiguous checkout target. Unknown remote contexts cannot be checked offline.", &offer)
		}
	}
	return d.commerceIssues
}

func (d *Document) commerceAdd(rule string, node *html.Node, level Severity, message, hint string, offer *commercecheck.Offer) {
	issue := d.At(node, level, message, hint)
	issue.Rule = rule
	if offer != nil {
		issue.Path = offer.Path
		if offer.Location.File != "" {
			issue.Related = append(issue.Related, offer.Location)
		}
	}
	if node != nil && node.Data == "script" && d.Options.JSONLDGenerated && d.Options.JSONLDSource != nil {
		issue.Related = append(issue.Related, *d.Options.JSONLDSource)
	}
	d.commerceIssues = append(d.commerceIssues, issue)
}

func commerceSecretsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	add := func(name string, raw []byte) {
		if at := commercecheck.SecretOffset(raw); at >= 0 {
			issues = append(issues, Diagnostic{Rule: "commerce.secrets", Severity: Error, Location: diagnostic.At(name, raw, at), Message: "Published text contains a Stripe secret-key-shaped value.", Hint: "Remove it from the template, browser data or bundled code; keep API/webhook secrets in the operator environment. This value is intentionally redacted; public pk_ keys/object IDs are allowed."})
		}
	}
	add("index.html", d.Raw)
	names := make([]string, 0, len(d.Options.Assets))
	for name := range d.Options.Assets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".js", ".mjs", ".cjs", ".css", ".json", ".html", ".txt", ".svg":
			add(name, d.Options.Assets[name])
		}
	}
	return issues
}

// moneyNumber finds amounts as people write them: 12, 12.00, 12,00, 1.200,50,
// 1,200.50, 1 200 or 12,–.
var moneyNumber = regexp.MustCompile(`\d{1,3}(?:[.,' \x{a0}\x{202f}]\d{3})+(?:[.,]\d{1,2})?|\d+(?:[.,]\d{1,2})?`)

var currencyTokens = map[string][]string{
	"EUR": {"€", "eur", "euro"}, "USD": {"$", "usd", "us$"}, "GBP": {"£", "gbp"}, "CHF": {"chf", "fr."},
	"JPY": {"¥", "jpy", "円"}, "SEK": {"sek", "kr"}, "NOK": {"nok", "kr"}, "DKK": {"dkk", "kr"}, "PLN": {"pln", "zł"},
}

// priceVisible reports whether visible text names the amount with its
// currency token within a few characters.
func (d *Document) priceVisible(price commercecheck.Money) bool {
	tokens := currencyTokens[strings.ToUpper(price.Currency)]
	tokens = append(tokens, strings.ToLower(price.Currency))
	for _, s := range visibleSegments(d) {
		text := strings.ToLower(s.text)
		for _, m := range moneyNumber.FindAllStringIndex(text, -1) {
			minor, ok := minorUnits(text[m[0]:m[1]], price.Scale)
			if !ok || minor != price.AmountMinor {
				continue
			}
			before, after := text[max(0, m[0]-8):m[0]], text[m[1]:min(len(text), m[1]+8)]
			for _, t := range tokens {
				if strings.Contains(before, t) || strings.Contains(after, t) {
					return true
				}
			}
		}
	}
	return false
}

// minorUnits converts a written amount to minor units for a currency with
// scale minor units per major unit (100 for cents). A separator followed by
// exactly three digits groups thousands; one or two digits are decimals.
func minorUnits(written string, scale int64) (int64, bool) {
	digits, decimals := "", ""
	cut := strings.LastIndexAny(written, ".,")
	if cut >= 0 && len(written)-cut-1 <= 2 {
		decimals = written[cut+1:]
		written = written[:cut]
	}
	for _, r := range written {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
	}
	major, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, false
	}
	minor := int64(0)
	if decimals != "" {
		if scale != 100 {
			return 0, false
		}
		if len(decimals) == 1 {
			decimals += "0"
		}
		minor, _ = strconv.ParseInt(decimals, 10, 64)
	}
	return major*scale + minor, true
}

func formatMinor(price commercecheck.Money) string {
	if price.Scale == 100 {
		return fmt.Sprintf("%d.%02d %s", price.AmountMinor/100, price.AmountMinor%100, strings.ToUpper(price.Currency))
	}
	return fmt.Sprintf("%d %s", price.AmountMinor/max(price.Scale, 1), strings.ToUpper(price.Currency))
}

// legalRule asks a page that sells for links to the owner's legal pages: a
// privacy policy everywhere, and an Impressum on German pages, where the law
// requires one for commercial sites.
func legalRule(d *Document) []Diagnostic {
	links := d.activeNodes("a[href]")
	sells := len(d.Options.Commerce.Offers) > 0
	for _, n := range links {
		if u, err := url.Parse(Attr(n, "href")); err == nil && (u.Hostname() == "buy.stripe.com" || u.Hostname() == "checkout.stripe.com") {
			sells = true
		}
	}
	if !sells {
		return nil
	}
	privacy, imprint := false, false
	for _, n := range links {
		if hidden(n) {
			continue
		}
		about := strings.ToLower(d.controlName(n) + " " + Attr(n, "href"))
		privacy = privacy || strings.Contains(about, "datenschutz") || strings.Contains(about, "privacy") || strings.Contains(about, "privacidad") || strings.Contains(about, "confidentialit")
		imprint = imprint || strings.Contains(about, "impressum") || strings.Contains(about, "imprint") || strings.Contains(about, "legal notice") || strings.Contains(about, "mentions légales")
	}
	var issues []Diagnostic
	if !privacy {
		issues = append(issues, d.Whole(Warning, "The page sells but links no privacy policy.", "Link the owner's privacy policy (a footer link is enough); never write legal text yourself. Missing text is an Open: question for the owner."))
	}
	lang := ""
	for _, n := range d.Elements {
		if n.Data == "html" {
			lang = strings.ToLower(Attr(n, "lang"))
			break
		}
	}
	if strings.HasPrefix(lang, "de") && !imprint {
		issues = append(issues, d.Whole(Warning, "A German page that sells links no Impressum.", "Link the owner's Impressum; for digital goods also show the Widerrufsbelehrung and collect consent to immediate delivery at checkout. Never write legal text yourself."))
	}
	return issues
}
