package check

import (
	"strings"
	"testing"
)

func TestFinalCheckoutPlaceholder(t *testing.T) {
	got := runOn(t, commerceRule("commerce.checkout"), "", `<a href="https://buy.stripe.com/REPLACE_PLINK">Buy</a>`, Options{})
	if len(got) != 1 || got[0].Severity != Warning || !strings.Contains(got[0].Message, "Placeholder checkout link") {
		t.Fatalf("findings: %+v", got)
	}
	if got := runOn(t, commerceRule("commerce.checkout"), "", `<a href="https://buy.stripe.com/a1B2c3D4">Buy</a>`, Options{}); len(got) != 0 {
		t.Fatalf("real link: %+v", got)
	}
}
