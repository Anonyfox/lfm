package check

import (
	"strings"
	"testing"
)

func TestGroundRule(t *testing.T) {
	for _, tc := range []struct {
		name, css, html string
		want            int
	}{
		{"gradient colour layer", "body{background:linear-gradient(white,transparent) #faf7f2}", "", 1},
		{"gradient image", "body{background:linear-gradient(#faf7f2,white)}", "", 0},
		{"broad bands", "body{background:repeating-linear-gradient(0deg,black 0px 40px,white 40px 80px)}", "", 0},
		{"ruled shorthand", "body{background:linear-gradient(black 1px,transparent 1px) 0 0/100% 20px}", "", 1},
		{"named ivory", "body{background:ivory}", "", 1},
		{"cream", "body{background:#f5f0e6}", "", 1},
		{"var shorthand", ":root{--ground:#faf7f2}body{background:var(--ground)}", "", 1},
		{"inline", "", `<main style="background:#faf9f5" data-design-reason="brand">x</main>`, 1},
		{"inert template", "", `<template><main style="background:#faf7f2">x</main></template>`, 0},
		{"hidden main", "", `<main hidden style="background:#faf7f2">x</main>`, 0},
		{"white", "body{background:#fff}", "", 0},
		{"dark", "body{background:#121212}", "", 0},
		{"cool", "body{background:#f1f3f0}", "", 0},
		{"saturated", "body{background:#fef3c7}", "", 0},
		{"dark alternate", "body{background:white}@media(prefers-color-scheme:dark){body{background:#faf7f2}}", "", 0},
		{"light default", "body{background:#faf7f2}@media(prefers-color-scheme:dark){body{background:black}}", "", 1},
		{"cascade", "body{background:#faf7f2}body{background:white}", "", 0},
		{"wrapper", "#app{background:#faf7f2}", `<div id="app">content</div>`, 1},
		{"floating badge", ".badge{position:absolute;top:1rem;background:repeating-linear-gradient(black 1px,white 2px)}", `<div class="badge">x</div>`, 0},
		{"small", ".badge{background:repeating-linear-gradient(0deg,transparent 0 1px,black 1px 2px)}", `<span class="badge">x</span>`, 0},
		{"nonrepeating image", "body{background-image:linear-gradient(black 1px,transparent 1px);background-size:100% 20px;background-repeat:no-repeat}", "", 0},
		{"lines", "body{background:repeating-linear-gradient(0deg,transparent 0 20px,black 20px 21px)}", "", 1},
		{"paper", "body{background:url(paper.webp)}", "", 1},
		{"overlay", ".overlay{position:fixed;inset:0;background:url(grain.png)}", `<div class="overlay"></div>`, 1},
		{"pseudo variables", ":root{--texture:url(noise.svg)}body::before{position:fixed;inset:0;background:var(--texture)}", "", 1},
		{"pseudo own variables", "body::before{--texture:url(noise.svg);position:fixed;inset:0;background:var(--texture)}", "", 1},
		{"inline important", "body{background:white!important}", `<main style="background:#faf7f2!important">x</main>`, 1},
		{"pseudo", "body::before{position:fixed;inset:0;background:url(noise.svg)}", "", 1},
		{"positioned small pseudo", ".badge{position:relative}.badge::before{position:absolute;inset:0;background:url(paper.svg)}", `<span class="badge">x</span>`, 0},
		{"absolute page pseudo", "body{position:relative}body::after{position:absolute;inset:0;background:url(grain.svg)}", "", 1},
		{"sized overlay", ".overlay{position:fixed;width:100vw;height:100vh;background:url(noise.svg)}", `<aside class="overlay"></aside>`, 1},
		{"inset sides", ".overlay{position:fixed;top:0;right:0;bottom:0;left:0;background:url(noise.svg)}", `<aside class="overlay"></aside>`, 1},
		{"small fixed", ".overlay{position:fixed;width:10px;height:10px;background:url(noise.svg)}", `<aside class="overlay"></aside>`, 0},
		{"small pseudo", ".badge::before{background:url(paper.svg)}", `<span class="badge">x</span>`, 0},
		{"noise", "body{background:url('data:image/svg+xml,%3Csvg%3E%3CfeTurbulence/%3E%3C/svg%3E')}", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `<!doctype html><html><head><style>` + tc.css + `</style></head><body>` + tc.html + `</body></html>`
			got := checkRule(t, "design.ground", raw)
			if len(got) != tc.want {
				t.Fatalf("want %d: %+v", tc.want, got)
			}
			for _, d := range got {
				if d.Severity != Error || strings.Contains(d.Hint, "data-design-reason") {
					t.Fatalf("%+v", d)
				}
			}
		})
	}
}

