package urlcheck

import (
	htmltemplate "html/template"
	"reflect"
	"strings"
	"testing"
)

func TestReferenceExtraction(t *testing.T) {
	raw := `<!-- <a href="https://ignored.example"> -->
 <a data-example="href='https://ignored.example'" HREF='https://example.org/?a=1&amp;b=2' ping="https://ping.one/ https://ping.two/">go</a>
 <picture><source srcset="data:image/png;base64,aGVsbG8= 1x, https://example.org/image.webp 2x"></picture>
 <form action="/submit"><button formaction="?preview=1">Preview</button></form>
 <svg xmlns="http://www.w3.org/2000/svg"><use xlink:href="#icon"/><a href="https://svg.example/">svg link</a></svg>
 <meta property="og:url" content="https://example.org/">
 <meta property="og:image" content="https://example.org/social.webp">
 <script>const x='<a href="https://ignored.example">';</script>
 <script type="application/ld+json">{"@context":{"url":"https://schema.org/url","@vocab":"https://schema.org/"},"@type":"https://schema.org/Product","url":"https://json.example/","sameAs":["https://social.example/"]}</script>`
	refs := AllReferences([]byte(raw))
	var values []string
	for _, ref := range refs {
		values = append(values, ref.Value)
		if ref.Offset < 0 || ref.End > len(raw) || ref.Offset > ref.End {
			t.Fatal(ref)
		}
	}
	want := []string{"https://example.org/?a=1&b=2", "https://ping.one/", "https://ping.two/", "data:image/png;base64,aGVsbG8=", "https://example.org/image.webp", "/submit", "?preview=1", "#icon", "https://svg.example/", "https://example.org/", "https://example.org/social.webp", "https://json.example/", "https://social.example/"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("%q", values)
	}
	// The first href offset is the attribute's value, not another attribute/tag.
	at := refs[0].Offset
	if !strings.HasPrefix(raw[at:], "https://example.org/?a=1&amp;b=2") {
		t.Fatal(at, raw[at:])
	}
}

