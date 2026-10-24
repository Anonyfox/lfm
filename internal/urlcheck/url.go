// Package urlcheck supplies shared, offline URL policy for config and HTML.
// Browser URL parsing is delegated to the pure Go WHATWG implementation.
package urlcheck

import (
	"fmt"
	"net/mail"
	"net/netip"
	neturl "net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	urlerrors "github.com/nlnwa/whatwg-url/errors"
	whatwg "github.com/nlnwa/whatwg-url/url"
)

// PublicURL is a local browser-parsed public HTTP(S) reference screen. It does
// not resolve DNS or prove reachability. Fragments are excluded by this contract.
func PublicURL(value string) bool {
	r := New("").Inspect(value)
	if !r.Valid() || (r.URL.Scheme() != "http" && r.URL.Scheme() != "https") || Scheme(value) == "" || r.URL.Hostname() == "" || r.URL.Username() != "" || r.URL.Password() != "" || strings.Contains(value, "#") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(r.URL.Hostname()), ".")
	for _, suffix := range []string{"localhost", "local", "invalid", "test"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return false
		}
	}
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		ip = ip.Unmap()
		return ip.IsGlobalUnicast() && !ip.IsPrivate()
	}
	return true
}

// Catalog lists the link rules that run on authoring sources before rendering.
// links.syntax, links.declared and links.behavior also judge final HTML and are
// defined once, with the final-document rules, in check.DefaultRules.
func Catalog() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{
			ID:      "links.registry",
			Summary: "Named URLs in lfm.json urls have usable names and no duplicate destinations",
			Help:    "Triggered by an empty URL name, a name awkward to use as a template path, two names for the same normalized destination, or a URL literal typed into visible copy. Every destination should have exactly one owner so a changed link is edited in one place. Reuse one purpose-based name such as urls.checkout at every placement, and render visible URLs from config; code samples are exempt.",
		},
		{
			ID:      "links.literal",
			Summary: "Templates take link destinations from config urls instead of hardcoded URLs",
			Help:    "A template attribute, URL-like JSON field or inline CSS contains a literal destination, including in inactive branches and partials. Hardcoded copies drift when the real link changes and slip past the URL checks. Declare the URL under urls in lfm.json and render it, e.g. href=\"{{.urls.checkout}}\"; local asset paths and same-page #fragment links are exempt.",
		},
	}
}

type Finding struct {
	Severity      diagnostic.Severity
	Message, Hint string
}
type Result struct {
	URL      *whatwg.Url
	Findings []Finding
}

// Analyzer owns a browser parser/base and a per-build cache; no shared mutable
// state and no HTTP requests. Parse repairs are evidence, never output rewrites.
type resolved struct {
	url *whatwg.Url
	err error
}

type Analyzer struct {
	resolved map[string]resolved
	parser   whatwg.Parser
	base     *whatwg.Url
	cache    map[string]Result
}

func New(base string) *Analyzer {
	p := whatwg.NewParser(whatwg.WithReportValidationErrors())
	b, _ := p.Parse(base)
	if b == nil {
		b, _ = p.Parse("https://micro.invalid/")
	}
	return &Analyzer{parser: p, base: b, cache: map[string]Result{}, resolved: map[string]resolved{}}
}
func (a *Analyzer) Resolve(raw string) (*whatwg.Url, error) {
	if r, ok := a.resolved[raw]; ok {
		return r.url, r.err
	}
	u, err := a.parser.BasicParser(raw, a.base, nil, whatwg.NoState)
	a.resolved[raw] = resolved{u, err}
	return u, err
}
func Scheme(raw string) string {
	// Read only the scheme prefix; the library parses the URL itself.
	prefix, _, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		return ""
	}
	for i, ch := range prefix {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || i > 0 && (ch >= '0' && ch <= '9' || ch == '+' || ch == '-' || ch == '.')) {
			return ""
		}
	}
	return strings.ToLower(prefix)
}
func Embedded(raw string) bool { s := Scheme(raw); return s == "data" || s == "blob" }
func LocalAsset(raw string) bool {
	s := strings.TrimSpace(raw)
	return strings.HasPrefix(s, "assets/") || strings.HasPrefix(s, "./assets/")
}
func Trustable(raw string) bool {
	s := Scheme(raw)
	return s != "javascript" && s != "vbscript" && !strings.ContainsAny(raw, "\x00\t\r\n")
}

