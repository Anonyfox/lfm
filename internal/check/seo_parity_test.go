package check

import (
	"fmt"
	"strings"
	"testing"
)

func parityGraph(nodes string) string {
	return `<script type="application/ld+json">{"@context":"https://schema.org","@graph":[` + nodes + `]}</script>`
}

func TestFAQParity(t *testing.T) {
	head := parityGraph(`{"@graph":[{"@type":"FAQPage","mainEntity":{"@id":"#q"}},{"@id":"#q","@type":"Question","name":"Which trail?","acceptedAnswer":{"@type":"Answer","text":"Use <b>North</b> &amp; East."}}]}`)
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"matching", "<h2>Which   <i>trail</i>?</h2><p>UseNorth &amp; East.</p>", 0},
		{"question", "<p>Use North &amp; East.</p>", 1},
		{"answer", "Which trail?<style>.answer{display:none}</style><p class=answer>Use North &amp; East.</p>", 1},
		{"hidden", "<script>Which trail?</script><p hidden>Use North &amp; East.</p>", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := runOn(t, faqRule, head, tc.body, Options{})
			if len(issues) != tc.want {
				t.Fatal(issues)
			}
			for _, i := range issues {
				if i.Severity != Warning || !strings.Contains(i.Message, "Which trail?") {
					t.Fatal(i)
				}
			}
		})
	}
}

func TestIdentityParity(t *testing.T) {
	const org = `{"@id":"#org","@type":"BookStore","name":"Oak","sameAs":["https://a.com","https://b.com"]}`
	for _, tc := range []struct {
		name, nodes string
		want        int
	}{
		{"reordered", org + "," + strings.Replace(org, `"https://a.com","https://b.com"`, `"https://b.com","https://a.com"`, 1), 0},
		{"conflict", org + "," + strings.Replace(org, "Oak", "Elm", 1), 1},
		{"sameAs conflict", org + "," + strings.Replace(org, "a.com", "c.com", 1), 1},
		{"partial node merges", org + `,{"@id":"#org","@type":"ProfessionalService","name":"Oak","description":"Shop"}`, 0},
		{"partial node conflicts", org + `,{"@id":"#org","@type":"ProfessionalService","name":"Elm"}`, 1},
		{"resolved", `{"@type":"WebPage","publisher":{"@id":"#org"}},` + org, 0},
		{"unresolved", `{"@type":"Article","author":{"@id":"#x"},"seller":{"@id":"#y"},"provider":{"@id":"#z"},"publisher":{"@id":"#p"}}`, 4},
		{"unnamed", `{"@type":"Article","publisher":{"@type":"School"}}`, 1},
		{"person", `{"@type":"Article","publisher":{"@type":"Person"}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := runOn(t, identityRule, parityGraph(tc.nodes), "", Options{})
			if len(issues) != tc.want {
				t.Fatal(issues)
			}
			for _, i := range issues {
				if i.Severity != Warning {
					t.Fatal(i)
				}
			}
		})
	}
}

func TestCardParity(t *testing.T) {
	const image = `<meta property="og:image" content="card.png">`
	large := `<meta name="twitter:card" content="summary_large_image">`
	for _, tc := range []struct {
		name, head    string
		emitted, want int
	}{
		{"absent", "", 0, 0}, {"missing card", image, 0, 1}, {"relative", image + large + `<meta name="twitter:image" content="https://example.com/p/card.png">`, 0, 0},
		{"different", image + large + `<meta name="twitter:image" content="/else.png">`, 0, 1},
		{"player", image + `<meta name="twitter:card" content="player">`, 300, 0}, {"app", image + `<meta name="twitter:card" content="app">`, 0, 0},
		{"small", image + large + `<meta property="og:image:width" content="600">`, 0, 1}, {"emitted", image + large, 600, 1}, {"recommended", image + large, 1200, 0},
		{"second image", image + large + image + `<meta property="og:image:width" content="400">`, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Options{URL: "https://example.com/p/"}
			if tc.emitted > 0 {
				o.Assets = map[string][]byte{"card.png": sharingPNG(t, tc.emitted, 630)}
			}
			issues := runOn(t, sharingCardRule, tc.head, "", o)
			if len(issues) != tc.want {
				t.Fatal(issues)
			}
			for _, i := range issues {
				if i.Severity != Warning {
					t.Fatal(i)
				}
			}
		})
	}
}

func TestLastmodParity(t *testing.T) {
	for _, tc := range []struct {
		name, file, loc, kind, last, modified string
		want                                  int
	}{
		{"directory", "p/index.html", "/p/", "FAQPage", "2025-02-03", "2025-02-03T23:15:10-08:00", 0},
		{"file", "a.html", "/a.html", "Article", "2025-02-03", "2025-02-04", 1},
		{"bad lastmod", "index.html", "/", "WebPage", "2025-02-31", "2025-02-04", 1},
		{"bad modified", "index.html", "/", "BlogPosting", "2025-02-03", "yesterday", 1},
		{"absent", "index.html", "/", "WebPage", "bad", "", 0},
		{"other type", "index.html", "/", "Person", "bad", "bad", 0},
		{"optional lastmod", "index.html", "/", "WebPage", "", "2026-05-01", 0},
		{"invalid date without lastmod", "index.html", "/", "WebPage", "", "bad", 1},
		{"other path", "index.html", "/other", "WebPage", "bad", "bad", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prop := ""
			if tc.modified != "" {
				prop = fmt.Sprintf(`,"dateModified":%q`, tc.modified)
			}
			doc, err := Parse([]byte(parityGraph(fmt.Sprintf(`{"@type":%q%s}`, tc.kind, prop))), Options{File: tc.file})
			if err != nil {
				t.Fatal(err)
			}
			xml := fmt.Sprintf(`<urlset xmlns="%s"><url><loc>https://example.com%s</loc><lastmod>%s</lastmod></url></urlset>`, sitemapNamespace, tc.loc, tc.last)
			if tc.last == "" {
				xml = strings.Replace(xml, "<lastmod></lastmod>", "", 1)
			}
			issues := SitemapDates([]*Document{doc}, strings.NewReader(xml))
			if len(issues) != tc.want {
				t.Fatal(issues)
			}
			for _, i := range issues {
				if i.Rule != "seo.lastmod" || i.Severity != Warning || i.Location.File != tc.file {
					t.Fatal(i)
				}
			}
		})
	}
}

func FuzzSitemapDates(f *testing.F) {
	f.Add(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>/</loc><lastmod>2025-01-01</lastmod></url></urlset>`)
	d, err := Parse([]byte(parityGraph(`{"@type":"Article","dateModified":"2025-01-02"}`)), Options{})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 8192 {
			t.Skip()
		}
		for _, i := range SitemapDates([]*Document{d}, strings.NewReader(body)) {
			if i.Severity != Warning {
				t.Fatal(i)
			}
		}
	})
}
