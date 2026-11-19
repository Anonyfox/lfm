package check

import (
	"strings"
	"testing"
)

const pageGraphFixture = `{"@type":"Organization","@id":"#org"},{"@type":"WebSite","@id":"#site"},{"@type":"WebPage","@id":"#webpage","isPartOf":{"@id":"#site"},"inLanguage":"EN-us","dateModified":"2026-10-01"},{"@type":"Article","publisher":{"@id":"#org"}}`

func TestGraphParity(t *testing.T) {
	const canonical = "https://example.org/page/"
	for _, tc := range []struct {
		name, from, to, want string
		count                int
	}{
		{"clean", "", "", "", 0},
		{"split", "", " ,{\"@id\":\"#org\",\"@type\":\"School\"}", "", 0},
		{"anonymous organizations", "", `,{"@type":"Organization"},{"@type":"Organization"}`, "3 Organization", 1},
		{"blank organizations", "", `,{"@id":"_:a","@type":"Organization"},{"@id":"_:a","@type":"Organization"}`, "3 Organization", 1},
		{"two sites", "", `,{"@id":"#other","@type":"WebSite"}`, "2 WebSite", 1},
		{"duplicate page", "", `,{"@type":["FAQPage","WebPage"],"@id":"#faq","isPartOf":{"@id":"#site"},"inLanguage":"en"}`, "2 WebPage", 1},
		{"article is not a page", "WebPage", "Article", "", 0},
		{"missing org", "Organization", "Person", "", 0},
		{"missing site", "WebSite", "Thing", "isPartOf", 1},
		{"separate FAQ and item", "", `,{"@type":"FAQPage"},{"@type":"ItemPage","@id":"https://example.org/item"}`, "", 0},
		{"linked pages", "", `,{"@type":"CollectionPage","hasPart":{"@type":"WebPage","@id":"/child"}},{"@type":"ItemList","itemListElement":[{"item":{"@type":"WebPage"}}]}`, "", 0},
		{"parent organization", `"@type":"Organization"`, `"@type":"Organization","parentOrganization":{"@type":"Organization"}`, "", 0},
		{"separate dataset creator", "", `,{"@type":"Dataset","creator":{"@type":"GovernmentOrganization"}}`, "", 0},
		{"inline publisher counted", "", `,{"publisher":{"@type":"Organization"}}`, "2 Organization", 2},
		{"related organization", `"@type":"Organization"`, `"@type":"Organization","parentOrganization":{"@id":"#parent","@type":"Organization"}`, "", 0},
		{"optional page metadata", `"@id":"#webpage","isPartOf":{"@id":"#site"},"inLanguage":"EN-us",`, "", "isPartOf", 1},
		{"unreferencable site", pageGraphFixture, `{"@type":"WebSite"},{"@type":"WebPage"}`, "", 0},
		{"wrong organization", pageGraphFixture, `{"@type":"Organization","@id":"#org","memberOf":{"@id":"#parent"}},{"@id":"#parent","@type":"Organization"},{"publisher":{"@id":"#parent"}}`, "different Organization", 1},
		{"several organizations", "", `,{"@type":"Organization","@id":"#other"},{"seller":{"@id":"#other"}}`, "2 Organization", 1},
		{"inline person", pageGraphFixture, `{"publisher":{"@type":"Person","@id":"#author","name":"Author"}}`, "", 0},
		{"wrong id", "#webpage", "https://example.org/other", "differs from the canonical", 1},
		{"wrong site", `"isPartOf":{"@id":"#site"}`, `"isPartOf":{"@id":"#org"}`, "isPartOf", 1},
		{"wrong lang", "EN-us", "fr", "inLanguage", 1},
		{"inline publisher", `"publisher":{"@id":"#org"}`, `"publisher":{"@id":"#org","name":"Publisher"}`, "publisher", 1},
		{"seller", "publisher", "seller", "", 0},
		{"inline seller", `"publisher":{"@id":"#org"}`, `"seller":{"@type":"Organization","@id":"#org","name":"Shop"}`, "seller", 1},
		{"provider", `"publisher":{"@id":"#org"}`, `"provider":{"@id":"#site"}`, "", 0},
		{"undefined", `"publisher":{"@id":"#org"}`, `"provider":{"@id":"#unknown"}`, "", 0},
		{"brand", "", `,{"@type":"Product","brand":{"@id":"#brand"}},{"@type":"Brand","@id":"#brand","owner":{"@id":"#org"}}`, "", 0},
		{"inline owner", "", `,{"@type":"Product","brand":{"@type":"Brand","owner":{"@type":"Organization","@id":"#org","name":"Shop"}}}`, "owner", 1},
		{"invalid date", "2026-10-01", "2026-02-30", "dateModified", 1},
		{"published", "", `,{"@type":"Article","datePublished":"soon"}`, "datePublished", 1},
		{"split dates", "", `,{"@id":"#a","datePublished":"2026-10-01T12:00Z"},{"@id":"#a","dateModified":"2026-10-01T13:00+02:00"}`, "precedes", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := pageGraphFixture + tc.to
			if tc.from != "" {
				nodes = strings.Replace(pageGraphFixture, tc.from, tc.to, 1)
			}
			issues := runOn(t, graphRule, `<link rel=canonical href="`+canonical+`">`+parityGraph(nodes), "", Options{URL: canonical})
			var messages []string
			for _, i := range issues {
				messages = append(messages, i.Message)
				if i.Severity != Warning || i.Hint == "" {
					t.Fatal(i)
				}
			}
			if len(issues) != tc.count || !strings.Contains(strings.Join(messages, "\n"), tc.want) {
				t.Fatal(issues)
			}
		})
	}
	for _, head := range []string{"", parityGraph(`{"@type":"FAQPage"}`), parityGraph(`{"@type":"Product"}`)} {
		if issues := runOn(t, graphRule, head, "", Options{}); len(issues) != 0 {
			t.Fatal(issues)
		}
	}
}

func TestGraphLanguageRegion(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		match bool
	}{
		{"de", "DE-de", true}, {"en-US", "en-GB", false}, {"de-Latn", "de-DE", true}, {"es-419", "es-MX", false},
	} {
		if graphLanguage(tc.a, tc.b) != tc.match {
			t.Fatal(tc)
		}
	}
}

func TestGraphWebPageIDOfNestedFile(t *testing.T) {
	const canonical = "https://example.org/hub/"
	for _, tc := range []struct {
		name, file, id string
		count          int
	}{
		{"child identifies itself", "hub/child/index.html", "https://example.org/hub/child/#webpage", 0},
		{"child names another page", "hub/child/index.html", "https://example.org/other/#webpage", 1},
		{"root is not exempt", "index.html", "https://example.org/#webpage", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			graph := `{"@type":"WebPage","@id":"` + tc.id + `"}`
			issues := runOn(t, graphRule, `<link rel=canonical href="`+canonical+`">`+parityGraph(graph), "", Options{URL: canonical, File: tc.file})
			if len(issues) != tc.count || tc.count == 1 && !strings.Contains(issues[0].Message, "differs from the canonical") {
				t.Fatal(issues)
			}
		})
	}
}
