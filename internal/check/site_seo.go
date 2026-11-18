package check

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"
)

// SiteIdentity compares Organization descriptions in folder traversal order.
func SiteIdentity(pages []*Document) []Diagnostic {
	type occurrence struct {
		org  organization
		page *Document
	}
	first := map[string]occurrence{}
	var issues []Diagnostic
	for _, d := range pages {
		g := d.seoGraph()
		if g == nil {
			continue
		}
		for _, org := range g.organizations {
			if strings.HasPrefix(org.id, "_:") {
				continue
			}
			key := org.id
			if ref, err := url.Parse(key); err == nil && !ref.IsAbs() {
				base := url.URL{Path: "/" + d.Options.File}
				key = base.ResolveReference(ref).String()
			}
			old, ok := first[key]
			if !ok {
				first[key] = occurrence{org, d}
				continue
			}
			if old.page == d || !old.org.conflicts(org, 3) {
				first[key] = occurrence{old.org.merge(org), old.page}
				continue
			}
			issue := d.At(org.node.script, Warning, fmt.Sprintf("Organization %q differs between %s and %s.", org.id, old.page.Options.File, d.Options.File), "Use consistent name, url and logo values across pages for this @id.")
			issue.Rule = "jsonld.identity"
			issue.Related = []Location{old.page.At(old.org.node.script, Warning, "", "").Location}
			issues = append(issues, issue)
		}
	}
	return issues
}

const sitemapNamespace = "http://www.sitemaps.org/schemas/sitemap/0.9"

func calendarDate(raw string) string {
	for _, layout := range []string{time.DateOnly, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04Z07:00", "2006-01-02T15:04", "2006-01-02T15:04:05-0700", "2006-01-02T15:04-0700"} {
		if _, err := time.Parse(layout, raw); err == nil {
			return raw[:10]
		}
	}
	return ""
}

func sitemapCanonical(d *Document) *url.URL {
	base, _ := url.Parse(strings.TrimSpace(d.Options.URL))
	for _, n := range d.activeNodes("link[rel]") {
		if n.Namespace != "" || !inHead(n) || !containsWord(Attr(n, "rel"), "canonical") {
			continue
		}
		href := strings.TrimSpace(Attr(n, "href"))
		if ref, err := url.Parse(href); err == nil && href != "" {
			if base != nil {
				ref = base.ResolveReference(ref)
			}
			if ref.IsAbs() && ref.Host != "" {
				return ref
			}
		}
	}
	if base != nil && base.IsAbs() && base.Host != "" {
		return base
	}
	return nil
}

func sitemapKey(u url.URL) string {
	u.Host = strings.ToLower(u.Host)
	// Keep escaped slashes inside path segments distinct from delimiters.
	if strings.HasSuffix(u.EscapedPath(), "/") {
		u.Path = strings.TrimSuffix(u.Path, "/")
		u.RawPath = strings.TrimSuffix(u.RawPath, "/")
	}
	return u.String()
}

func sitemapPath(u url.URL) string {
	file := strings.TrimPrefix(u.Path, "/")
	if file == "" || strings.HasSuffix(file, "/") {
		file += "index.html"
	}
	return file
}

func lookupSitemapPage(byFile map[string]*Document, u url.URL) *Document {
	file := sitemapPath(u)
	if d := byFile[file]; d != nil {
		return d
	}
	return byFile[path.Join(file, "index.html")]
}

// SitemapDates reads only direct protocol url entries and compares local calendar dates.
// A sitemap loc is compared to the one page that uniquely has that canonical URL, so a
// page staged as index.html still matches its own entry. If several pages share a
// canonical, or a page has none, the entry is matched by file path, so children that
// canonicalize to a parent hub are not compared to the hub's lastmod.
func SitemapDates(pages []*Document, input io.Reader) []Diagnostic {
	byFile := map[string]*Document{}
	byCanonical := map[string][]*Document{}
	for _, d := range pages {
		if u := sitemapCanonical(d); u != nil {
			key := sitemapKey(*u)
			byCanonical[key] = append(byCanonical[key], d)
			continue
		}
		byFile[d.Options.File] = d
	}
	dec := xml.NewDecoder(input)
	depth := 0
	var issues []Diagnostic
	for {
		token, err := dec.Token()
		if err != nil {
			break
		}
		switch el := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 && el.Name != (xml.Name{Space: sitemapNamespace, Local: "urlset"}) {
				return nil
			}
			if depth != 2 || el.Name != (xml.Name{Space: sitemapNamespace, Local: "url"}) {
				continue
			}
			var entry struct {
				Loc     string  `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 loc"`
				Lastmod *string `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 lastmod"`
			}
			if dec.DecodeElement(&entry, &el) != nil {
				return issues
			}
			depth--
			u, err := url.Parse(strings.TrimSpace(entry.Loc))
			if err != nil || strings.TrimSpace(entry.Loc) == "" {
				continue
			}
			matched := byCanonical[sitemapKey(*u)]
			switch len(matched) {
			case 1:
				issues = append(issues, pageDates(matched[0], entry.Lastmod)...)
			case 0:
				if d := lookupSitemapPage(byFile, *u); d != nil {
					issues = append(issues, pageDates(d, entry.Lastmod)...)
				}
			default:
				subset := map[string]*Document{}
				for _, d := range matched {
					subset[d.Options.File] = d
				}
				if d := lookupSitemapPage(subset, *u); d != nil {
					issues = append(issues, pageDates(d, entry.Lastmod)...)
				}
			}
		case xml.EndElement:
			depth--
		}
	}
	return issues
}

func pageDates(d *Document, sitemapDate *string) []Diagnostic {
	g := d.seoGraph()
	if g == nil {
		return nil
	}
	var issues []Diagnostic
	var lastmod string
	if sitemapDate != nil {
		lastmod = strings.TrimSpace(*sitemapDate)
	}
	last := calendarDate(lastmod)
	for _, n := range g.nodes {
		if !hasSchemaType(n.value, webPageTypes) {
			continue
		}
		for _, v := range schemaValues(n.value, "dateModified") {
			modified, _ := scalar(v).(string)
			day := calendarDate(modified)
			if day != "" && (sitemapDate == nil || day == last) {
				continue
			}
			msg := fmt.Sprintf("Sitemap lastmod %q and dateModified %q have different dates.", lastmod, modified)
			switch {
			case day == "" && sitemapDate != nil && last == "":
				msg = fmt.Sprintf("Sitemap lastmod %q and dateModified %q are invalid ISO 8601 dates.", lastmod, modified)
			case day == "":
				msg = fmt.Sprintf("dateModified %q is an invalid ISO 8601 date.", modified)
			case last == "":
				msg = fmt.Sprintf("Sitemap lastmod %q is an invalid ISO 8601 date.", lastmod)
			}
			issue := d.At(n.script, Warning, msg, "Use the same modification date in sitemap.xml and the page's JSON-LD.")
			issue.Rule = "seo.lastmod"
			issues = append(issues, issue)
		}
	}
	return issues
}
