package check

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// Property: Analyze on any input of a few KB is deterministic, finishes well
// within a second and locates every diagnostic inside the input.
func FuzzAnalyzeBounded(f *testing.F) {
	for _, seed := range []string{"", `<!doctype html><html lang=en><title>T</title><p>hello`, `<svg><text>Hi</text></svg>`, `<script>let a="</script>"</script>`,
		strings.Repeat("<div>", 500), strings.Repeat(`<a href="#x" class="a b c">x</a>`, 60),
		`<style>` + strings.Repeat(".a .b > .c ~ .d { color: red; background: url(x.png) } ", 40) + `</style>` + strings.Repeat(`<p class="a b c d">x</p>`, 40),
		`<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@id":"#offer","@type":"Offer"},{"@id":"#offer","price":49,"priceCurrency":"EUR"}]}</script>`,
		`<meta property=og:url content="http://[::1]/"><meta name=robots content="nosnippet,max-snippet:0">`,
		`<fieldset disabled><legend><input autofocus></legend><label><input><input></label></fieldset><div aria-hidden=true><button tabindex=2>Go</button></div>`,
		`<div style="` + strings.Repeat("color:red;", 300) + `">x</div>`, `<table>` + strings.Repeat("<tr><td>", 200),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}
		start := time.Now()
		report, err := Analyze([]byte(input), Options{})
		if elapsed := time.Since(start); elapsed > slowdown*time.Second {
			t.Fatalf("Analyze took %v on %d bytes", elapsed, len(input))
		}
		again, err2 := Analyze([]byte(input), Options{})
		if !reflect.DeepEqual(report, again) || (err == nil) != (err2 == nil) {
			t.Fatal("Analyze is not deterministic")
		}
		for _, d := range report.Diagnostics {
			if d.Message == "" || d.Location.Offset < 0 || d.Location.Offset > len(input) {
				t.Fatalf("bad diagnostic %+v", d)
			}
		}
	})
}
