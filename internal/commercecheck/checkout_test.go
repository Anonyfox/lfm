package commercecheck

import (
	"strings"
	"testing"
)

func TestCheckoutVariantsAndParameters(t *testing.T) {
	for _, raw := range []string{
		"https://buy.stripe.com/ABC123", "https://book.stripe.com/ABC123", "https://donate.stripe.com/ABC123",
		"https://payments.example.com/custom-stripe-route?future=allowed",
		"https://buy.stripe.com/ABC123?utm_source=my_campaign&client_reference_id=order_123&prefilled_email=fox%40example.com&prefilled_promo_code=SAVE10&locale=de&prefilled_amount=4900",
	} {
		if issues := Checkout(raw); len(issues) != 0 {
			t.Fatal(raw, issues)
		}
	}
	for _, tc := range []struct {
		raw, fragment string
		hard          bool
	}{
		{"http://buy.stripe.com/ABC", "absolute HTTPS", true},
		{"price_ABC", "absolute HTTPS", true},
		{"https://user:pass@buy.stripe.com/ABC", "credentials", true},
		{"https://api.stripe.com/v1/payment_links", "management/API", true},
		{"https://127.1/checkout", "public browser", true},
		{"https://10.0.0.1/checkout", "public browser", true},
		{"https://shop.local/checkout", "public browser", true},
		{"https://buy.stripe.com/test_ABC", "test-mode", false},
		{"https://checkout.stripe.com/c/pay/cs_live_ABC", "Session URL", false},
		{"https://buy.stripe.com/plink_ABC", "path", false},
		{"https://buy.stripe.com/ABC?utm_source=bad%20value", "discarded", false},
		{"https://buy.stripe.com/ABC?client_reference_id=" + strings.Repeat("a", 201), "discarded", false},
		{"https://buy.stripe.com/ABC?custom_fields%5B0%5D=private-answer", "not in the documented", false},
		{"https://buy.stripe.com/ABC?prefilled_email=bad", "email prefill", false},
		{"https://buy.stripe.com/ABC?prefilled_email=a%40b.com&locked_prefilled_email=x%40y.com", "Both editable", false},
		{"https://buy.stripe.com/ABC?prefilled_promo_code=BAD-CODE", "not alphanumeric", false},
		{"https://buy.stripe.com/ABC?prefilled_amount=49.00", "integer minor", false},
		{"https://buy.stripe.com/ABC?locale=", "locale is empty", false},
		{"https://buy.stripe.com/ABC?locale=German", "language tag", false},
		{"https://buy.stripe.com/ABC?utm_source=a&utm_source=b", "repeats", false},
		{"https://buy.stripe.com/ABC?x=%ZZ", "unambiguously", true},
		{"https://buy.stripe.com/ABC#value", "fragment", false},
	} {
		found := false
		for _, issue := range Checkout(tc.raw) {
			if strings.Contains(issue.Message, tc.fragment) && issue.Error == tc.hard {
				found = true
			}
		}
		if !found {
			t.Fatal(tc, Checkout(tc.raw))
		}
	}
	private := "private-answer"
	for _, issue := range Checkout("https://buy.stripe.com/ABC?custom_fields%5B0%5D=" + private) {
		if strings.Contains(issue.Message+issue.Hint, private) {
			t.Fatal("answer leaked", issue)
		}
	}
	if Identity("https://buy.stripe.com/test_ABC") == Identity("https://buy.stripe.com/ABC") || Identity("https://buy.stripe.com/ABC") == Identity("https://buy.stripe.com/abc") || Identity("https://buy.stripe.com/ABC/") == Identity("https://buy.stripe.com/ABC") {
		t.Fatal("distinct paths collapsed")
	}
}
