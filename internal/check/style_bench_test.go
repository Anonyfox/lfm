package check

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkStyles uses deterministic utility CSS, or an offline captured page.
func BenchmarkStyles(b *testing.B) {
	var body, css strings.Builder
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&body, `<div class="group"><span class="u%d md:flex w-1/2" data-label="a.b #c > d">Text</span></div>`, i%2000)
	}
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&css, `.group > .u%d.md\:flex[data-label]{color:red}.group .u%d:not(.off){display:block}`, i%2000, i%2000)
	}
	raw := []byte("<!doctype html><html><head><style>" + css.String() + "</style></head><body>" + body.String() + "</body></html>")
	options := Options{}
	real := os.Getenv("LFM_BENCH_PAGE")
	if real != "" {
		var err error
		raw, err = os.ReadFile(filepath.Join(real, "index.html"))
		if err != nil {
			b.Fatal(err)
		}
		d, err := Parse(raw, Options{})
		if err != nil {
			b.Fatal(err)
		}
		options.Stylesheets = map[string][]byte{}
		for _, n := range d.Elements {
			if n.Data == "link" && relHas(n, "stylesheet") {
				href := Attr(n, "href")
				data, err := os.ReadFile(filepath.Join(real, strings.TrimPrefix(href, "/")))
				if err != nil {
					b.Fatal(err)
				}
				options.Stylesheets[href] = data
			}
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if real != "" {
			if _, err := Analyze(raw, options); err != nil {
				b.Fatal(err)
			}
		} else {
			d, err := Parse(raw, options)
			if err != nil {
				b.Fatal(err)
			}
			d.styles()
		}
	}
}
