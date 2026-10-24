package commercecheck

import (
	"net/mail"
	"net/url"
	"regexp"
	"strings"

	"github.com/Anonyfox/lfm/internal/urlcheck"
	"golang.org/x/text/language"
)

type URLFinding struct {
	Error         bool
	Message, Hint string
}

var trackingValue = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var hostedPath = regexp.MustCompile(`^/(?:test_)?[A-Za-z0-9]+$`)
var plainName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func StripeHost(host string) bool {
	switch strings.ToLower(host) {
	case "buy.stripe.com", "book.stripe.com", "donate.stripe.com", "checkout.stripe.com":
		return true
	}
	return false
}

// Checkout never fetches a destination or assumes that a custom domain is not
// Stripe. Parameter checks apply to confirmed standard Payment Link hosts only.
func Checkout(raw string) []URLFinding {
	var issues []URLFinding
	add := func(hard bool, message, hint string) { issues = append(issues, URLFinding{hard, message, hint}) }
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || strings.ContainsAny(raw, "\r\n\t \\") {
		return []URLFinding{{true, "Checkout destination must be an absolute HTTPS URL without credentials or whitespace.", "Reference the actual reusable hosted Stripe URL through urls; a Product/Price/Payment Link ID is not a URL."}}
	}
	host := strings.ToLower(u.Hostname())
	if urlcheck.Placeholder(raw) {
		return []URLFinding{{false, "Placeholder checkout link.", "Paste the real Payment Link URL from Stripe into the named checkout URL."}}
	}
	if !urlcheck.PublicURL(strings.SplitN(raw, "#", 2)[0]) {
		add(true, "Checkout destination is not a public browser URL.", "Use a public Stripe/custom checkout hostname; local/private addresses, malformed hosts and credentials cannot serve public buyers. DNS and live state still require deployment checks.")
	}
	if host == "api.stripe.com" || host == "dashboard.stripe.com" {
		add(true, "Stripe management/API URL used as buyer checkout.", "Use the Payment Link object's public url instead of its API/Dashboard URL.")
	}
	if u.Fragment != "" || strings.HasSuffix(raw, "#") {
		add(false, "Checkout destination has a fragment.", "Verify the hosted page understands it; fragments are not Stripe custom-field prefill and never reach the server.")
	}
	if !StripeHost(host) {
		return issues // custom Stripe domain: normal URL validation still applies
	}
	if host == "checkout.stripe.com" {
		add(false, "Checkout Session URL used as a reusable product link.", "Prefer a stable Payment Link; Session URLs can expire. Keep this only when its lifecycle is intentionally managed outside the static artifact.")
		return issues
	}
	if !hostedPath.MatchString(u.EscapedPath()) {
		add(false, "Stripe Payment Link path has an unexpected shape.", "Copy its exact public url from Stripe; don't place prod_, price_ or plink_ IDs in the URL path. A newer provider format can be retained deliberately.")
	}
	if strings.HasPrefix(u.Path, "/test_") {
		add(false, "Stripe test-mode Payment Link is present.", "Keep it for test builds; replace the named checkout URL with the live Payment Link before selling.")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		add(true, "Checkout query cannot be parsed unambiguously.", "Construct parameters with URL/URLSearchParams; fix malformed percent escapes and unescaped separators.")
		return issues
	}
	for _, name := range keys(query) {
		values := query[name]
		if len(values) != 1 {
			add(false, "Checkout repeats query parameter "+safeName(name)+".", "Keep one deliberate value; don't depend on first/last-value precedence.")
			continue
		}
		value := values[0]
		switch name {
		case "client_reference_id", "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content":
			maximum := 150
			if name == "client_reference_id" {
				maximum = 200
			}
			if len(value) > maximum || !trackingValue.MatchString(value) {
				add(false, "Checkout "+name+" may be silently discarded by Stripe.", "Use only ASCII letters, digits, underscores and hyphens; maximum 150 characters for UTMs or 200 for client_reference_id.")
			}
		case "prefilled_email", "locked_prefilled_email":
			if address, err := mail.ParseAddress(value); err != nil || address.Address != value {
				add(false, "Checkout email prefill is not a plain email address.", "Use an encoded address without a display name; locked prefill is not authentication.")
			}
		case "prefilled_promo_code":
			if !alphanumeric.MatchString(value) {
				add(false, "Checkout promotion prefill is not alphanumeric.", "Use a valid Stripe promotion code and enable allow_promotion_codes on the Payment Link.")
			}
		case "prefilled_amount":
			if value == "" || strings.ContainsFunc(value, func(r rune) bool { return r < '0' || r > '9' }) {
				add(false, "Checkout amount prefill is not an integer minor-unit amount.", "This parameter only applies to customer-chosen amounts; it cannot change a fixed Price.")
			}
		case "locale":
			if _, err := language.Parse(value); value != "auto" && (err != nil || strings.Contains(value, "_") || strings.TrimSpace(value) == "") {
				add(false, "Checkout locale is empty or not a recognized language tag.", "Use a Stripe-supported Checkout locale or omit the parameter for automatic selection; valid language syntax alone does not establish Stripe support.")
			}
		default:
			add(false, "Checkout query parameter "+safeName(name)+" is not in the documented Payment Link set.", "Verify newly supported parameters. Custom-field answers, metadata and arbitrary quiz state are not transferred through invented parameters; collect needed answers in Stripe.")
		}
	}
	if query.Has("prefilled_email") && query.Has("locked_prefilled_email") {
		add(false, "Both editable and locked email prefills are set.", "Keep the intended one; locked_prefilled_email takes precedence.")
	}
	return issues
}

// Identity deliberately excludes only query/fragment; it never equates distinct
// paths, removes trailing slashes or strips the test-mode prefix.
func Identity(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = "", "", "", false
	return u.String()
}

func safeName(value string) string {
	// Even a query name can contain a payload. Reproduce only short identifier
	// shapes without a recognizable secret; parameter values are never printed.
	if len(value) < 40 && plainName.MatchString(value) && !Secret(value) {
		return value
	}
	return "(nonstandard name)"
}
