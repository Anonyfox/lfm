package htmlcompact

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func serializedHTML(t testing.TB, source string) []byte {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := html.Render(&out, doc); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestHTMLCompactionPreservesSemantics(t *testing.T) {
	for name, source := range map[string]string{
		"spacing": `<!doctype html><html lang="en"><body><span>A</span>  <span>B</span>
<div style="white-space:break-spaces">  Leading\tand  repeated
spaces  </div><pre>

  preformatted\t  text
</pre><textarea>

  textarea  text
</textarea><p>&nbsp; &amp; &#34; &#39; café 🐺</p></body></html>`,
		"attributes": `<html lang=en><body><input id="field" disabled="false" value="" data-present=""><div class="  one  two " data-entity="&amp;copy &amp;#34; &quot; ' &lt;" aria-label="  A  B  " title="a&#9;b&#10;c" data-special="=/&amp;&quot;'` + "`" + `&lt;&gt;">text</div><img src="./" alt=""><a href="https://example.com/?a=1&amp;b=2">link</a></body></html>`,
		"raw-text": `<title>  title  &amp; more </title><style id="css">.x::after {content:"  a  b  "} .x {white-space:pre-wrap}</style><script id="js">const value = "  a  b  "; /* kept here */
window.answer = value;</script><script type="application/json"> { "copy": "  a  b  ", "literal": "\u003c/script>" } </script><template id="tpl"><p data-copy="a  b">  inert  text  </p></template>`,
		"foreign":          `<svg viewBox="0 0 100 50" xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="paint"><stop offset="0" stop-color="red"/></linearGradient></defs><text xml:space="preserve">  A  B  </text><svg viewBox="0 0 2 2"><path d="M0 0h2v2z"/></svg><foreignObject><div class="  one  two ">  foreign  HTML  </div></foreignObject><use href="#paint"/></svg><math><mtext>  x  y  </mtext><annotation-xml encoding="text/html"><p data-value="a  b">  annotation  </p></annotation-xml></math>`,
		"comments":         `<!doctype html><!--marker--><html lang=en><body>A<!-- intentional separator -->B<!--[if IE]>legacy<![endif]--><!--#include virtual="header.html" --><div data-empty=""></div></body></html>`,
		"names":            `<div id="target" data-copy="&amp;quot;" aria-labelledby="label" class="a  b"><label id="label" for="field">Name</label><input id="field" name="answer" value="on"><a name="target" href="#target">Go</a></div>`,
		"fallback-content": `<noscript><p class="  one  two ">  no JS  </p><link rel="stylesheet" href="./full.css"></noscript><noembed><p title="  raw  ">unchanged</p></noembed><noframes><div id="raw">  raw  text  </div></noframes><div id="compacted">rest</div>`,
	} {
		t.Run(name, func(t *testing.T) {
			before := serializedHTML(t, source)
			original := bytes.Clone(before)
			after, report := Compact(before)
			if report.Skipped != "" {
				t.Fatal(report.Skipped)
			}
			if !bytes.Equal(before, original) {
				t.Fatal("input mutated")
			}
			a, _ := html.Parse(bytes.NewReader(before))
			b, _ := html.Parse(bytes.NewReader(after))
			if !sameHTMLTree(a, b) {
				t.Fatalf("DOM changed:\n%s\n%s", before, after)
			}
			a, _ = html.ParseWithOptions(bytes.NewReader(before), html.ParseOptionEnableScripting(false))
			b, _ = html.ParseWithOptions(bytes.NewReader(after), html.ParseOptionEnableScripting(false))
			if !sameHTMLTree(a, b) {
				t.Fatal("no-JavaScript DOM changed")
			}
			if report.AfterBytes != len(after) || report.SavedBytes != len(before)-len(after) || len(after) > len(before) {
				t.Fatal(report)
			}
			again, r := Compact(after)
			if !bytes.Equal(again, after) || r.Skipped != "" {
				candidate, _ := compactSyntax(after)
				t.Fatalf("not idempotent: %+v\nbefore %s\ncandidate %s", r, after, candidate)
			}
		})
	}
}

func TestHTMLCompactionChangesOnlySafeSyntax(t *testing.T) {
	before := serializedHTML(t, `<div id="box" data-empty="" title="He said &quot;hello&quot;" class="  a  b ">  untouched  </div><img src="./" alt="">`)
	after, r := Compact(before)
	if r.SavedBytes == 0 || r.Skipped != "" {
		t.Fatal(r)
	}
	for _, want := range []string{`id=box`, `data-empty`, `title='He said "hello"'`, `class="  a  b "`, `>  untouched  </div>`, `src=./ `} {
		if !bytes.Contains(after, []byte(want)) {
			t.Fatalf("missing %q: %s", want, after)
		}
	}
}

func TestHTMLCompactionFallsBackOnChangedDOM(t *testing.T) {
	raw := []byte(`<div class="  one  two ">  original  text  </div>`)
	candidate := []byte(`<div class="one two"> original text </div>`)
	after, r := acceptCompaction(raw, candidate, nil)
	if !bytes.Equal(raw, after) || r.Skipped == "" || r.SavedBytes != 0 {
		t.Fatal(r)
	}
}

func FuzzHTMLCompaction(f *testing.F) {
	for _, seed := range []string{`<p title="a&quot;b"> a  b </p>`, `<svg><svg></svg><text xml:space="preserve">  x  </text></svg>`, `<textarea>\n  x </textarea>`, `<script>"</script>"</script>`, `<p class="a  b">&amp;#34; &copy x</p>`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		// The production pass receives html.Render output, not arbitrary source.
		if len(source) > 64*1024 {
			t.Skip()
		}
		doc, err := html.Parse(strings.NewReader(source))
		if err != nil {
			return
		}
		var rendered bytes.Buffer
		if html.Render(&rendered, doc) != nil {
			return
		}
		before := bytes.Clone(rendered.Bytes())
		after, _ := Compact(rendered.Bytes())
		a, _ := html.Parse(bytes.NewReader(before))
		b, _ := html.Parse(bytes.NewReader(after))
		if !sameHTMLTree(a, b) || !bytes.Equal(before, rendered.Bytes()) || len(after) > len(before) {
			t.Fatal("compaction changed semantics/input or increased bytes")
		}
	})
}

func BenchmarkHTMLCompaction(b *testing.B) {
	for _, size := range []struct {
		name    string
		repeats int
	}{{"100Blocks", 100}, {"2000Blocks", 2000}} {
		b.Run(size.name, func(b *testing.B) {
			raw := serializedHTML(b, `<html lang="en"><body>`+strings.Repeat(`<p class="one  two" data-empty="" title="Example"> A  <span id="value">B</span>  C </p>`+"\n", size.repeats)+`</body></html>`)
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				Compact(raw)
			}
		})
	}
}