func TestPlatformJSONReferencesAndBindings(t *testing.T) {
	for _, mime := range []string{"application/json", "application/vnd.discord.component-embed+json"} {
		raw := `<script id="discord:component-embed" type="` + mime + `">{"component":{"type":17,"components":[{"type":10,"content":"[Visit](https://example.com/)"},{"type":2,"url":"https://example.com/?a=1&b=2"}]}}</script>`
		refs := AllReferences([]byte(raw))
		if len(refs) != 1 || refs[0].Value != "https://example.com/?a=1&b=2" {
			t.Fatal("component prose was parsed as a structural URL", refs)
		}
	}
	for _, raw := range []string{
		`<meta name=fc:miniapp content='{"imageUrl":"{{.urls.image}}","button":{"action":{"url":"{{.urls.result}}"}}}'>`,
		`<script id="discord:component-embed" type="application/json">{"component":{"url":"{{.urls.result}}"}}</script>`,
	} {
		files := map[string][]byte{"page": []byte(raw)}
		parsed, err := htmltemplate.New("page").Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if issues := Template(parsed, files); len(issues) != 0 {
			t.Fatal("named JSON interpolation mistaken for literal", issues)
		}
	}
	refs := References([]byte(`<meta property=fc:miniapp content='{"imageUrl":"https://example.com/card.png","button":{"action":{"url":"https://example.com/?a=1&amp;b=2"}}}'>`))
	if len(refs) != 2 || refs[1].Value != "https://example.com/?a=1&b=2" {
		t.Fatal("nested attribute decoding failed", refs)
	}
}
func TestCandidatesCommasAndDescriptors(t *testing.T) {
	raw := `one.webp 1x, data:image/svg+xml,%3Csvg,%3E 2x, two.webp, three.webp 400w`
	candidates := Candidates(raw)
	if len(candidates) != 4 || candidates[1].URL != "data:image/svg+xml,%3Csvg,%3E" || candidates[2].URL != "two.webp" || candidates[3].Descriptor != "400w" {
		t.Fatal(candidates)
	}
	for _, c := range candidates {
		if raw[c.Offset:c.Offset+len(c.URL)] != c.URL {
			t.Fatal(c)
		}
	}
}
func TestTemplateOwnershipUsesParseTrees(t *testing.T) {
	tests := []struct {
		source string
		want   int
	}{
		{`<a href="https://example.org/">go</a>`, 1},
		{`<a href="{{.urls.checkout}}">go</a>`, 0},
		{`<a href='{{printf "%s" .urls.checkout}}'>go</a>`, 0},
		{`<a href='{{"https://example.org/"}}'>go</a>`, 1},
		{`<a href="#part">part</a>`, 0},
		{`<a href="../other/">other</a>`, 1},
		{`<a href="assets/brochure.pdf">download</a>`, 0},
		{`<img src="assets/image.webp"><img src="./image.webp"><script src="assets/scripts/main.js"></script>`, 0},
		{`<img src="https://example.org/image.webp">`, 1},
		{`<svg xmlns="http://www.w3.org/2000/svg"><use href="#icon"/></svg>`, 0},
		{`{{if .off}}<a href="https://dormant.example/">go</a>{{end}}`, 1},
		{`<a href="https://{{.data.host}}/">go</a>`, 1},
		{`<!-- <a href="https://ignored.example/"> --><script>const x='https://ignored.example/';</script><pre>&lt;a href="https://example.org"&gt;</pre>`, 0},
		{`<meta property="og:url" content="https://example.org/">`, 1},
		{`<script type="application/ld+json">{"@context":"https://schema.org","url":"https://example.org/"}</script>`, 1},
		{`{{- if .on -}}<a href="https://example.org/">{{ .title }}</a>{{- end -}}`, 1},
		{`<a href={{.urls.checkout}}>go</a>`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			template, err := htmltemplate.New("index.html.tmpl").Parse(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			issues := Template(template, map[string][]byte{"index.html.tmpl": []byte(tc.source)})
			if len(issues) != tc.want {
				t.Fatalf("want %d: %+v", tc.want, issues)
			}
		})
	}
}
func TestLiteralPartials(t *testing.T) {
	main := `{{template "card" .}}`
	partial := `{{define "card"}}<a href="https://partial.example/">go</a>{{end}}`
	template := htmltemplate.Must(htmltemplate.New("index.html.tmpl").Parse(main))
	htmltemplate.Must(template.New("partials/card").Parse(partial))
	issues := Template(template, map[string][]byte{"index.html.tmpl": []byte(main), "partials/card": []byte(partial)})
	if len(issues) != 1 || issues[0].Location.File != "partials/card" {
		t.Fatal(issues)
	}
}
func FuzzHTMLReferences(f *testing.F) {
	for _, seed := range []string{`<a href="https://example.org/">go</a>`, `<img srcset="data:x,abc 1x, image 2x">`, `<script type=application/ld+json>{"url":"https://x.example/"}</script>`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 8192 {
			t.Skip()
		}
		for _, ref := range AllReferences([]byte(raw)) {
			if ref.Offset < 0 || ref.End < ref.Offset || ref.End > len(raw) {
				t.Fatalf("bad source span %+v", ref)
			}
		}
	})
}

func TestCSSReferences(t *testing.T) {
	raw := `@namespace url(http://www.w3.org/2000/svg); @import "https://cdn.example/base.css"; .hero{background:URL("ht\74 ps://cdn.example/image.webp");content:"https://not-a-resource.example/";background-image:image-set("https://cdn.example/one.webp" 1x,url(https://cdn.example/two.webp) 2x)}`
	refs := CSSReferences([]byte(raw), 0)
	var values []string
	for _, ref := range refs {
		values = append(values, ref.Value)
	}
	want := []string{"https://cdn.example/base.css", "https://cdn.example/image.webp", "https://cdn.example/one.webp", "https://cdn.example/two.webp"}
	if !reflect.DeepEqual(values, want) {
		t.Fatal(values)
	}
	refs = AllReferences([]byte(`<style>.x{background:url(https://bad.example:99999/image)}</style><div style="background:url('https://cdn.example/?a=1&amp;b=2')"></div>`))
	if len(refs) != 2 || refs[1].Value != "https://cdn.example/?a=1&b=2" {
		t.Fatal(refs)
	}
}

