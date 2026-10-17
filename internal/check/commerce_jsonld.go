package check

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/urlcheck"
	"golang.org/x/net/html"
)

type commerceRecord struct {
	value     map[string]any
	node      *html.Node
	sku       string
	ambiguous bool
	potential bool
	explicit  bool
	graph     string
	skip      bool
}

func schemaValues(value map[string]any, name string) []any {
	var values []any
	for _, prefix := range []string{"https://schema.org/", "http://schema.org/"} {
		if items, ok := value[prefix+name].([]any); ok {
			values = append(values, items...)
		}
	}
	return values
}

func schemaType(value map[string]any, name string) bool {
	types, _ := value["@type"].([]any)
	for _, kind := range types {
		if kind == "https://schema.org/"+name || kind == "http://schema.org/"+name {
			return true
		}
	}
	return false
}

func scalar(value any) any {
	if object, ok := value.(map[string]any); ok {
		if raw, exists := object["@value"]; exists {
			return raw
		}
		if id, ok := object["@id"].(string); ok {
			return id
		}
	}
	return nil
}

func schemaScalar(value map[string]any, name string) any {
	items := schemaValues(value, name)
	if len(items) == 1 {
		return scalar(items[0])
	}
	return nil
}

func schemaText(value map[string]any, name string) string {
	text, _ := schemaScalar(value, name).(string)
	return text
}

// Records are derived from expanded IRIs, not compact field spellings. Parent
// identity is propagated only through actual offer/action/item relationships.
// @id references are resolved within each script; ambiguous identity is skipped.
func commerceRecords(blocks []ldBlock) []*commerceRecord {
	var all []*commerceRecord
	for _, block := range blocks {
		var records []*commerceRecord
		byID := map[string][]*commerceRecord{}
		var walk func(any, string, bool, string)
		walk = func(raw any, inherited string, potential bool, graph string) {
			switch value := raw.(type) {
			case []any:
				for _, item := range value {
					walk(item, inherited, potential, graph)
				}
			case map[string]any:
				sku := schemaText(value, "sku")
				explicit := sku != ""
				if sku == "" {
					sku = inherited
				}
				record := &commerceRecord{value: value, node: block.node, sku: sku, potential: potential, explicit: explicit, graph: graph}
				records = append(records, record)
				if id, ok := value["@id"].(string); ok && len(value) > 1 {
					byID[graph+"\x00"+id] = append(byID[graph+"\x00"+id], record)
				}
				names := make([]string, 0, len(value))
				for name := range value {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					childSKU := ""
					local := strings.TrimPrefix(strings.TrimPrefix(name, "https://schema.org/"), "http://schema.org/")
					if local == "offers" || local == "potentialAction" || local == "itemOffered" {
						childSKU = sku
					}
					childGraph := graph
					if name == "@graph" {
						if id, ok := value["@id"].(string); ok {
							childGraph = id
						}
					}
					walk(value[name], childSKU, local == "potentialAction", childGraph)
				}
			}
		}
		walk(block.data, "", false, "")
		// JSON-LD allows one node's properties to be split across objects. Coalesce
		// those facts into a private view; repeated/conflicting scalar values stay
		// multiple values, so agreement checks cannot silently select one price.
		for _, group := range byID {
			if len(group) < 2 {
				continue
			}
			merged := map[string]any{}
			seen := map[string]map[string]bool{}
			for i, record := range group {
				for name, raw := range record.value {
					if name == "@id" {
						merged[name] = raw
						continue
					}
					values, array := raw.([]any)
					if !array {
						merged[name] = raw
						continue
					}
					if seen[name] == nil {
						seen[name] = map[string]bool{}
					}
					for _, value := range values {
						encoded, _ := json.Marshal(value)
						if !seen[name][string(encoded)] {
							prior, _ := merged[name].([]any)
							merged[name] = append(prior, value)
							seen[name][string(encoded)] = true
						}
					}
				}
				if i > 0 {
					record.skip = true
				}
			}
			for _, record := range group {
				record.value = merged
				if sku := schemaText(merged, "sku"); sku != "" {
					record.sku, record.explicit = sku, true
				}
				if len(schemaValues(merged, "sku")) > 1 {
					record.sku, record.ambiguous = "", true
				}
			}
		}
		merge := func(target *commerceRecord, sku string) {
			if sku == "" || target.ambiguous || target.explicit {
				return
			}
			if target.sku != "" && target.sku != sku {
				target.sku, target.ambiguous = "", true
			} else {
				target.sku = sku
			}
		}
		for range 2 {
			for _, record := range records {
				for _, property := range []string{"offers", "potentialAction", "itemOffered", "object"} {
					for _, raw := range schemaValues(record.value, property) {
						child, ok := raw.(map[string]any)
						if !ok {
							continue
						}
						id, _ := child["@id"].(string)
						if property == "offers" || property == "potentialAction" {
							for _, target := range byID[record.graph+"\x00"+id] {
								merge(target, record.sku)
								if property == "potentialAction" {
									target.potential = true
								}
							}
						} else if schemaType(record.value, "Offer") && property == "itemOffered" || schemaType(record.value, "BuyAction") && property == "object" {
							merge(record, schemaText(child, "sku"))
							for _, target := range byID[record.graph+"\x00"+id] {
								if !target.ambiguous {
									merge(record, target.sku)
								}
							}
						}
					}
				}
			}
		}
		for _, record := range records {
			if !record.skip {
				all = append(all, record)
			}
		}
	}
	return all
}

