package htmlcompact

import (
	"bytes"
	"testing"

	"golang.org/x/net/html"
)

// Property: on arbitrary input Compact either returns the input unchanged or
// a smaller document that parses to the same tree, with and without scripting.
func FuzzCompactArbitrary(f *testing.F) {
	for _, seed := range []string{`<p title="a&quot;b"> a  b </p>`, `<svg><svg></svg><text xml:space="preserve">  x  </text></svg>`, "<textarea>\n  x </textarea>", `<script>"</script>"</script>`,
		`<div id="x" class='a' data-x=1/>`, `<a href=x/ >`, `<noscript><p class="a">x</p></noscript>`, `<input value=a&#34;b>`, `<p a="&#39;" b='&#34;'>`, "<b\x00 c=\"d\">", `<x =a>`, `<math><mi a="b"/></math>`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 16*1024 {
			t.Skip()
		}
		raw := []byte(source)
		after, report := Compact(raw)
		if !bytes.Equal(raw, []byte(source)) {
			t.Fatal("input mutated")
		}
		if report.BeforeBytes != len(raw) || report.AfterBytes != len(after) || report.SavedBytes != len(raw)-len(after) {
			t.Fatalf("inconsistent report %+v", report)
		}
		if again, _ := Compact(raw); !bytes.Equal(again, after) {
			t.Fatal("not deterministic")
		}
		if bytes.Equal(after, raw) {
			return
		}
		if len(after) >= len(raw) {
			t.Fatal("compaction did not shrink")
		}
		for _, scripting := range []bool{true, false} {
			a, err := html.ParseWithOptions(bytes.NewReader(raw), html.ParseOptionEnableScripting(scripting))
			b, err2 := html.ParseWithOptions(bytes.NewReader(after), html.ParseOptionEnableScripting(scripting))
			if err != nil || err2 != nil || !sameHTMLTree(a, b) {
				t.Fatalf("scripting=%v: DOM changed\n%q\n%q", scripting, raw, after)
			}
		}
	})
}