func TestOrdinaryJSONStringsAreNotURLs(t *testing.T) {
	refs := JSONReferences([]byte(`{"title":"Question: why?","prompt":"Note: use 1:2","appUri":"myapp:item/1","@context":{"url":"https://schema.org/url"}}`), 0)
	if len(refs) != 1 || refs[0].Value != "myapp:item/1" {
		t.Fatal(refs)
	}
}
func TestCSSLiteralTemplateRules(t *testing.T) {
	for _, tc := range []struct {
		source string
		count  int
	}{
		{`<style>.hero{background:url('https://cdn.example/image')}</style>`, 1},
		{`<div style="background:url('https://cdn.example/image')"></div>`, 1},
		{`<div style="background:url('{{.urls.image}}')"></div>`, 0},
		{`<div style="background:url('https://{{.data.host}}/image')"></div>`, 1},
		{`<style>.hero{background:url(assets/images/hero.webp)}</style>`, 0},
	} {
		template, err := htmltemplate.New("index.html.tmpl").Parse(tc.source)
		if err != nil {
			t.Fatal(err)
		}
		issues := Template(template, map[string][]byte{"index.html.tmpl": []byte(tc.source)})
		if len(issues) != tc.count {
			t.Fatal(tc.source, issues)
		}
	}
}

func TestVisibleURLCopyAdvisory(t *testing.T) {
	source := `<p>Visit https://example.org for more.</p><pre>https://code.example/</pre><code>https://code.example/</code><script>"https://code.example/"</script><!-- https://ignored.example/ -->`
	positions := VisibleURLLiterals([]byte(source))
	if len(positions) != 1 {
		t.Fatal(positions)
	}
	template := htmltemplate.Must(htmltemplate.New("index.html.tmpl").Parse(source))
	issues := Template(template, map[string][]byte{"index.html.tmpl": []byte(source)})
	if len(issues) != 1 || issues[0].Rule != "links.registry" || issues[0].Severity != "warn" {
		t.Fatal(issues)
	}
}

func TestRefreshDestinations(t *testing.T) {
	for _, tc := range []struct {
		value, want string
		ok          bool
	}{
		{`0; URL='https://example.org/next'`, "https://example.org/next", true},
		{`10, /next`, "/next", true}, {`  .5 ; url = "https://example.org/"`, "https://example.org/", true},
		{`0; usual-path`, "usual-path", true}, {`0; URLish`, "URLish", true},
		{`10`, "", false}, {`-1; url=https://example.org`, "", false},
	} {
		got, _, ok := RefreshURL(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Fatal(tc.value, got, ok)
		}
	}
	refs := References([]byte(`<meta http-equiv=refresh content="0;url=https://example.org:99999/">`))
	if len(refs) != 1 || refs[0].Value != "https://example.org:99999/" {
		t.Fatal(refs)
	}
}

func FuzzTemplateURLSources(f *testing.F) {
	for _, seed := range []string{`<a href="{{.urls.checkout}}">Pay</a>`, `{{if .on}}<a href="https://example.org">go</a>{{end}}`, `<style>.x{background:url('{{.urls.image}}')}</style>`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 8192 {
			t.Skip()
		}
		template, err := htmltemplate.New("index.html.tmpl").Parse(raw)
		if err != nil {
			t.Skip()
		}
		for _, finding := range Template(template, map[string][]byte{"index.html.tmpl": []byte(raw)}) {
			if finding.Location.Offset < 0 || finding.Location.Offset > len(raw) || finding.Message == "" || finding.Hint == "" {
				t.Fatal(finding)
			}
		}
	})
}

func TestRelativeStructuredURLsAndIdentifiers(t *testing.T) {
	refs := JSONReferences([]byte(`{"url":"../other/","@id":"#organization","sameAs":["/profile/"],"appUri":"myapp:item/1","urlKey":"checkout","@graph":[{"@id":"_:b1"}]}`), 0)
	if len(refs) != 4 {
		t.Fatal(refs)
	}
}