func FuzzGroundRule(f *testing.F) {
	f.Add(`body{background:var(--ground,#faf7f2)}body::before{position:fixed;inset:0;background:url(noise.svg)}`)
	f.Add(`body{background:url('data:image/svg+xml;base64,PHN2Zz48ZmVUdXJidWxlbmNlLz48L3N2Zz4=')}`)
	f.Fuzz(func(t *testing.T, css string) {
		if len(css) > 4096 {
			t.Skip()
		}
		d, err := Parse([]byte(`<style>`+css+`</style><body><main>x</main>`), Options{})
		if err == nil {
			groundRule(d)
		}
	})
}

func TestGroundTextureFilesAndData(t *testing.T) {
	for _, image := range []string{`url(texture.svg)`, `url('data:image/svg+xml;base64,PHN2Zz48ZmVUdXJidWxlbmNlLz48L3N2Zz4=')`} {
		d, _ := Parse([]byte(`<body style="background-image:`+image+`">`), Options{Assets: map[string][]byte{"texture.svg": []byte(`<svg><filter><feTurbulence/></filter></svg>`)}})
		if got := groundRule(d); len(got) != 1 {
			t.Fatalf("%s: %+v", image, got)
		}
	}
}

func TestGroundCanBeDisabled(t *testing.T) {
	d, _ := Parse([]byte(`<body style="background:#faf7f2" data-design-reason="brand">`), Options{})
	if got := Run(d, []Rule{testRule("design.ground", groundRule)}, map[string]Severity{"design.ground": Off}); got.Errors != 0 || len(got.Diagnostics) != 0 {
		t.Fatal(got)
	}
}

func TestNotebookAndParchmentGroundTextures(t *testing.T) {
	for _, tc := range []struct{ name, declarations string }{
		{"horizontal repeating rules", `background-image:repeating-linear-gradient(to bottom,transparent 0 23px,#b8cddd 23px 24px)`},
		{"horizontal sized rules", `background-image:linear-gradient(#b8cddd 1px,transparent 1px);background-size:100% 24px;background-repeat:repeat`},
		{"notebook margin and rules", `background-image:linear-gradient(90deg,transparent 39px,#e89999 39px 40px,transparent 40px),repeating-linear-gradient(0deg,transparent 0 23px,#b8cddd 23px 24px)`},
		{"squared paper", `background-image:linear-gradient(#b8cddd 1px,transparent 1px),linear-gradient(90deg,#b8cddd 1px,transparent 1px);background-size:24px 24px;background-repeat:repeat`},
		{"repeating grid", `background-image:repeating-linear-gradient(0deg,transparent 0 23px,#b8cddd 23px 24px),repeating-linear-gradient(90deg,transparent 0 23px,#b8cddd 23px 24px)`},
		{"parchment image", `background-image:url(parchment.webp)`},
		{"paper image", `background-image:url(handmade-paper.png)`},
		{"SVG noise data", `background-image:url('data:image/svg+xml,%3Csvg%3E%3Cfilter%3E%3CfeTurbulence/%3E%3C/filter%3E%3C/svg%3E')`},
		{"SVG noise file", `background-image:url(texture.svg)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, small := range []bool{false, true} {
				selector, contents, want := "body", "", 1
				if small {
					selector, contents, want = ".sample", `<span class="sample">Sample chart</span>`, 0
				}
				raw := `<!doctype html><html><head><style>` + selector + `{` + tc.declarations + `}</style></head><body>` + contents + `</body></html>`
				d, err := Parse([]byte(raw), Options{Assets: map[string][]byte{"texture.svg": []byte(`<svg><filter><feTurbulence/></filter></svg>`)}})
				if err != nil {
					t.Fatal(err)
				}
				got := groundRule(d)
				if len(got) != want {
					t.Fatalf("small=%v want %d findings: %+v", small, want, got)
				}
				for _, finding := range got {
					if finding.Severity != Error {
						t.Fatalf("%+v", finding)
					}
				}
			}
		})
	}
}
