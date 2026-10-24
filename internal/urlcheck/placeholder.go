package urlcheck

import (
	"net/url"
	"regexp"
	"strings"
)

var placeholderURLToken = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:REPLACE|TODO|XXX|PLACEHOLDER|CHANGEME)(?:$|[^a-z0-9])|(?:^|[^a-z0-9])YOUR_`)

// Placeholder reports template markers and reserved example destinations.
func Placeholder(raw string) bool {
	if templatePlaceholder(raw) {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "example.com" || host == "example.org" || strings.HasSuffix(host, ".example.com") || strings.HasSuffix(host, ".example.org") {
		return true
	}
	return placeholderURLToken.MatchString(u.Host + u.Path + "?" + u.RawQuery + "#" + u.Fragment)
}

func templatePlaceholder(raw string) bool {
	return strings.Contains(raw, "{{") || strings.Contains(raw, "}}") || strings.Contains(raw, "${")
}
