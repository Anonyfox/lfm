package target

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const pageHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Test</title><link rel="canonical" href="https://example.org/"></head><body><main><h1>Test page</h1></main></body></html>`

func write(t testing.TB, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func load(t *testing.T, s string) Result {
	t.Helper()
	r, err := Read(context.Background(), s, "test")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestURLFetch(t *testing.T) {
	var methods atomic.Int32
	assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("request carried state or used non-GET")
		}
		if r.UserAgent() != "lfm/test (+https://lfm.cash)" {
			t.Error(r.UserAgent())
		}
		methods.Add(1)
		fmt.Fprint(w, "body{color:red}")
	}))
	defer assets.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "secret=value")
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/page", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Add("X-Robots-Tag", "googlebot: noindex")
		fmt.Fprint(w, strings.Replace(pageHTML, "</head>", `<link rel="stylesheet" href="`+assets.URL+`/style.css"></head>`, 1))
	}))
	defer server.Close()
	r := load(t, strings.Replace(server.URL, "http://", "http://user:password@", 1))
	if r.URL != server.URL+"/page" || r.Identity != "https://example.org/" || r.Canonical != r.Identity || r.Lang != "en" || len(r.Redirects) != 1 || r.Redirects[0] != r.URL || len(r.Files) != 2 || methods.Load() != 1 {
		t.Fatalf("metadata: %+v", r)
	}
	if len(r.Options.Stylesheets) != 1 || len(r.Options.RobotsHeader) != 1 || r.Options.File != "page" {
		t.Fatalf("options: %+v", r.Options)
	}
}

func TestPageFailures(t *testing.T) {
	for _, tc := range []struct {
		name, kind, body string
		status           int
	}{
		{"404", "text/html", pageHTML, 404}, {"500", "text/html", pageHTML, 500}, {"png", "image/png", "image", 200}, {"json", "application/json", "{}", 200}, {"plain HTML", "text/plain", pageHTML, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.kind)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			if _, err := Read(context.Background(), s.URL, "test"); err == nil {
				t.Fatal("expected unreadable page")
			}
		})
	}
	for _, kind := range []string{"text/html", "application/xhtml+xml", "application/octet-stream"} {
		t.Run(kind, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", kind)
				fmt.Fprint(w, pageHTML)
			}))
			defer s.Close()
			load(t, s.URL)
		})
	}
}
func TestRedirectLimit(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, r.URL.Path+"x", http.StatusFound) }))
	defer s.Close()
	if _, err := Read(context.Background(), s.URL, "test"); err == nil || !strings.Contains(err.Error(), "5 redirects") {
		t.Fatal(err)
	}
}

func TestCanonical(t *testing.T) {
	for _, tc := range []struct{ href, want string }{{"/product", "/product"}, {"", ""}, {"data:text/plain,no", ""}} {
		t.Run(tc.href, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, strings.Replace(pageHTML, "https://example.org/", tc.href, 1))
			}))
			defer s.Close()
			r := load(t, s.URL)
			want := s.URL
			if tc.want != "" {
				want += tc.want
			}
			if r.Identity != want {
				t.Fatalf("identity %q want %q", r.Identity, want)
			}
		})
	}
}

func TestFolder(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	write(t, outside, "secret.css", "SECRET")
	write(t, dir, "index.html", strings.Replace(pageHTML, "</head>", `<link rel="stylesheet" href="https://example.org/assets/a.css"><script src="https://remote.invalid/x.js"></script><link rel="stylesheet" href="../secret.css"><link rel="stylesheet" href="%2e%2e/secret.css"><link rel="stylesheet" href="/etc/passwd"><link rel="stylesheet" href="escape/secret.css"></head>`, 1))
	write(t, dir, "assets/a.css", "body{color:red}")
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{dir, filepath.Join(dir, "index.html")} {
		r := load(t, arg)
		if r.Identity != "https://example.org/" || string(r.Options.Stylesheets["https://example.org/assets/a.css"]) != "body{color:red}" {
			t.Fatal(r.Options)
		}
		for _, f := range r.Files {
			if strings.Contains(f.SHA256, "SECRET") {
				t.Fatal("escaped root")
			}
		}
		for _, b := range r.Options.Assets {
			if strings.Contains(string(b), "SECRET") {
				t.Fatal("escaped root")
			}
		}
	}
	write(t, dir, "lfm.json", "{}")
	if _, err := Read(context.Background(), dir, "test"); err == nil || !strings.Contains(err.Error(), "lfm compile") {
		t.Fatal(err)
	}
}
func TestLocalAndSchemeFailures(t *testing.T) {
	for _, arg := range []string{"ftp://x", "file:///tmp/a.html", "data:text/html,x", "javascript:alert(1)", t.TempDir(), filepath.Join(t.TempDir(), "missing.html")} {
		if _, err := Read(context.Background(), arg, "test"); err == nil {
			t.Fatal(arg)
		}
	}
}

func TestLimits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", pageHTML)
	lim := defaultLimits
	lim.page = 10
	if _, err := read(context.Background(), dir, "test", lim); err == nil {
		t.Fatal("page cap")
	}
	write(t, dir, "index.html", strings.Replace(pageHTML, "</head>", `<link rel="stylesheet" href="a.css"><link rel="stylesheet" href="b.css"><img src="image.png"></head>`, 1))
	write(t, dir, "a.css", strings.Repeat("a", 100))
	write(t, dir, "b.css", strings.Repeat("b", 100))
	write(t, dir, "image.png", strings.Repeat("x", 100))
	lim = defaultLimits
	lim.file = 20
	lim.image = 10
	r, err := read(context.Background(), dir, "test", lim)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Files[1:] {
		if !f.Truncated || f.Bytes != 100 {
			t.Fatalf("%+v", f)
		}
	}
	lim.count = 2
	r, err = read(context.Background(), dir, "test", lim)
	if err != nil {
		t.Fatal(err)
	}
	skipped := 0
	for _, f := range r.Files {
		if f.Error != "" {
			skipped++
		}
	}
	if skipped != 2 {
		t.Fatal(r.Files)
	}
	lim = defaultLimits
	lim.total = len(r.Page) + 15
	r, err = read(context.Background(), dir, "test", lim)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Files[1].Truncated {
		t.Fatal(r.Files)
	}
}
func TestImageSizeAndSubresourceErrors(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, strings.Replace(pageHTML, "</head>", `<img src="/large.png"><script src="/missing.js"></script></head>`, 1))
		case "/large.png":
			w.Header().Set("Content-Length", "1000")
			fmt.Fprint(w, strings.Repeat("x", 1000))
		default:
			http.Error(w, "missing", 500)
		}
	}))
	defer s.Close()
	lim := defaultLimits
	lim.image = 20
	r, err := read(context.Background(), s.URL, "test", lim)
	if err != nil {
		t.Fatal(err)
	}
	if r.Files[1].Bytes != 1000 || !r.Files[1].Truncated || r.Files[2].Status != 500 {
		t.Fatal(r.Files)
	}
}
func TestTimeouts(t *testing.T) {
	for _, whole := range []bool{false, true} {
		t.Run(fmt.Sprint(whole), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					w.Header().Set("Content-Type", "text/html")
					fmt.Fprint(w, strings.Replace(pageHTML, "</head>", `<script src="/slow.js"></script></head>`, 1))
					return
				}
				<-r.Context().Done()
			}))
			defer s.Close()
			lim := defaultLimits
			if whole {
				lim.run = 200 * time.Millisecond
			} else {
				lim.request = 100 * time.Millisecond
			}
			r, err := read(context.Background(), s.URL, "test", lim)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Files) != 2 || r.Files[1].Error == "" {
				t.Fatal(r.Files)
			}
		})
	}
}
func TestParallelDeterministic(t *testing.T) {
	var active, maxActive atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, strings.Replace(pageHTML, "</head>", strings.Repeat("", 0)+func() string {
				var b strings.Builder
				for i := 0; i < 30; i++ {
					fmt.Fprintf(&b, `<script src="/%02d.js"></script>`, i)
				}
				return b.String()
			}()+"</head>", 1))
			return
		}
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := maxActive.Load()
			if n <= old || maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		fmt.Fprint(w, "const x=1;")
	}))
	defer s.Close()
	a := load(t, s.URL)
	b := load(t, s.URL)
	if maxActive.Load() > 8 || !reflect.DeepEqual(a.Files, b.Files) {
		t.Fatalf("max %d; nondeterministic files", maxActive.Load())
	}
	if len(a.Files) != 31 {
		t.Fatal(len(a.Files))
	}
}
func TestCSSFontsAndImport(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", strings.Replace(pageHTML, "</head>", `<link rel="stylesheet" href="assets/a.css"></head>`, 1))
	write(t, dir, "assets/a.css", `@import "b.css";@font-face{font-family:A;src:url(a.woff2);font-display:swap}`)
	write(t, dir, "assets/b.css", `@import "deep.css";@font-face{font-family:B;src:url(b.woff2);font-display:swap}`)
	write(t, dir, "assets/a.woff2", "font-a")
	write(t, dir, "assets/b.woff2", "font-b")
	write(t, dir, "assets/deep.css", "not fetched")
	r := load(t, dir)
	if len(r.Files) != 5 || len(r.Options.FontAssets) != 2 || len(r.Options.Stylesheets) != 2 {
		t.Fatalf("files %v; fonts %v; css %v", r.Files, r.Options.FontAssets, r.Options.Stylesheets)
	}
}
func FuzzTargetPaths(f *testing.F) {
	for _, s := range []string{"../secret", "%2e%2e/a", "/assets/a.css", "a\\b", "https://x/a"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = unsafePath(s)
		_ = confinedPath("/assets/css/styles.css", s)
	})
}

func TestStylesheetAliasesAndExtensionlessImport(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", strings.Replace(pageHTML, "</head>", `<link rel="stylesheet" href="a"><link rel="stylesheet" href="./a"></head>`, 1))
	write(t, dir, "a", `@import "b";body{color:#123456}`)
	write(t, dir, "b", `@import "not-read";p{margin:0}`)
	r := load(t, dir)
	if len(r.Files) != 3 || len(r.Options.Stylesheets) != 3 || string(r.Options.Stylesheets["a"]) != string(r.Options.Stylesheets["./a"]) {
		t.Fatalf("files %+v; CSS %+v", r.Files, r.Options.Stylesheets)
	}
}
func TestLocalIdentityWithoutCanonical(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "page.html", strings.Replace(pageHTML, `<link rel="canonical" href="https://example.org/">`, "", 1))
	r := load(t, filepath.Join(dir, "page.html"))
	if r.Identity != "" || r.Canonical != "" || r.Options.URL != "" || r.Options.File != "page.html" {
		t.Fatalf("%+v", r)
	}
}
func TestDormantCanonicalIgnored(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", strings.Replace(pageHTML, `<link rel="canonical" href="https://example.org/">`, `<template><link rel="canonical" href="https://wrong.example/"></template><link rel="canonical" href="https://example.org/">`, 1))
	if r := load(t, dir); r.Identity != "https://example.org/" {
		t.Fatal(r.Identity)
	}
}

func TestConfinedParentResource(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", strings.Replace(pageHTML, "</head>", `<link rel="stylesheet" href="assets/css/a.css"></head>`, 1))
	write(t, dir, "assets/css/a.css", `@font-face{font-family:Sample;src:url(../fonts/a.woff2);font-display:swap}`)
	write(t, dir, "assets/fonts/a.woff2", "font header")
	r := load(t, dir)
	if len(r.Files) != 3 || len(r.Options.FontAssets) != 1 {
		t.Fatalf("files %+v; fonts %+v", r.Files, r.Options.FontAssets)
	}
}

func TestLocalEscapedName(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "100% complete.html", pageHTML)
	r := load(t, filepath.Join(dir, "100% complete.html"))
	if r.Options.File != "100% complete.html" {
		t.Fatal(r.Options.File)
	}
}
