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

func TestFAQVisibleTokens(t *testing.T) {
	const q = "Which release?"
	for _, tc := range []struct{ name, question, answer, body, warning string }{
		{"stable notes", q, "See VS Code 1.145 release notes.", "<h2>Which release?</h2><p>See <a>VS Code 1.145 release notes</a></p><h2>02 Next</h2>", ""},
		{"release history", q, "Patch releases can arrive between minor releases. Official release history.", "Which release?<p>Patch releases can arrive between minor releases. <a>Official release history</a></p><h2>03 Next</h2>", ""},
		{"rollback settings", q, "Extension updates have a separate setting. Previous versions and update settings.", "Which release?<p>Extension updates have a separate setting.</p><pre><code>{ &quot;update.mode&quot;: &quot;none&quot; }</code></pre><p><a>Previous versions and update settings</a> · <a>1.143.0 release notes</a></p>", ""},
		{"insiders link", q, "Keep a separate Stable installation for daily work. Insiders builds and installation.", "Which release?<p>Keep a separate Stable installation for daily work. <a>Insiders builds and installation</a></p>", ""},
		{"German quotes", "Ista zeigt die Aufteilung nur informativ – was heißt das?", "Sie ist informativ.", "<h2>Ista zeigt die Aufteilung nur „informativ“ – was heißt das?</h2><p>Sie ist informativ.</p>", ""},
		{"fold and compatibility", "Straße ４?", "Café daily.", "<h2>STRASSE 🚲 4</h2><p>Cafe\u0301 daily</p>", ""},
		{"missing question", q, "Choose stable.", "<h2>Which is the release?</h2><p>Choose stable.</p>", "FAQ question"},
		{"changed number", q, "Update every 7 days.", "Which release?<p>Update every 17 days.</p>", "The accepted answer"},
		{"changed fact", q, "Choose stable.", "Which release?<p>Choose unstable.</p>", "The accepted answer"},
		{"truncated", q, "Download the package. Install the update.", "Which release?<p>Download the package.</p>", "The accepted answer"},
		{"swapped", q, "Download the package. Install the update.", "Which release?<p>Install the update. Download the package.</p>", "The accepted answer"},
		{"at bound", q, "Start here.", "Which release?<p>Start " + strings.Repeat("extra ", 12) + "here.</p>", ""},
		{"over bound", q, "Start here.", "Which release?<p>Start " + strings.Repeat("extra ", 13) + "here.</p>", "The accepted answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head := parityGraph(fmt.Sprintf(`{"@type":"FAQPage","mainEntity":{"@type":"Question","name":%q,"acceptedAnswer":{"@type":"Answer","text":%q}}}`, tc.question, tc.answer))
			issues := runOn(t, faqRule, head, tc.body, Options{})
			if tc.warning == "" && len(issues) == 0 {
				return
			}
			if tc.warning == "" || len(issues) != 1 || issues[0].Severity != Warning || !strings.HasPrefix(issues[0].Message, tc.warning) {
				t.Fatal(issues)
			}
		})
	}
}

func FuzzFAQMatch(f *testing.F) {
	f.Add("a b a c", "a c", uint8(0))
	f.Add(strings.Repeat("word ", 80)+"extra end", strings.Repeat("word ", 70)+"end", uint8(1))
	f.Fuzz(func(t *testing.T, pageText, query string, budget uint8) {
		page, want := faqTokens(pageText), faqTokens(query)
		if len(page) > 512 || len(want) > 256 {
			return
		}
		extras := int(budget) % 13
		expected := false
		for i := range page {
			matched := 0
			for _, token := range page[i:min(len(page), i+len(want)+extras)] {
				if matched < len(want) && token == want[matched] {
					matched++
				}
			}
			if len(want) > 0 && matched == len(want) {
				expected = true
				break
			}
		}
		if faqMatch(page, want, extras) != expected {
			t.Fatalf("page %q, query %q, extra budget %d: want %v", page, want, extras, expected)
		}
	})
}

func BenchmarkFAQRepetitivePage(b *testing.B) {
	page := strings.Fields(strings.Repeat("word ", 8192))
	want := strings.Fields(strings.Repeat("word ", 1024) + "end")
	for b.Loop() {
		faqMatch(page, want, faqAnswerExtras)
	}
}

func TestLastmodCanonicalSource(t *testing.T) {
	const sitemap = `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url><loc>https://example.com/</loc><lastmod>2025-02-01</lastmod></url>
	<url><loc>https://example.com/cargo/</loc><lastmod>2025-02-02</lastmod></url>
	<url><loc>https://other.example/cargo/</loc><lastmod>2025-02-03</lastmod></url></urlset>`
	for _, tc := range []struct {
		name, head, optionURL string
		want                  int
	}{
		{"head overrides folder URL", `<link rel=canonical href="https://example.com/cargo">`, "https://example.com/", 0},
		{"absolute option", "", "https://example.com/cargo", 0},
		{"absolute option mismatch", "", "https://other.example/cargo", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.head+parityGraph(`{"@type":"Article","dateModified":"2025-02-02"}`)), Options{URL: tc.optionURL})
			if err != nil {
				t.Fatal(err)
			}
			if got := SitemapDates([]*Document{doc}, strings.NewReader(sitemap)); len(got) != tc.want {
				t.Fatalf("got %v, want %d warnings", got, tc.want)
			}
		})
	}
}

func TestLastmodSharedCanonical(t *testing.T) {
	const sitemap = `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url><loc>https://example.com/hub/</loc><lastmod>2026-11-18</lastmod></url></urlset>`
	page := func(file, canonical, day string) *Document {
		t.Helper()
		doc, err := Parse([]byte(fmt.Sprintf(`<link rel="canonical" href="https://example.com%s">%s`, canonical, parityGraph(fmt.Sprintf(`{"@type":"WebPage","dateModified":%q}`, day)))), Options{File: file})
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	hub, child := page("hub/index.html", "/hub/", "2026-11-18"), page("hub/aachen/index.html", "/hub/", "2026-10-27")
	if got := SitemapDates([]*Document{hub, child}, strings.NewReader(sitemap)); len(got) != 0 {
		t.Fatalf("shared canonical compared the child: %v", got)
	}
	hubBad := page("hub/index.html", "/hub/", "2026-10-01")
	got := SitemapDates([]*Document{hubBad, child}, strings.NewReader(sitemap))
	if len(got) != 1 || got[0].Location.File != "hub/index.html" {
		t.Fatalf("hub mismatch should warn only on the hub file: %v", got)
	}
}
