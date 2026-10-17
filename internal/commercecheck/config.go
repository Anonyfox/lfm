// Package commercecheck checks public offer snapshots without contacting Stripe.
// Only the documented optional commerce convention has typed fields; extensions
// remain ordinary data and are never normalized or removed from the template.
package commercecheck

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

type Offer struct {
	Path, SKU, Name, CheckoutURLKey, CheckoutURL string
	Price                                        *Money
	Location                                     diagnostic.Location
}

type Facts struct {
	Offers          []Offer
	Consumed        []string // operational fields checked here, rather than rendered
	CheckedCheckout map[string]bool
}

func Catalog() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{ID: "commerce.config", Description: "Optional offer shapes, unique SKUs, named checkout references and Stripe object IDs"},
		{ID: "commerce.price", Description: "Exact nonnegative minor-unit amounts, currency pairs and Stripe charge conventions"},
		{ID: "commerce.fields", Description: "Declared Stripe custom-field counts, keys, labels, options, bounds and defaults"},
	}
}

type checker struct {
	sources jsonsrc.Sources
	facts   Facts
	issues  []diagnostic.Diagnostic
}

func (c *checker) add(rule string, severity diagnostic.Severity, path, message, hint string) {
	at, ok := c.sources.Nearest(path)
	if !ok {
		at = diagnostic.Location{File: "lfm.json", Line: 1, Column: 1}
	}
	c.issues = append(c.issues, diagnostic.Diagnostic{Rule: rule, Severity: severity, Location: at, Path: path, Message: message, Hint: hint})
}

func (c *checker) consume(path string) { c.facts.Consumed = append(c.facts.Consumed, path) }

