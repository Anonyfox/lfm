package check

import (
	"slices"
	"testing"

	"github.com/andybalholm/cascadia"
)

const selectorFixture = `<!doctype html><html><body><main id="App" class="group a"><div id="x" class="a a md:flex w-1/2" data-label="a.b #c > d ] (" data-other="x'y"><span class="b c">Text</span><span class="a b" id="dup"></span></div><div class="b" id="dup"><span class="c"></span></div><p class="a+b a b" data-label="plain"></p><svg viewBox="0 0 1 1"><linearGradient id="paint" class="b"></linearGradient></svg><xÄ class="a&#160;b"></xÄ></main><footer class="a a" class="a" id="dup" id="dup"></footer></body></html>`

func assertIndexedSelector(t testing.TB, selector string) {
	t.Helper()
	sel, err := cascadia.Parse(selector)
	if err != nil {
		return
	}
	d, err := Parse([]byte(selectorFixture), Options{})
	if err != nil {
		t.Fatal(err)
	}
	index := newElementIndex(d.Elements)
	got := index.match(d.Root, selector, sel)
	want := cascadia.QueryAll(d.Root, sel)
	if !slices.Equal(got, want) {
		t.Fatalf("%q key=%v: indexed %v, query %v", selector, index.keys[selector], got, want)
	}
}

func TestIndexedSelectorsAgree(t *testing.T) {
	for _, s := range []string{".missing .b", ".group > .a .c", ".missing + .b .c", ".group > div + div .c", ".group :is(.a,.b) .c", ".group :not(.missing) .c", "#App .b", "#app .b", "MAIN > DIV .c", ".md\\:flex .c", ".a\\+b .c", ".group .b ", "xÄ", `.a\a0 b`, "*", `:contains("Text")`, `:matches("Text")`, `:contains("")`, "div", "DIV", "linearGradient", "#App", "#app", "#dup", ".a", ".a.b", ".group .b", "main > div.a > span.b", "div + div.b", "div ~ p.a", ".md\\:flex", ".w-1\\/2", ".a\\+b", `.\61 `, `#\41 pp`, `.a[data-label="a.b #c > d ] ("]`, `[data-label="a.b #c > d ] ("]`, `[data-other="x'y"].a`, ".a:not(.b)", ":not(.x)", ":is(.a,.b)", ":where(.a,.b)", ".a:is(.a,.b)", ".a:not(:is(.x,.y))", "div:has(> .b)", "div:nth-child(2)", "[data-label]", ":root", ":empty", "svg|linearGradient", "*|div", "|div", ".a\n>\t.b", ".a\r.b", ".a\f.b", ".group [data-label]", ".missing [data-label]", "[DATA-LABEL]", "[viewBox]", "[viewbox]", "svg [id]", "[xlink|href]", ".group *", ".missing *", ".group>:last-child", ".missing>:last-child", "main:not(.x) span", ".group:hover .c", ".missing:hover .c", ".group:not(.group) .c", "div.b + p .c", "div + div span", ".a + .b span", ".b ~ p", "#x ~ p", "#x > span ~ span", "main [data-other] span", "[data-label] span", "main > [data-label] > .b", ":is(main) .c", "main :is(.missing, .b) .c", "main::before .c", ":root .c", "html .c", "HTML BODY .c", ".group > div > span.c", ".group div div span", `[data-label!="plain"]`, `[DATA-LABE!="0"]`} {
		t.Run(s, func(t *testing.T) { assertIndexedSelector(t, s) })
	}
	for _, s := range splitSelectors(`.a::before, .b::after, .group > .md\:flex:hover`) {
		s, _, _ = stripPseudo(s)
		assertIndexedSelector(t, s)
	}
}

func FuzzIndexedSelectors(f *testing.F) {
	for _, s := range []string{".a.b", `.md\:flex`, `.a[data-label="a.b #c > d ] ("]`, ":not(.x)", ":is(.a,.b)", "div > span", "*", `.\61 `} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			t.Skip()
		}
		assertIndexedSelector(t, s)
	})
}

func TestRequiredSelectorKeys(t *testing.T) {
	for _, tc := range []struct {
		selector string
		kind     byte
		name     string
	}{
		{`.group > .md\:flex:not(.off)`, '.', "md:flex"},
		{`.group .w-1\/2`, '.', "w-1/2"},
		{`.a[data-label=".b #c > d ] ("]`, '.', "a"},
		{`:not(.x)`, 0, ""},
		{`:is(.a,.b)`, 0, ""},
		{`:where(.a,.b).c`, '.', "c"},
		{`DIV:not(.x)`, 't', "div"},
		{`main > #App.a`, '#', "App"},
		{`.group [data-Label="x"]`, '[', "data-label"},
		{`div[data-x]`, '[', "data-x"},
		{`[xlink|href]`, 0, ""},
		{`:not([data-x])`, 0, ""},
		{`[data-x!="y"]`, 0, ""},
	} {
		kind, name := keySelector(tc.selector)
		if kind != tc.kind || name != tc.name {
			t.Errorf("%s: %c %q; want %c %q", tc.selector, kind, name, tc.kind, tc.name)
		}
	}
}

func TestAncestorKeys(t *testing.T) {
	keys := func(parts ...string) keySet {
		var k keySet
		for _, p := range parts {
			k = k.union(keyBits(p[0], p[1:]))
		}
		return k
	}
	for _, tc := range []struct {
		selector string
		want     keySet
	}{
		{".a .b", keys(".a")},
		{"main > .a[data-x] .b", keys("tmain", ".a")},
		{".a:not(.x) [data-y]", keys(".a")},
		{".a + .b .c", keys(".b")},
		{".a .b ~ .c", keys(".a")},
		{"#App:hover > *", keys("#App")},
		{"DIV .x", keys("tdiv")},
		{":is(.a) .b", keySet{}},
		{".a .b, .c", keySet{}},
		{"svg|g .b", keySet{}},
		{".a", keySet{}},
	} {
		if got := ancestorKeys(tc.selector); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.selector, got, tc.want)
		}
	}
}

func TestAncestorFilterWithPartialIndex(t *testing.T) {
	d, err := Parse([]byte(selectorFixture), Options{})
	if err != nil {
		t.Fatal(err)
	}
	sel, err := cascadia.Parse(".group > div .c")
	if err != nil {
		t.Fatal(err)
	}
	spans := cascadia.QueryAll(d.Root, cascadia.MustCompile("span"))
	index := newElementIndex(spans)
	if got, want := index.match(d.Root, ".group > div .c", sel), cascadia.QueryAll(d.Root, sel); !slices.Equal(got, want) {
		t.Fatalf("indexed %v, query %v", got, want)
	}
}
