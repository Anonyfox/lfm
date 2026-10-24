package commercecheck

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var secretShape = regexp.MustCompile(`(?:sk_live_|sk_test_|rk_live_|rk_test_|whsec_)[A-Za-z0-9]{16}`)

// Property: SecretOffset is the leftmost start of any Stripe secret-key shape.
func FuzzSecretOffset(f *testing.F) {
	key := strings.Repeat("a", 24)
	for _, seed := range []string{"sk_test_" + key, "sk_test_short sk_live_" + key, "sk_live_sk_live_" + key, "whsec_whsec_" + key, "pk_live_" + key, "rk_test_" + key[:15]} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		want := -1
		if at := secretShape.FindStringIndex(raw); at != nil {
			want = at[0]
		}
		if got := SecretOffset([]byte(raw)); got != want {
			t.Fatalf("SecretOffset(%q) = %d, want %d", raw, got, want)
		}
	})
}

// Property: Decimal accepts a decimal the same way as text or json.Number, and
// every accepted value is a bounded exact rational.
func FuzzDecimal(f *testing.F) {
	for _, seed := range []string{"49.00", "4.9e1", "-0", "1e18", "1e19", "1e-18", "1/2", "1,000", "NaN", "+1.5E+3", "0x10", strings.Repeat("9", 129)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		fromText, ok := Decimal(text)
		fromNumber, okNumber := Decimal(json.Number(text))
		if ok != okNumber || ok && fromText.Cmp(fromNumber) != 0 {
			t.Fatalf("%q: text %v (%v), json.Number %v (%v)", text, fromText, ok, fromNumber, okNumber)
		}
		if !ok {
			return
		}
		if len(text) > 128 || fromText.Num().BitLen() > 1024 || fromText.Denom().BitLen() > 1024 {
			t.Fatalf("%q decoded to an unbounded value", text)
		}
	})
}

// Property: Identity is idempotent, ignores query and fragment, and never
// returns either.
func FuzzIdentity(f *testing.F) {
	for _, seed := range []string{"https://buy.stripe.com/test_ABC", "HTTPS://Buy.Stripe.com/ABC/", "https://buy.stripe.com/a%2Fb", "http://x/", "https://u@x/", "https://x/a b", "https://[::1]:443/x"} {
		f.Add(seed, "q=1", "frag")
	}
	f.Fuzz(func(t *testing.T, raw, query, fragment string) {
		id := Identity(raw)
		if id == "" {
			return
		}
		if Identity(id) != id || strings.ContainsAny(id, "?#") {
			t.Fatalf("Identity(%q) = %q is not a fixed point", raw, id)
		}
		if !strings.ContainsAny(raw, "?#") {
			if with := Identity(raw + "?" + url.PathEscape(query) + "#" + url.PathEscape(fragment)); with != id {
				t.Fatalf("query/fragment changed identity: %q vs %q", with, id)
			}
		}
	})
}

// Property: Checkout findings are complete and never echo query values or
// secret-shaped text from the URL.
func FuzzCheckout(f *testing.F) {
	for _, seed := range []string{"https://buy.stripe.com/REPLACE_PLINK", "https://example.org/pay", "https://buy.stripe.com/YOUR_LINK", "https://buy.stripe.com/ABC123?utm_source=x&prefilled_email=fox%40example.com", "http://buy.stripe.com/ABC", "https://checkout.stripe.com/c/pay/cs_live_ABC", "https://buy.stripe.com/ABC?x=%ZZ", "https://10.0.0.1/x#y", "https://buy.stripe.com/test_ABC?locale=de&locale=en"} {
		f.Add(seed, "custom_fields[0]", "private-answer")
	}
	f.Fuzz(func(t *testing.T, raw, name, value string) {
		if len(raw)+len(name)+len(value) > 4096 {
			t.Skip()
		}
		for _, issue := range Checkout(raw) {
			if issue.Message == "" || issue.Hint == "" || Secret(issue.Message+" "+issue.Hint) {
				t.Fatalf("bad finding %+v", issue)
			}
		}
		marker := "Zq9" + value + "Zq9"
		for _, issue := range Checkout("https://buy.stripe.com/ABC?" + url.QueryEscape(name) + "=" + url.QueryEscape(marker)) {
			if strings.Contains(issue.Message+issue.Hint, "Zq9") {
				t.Fatalf("query value leaked: %+v", issue)
			}
		}
	})
}
