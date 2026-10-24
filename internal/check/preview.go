package check

import (
	"net"
	"net/url"
	"strings"
)

// LoopbackPreview reports whether the inspected page was served on loopback.
func (d *Document) LoopbackPreview() bool { return loopbackURL(d.Options.ServedURL) }

func loopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	return strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()
}

// previewURL maps only the served origin onto the declared public canonical.
// The boolean marks a loopback URL whose public-host requirements cannot be judged.
func (d *Document) previewURL(raw string) (string, bool) {
	if !d.LoopbackPreview() || !loopbackURL(raw) {
		return raw, false
	}
	u, _ := url.Parse(raw)
	served, _ := url.Parse(d.Options.ServedURL)
	live, err := url.Parse(d.Options.URL)
	if err == nil && publicURL(d.Options.URL) && strings.EqualFold(u.Scheme, served.Scheme) && strings.EqualFold(u.Host, served.Host) {
		u.Scheme, u.Host = live.Scheme, live.Host
		return u.String(), false
	}
	return raw, true
}

// publicPreviewURL defers only the preview origin, retaining URL-shape checks.
func publicPreviewURL(raw string, local bool) bool {
	if !local {
		return publicURL(raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	u.Host, u.Scheme = "example.org", "https"
	return publicURL(u.String())
}
