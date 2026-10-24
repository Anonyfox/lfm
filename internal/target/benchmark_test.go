package target_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/target"
)

const benchmarkHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Test</title><link rel="canonical" href="https://example.org/"></head><body><main><h1>Test page</h1></main></body></html>`

func benchmarkWrite(b *testing.B, dir, name, body string) {
	b.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkCheckFolder(b *testing.B) {
	for _, large := range []bool{false, true} {
		b.Run(fmt.Sprint(large), func(b *testing.B) {
			dir := b.TempDir()
			body := benchmarkHTML
			if large {
				body = strings.Replace(body, "</main>", strings.Repeat(`<p class="text">A concrete paragraph about the contents of this page and its purpose.</p>`, 12000)+"</main>", 1)
			}
			body = strings.Replace(body, "</head>", `<link rel="stylesheet" href="a.css"></head>`, 1)
			for i := 0; i < 3; i++ {
				body = strings.Replace(body, "</main>", fmt.Sprintf(`<img src="image%d.png" width="1" height="1" alt="Sample"></main>`, i), 1)
				image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aL1sAAAAASUVORK5CYII=")
				benchmarkWrite(b, dir, fmt.Sprintf("image%d.png", i), string(image))
			}
			benchmarkWrite(b, dir, "index.html", body)
			var css strings.Builder
			for i := 0; i < 3000; i++ {
				fmt.Fprintf(&css, ".item%d{color:#123456;padding:1px}", i)
			}
			for i := 0; i < 3; i++ {
				fmt.Fprintf(&css, "@font-face{font-family:Font%d;src:url(font%d.woff2);font-display:swap}", i, i)
				benchmarkWrite(b, dir, fmt.Sprintf("font%d.woff2", i), "font header")
			}
			benchmarkWrite(b, dir, "a.css", css.String())
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				r, err := target.Read(context.Background(), dir, "bench")
				if err != nil {
					b.Fatal(err)
				}
				_ = check.Run(r.Document, check.DefaultRules(), nil)
			}
		})
	}
}

func BenchmarkCheckSite(b *testing.B) {
	dir := b.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../site")); err != nil {
		b.Fatal(err)
	}
	if _, err := compile.Compile(context.Background(), dir, compile.Options{Publish: true}); err != nil {
		b.Fatal(err)
	}
	output := filepath.Join(dir, "output")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r, err := target.Read(context.Background(), output, "bench")
		if err != nil {
			b.Fatal(err)
		}
		_ = check.Run(r.Document, check.DefaultRules(), nil)
	}
}
