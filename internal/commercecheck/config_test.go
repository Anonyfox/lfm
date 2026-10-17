package commercecheck

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

func checkConfig(t *testing.T, raw string) (Facts, []diagnostic.Diagnostic) {
	t.Helper()
	var values map[string]any
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&values); err != nil {
		t.Fatal(err)
	}
	sources := jsonsrc.Index(map[string][]byte{"lfm.json": []byte(raw)})
	data, _ := values["data"].(map[string]any)
	urls := map[string]string{}
	if named, ok := values["urls"].(map[string]any); ok {
		for key, value := range named {
			urls[key], _ = value.(string)
		}
	}
	facts, issues := Check(data, urls, sources)
	return facts, issues
}

const validConfig = `{"urls":{"buyAudit":"https://buy.stripe.com/ABC123"},"data":{"commerce":{"offers":{"audit":{"sku":"lab-audit","name":"Audit 🐺","checkoutUrlKey":"buyAudit","price":{"amountMinor":4900,"currency":"eur"},"stripe":{"productId":"prod_ABC123","priceId":"price_XYZ","paymentLinkId":"plink_ABC","customFields":[{"key":"address","label":{"type":"custom","custom":"Website URL"},"type":"text","text":{"minimum_length":3,"maximum_length":255,"default_value":"https://example.com"}}]},"editorial":{"anything":true}}}}}}`

func found(issues []diagnostic.Diagnostic, rule string, level diagnostic.Severity, fragment string) bool {
	for _, issue := range issues {
		if issue.Rule == rule && issue.Severity == level && strings.Contains(issue.Message, fragment) {
			return true
		}
	}
	return false
}

func TestOptionalConfigAndExtensions(t *testing.T) {
	facts, issues := checkConfig(t, validConfig)
	if len(issues) != 0 || len(facts.Offers) != 1 || facts.Offers[0].Price.Currency != "EUR" {
		t.Fatal(facts, issues)
	}
	if !facts.CheckedCheckout["https://buy.stripe.com/ABC123"] || len(facts.Consumed) < 7 {
		t.Fatal(facts)
	}
	for _, raw := range []string{`{}`, `{"data":{"commerce":false}}`, `{"data":{"commerce":"custom copy"}}`, `{"data":{"commerce":[]}}`, `{"data":{"commerce":{"custom":42}}}`, `{"data":{"commerce":{"offers":{}}}}`} {
		if facts, issues := checkConfig(t, raw); len(facts.Offers) != 0 || len(issues) != 0 {
			t.Fatal(raw, facts, issues)
		}
	}
	for _, replacement := range []string{`"sku":"独自 /:, %~"`, `"sku":"different casing"`} {
		if _, issues := checkConfig(t, strings.Replace(validConfig, `"sku":"lab-audit"`, replacement, 1)); len(issues) != 0 {
			t.Fatal(issues)
		}
	}
	for _, offer := range []string{`{"name":"Quote","sku":"quote","checkoutUrlKey":"buyAudit"}`, `{"extension":true}`} {
		_, issues := checkConfig(t, `{"urls":{"buyAudit":"https://checkout.example.com/my-link"},"data":{"commerce":{"offers":{"draft":`+offer+`}}}}`)
		for _, issue := range issues {
			if issue.Severity == diagnostic.Error {
				t.Fatal(issues)
			}
		}
	}
}

func TestConfigFailuresAggregateAndLocate(t *testing.T) {
	for _, tc := range []struct {
		from, to, rule, message string
		severity                diagnostic.Severity
	}{
		{`"checkoutUrlKey":"buyAudit"`, `"checkoutUrlKey":"missing"`, "commerce.config", "does not name", diagnostic.Error},
		{`"amountMinor":4900`, `"amountMinor":49.5`, "commerce.price", "exact JSON integer", diagnostic.Error},
		{`"amountMinor":4900`, `"amountMinor":-1`, "commerce.price", "nonnegative", diagnostic.Error},
		{`"amountMinor":4900`, `"amountMinor":"4900"`, "commerce.price", "JSON integer", diagnostic.Error},
		{`"amountMinor":4900`, `"amountMinor":9007199254740992`, "commerce.price", "safe numeric", diagnostic.Error},
		{`"currency":"eur"`, `"currency":"€"`, "commerce.price", "three-letter", diagnostic.Error},
		{`"currency":"eur"`, `"currency":"ZZZ"`, "commerce.price", "precision", diagnostic.Warning},
		{`"priceId":"price_XYZ"`, `"priceId":"prod_XYZ"`, "commerce.config", "expected price_", diagnostic.Warning},
		{`"checkoutUrlKey":"buyAudit"`, `"CheckoutUrlKey":"buyAudit"`, "commerce.config", "differs in case", diagnostic.Warning},
		{`https://buy.stripe.com/ABC123`, `https://dashboard.stripe.com/payment_links/plink_ABC`, "commerce.checkout", "management/API", diagnostic.Error},
		{`https://buy.stripe.com/ABC123`, `http://buy.stripe.com/ABC123`, "commerce.checkout", "absolute HTTPS", diagnostic.Error},
		{`https://buy.stripe.com/ABC123`, `https://buy.stripe.com/ABC123?prefilled_amount=100`, "commerce.checkout", "fixed-price", diagnostic.Warning},
	} {
		t.Run(tc.message+tc.to, func(t *testing.T) {
			_, issues := checkConfig(t, strings.Replace(validConfig, tc.from, tc.to, 1))
			if !found(issues, tc.rule, tc.severity, tc.message) {
				t.Fatal(issues)
			}
			for _, issue := range issues {
				if issue.Location.File != "lfm.json" || issue.Location.Column < 1 || issue.Path == "" || issue.Hint == "" {
					t.Fatal(issue)
				}
			}
		})
	}
	for _, raw := range []string{`{"data":{"commerce":{"offers":[]}}}`, `{"data":{"commerce":{"offers":{"a":null}}}}`} {
		_, issues := checkConfig(t, raw)
		if !found(issues, "commerce.config", diagnostic.Error, "object") {
			t.Fatal(raw, issues)
		}
	}
	_, issues := checkConfig(t, `{"data":{"commerce":{"offers":{"a":{"sku":"same","price":{"amountMinor":-1,"currency":42}},"b":{"sku":"same"}}}}}`)
	if !found(issues, "commerce.config", diagnostic.Error, "same SKU") || !found(issues, "commerce.price", diagnostic.Error, "nonnegative") || !found(issues, "commerce.price", diagnostic.Error, "three-letter") {
		t.Fatal(issues)
	}
	_, issues = checkConfig(t, `{"urls":{"a":"https://buy.stripe.com/ABC","b":"https://buy.stripe.com/ABC?utm_source=x"},"data":{"commerce":{"offers":{"a":{"sku":"a","name":"A","checkoutUrlKey":"a"},"b":{"sku":"b","name":"B","checkoutUrlKey":"b"}}}}}`)
	if !found(issues, "commerce.config", diagnostic.Warning, "share the same") {
		t.Fatal(issues)
	}
}

