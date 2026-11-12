package compile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"golang.org/x/net/html"
)

func TestOptimizationCacheReconstructsFinalArtifact(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	tool := filepath.Join(t.TempDir(), "version")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho fixture\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LFM_CHROME", tool)
	t.Setenv("LFM_LIGHTHOUSE", tool)
	root := fixture(t)
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	run := func(doc *html.Node, result *Result) error {
		calls++
		var heading *html.Node
		htmlx.Walk(doc, func(n *html.Node) {
			if n.Data == "h1" {
				htmlx.SetAttr(n, "class", "optimized")
				heading = n
			}
		})
		// Built nodes keep insertion order; parsing renders a link's rel
		// before target, so a cache hit must match a fresh run byte for byte.
		link := htmlx.Element("a", "badge")
		htmlx.SetAttr(link, "href", "https://example.com/")
		htmlx.SetAttr(link, "target", "_blank")
		htmlx.SetAttr(link, "rel", "noopener")
		heading.Parent.AppendChild(link)
		result.Release = &release.Result{Applied: true, Stylesheet: "full.css"}
		return nil
	}
	var pages [][]byte
	for i := 0; i < 2; i++ {
		b := &builder{ctx: context.Background(), root: root, project: p, files: map[string]emitted{"photo.png": {name: "photo.png", contents: []byte("unchanged bytes")}}}
		doc, _ := html.Parse(bytes.NewBufferString("<!doctype html><h1>Page</h1>"))
		var result Result
		if err := b.optimizeWithCache(doc, &result, func(doc *html.Node, result *Result) error {
			b.files["full.css"] = emitted{name: "full.css", contents: []byte("full CSS")}
			return run(doc, result)
		}); err != nil {
			t.Fatal(err)
		}
		if !result.Release.Applied || string(b.files["full.css"].contents) != "full CSS" || string(b.files["photo.png"].contents) != "unchanged bytes" {
			t.Fatal("cache lost artifact state", result.Release, b.files)
		}
		var page bytes.Buffer
		if err := html.Render(&page, doc); err != nil {
			t.Fatal(err)
		}
		pages = append(pages, page.Bytes())
	}
	if calls != 1 || !bytes.Equal(pages[0], pages[1]) {
		t.Fatal("optimization not reused", calls)
	}
	write(t, root, "public/extra.txt", "new public bytes")
	b := &builder{ctx: context.Background(), root: root, project: p, files: map[string]emitted{"photo.png": {name: "photo.png", contents: []byte("unchanged bytes")}}}
	doc, _ := html.Parse(bytes.NewBufferString("<!doctype html><h1>Page</h1>"))
	if err := b.optimizeWithCache(doc, &Result{}, run); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("public asset change reused optimization")
	}
}

func TestReleaseCompileReusesAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := release.BrowserPath(); err != nil {
		t.Skip(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := fixture(t)
	first, err := Compile(context.Background(), root, Options{Release: true, Validate: true})
	if err != nil {
		t.Fatalf("%v %+v", err, first.Validation)
	}
	second, err := Compile(context.Background(), root, Options{Release: true, Validate: true})
	if err != nil {
		t.Fatalf("%v %+v", err, second.Validation)
	}
	if second.Release.Audit.Reused == "" {
		t.Fatal("identical release was audited again")
	}
	fast, err := Compile(context.Background(), root, Options{Release: true})
	if err != nil || !reflect.DeepEqual(first.Artifact, fast.Artifact) {
		t.Fatalf("fast release changed validated bytes: %v", err)
	}
	if !reflect.DeepEqual(first.Artifact, second.Artifact) {
		t.Fatal("reused optimization changed artifact")
	}
}