func (a *Analyzer) Inspect(raw string) Result {
	if r, ok := a.cache[raw]; ok {
		return r
	}
	r := Result{}
	add := func(level diagnostic.Severity, message, hint string) {
		r.Findings = append(r.Findings, Finding{level, message, hint})
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		add(diagnostic.Warning, "Empty URL resolves to the current page.", "Use the configured page URL for self-links, or a button for an action.")
	}
	if !utf8.ValidString(raw) {
		add(diagnostic.Error, "URL contains invalid UTF-8.", "Use valid Unicode or percent-encoded bytes.")
	}
	control := false
	invisible := false
	for _, ch := range raw {
		if ch < 0x20 || ch == 0x7f {
			control = true
		}
		if unicode.Is(unicode.Cf, ch) {
			invisible = true
		}
	}
	if control {
		add(diagnostic.Error, "URL contains control characters that browsers can remove or encode.", "Remove tabs, line breaks and control bytes from the configured destination.")
	}
	if invisible {
		add(diagnostic.Warning, "URL contains invisible Unicode formatting characters.", "Confirm these characters are intended; encode deliberate path characters and inspect the hostname.")
	}
	if raw != trimmed {
		add(diagnostic.Warning, "URL has surrounding whitespace.", "Keep the configured URL trimmed; browsers may silently discard surrounding whitespace.")
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http//") || strings.HasPrefix(lower, "https//") || strings.HasPrefix(lower, "http;//") || strings.HasPrefix(lower, "https;//") {
		add(diagnostic.Error, "HTTP(S) protocol separator is malformed.", "Use http:// or https://, including the colon.")
	}
	scheme := Scheme(trimmed)
	if scheme == "javascript" || scheme == "vbscript" {
		add(diagnostic.Error, "Script pseudo-URL is not a web destination.", "Use a real href for navigation or a button with a script handler for an action.")
	}
	if scheme == "http" || scheme == "https" {
		tail := trimmed[len(scheme)+1:]
		if strings.HasPrefix(strings.ToLower(tail), "//http://") || strings.HasPrefix(strings.ToLower(tail), "//https://") {
			add(diagnostic.Error, "HTTP(S) URL repeats a protocol in its hostname/path.", "Remove the duplicated http:// or https:// prefix.")
		}
		if !strings.HasPrefix(tail, "//") || strings.HasPrefix(tail, "///") {
			add(diagnostic.Error, "HTTP(S) URL must use scheme://host without missing or extra slashes.", "Correct the explicit protocol and hostname; a browser repair can silently change the destination.")
		}
	} else if likelyProtocolTypo(scheme) {
		add(diagnostic.Error, "URL scheme looks like a misspelling of http/https.", "Use http:// or https://; leave deliberate application schemes explicit in config.")
	} else if scheme != "" && !knownSchemes[scheme] {
		add(diagnostic.Warning, "URL uses a custom/application scheme.", "Confirm the handler exists for your audience. Custom schemes are allowed; prefer HTTPS for ordinary web links.")
	}
	if strings.HasPrefix(trimmed, "//") {
		add(diagnostic.Warning, "Protocol-relative URL inherits the page's scheme.", "Prefer an explicit https:// destination when that is the intended endpoint.")
	}
	bareEmail := false
	if scheme == "" && !strings.ContainsAny(trimmed, "/ ?#") && strings.Contains(trimmed, "@") {
		if address, err := mail.ParseAddress(trimmed); err == nil && address.Address == trimmed {
			bareEmail = true
			add(diagnostic.Warning, "Email address is missing the mailto: scheme.", "Use mailto:name@example.org for a contact link; a bare address resolves as a relative web path.")
		}
	}
	if scheme == "" && !bareEmail && looksHost(trimmed) {
		add(diagnostic.Warning, "URL resembles a hostname but has no protocol.", "If this is an external site, add https://; otherwise make the relative path explicit with ./ or /.")
	}
	if scheme == "" && strings.Contains(strings.ToLower(trimmed), "://") {
		add(diagnostic.Warning, "Relative URL contains a protocol-looking substring.", "Check for a missing/extra leading slash or a malformed protocol; keep it only for an intentional relative path.")
	}
	if strings.Contains(trimmed, "&amp;") {
		add(diagnostic.Warning, "URL appears HTML-escaped before templating.", "Store raw & in JSON. html/template performs HTML escaping; pre-escaping changes query parameter names.")
	}
	if templatePlaceholder(raw) {
		add(diagnostic.Error, "URL still contains a template placeholder.", "Store the actual destination in config; config strings are not recursively templated.")
	}
	u, err := a.Resolve(raw)
	r.URL = u
	if err != nil {
		// Library error strings contain the full input, including query credentials.
		add(diagnostic.Error, "Browser URL parser rejects this destination: "+string(urlerrors.Type(err))+".", "Check hostname/IDNA, bracketed IPv6, port (0–65535), protocol and percent encoding.")
	} else {
		reported := map[urlerrors.ErrorType]bool{}
		for _, e := range u.ValidationErrors() {
			kind := urlerrors.Type(e)
			if reported[kind] {
				continue
			}
			reported[kind] = true
			if kind == urlerrors.SpecialSchemeMissingFollowingSolidus && (scheme == "http" || scheme == "https") {
				continue
			}
			if kind == urlerrors.InvalidCredentials {
				continue
			}
			add(diagnostic.Warning, "Browser URL parser repairs/reports: "+string(kind)+".", "Correct the source spelling/encoding unless this browser normalization is deliberate; the compiler does not rewrite it.")
		}
		if u.Username() != "" || u.Password() != "" {
			add(diagnostic.Warning, "URL embeds user information before the host.", "Remove credentials/userinfo from public links; the actual host is the part after @.")
		}
		if u.Scheme() == "http" || u.Scheme() == "https" {
			if !u.IsIPv4() && !u.IsIPv6() && oddDNSName(u.Hostname()) {
				add(diagnostic.Warning, "Hostname has unusual DNS-label spelling.", "Check empty labels, punctuation and leading/trailing hyphens. Browser parsing does not prove that DNS resolves; deliberate intranet names remain allowed.")
			}
			if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "[::1]" {
				add(diagnostic.Warning, "URL points to a loopback host.", "Use the production destination before deployment; keep loopback only for intentional local development.")
			}
		}
	}
	if scheme == "mailto" {
		inspectMailto(trimmed, add)
	}
	if scheme == "tel" || scheme == "sms" {
		target := strings.SplitN(trimmed[len(scheme)+1:], "?", 2)[0]
		if strings.HasPrefix(target, "//") || !strings.ContainsAny(target, "0123456789") {
			add(diagnostic.Warning, "Phone/message URI has no recognizable number or uses //.", "Use e.g. tel:+4940123456; verify any extension/parameter format with the intended handler.")
		}
	}
	a.cache[raw] = r
	return r
}

