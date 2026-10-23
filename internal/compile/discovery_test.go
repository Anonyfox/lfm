package compile

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/project"
	"golang.org/x/net/html"
)

func TestDiscoveryFiles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, url, markup string
		sitemap, robots   bool
	}{
		{"root", "https://example.com/", "", true, true},
		{"subfolder", "https://example.com/product/", "", true, false},
		{"escaped URL", "https://example.com/a&b/", "", true, false},
		{"noindex", "https://example.com/", `<meta name=robots content="INDEX, NoIndex">`, false, true},
		{"googlebot", "https://example.com/", `<meta name=" GOOGLEBOT " content="none">`, false, true},
		{"body", "https://example.com/", `</head><body><meta name=robots content=noindex>`, false, true},
		{"private subfolder", "https://example.com/product/", `<meta name=robots content=noindex>`, false, false},
		{"nofollow", "https://example.com/", `<meta name=robots content=nofollow>`, true, true},
		{"nosnippet", "https://example.com/", `<meta name=robots content=nosnippet>`, true, true},
		{"unknown bot", "https://example.com/", `<meta name=custombot content=noindex>`, true, true},
		{"template", "https://example.com/", `<template><meta name=robots content=noindex></template>`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader("<html><head>" + tc.markup + "</head><body>Page</body></html>"))
			if err != nil {
				t.Fatal(err)
			}
			b := builder{config: project.Config{URL: tc.url}, files: map[string]emitted{}}
			if err := b.discoveryFiles(doc); err != nil {
				t.Fatal(err)
			}
			if _, ok := b.files["sitemap.xml"]; ok != tc.sitemap {
				t.Fatalf("sitemap present: %v, want %v", ok, tc.sitemap)
			}
			if _, ok := b.files["robots.txt"]; ok != tc.robots {
				t.Fatalf("robots present: %v, want %v", ok, tc.robots)
			}
			if tc.sitemap {
				var sitemap struct {
					XMLName xml.Name
					URLs    []struct {
						Loc string `xml:"loc"`
					} `xml:"url"`
				}
				data := b.files["sitemap.xml"].contents
				if err := xml.Unmarshal(data, &sitemap); err != nil {
					t.Fatal(err)
				}
				if sitemap.XMLName != (xml.Name{Space: "http://www.sitemaps.org/schemas/sitemap/0.9", Local: "urlset"}) || len(sitemap.URLs) != 1 || sitemap.URLs[0].Loc != tc.url || strings.Contains(string(data), "lastmod") {
					t.Fatalf("sitemap: %s", data)
				}
			}
			if tc.robots {
				want := "User-agent: *\nAllow: /\n"
				if tc.sitemap {
					want += "Sitemap: " + tc.url + "sitemap.xml\n"
				}
				if got := string(b.files["robots.txt"].contents); got != want {
					t.Fatalf("robots: %q, want %q", got, want)
				}
			}
		})
	}
}

func TestAuthoredDiscoveryFilesWin(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"robots.txt", "sitemap.xml"} {
		t.Run(name, func(t *testing.T) {
			root := fixture(t)
			changeConfig(t, root, func(c map[string]any) {
				c["url"] = "https://example.com/"
				c["build"].(map[string]any)["assets"].(map[string]any)["overrides"] = map[string]any{"assets/" + name: "file"}
			})
			const authored = "author controls this file\n"
			write(t, root, "assets/"+name, authored)
			write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</body>", `<a href="assets/`+name+`" download>Download</a></body>`, 1))
			r, err := Compile(context.Background(), root, Options{Publish: true})
			if err != nil {
				t.Fatal(err, r.Validation)
			}
			if got := read(t, root, "output/"+name); got != authored {
				t.Fatalf("authored %s replaced: %q", name, got)
			}
		})
	}
}

func TestDiscoveryFilesNeverListDownloads(t *testing.T) {
	t.Parallel()
	root := bookProject(t)
	changeConfig(t, root, func(c map[string]any) { c["url"] = "https://example.com/" })
	r, err := Compile(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	if _, ok := r.Artifact[bookPath+"/index.html"]; !ok {
		t.Fatal("fixture did not emit its download page")
	}
	for _, name := range []string{"robots.txt", "sitemap.xml"} {
		data, ok := r.Artifact[name]
		if !ok || strings.Contains(string(data), bookPath) || strings.Contains(string(data), "Disallow") {
			t.Fatalf("%s: %q", name, data)
		}
	}
}

func TestCompileNoindexRemovesSitemapAndAllowsCrawling(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	changeConfig(t, root, func(c map[string]any) {
		c["url"] = "https://example.com/"
		c["build"].(map[string]any)["validation"] = map[string]any{"rules": map[string]any{"seo.indexability": "off"}}
	})
	if _, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "</head>", `<meta name="googlebot" content="noindex"></head>`, 1))
	r, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	if _, ok := r.Artifact["sitemap.xml"]; ok {
		t.Fatal("noindex page still has a sitemap")
	}
	if _, err := os.Stat(filepath.Join(root, "output/sitemap.xml")); !os.IsNotExist(err) {
		t.Fatalf("old sitemap survived publish: %v", err)
	}
	if got := read(t, root, "output/robots.txt"); got != "User-agent: *\nAllow: /\n" {
		t.Fatalf("crawling blocked or Sitemap line retained: %q", got)
	}
	assertManifest(t, r)
}
