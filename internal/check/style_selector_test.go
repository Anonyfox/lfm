package check

import (
	"slices"
	"testing"

	"github.com/andybalholm/cascadia"
)

const selectorFixture = `<!doctype html><html><body><main id="App" class="group a"><div id="x" class="a a md:flex w-1/2" data-label="a.b #c > d ] (" data-other="x'y"><span class="b c">Text</span><span class="a b" id="dup"></span></div><div class="b" id="dup"><span class="c"></span></div><p class="a+b a b" data-label="plain"></p><svg><linearGradient id="paint" class="b"></linearGradient></svg><xÄ class="a&#160;b"></xÄ></main><footer class="a a" class="a" id="dup" id="dup"></footer></body></html>`

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
	for _, s := range []string{"xÄ", `.a\a0 b`, "*", `:contains("Text")`, `:matches("Text")`, `:contains("")`, "div", "DIV", "linearGradient", "#App", "#app", "#dup", ".a", ".a.b", ".group .b", "main > div.a > span.b", "div + div.b", "div ~ p.a", ".md\\:flex", ".w-1\\/2", ".a\\+b", `.\61 `, `#\41 pp`, `.a[data-label="a.b #c > d ] ("]`, `[data-label="a.b #c > d ] ("]`, `[data-other="x'y"].a`, ".a:not(.b)", ":not(.x)", ":is(.a,.b)", ":where(.a,.b)", ".a:is(.a,.b)", ".a:not(:is(.x,.y))", "div:has(> .b)", "div:nth-child(2)", "[data-label]", ":root", ":empty", "svg|linearGradient", "*|div", "|div", ".a\n>\t.b", ".a\r.b", ".a\f.b"} {
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
	} {
		kind, name := keySelector(tc.selector)
		if kind != tc.kind || name != tc.name {
			t.Errorf("%s: %c %q; want %c %q", tc.selector, kind, name, tc.kind, tc.name)
		}
	}
}