// Check does not require commerce on knowledge/tools/free pages. Unknown fields
// within the convention remain available for arbitrary authoring and extensions.
func Check(data map[string]any, urls map[string]string, sources jsonsrc.Sources) (Facts, []diagnostic.Diagnostic) {
	c := checker{sources: sources, facts: Facts{CheckedCheckout: map[string]bool{}}}
	raw, present := data["commerce"]
	if !present {
		return c.facts, nil
	}
	commerce, object := raw.(map[string]any)
	if !object {
		return c.facts, nil // an existing custom flag/string is not an offer declaration
	}
	raw, present = commerce["offers"]
	if !present {
		return c.facts, c.issues // commerce may hold other page-specific data
	}
	offers := c.object(raw, "data.commerce.offers", "commerce.config")
	if offers == nil {
		return c.facts, c.issues
	}
	seen := map[string]Offer{}
	for _, key := range keys(offers) {
		path := jsonsrc.Child("data.commerce.offers", key)
		if strings.TrimSpace(key) == "" {
			c.add("commerce.config", diagnostic.Error, path, "Offer key is empty.", "Use a stable purpose-based object key.")
		}
		value := c.object(offers[key], path, "commerce.config")
		if value == nil {
			continue
		}
		offer := Offer{Path: path}
		offer.Location, _ = sources.Nearest(path)
		for _, name := range []string{"sku", "name", "checkoutUrlKey"} {
			child := path + "." + name
			if _, exists := value[name]; !exists {
				c.add("commerce.config", diagnostic.Warning, child, "Offer has no "+name+".", "Declare this public offer fact to enable identity/checkout checks; omit commerce.offers for a page without sellable offers.")
				continue
			}
			text, valid := c.text(value[name], child, "commerce.config", 0)
			if !valid {
				continue
			}
			switch name {
			case "sku":
				offer.SKU = text
				c.consume(child)
				if strings.ContainsFunc(text, unicode.IsControl) {
					c.add("commerce.config", diagnostic.Warning, child, "SKU contains control characters.", "Use a stable printable public identifier; control characters cannot form a UCP permalink item ID.")
				}
			case "name":
				offer.Name = text
			case "checkoutUrlKey":
				c.consume(child)
				offer.CheckoutURLKey = text
				target, exists := urls[text]
				if !exists {
					c.add("commerce.config", diagnostic.Error, child, "checkoutUrlKey does not name an entry in urls.", "Store the hosted Stripe link once under urls, then use that exact key here and in purchase anchors.")
				} else {
					offer.CheckoutURL = target
					c.consume(jsonsrc.Child("urls", text))
					if !c.facts.CheckedCheckout[target] {
						c.facts.CheckedCheckout[target] = true
						for _, finding := range Checkout(target) {
							level := diagnostic.Warning
							if finding.Error {
								level = diagnostic.Error
							}
							c.add("commerce.checkout", level, jsonsrc.Child("urls", text), finding.Message, finding.Hint)
						}
					}
				}
			}
		}
		if offer.SKU != "" {
			if prior, duplicate := seen[offer.SKU]; duplicate {
				c.add("commerce.config", diagnostic.Error, path+".sku", "Two offers declare the same SKU.", "Assign a unique sellable-unit identifier to each offer; keep a bundle or variant explicit.")
				c.issues[len(c.issues)-1].Related = []diagnostic.Location{prior.Location}
			} else {
				seen[offer.SKU] = offer
			}
		}
		if raw, exists := value["price"]; exists {
			offer.Price = c.money(raw, path+".price")
			if offer.Price != nil {
				if u, err := url.Parse(offer.CheckoutURL); err == nil && StripeHost(u.Hostname()) && u.Query().Has("prefilled_amount") {
					c.add("commerce.checkout", diagnostic.Warning, path+".checkoutUrlKey", "Amount prefill cannot change a fixed-price offer.", "prefilled_amount applies only to customer-chosen amounts. Use the correct fixed Price/Payment Link or omit the fixed price snapshot for variable pricing.")
				}
			}
		}
		if raw, exists := value["stripe"]; exists {
			stripe := c.object(raw, path+".stripe", "commerce.config")
			for _, field := range []struct{ name, prefix string }{{"productId", "prod_"}, {"priceId", "price_"}, {"paymentLinkId", "plink_"}} {
				if raw, exists := stripe[field.name]; exists {
					child := path + ".stripe." + field.name
					c.consume(child)
					if text, valid := c.text(raw, child, "commerce.config", 0); valid && !objectID(text, field.prefix) {
						c.add("commerce.config", diagnostic.Warning, child, "Stripe object ID does not have the expected "+field.prefix+" shape.", "Read back the correct object ID from Stripe; a Product, Price, Payment Link ID and hosted URL are different values. Syntax cannot prove live state.")
					}
				}
			}
			if raw, exists := stripe["customFields"]; exists {
				c.consume(path + ".stripe.customFields")
				c.fields(raw, path+".stripe.customFields")
			}
		}
		// Catch case-only mistakes without rejecting arbitrary new authoring keys.
		for _, key := range keys(value) {
			for _, known := range []string{"sku", "name", "checkoutUrlKey", "price", "stripe"} {
				if key != known && strings.EqualFold(key, known) {
					c.add("commerce.config", diagnostic.Warning, jsonsrc.Child(path, key), "Offer field differs in case from "+known+".", "Use the documented spelling for automatic checks; unknown extension fields remain allowed.")
				}
			}
		}
		c.facts.Offers = append(c.facts.Offers, offer)
	}
	seenCheckout := map[string]Offer{}
	for _, offer := range c.facts.Offers {
		identity := Identity(offer.CheckoutURL)
		if identity == "" {
			continue
		}
		if prior, exists := seenCheckout[identity]; exists && prior.SKU != offer.SKU {
			c.add("commerce.config", diagnostic.Warning, offer.Path+".checkoutUrlKey", "Distinct offers share the same checkout destination.", "Verify that variants/bundles really select the intended purchase; query attribution alone does not select a different Stripe Price.")
			c.issues[len(c.issues)-1].Related = []diagnostic.Location{prior.Location}
		} else {
			seenCheckout[identity] = offer
		}
	}
	return c.facts, c.issues
}

func (c *checker) object(raw any, path, rule string) map[string]any {
	value, ok := raw.(map[string]any)
	if !ok {
		c.add(rule, diagnostic.Error, path, "Expected an object at "+path+".", "Use a JSON object for this declared contract; custom fields may remain alongside its documented keys.")
		return nil
	}
	return value
}

func (c *checker) text(raw any, path, rule string, maximum int) (string, bool) {
	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" || maximum > 0 && len([]rune(value)) > maximum {
		limit := ""
		if maximum > 0 {
			limit = fmt.Sprintf(" of at most %d characters", maximum)
		}
		c.add(rule, diagnostic.Error, path, "Expected nonempty text"+limit+".", "Correct the declared value at this path; omit an unused optional field instead of supplying null/empty data.")
		return "", false
	}
	return value, true
}

var alphanumeric = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func objectID(value, prefix string) bool {
	suffix, ok := strings.CutPrefix(value, prefix)
	return ok && alphanumeric.MatchString(suffix)
}

func keys[T any](values map[string]T) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