func TestMoneyPrecisionAndStripeExceptions(t *testing.T) {
	for _, tc := range []struct {
		code    string
		amount  int
		display string
	}{{"EUR", 4900, "49.00"}, {"JPY", 4900, "4900"}, {"MGA", 4900, "4900"}, {"ISK", 4900, "49"}, {"UGX", 4900, "49"}, {"HUF", 4900, "49"}, {"TWD", 4900, "49"}, {"KWD", 4900, "4.9"}, {"EUR", 0, "0"}} {
		raw := strings.Replace(validConfig, `"currency":"eur"`, `"currency":"`+tc.code+`"`, 1)
		if tc.amount == 0 {
			raw = strings.Replace(raw, `"amountMinor":4900`, `"amountMinor":0`, 1)
		}
		facts, issues := checkConfig(t, raw)
		if len(issues) != 0 {
			t.Fatal(tc, issues)
		}
		if equal, valid := facts.Offers[0].Price.Matches(tc.display); !equal || !valid {
			t.Fatal(tc, facts)
		}
	}
	_, issues := checkConfig(t, strings.Replace(strings.Replace(validConfig, `"currency":"eur"`, `"currency":"UGX"`, 1), `"amountMinor":4900`, `"amountMinor":49`, 1))
	if !found(issues, "commerce.price", diagnostic.Error, "divisible") {
		t.Fatal(issues)
	}
	for _, raw := range []any{"1e99999999", "1/2", "1,000", "NaN", true, []any{49}, json.Number("1e1000000")} {
		if _, valid := Decimal(raw); valid {
			t.Fatal(raw)
		}
	}
	for _, raw := range []any{"49.00", json.Number("4.9e1"), 49.0, int64(49)} {
		if value, valid := Decimal(raw); !valid || value.String() != "49/1" {
			t.Fatal(raw, value)
		}
	}
}

func TestSecretShapesAndRedaction(t *testing.T) {
	value := "sk_test_" + strings.Repeat("a", 24)
	for _, text := range []string{value, "prefix " + value, "whsec_" + strings.Repeat("b", 24), "rk_live_" + strings.Repeat("c", 24)} {
		if !Secret(text) {
			t.Fatal(text)
		}
	}
	for _, text := range []string{"pk_live_" + strings.Repeat("a", 24), "sk_test_REPLACE", "prod_" + strings.Repeat("b", 24), "customer token secret", strings.Repeat("a", 10000)} {
		if Secret(text) {
			t.Fatal(text)
		}
	}
	for _, raw := range [][]byte{[]byte(value), []byte("sk_test_short " + value)} {
		if at := SecretOffset(raw); at < 0 || !bytes.HasPrefix(raw[at:], []byte(value)) {
			t.Fatal(at)
		}
	}
	raw := []byte(`{"data":{"private":"` + value + `"}}`)
	sources := jsonsrc.Index(map[string][]byte{"lfm.json": raw})
	var values map[string]any
	json.Unmarshal(raw, &values)
	issues := ConfigSecrets(values, sources)
	if len(issues) != 1 || issues[0].Path != "data.private" || strings.Contains(issues[0].String(), value) {
		t.Fatal(issues)
	}
	before := append([]byte(nil), raw...)
	ConfigSecrets(values, sources)
	if !reflect.DeepEqual(before, raw) {
		t.Fatal("mutated input")
	}
}

func FuzzCommerceConfig(f *testing.F) {
	for _, seed := range []string{validConfig, `{"data":{"commerce":{"offers":{"x":{"price":null,"stripe":{"customFields":[null]}}}}}}`, `{}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 20000 {
			t.Skip()
		}
		var values map[string]any
		d := json.NewDecoder(strings.NewReader(raw))
		d.UseNumber()
		if d.Decode(&values) != nil {
			return
		}
		data, _ := values["data"].(map[string]any)
		_, _ = Check(data, nil, jsonsrc.Sources{})
		_ = ConfigSecrets(values, jsonsrc.Sources{})
	})
}
