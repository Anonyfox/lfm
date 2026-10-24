package commercecheck

import (
	"strings"
	"testing"
)

func TestCheckoutPlaceholder(t *testing.T) {
	for _, raw := range []string{"https://buy.stripe.com/REPLACE_PLINK", "https://buy.stripe.com/todo", "https://buy.stripe.com/XXX", "https://buy.stripe.com/placeholder", "https://buy.stripe.com/YOUR_LINK", "https://buy.stripe.com/changeme", "https://example.com/pay", "https://example.org/pay"} {
		got := Checkout(raw)
		if len(got) != 1 || got[0].Error || !strings.Contains(got[0].Message, "Placeholder checkout link") || !strings.Contains(got[0].Hint, "Stripe") {
			t.Errorf("%s: %+v", raw, got)
		}
	}
	for _, raw := range []string{"https://buy.stripe.com/a1B2c3D4", "https://buy.stripe.com/abXXXcd"} {
		if got := Checkout(raw); len(got) != 0 {
			t.Errorf("%s: %+v", raw, got)
		}
	}
}
