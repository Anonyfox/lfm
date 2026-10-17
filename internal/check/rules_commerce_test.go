package check

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/commercecheck"
)

func commerceFixture() Options {
	return Options{URL: "https://example.com/", Commerce: commercecheck.Facts{Offers: []commercecheck.Offer{{Path: "data.commerce.offers.audit", SKU: "lab-audit", Name: "Audit", CheckoutURL: "https://buy.stripe.com/ABC", Price: &commercecheck.Money{AmountMinor: 4900, Currency: "EUR", Scale: 100}, Location: Location{File: "lfm.json", Line: 10, Column: 1}}}}}
}

const commerceAnchor = `<a href="https://buy.stripe.com/ABC">Buy the audit · €49</a>`
const commerceLD = `{"@context":"https://schema.org","@type":"Product","sku":"lab-audit","name":"Audit","offers":{"@type":"Offer","price":"49.00","priceCurrency":"EUR"},"potentialAction":{"@type":"BuyAction","target":{"@type":"EntryPoint","urlTemplate":"https://buy.stripe.com/ABC"}}}`

func commerceDoc(t *testing.T, content, data string, options Options) *Document {
	t.Helper()
	raw := `<!doctype html><html><head><link rel=canonical href="https://example.com/"></head><body>` + content
	if data != "" {
		raw += `<script type="application/ld+json">` + data + `</script>`
	}
	raw += `</body></html>`
	d, err := Parse([]byte(raw), options)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func commerceFindings(d *Document, id string) []Diagnostic {
	return Run(d, []Rule{{ID: id, Check: commerceRule(id)}}, nil).Diagnostics
}

func TestCommerceGraphAliasesReferencesAndPrecision(t *testing.T) {
	for _, data := range []string{
		commerceLD,
		strings.Replace(commerceLD, `"@context":"https://schema.org"`, `"@context":{"@vocab":"https://schema.org/","cost":"price","code":"sku"}`, 1),
		`{"@context":"https://schema.org","@graph":[{"@id":"#product","@type":"Product","sku":"lab-audit","offers":{"@id":"#offer"},"potentialAction":{"@id":"#buy"}},{"@id":"#offer","@type":"Offer","price":49,"priceCurrency":"eur"},{"@id":"#buy","@type":"BuyAction","object":{"@id":"#product"},"target":"https://buy.stripe.com/ABC"}]}`,
		`{"@context":"http://schema.org/","@type":"Offer","price":49.0,"priceCurrency":"EUR","itemOffered":{"@type":"Service","sku":"lab-audit"}}`,
		`{"@context":"https://schema.org","@type":"Offer","price":"49.00","priceCurrency":"EUR","url":"https://buy.stripe.com/ABC"}`,
	} {
		data = strings.ReplaceAll(strings.ReplaceAll(data, `"price":`, `"cost":`), `"sku":`, `"code":`)
		if !strings.Contains(data, `"cost":"price"`) {
			data = strings.ReplaceAll(strings.ReplaceAll(data, `"cost":`, `"price":`), `"code":`, `"sku":`)
		}
		d := commerceDoc(t, commerceAnchor, data, commerceFixture())
		before := bytes.Clone(d.Raw)
		for _, id := range []string{"commerce.checkout", "commerce.discovery", "commerce.structured-data"} {
			if issues := commerceFindings(d, id); len(issues) != 0 {
				t.Fatal(data, id, issues)
			}
		}
		if !bytes.Equal(before, d.Raw) {
			t.Fatal("modified final bytes")
		}
		first := d.commerceFacts()
		if !reflect.DeepEqual(first, d.commerceFacts()) {
			t.Fatal("unstable cache")
		}
	}
	for _, tc := range []struct {
		from, to, message string
		level             Severity
	}{
		{`"49.00"`, `"0.49"`, "disagrees", Error},
		{`"49.00"`, `-1`, "negative", Error},
		{`"49.00"`, `"49,00"`, "machine-readable", Warning},
		{`"EUR"`, `"USD"`, "currency differs", Warning},
		{`"https://buy.stripe.com/ABC"`, `"http://buy.stripe.com/ABC"`, "public absolute HTTPS", Error},
		{`"https://buy.stripe.com/ABC"`, `"https://buy.stripe.com/XYZ"`, "differs", Warning},
	} {
		d := commerceDoc(t, commerceAnchor, strings.Replace(commerceLD, tc.from, tc.to, 1), commerceFixture())
		issues := commerceFindings(d, "commerce.structured-data")
		if !hasMessage(issues, tc.level, tc.message) {
			t.Fatal(tc, issues)
		}
		for _, issue := range issues {
			if issue.Location.File != "index.html" || issue.Hint == "" || len(issue.Related) == 0 {
				t.Fatal(issue)
			}
		}
	}
}

func TestCommerceIdentityScopeAndUncertainData(t *testing.T) {
	// A parent's family SKU must not override an explicit variant SKU.
	data := `{"@context":"https://schema.org","@type":"Product","sku":"family","offers":{"@type":"Offer","sku":"lab-audit","price":49,"priceCurrency":"EUR"}}`
	d := commerceDoc(t, commerceAnchor, data, commerceFixture())
	if issues := commerceFindings(d, "commerce.discovery"); len(issues) != 0 {
		t.Fatal(issues)
	}
	// Split declarations of the same ID are one semantic node, not incomplete
	// offers or independently selected prices. Conflicting prices stay ambiguous.
	for _, extra := range []string{`"description":"Same node"`, `"price":"49.00"`, `"price":"99.00"`} {
		data := `{"@context":"https://schema.org","@graph":[{"@id":"#offer","@type":"Offer","sku":"lab-audit"},{"@id":"#offer","price":"49.00","priceCurrency":"EUR"},{"@id":"#offer",` + extra + `}]}`
		d := commerceDoc(t, commerceAnchor, data, commerceFixture())
		issues := commerceFindings(d, "commerce.structured-data")
		if hasMessage(issues, Error, "disagrees") || hasMessage(issues, Warning, "lacks price") {
			t.Fatal("split ID facts guessed", issues)
		}
		if extra == `"price":"99.00"` && !hasMessage(issues, Warning, "machine-readable") {
			t.Fatal("ambiguous price was hidden", issues)
		}
	}
	// Same @id in different named graphs must not leak the product identity.
	data = `{"@context":"https://schema.org","@graph":[{"@id":"#one","@graph":[{"@type":"Product","sku":"lab-audit","offers":{"@id":"#offer"}},{"@id":"#offer","@type":"Offer","price":49,"priceCurrency":"EUR"}]},{"@id":"#two","@graph":[{"@id":"#offer","@type":"Offer","price":999,"priceCurrency":"EUR"}]}]}`
	d = commerceDoc(t, commerceAnchor, data, commerceFixture())
	if issues := commerceFindings(d, "commerce.structured-data"); len(issues) != 0 {
		t.Fatal("named graphs conflated", issues)
	}
	for _, data := range []string{
		`{"@context":"https://schema.org","@type":"Offer","name":"Quote on request"}`,
		`{"@context":"https://schema.org","@type":"BuyAction","actionStatus":"CompletedActionStatus"}`,
		`{"@context":"https://other.example/context","@type":"Offer","price":-1}`,
		`{"@context":{"@vocab":"https://other.example/"},"@type":"Offer","price":-1}`,
	} {
		d := commerceDoc(t, "", data, Options{})
		if issues := commerceFindings(d, "commerce.structured-data"); len(issues) != 0 {
			t.Fatal(data, issues)
		}
	}
	d = commerceDoc(t, commerceAnchor, `{"@context":"https://schema.org","@type":"Product","sku":"lab-audit","potentialAction":{"@type":"BuyAction"}}`, commerceFixture())
	if issues := commerceFindings(d, "commerce.structured-data"); !hasMessage(issues, Warning, "no usable target") {
		t.Fatal(issues)
	}
	// Inert examples are not live offers/actions.
	d = commerceDoc(t, `<template><script type="application/ld+json">`+strings.Replace(commerceLD, `"49.00"`, `-1`, 1)+`</script></template>`, "", Options{})
	if issues := commerceFindings(d, "commerce.structured-data"); len(issues) != 0 {
		t.Fatal(issues)
	}
}

func TestCheckoutDiscoveryUsesInitialNativeAnchors(t *testing.T) {
	for _, tc := range []struct {
		content string
		missing bool
	}{
		{commerceAnchor, false},
		{`<div hidden>` + commerceAnchor + `</div>`, true},
		{`<div inert>` + commerceAnchor + `</div>`, true},
		{`<div aria-hidden=true>` + commerceAnchor + `</div>`, true},
		{`<dialog>` + commerceAnchor + `</dialog>`, true},
		{`<dialog open>` + commerceAnchor + `</dialog>`, false},
		{`<details><summary>Buy</summary>` + commerceAnchor + `</details>`, true},
		{`<details open><summary>Buy</summary>` + commerceAnchor + `</details>`, false},
		{`<template>` + commerceAnchor + `</template>`, true},
		{`<button type=button onclick="location.href='https://buy.stripe.com/ABC'">Buy</button>`, true},
		{`<a aria-disabled=true href="https://buy.stripe.com/ABC">Buy</a>`, true},
		{commerceAnchor + commerceAnchor, false},
	} {
		d := commerceDoc(t, tc.content, commerceLD, commerceFixture())
		issues := commerceFindings(d, "commerce.discovery")
		if hasMessage(issues, Warning, "native checkout anchor") != tc.missing {
			t.Fatal(tc, issues)
		}
	}
	for _, body := range []string{`<form action="https://buy.stripe.com/ABC"><button>Buy</button></form>`, `<a download href="https://buy.stripe.com/ABC">Buy</a>`, `<a href="https://buy.stripe.com/test_ABC?custom_fields%5B0%5D=brief">Buy</a>`} {
		d := commerceDoc(t, body, "", Options{})
		if issues := commerceFindings(d, "commerce.checkout"); len(issues) == 0 {
			t.Fatal(body)
		}
	}
	d := commerceDoc(t, `<a href="https://payments.example.com/a">Buy</a>`, strings.Replace(commerceLD, "https://buy.stripe.com/ABC", "https://payments.example.com/a", 1), commerceFixture())
	d.Options.Commerce.Offers[0].CheckoutURL = "https://payments.example.com/a"
	if issues := commerceFindings(d, "commerce.discovery"); len(issues) != 0 {
		t.Fatal(issues)
	}
}

func TestSecretDetectionInExactArtifactAndTextSidecars(t *testing.T) {
	secret := "sk_test_" + strings.Repeat("x", 24)
	d := commerceDoc(t, `<script>const key="`+secret+`"</script>`, "", Options{Assets: map[string][]byte{"bundle.js": []byte(secret), "image.webp": []byte(secret)}})
	issues := commerceSecretsRule(d)
	if len(issues) != 2 || issues[1].Location.File != "bundle.js" {
		t.Fatal(issues)
	}
	for _, issue := range issues {
		if strings.Contains(issue.String(), secret) || issue.Severity != Error {
			t.Fatal(issue)
		}
	}
}

func BenchmarkAnalyzeCommerce(b *testing.B) {
	raw := []byte(`<!doctype html><html><head><link rel=canonical href="https://example.com/"></head><body>` + commerceAnchor + `<script type="application/ld+json">` + commerceLD + `</script></body></html>`)
	options := commerceFixture()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Analyze(raw, options)
	}
}
