package compile

import (
	"bytes"
	"encoding/xml"
	"net/url"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/htmlx"
	"golang.org/x/net/html"
)

// discoveryFiles runs after all authored assets have their output names. Only
// the configured canonical is listed; deliverable paths are never inputs here.
func (b *builder) discoveryFiles(doc *html.Node) error {
	noIndex := false
	htmlx.Walk(doc, func(n *html.Node) {
		blocked, _ := check.RobotsDirectives(n)
		noIndex = noIndex || blocked
	})
	if !noIndex {
		if _, authored := b.files["sitemap.xml"]; !authored {
			var sitemap bytes.Buffer
			sitemap.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\"><url><loc>")
			if err := xml.EscapeText(&sitemap, []byte(b.config.URL)); err != nil {
				return err
			}
			sitemap.WriteString("</loc></url></urlset>\n")
			if err := b.emit("sitemap.xml", "canonical sitemap", sitemap.Bytes()); err != nil {
				return err
			}
		}
	}
	u, err := url.Parse(b.config.URL)
	if err != nil {
		return err
	}
	if u.Path == "/" {
		if _, authored := b.files["robots.txt"]; !authored {
			robots := "User-agent: *\nAllow: /\n"
			if !noIndex {
				robots += "Sitemap: " + b.config.URL + "sitemap.xml\n"
			}
			return b.emit("robots.txt", "crawler discovery", []byte(robots))
		}
	}
	return nil
}