func actionTargets(value map[string]any) []string {
	var targets []string
	for _, target := range schemaValues(value, "target") {
		if text, ok := scalar(target).(string); ok {
			targets = append(targets, text)
			continue
		}
		if entry, ok := target.(map[string]any); ok {
			for _, template := range schemaValues(entry, "urlTemplate") {
				if text, ok := scalar(template).(string); ok {
					targets = append(targets, text)
				}
			}
		}
	}
	return targets
}

type commerceMatcher struct {
	offers     []commercecheck.Offer
	skus, urls map[string]int
}

func newCommerceMatcher(offers []commercecheck.Offer) commerceMatcher {
	m := commerceMatcher{offers: offers, skus: map[string]int{}, urls: map[string]int{}}
	add := func(index map[string]int, key string, at int) {
		if key == "" {
			return
		}
		if _, exists := index[key]; exists {
			index[key] = -1
		} else {
			index[key] = at
		}
	}
	for i, offer := range offers {
		add(m.skus, offer.SKU, i)
		add(m.urls, commercecheck.Identity(offer.CheckoutURL), i)
	}
	return m
}

func (m commerceMatcher) match(record *commerceRecord) int {
	if record.ambiguous {
		return -1
	}
	if record.sku != "" {
		if at, exists := m.skus[record.sku]; exists {
			return at
		}
		return -1
	}
	match := -1
	for _, target := range append(actionTargets(record.value), schemaText(record.value, "url")) {
		if at, exists := m.urls[commercecheck.Identity(target)]; exists {
			if at < 0 || match >= 0 && match != at {
				return -1
			}
			match = at
		}
	}
	return match
}

func (d *Document) checkOfferRecord(record *commerceRecord, matcher commerceMatcher, matched map[int]bool) {
	var offer *commercecheck.Offer
	if index := matcher.match(record); index >= 0 {
		offer = &matcher.offers[index]
		matched[index] = true
	}
	price := schemaScalar(record.value, "price")
	code, _ := schemaScalar(record.value, "priceCurrency").(string)
	hasPrice, hasCode := len(schemaValues(record.value, "price")) > 0, len(schemaValues(record.value, "priceCurrency")) > 0
	validPrice := false
	add := func(level Severity, message, hint string) {
		d.commerceAdd("commerce.structured-data", record.node, level, message, hint, offer)
	}
	if hasPrice {
		if number, valid := commercecheck.Decimal(price); !valid {
			add(Warning, "Offer price is not one machine-readable decimal value.", "Use a JSON number or decimal string without symbols/grouping. Multiple prices and priceSpecification need deliberate consumer-specific review; no value is guessed.")
		} else if number.Sign() < 0 {
			add(Error, "Offer price is negative.", "Use the actual nonnegative public price; model discounts separately rather than publishing a negative product price.")
		} else {
			validPrice = true
		}
	}
	if hasCode && strings.TrimSpace(code) == "" {
		add(Warning, "Offer priceCurrency is empty, non-text or ambiguous.", "Use the currency code corresponding to this offer's price; keep alternative currencies as distinct offers.")
	}
	if offer != nil && offer.Price != nil {
		if !hasPrice || !hasCode {
			add(Warning, "Declared fixed-price Offer lacks price or priceCurrency.", "Render both from the same public config snapshot; do not invent a price for a quote/variable offer.")
		} else if code != "" && !strings.EqualFold(code, offer.Price.Currency) {
			add(Warning, "Structured offer currency differs from its configured price snapshot.", "Verify a deliberately localized price/variant; otherwise derive priceCurrency from the same config price. A different currency is not automatically converted.")
		} else if code != "" && validPrice {
			if equal, comparable := offer.Price.Matches(price); comparable && !equal {
				add(Error, "Structured offer price disagrees with its configured minor-unit amount.", "Derive the JSON-LD decimal from the same published price snapshot, using Stripe's charge currency units and the same tax/billing basis; do not duplicate a stale display price.")
			}
		}
	} else if hasPrice != hasCode {
		add(Warning, "Structured Offer declares only one of price and priceCurrency.", "Supply a coherent price/currency pair when publishing a price. Quote-only offers can omit both.")
	}
}

func (d *Document) checkBuyRecord(record *commerceRecord, matcher commerceMatcher) {
	var offer *commercecheck.Offer
	if index := matcher.match(record); index >= 0 {
		offer = &matcher.offers[index]
	}
	if !record.potential && offer == nil {
		return
	} // past BuyAction events need no execution target
	targets := actionTargets(record.value)
	if len(targets) == 0 {
		d.commerceAdd("commerce.structured-data", record.node, Warning, "Potential BuyAction has no usable target or EntryPoint urlTemplate.", "Point the action at its configured hosted checkout; a Product/Price/Payment Link ID alone cannot be navigated.", offer)
	}
	for _, target := range targets {
		if urlcheck.Scheme(target) != "https" || !publicURL(strings.SplitN(target, "#", 2)[0]) {
			d.commerceAdd("commerce.structured-data", record.node, Error, "BuyAction target is not a public absolute HTTPS destination.", "Use a hosted checkout URL or an HTTPS EntryPoint urlTemplate without credentials. Keep public product identity separate from checkout.", offer)
		} else if offer != nil && offer.CheckoutURL != "" && !strings.ContainsAny(target, "{}") && commercecheck.Identity(target) != commercecheck.Identity(offer.CheckoutURL) {
			d.commerceAdd("commerce.structured-data", record.node, Warning, "BuyAction target differs from the identified offer's configured checkout.", "Use that offer's named URL or verify an intentional alias/permalink; do not send a product/variant to another offer's checkout.", offer)
		}
	}
}
