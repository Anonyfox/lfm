package check

import (
	"net/url"
	"path/filepath"
	"sort"
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
