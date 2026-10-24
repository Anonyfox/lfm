package compile

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/release"
)

func outputMatch(t *testing.T, root, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "output", pattern))
	if err != nil || len(matches) != 1 {
		t.Fatalf("output pattern %s: %v, %v", pattern, matches, err)
	}
	return filepath.Base(matches[0])
}

func TestEmittedCodeContentHashes(t *testing.T) {
	for _, mode := range []string{"auto inline", "auto packed", "file", "override", "template", "classic", "release"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "release" {
				if testing.Short() {
					t.Skip("browser integration")
				}
				if _, err := release.BrowserPath(); err != nil {
					t.Skip(err)
				}
			}
			root := autoFixture(t)
			css := "h1{color:#123456}"
			if mode == "auto packed" || mode == "release" {
				for i := range 2000 {
					css += fmt.Sprintf(".unused%d{padding:%dpx;color:blue}", i, i)
				}
			}
			write(t, root, "theme.css", css)
			js := `window.value="hello"`
			if mode == "auto packed" {
				js = `window.value="` + strings.Repeat("abc", 18000) + `"`
			}
			write(t, root, "app.js", js)
			script, style := `{{script "app.js"}}`, `{{style "theme.css"}}`
			switch mode {
			case "file":
				changeConfig(t, root, func(c map[string]any) { c["build"].(map[string]any)["assets"] = map[string]any{"default": "file"} })
			case "override", "template", "release":
				changeConfig(t, root, func(c map[string]any) {
					c["build"].(map[string]any)["assets"] = map[string]any{"overrides": map[string]any{"app.js": "file", "theme.css": "file"}}
				})
			}
			if mode == "template" {
				script += `<template>{{script "app.js"}}{{style "theme.css"}}</template>`
			}
			if mode == "classic" {
				script = `<script async src="app.js"></script><script defer src="other.js"></script>`
				write(t, root, "other.js", `window.other=1`)
			}
			write(t, root, "index.html.tmpl", helperHTML(script, style))
			compile := func() Result {
				t.Helper()
				r, err := Compile(context.Background(), root, Options{Publish: true, Release: mode == "release"})
				if err != nil {
					t.Fatalf("%v %+v", err, r.Validation)
				}
				refs := map[string]bool{}
				for _, n := range parsePage(t, read(t, root, "output/index.html")) {
					if n.Data == "link" {
						refs[htmlx.Attr(n, "href")] = true
					}
					if n.Data == "script" {
						refs[htmlx.Attr(n, "src")] = true
					}
				}
				code := map[string]bool{}
				for name, raw := range r.Artifact {
					if !strings.HasSuffix(name, ".css") && !strings.HasSuffix(name, ".js") {
						continue
					}
					if !regexp.MustCompile(`-[0-9a-f]{12}\.(css|js)$`).MatchString(name) {
						t.Errorf("unhashed code: %s", name)
					}
					hash := fmt.Sprintf("%x", sha256.Sum256(raw))[:12]
					if !strings.HasSuffix(name, "-"+hash+filepath.Ext(name)) {
						t.Errorf("wrong content hash: %s", name)
					}
					if !refs[fileURL(name)] {
						t.Errorf("missing HTML reference: %s", name)
					}
					code[fileURL(name)] = true
				}
				for ref := range refs {
					if (strings.HasSuffix(ref, ".css") || strings.HasSuffix(ref, ".js")) && !code[ref] {
						t.Errorf("reference without output: %s", ref)
					}
				}
				if mode == "auto inline" && len(code) != 0 {
					t.Fatal("small code emitted")
				}
				if mode != "auto inline" && len(code) == 0 {
					t.Fatal("no emitted code tested")
				}
				if mode == "auto packed" && (r.Cost.ExternalCSSBytes <= 20*1024 || r.Cost.ExternalJSBytes <= 50*1024) {
					t.Fatal("packing thresholds not crossed", r.Cost)
				}
				if mode == "release" && (r.Release == nil || !r.Release.Applied) {
					t.Fatal("critical split not applied", r.Release)
				}
				return r
			}
			before := compile()
			if mode == "file" || mode == "auto packed" {
				write(t, root, "theme.css", strings.Replace(css, "#123456", "#123457", 1))
				after := compile()
				for name := range before.Artifact {
					if strings.HasSuffix(name, ".css") {
						if _, ok := after.Artifact[name]; ok {
							t.Errorf("CSS name unchanged: %s", name)
						}
					}
				}
			}
		})
	}
}
