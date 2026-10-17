package compile

import (
	"strings"
	"testing"
)

func commerceProject(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["urls"] = map[string]any{"buy": "https://buy.stripe.com/ABC123"}
		c["data"].(map[string]any)["commerce"] = map[string]any{"offers": map[string]any{"audit": map[string]any{"name": "Audit", "sku": "lab-audit", "checkoutUrlKey": "buy", "price": map[string]any{"amountMinor": 4900, "currency": "EUR"}, "stripe": map[string]any{"productId": "prod_ABC", "priceId": "price_ABC", "paymentLinkId": "plink_ABC", "customFields": []any{}}}}}
		c["jsonld"] = map[string]any{"@context": "https://schema.org", "@type": "Service", "name": "{{.data.commerce.offers.audit.name}}", "sku": "{{.data.commerce.offers.audit.sku}}", "offers": map[string]any{"@type": "Offer", "price": "49.00", "priceCurrency": "{{.data.commerce.offers.audit.price.currency}}"}, "potentialAction": map[string]any{"@type": "BuyAction", "target": "{{.urls.buy}}"}}
	})
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<a href="{{.urls.buy}}">{{.data.commerce.offers.audit.name}} · €49</a></body>`, 1))
	return root
}

func TestCommercePreflightFinalGateAndRuleControls(t *testing.T) {
	root := commerceProject(t)
	r, err := Compile(root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	for _, issue := range r.Validation.Diagnostics {
		if strings.HasPrefix(issue.Rule, "commerce.") || issue.Rule == "template.unused" && strings.Contains(issue.Path, "commerce") {
			t.Fatal(issue)
		}
	}
	before := read(t, root, "output/index.html")
	changeConfig(t, root, func(c map[string]any) { c["jsonld"].(map[string]any)["offers"].(map[string]any)["price"] = "4900" })
	r, err = Compile(root, Options{Publish: true})
	if err == nil || r.Published || read(t, root, "output/index.html") != before {
		t.Fatal("price mismatch published", err, r.Validation)
	}
	found := false
	for _, issue := range r.Validation.Diagnostics {
		if issue.Rule == "commerce.structured-data" && issue.Severity == "error" {
			found = true
			if issue.Location.File != "index.html" || len(issue.Related) < 2 {
				t.Fatal(issue)
			}
		}
	}
	if !found {
		t.Fatal(r.Validation)
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"commerce.structured-data": "warn"}}
	})
	if r, err = Compile(root, Options{Publish: true}); err != nil || !r.Published {
		t.Fatal("override ignored", err, r.Validation)
	}
	before = read(t, root, "output/index.html")
	changeConfig(t, root, func(c map[string]any) {
		offer := c["data"].(map[string]any)["commerce"].(map[string]any)["offers"].(map[string]any)["audit"].(map[string]any)
		offer["checkoutUrlKey"] = "missing"
		offer["price"].(map[string]any)["amountMinor"] = -1
	})
	r, err = Compile(root, Options{Publish: true})
	if err == nil || r.Published || r.Validation.Stage != "template" || r.Validation.Warnings != 0 || read(t, root, "output/index.html") != before {
		t.Fatal(err, r.Validation)
	}
	for _, rule := range []string{"commerce.config", "commerce.price"} {
		found := false
		for _, issue := range r.Validation.Diagnostics {
			if issue.Rule == rule && issue.Severity == "error" {
				found = true
				if issue.Location.File != "lfm.json" || issue.Path == "" || issue.Hint == "" {
					t.Fatal(issue)
				}
			}
		}
		if !found {
			t.Fatal(rule, r.Validation)
		}
	}
	changeConfig(t, root, func(c map[string]any) {
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"commerce.config": "off", "commerce.price": "off", "commerce.structured-data": "off"}}
	})
	if r, err = Compile(root, Options{Publish: true}); err != nil || !r.Published {
		t.Fatal("config override failed", err, r.Validation)
	}
}

func TestCommerceDatasetLocationsAndSecretGate(t *testing.T) {
	root := commerceProject(t)
	if _, err := Compile(root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	changeConfig(t, root, func(c map[string]any) { delete(c["data"].(map[string]any), "commerce") })
	write(t, root, "data/commerce.json", `{"offers":{"audit":{"name":"Audit","sku":"lab-audit","checkoutUrlKey":"absent","price":{"amountMinor":-1,"currency":"EUR"}}}}`)
	r, err := Compile(root, Options{Publish: true})
	if err == nil || r.Published || read(t, root, "output/index.html") != before {
		t.Fatal(err)
	}
	for _, issue := range r.Validation.Diagnostics {
		if strings.HasPrefix(issue.Rule, "commerce.") && issue.Location.File != "data/commerce.json" {
			t.Fatal(issue)
		}
	}
	root = fixture(t)
	if _, err := Compile(root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	before = read(t, root, "output/index.html")
	secret := "sk_test_" + strings.Repeat("x", 24)
	changeConfig(t, root, func(c map[string]any) { c["data"].(map[string]any)["privateKey"] = secret })
	r, err = Compile(root, Options{Publish: true})
	if err == nil || r.Published || read(t, root, "output/index.html") != before {
		t.Fatal("secret passed preflight", err)
	}
	for _, issue := range r.Validation.Diagnostics {
		if strings.Contains(issue.String(), secret) {
			t.Fatal("secret disclosed")
		}
	}
}