var knownSchemes = map[string]bool{"http": true, "https": true, "mailto": true, "tel": true, "sms": true, "geo": true, "webcal": true, "ftp": true, "ftps": true, "magnet": true, "data": true, "blob": true, "file": true, "javascript": true, "vbscript": true}

func likelyProtocolTypo(s string) bool {
	switch s {
	case "htp", "htps", "htpp", "htpps", "htt", "httsp", "hhttp", "hhttps", "httpss", "httpx", "hptt", "hptts":
		return true
	}
	return false
}
func looksHost(s string) bool {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, ".") || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "?") {
		return false
	}
	head := strings.SplitN(s, "/", 2)[0]
	head = strings.SplitN(head, "?", 2)[0]
	head = strings.SplitN(head, "#", 2)[0]
	if strings.HasPrefix(strings.ToLower(head), "www.") {
		return true
	}
	for _, suffix := range []string{".com", ".org", ".net", ".de", ".io", ".co", ".app", ".dev"} {
		if strings.HasSuffix(strings.ToLower(head), suffix) {
			return true
		}
	}
	return false
}
func inspectMailto(raw string, add func(diagnostic.Severity, string, string)) {
	address, query, _ := strings.Cut(raw[len("mailto:"):], "?")
	if strings.HasPrefix(address, "//") {
		add(diagnostic.Warning, "mailto URI uses // before the recipient.", "Use mailto:name@example.com without authority slashes.")
	}
	recipients, err := neturl.PathUnescape(address)
	if err != nil {
		add(diagnostic.Warning, "mailto recipient has malformed percent encoding.", "Encode recipient bytes correctly; use raw & only between query fields.")
		return
	}
	if recipients != "" {
		for _, recipient := range strings.Split(recipients, ",") {
			parsed, err := mail.ParseAddress(recipient)
			if err != nil || parsed.Address != recipient {
				add(diagnostic.Warning, "mailto recipient does not resemble a plain email address.", "Use an addr-spec recipient; percent-encode special characters and verify the address manually.")
				break
			}
		}
	}
	params, err := neturl.ParseQuery(query)
	if err != nil {
		add(diagnostic.Warning, "mailto query has malformed escaping/separators.", "Use properly percent-encoded subject/body fields and & between parameters.")
	}
	if recipients == "" && params.Get("to") == "" && params.Get("cc") == "" && params.Get("bcc") == "" {
		add(diagnostic.Warning, "mailto URI has no recipient.", "Supply an address unless opening an intentionally blank draft is the feature.")
	}
}

func (r Result) Valid() bool {
	for _, finding := range r.Findings {
		if finding.Severity == diagnostic.Error {
			return false
		}
	}
	return r.URL != nil
}
func (a *Analyzer) Key(raw string) string {
	if u, err := a.Resolve(raw); err == nil {
		return u.Href(false)
	}
	return ""
}
func URLHint(attribute string) string {
	return fmt.Sprintf("Move this %s destination to lfm.json urls.<name> and render {{.urls.<name>}}; keep local assets/ paths in HTML.", attribute)
}

func oddDNSName(host string) bool {
	host = strings.TrimSuffix(host, ".")
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return true
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
				return true
			}
		}
	}
	return false
}
