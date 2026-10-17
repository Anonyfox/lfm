package check

import (
	"strings"
	"testing"
)

func TestSyntax(t *testing.T) {
	for _, input := range []string{
		`<!doctype html><html><head><title>T</title></head><body><p>A</p></body></html>`,
		`<ul><li>one<li>two</ul>`, `<table><tr><td>A<td>B</table>`, `<p>one<p>two`,
		`<p>one<div>two</div>`, `<svg viewBox="0 0 10 10"><path d="M0 0"/></svg>`,
		`<math><mi>x</mi></math>`, `<x-card data-name=demo>Text</x-card>`,
		`<svg><text><![CDATA[A < B & C]]></text></svg>`,
		`<svg><foreignObject><p>HTML text</p></foreignObject></svg>`,
		`<div data-text='id="x" id="y"'></div>`,
		`<div id = "x" data-other='y' hidden></div>`,
		`<svg><circle cx="2" cy="2"/></svg>`, `<div></div >`,
		`<template><p>Text</p></template>`, `<noscript><p>Fallback</p></noscript>`,
		`<script>const x="<b>";</script><style>.x::after{content:"<i>"}</style>`,
	} {
		t.Run("valid/"+input, func(t *testing.T) {
			if issues := checkRule(t, "html.syntax", input); len(issues) != 0 {
				t.Fatalf("%+v", issues)
			}
		})
	}
	for _, input := range []string{
		`<div id=a id=b></div>`, `<div/>`, `<div><span></div>`, `<p><div>A</div></p>`,
		`<a href=x><a href=y>Link</a></a>`, `<table><div>A</div><tr><td>B</td></tr></table>`,
		`<head><img src=x><meta name=description content=x></head>`, `<head><iframe></iframe></head>`,
		`<input></input>`, `<div foo="oops`, `<!-- unclosed`, "<p>\x00</p>",
		`<html><head></head><body></body><body></body></html>`,
		`<div id="x"class="a"></div>`, `<div foo=bar=baz></div>`, `<div fo"o=x></div>`,
		`<svg id=a id=b></svg>`, `<svg><path id=a id=b/></svg>`, `<div></div class=x>`,
	} {
		t.Run("invalid/"+input, func(t *testing.T) {
			if issues := checkRule(t, "html.syntax", input); len(issues) == 0 {
				t.Fatal("missed invalid markup")
			}
		})
	}
}

func TestDocument(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int
	}{
		{`<!doctype html><meta charset=utf-8><title>T</title>`, 0},
		{`<meta charset=utf-8>`, 1}, {`<!doctype html><meta charset=windows-1252>`, 1},
		{`<!doctype html><meta charset=utf-8><meta charset=utf-8>`, 1},
		{`<!doctype html><!--` + strings.Repeat(" ", 1024) + `--><meta charset=utf-8>`, 1},
	} {
		if got := len(checkRule(t, "html.document", tc.input)); got != tc.want {
			t.Fatalf("%s: got %d want %d", tc.input, got, tc.want)
		}
	}
}

func TestContentModels(t *testing.T) {
	for _, input := range []string{
		`<ul><div>bad</div></ul>`, `<ul>bad</ul>`, `<li>orphan</li>`,
		`<button><input></button>`, `<a href=x><button>bad</button></a>`,
		`<picture><div>bad</div></picture>`, `<summary>orphan</summary>`,
		`<h1><div>Block</div></h1>`, `<span><div>Block</div></span>`,
	} {
		if len(checkRule(t, "html.content-model", input)) == 0 {
			t.Fatalf("missed %s", input)
		}
	}
	for _, input := range []string{
		`<ul><li>good</li></ul>`, `<dl><div><dt>T</dt><dd>D</dd></div></dl>`,
		`<details><summary>Title</summary><p>Body</p></details>`,
		`<a href=x><span>Link</span></a>`, `<picture><source srcset=x><img src=x></picture>`,
		`<svg><a href=x><text>Link</text></a></svg>`,
		`<button><template><button>Inert</button></template>Outer</button>`,
	} {
		if issues := checkRule(t, "html.content-model", input); len(issues) != 0 {
			t.Fatalf("%s: %+v", input, issues)
		}
	}
}

func TestIDsReferencesJSON(t *testing.T) {
	for _, input := range []string{`<p id=x></p><p id=x></p>`, `<p id="a b"></p>`, `<p id=""></p>`} {
		if len(checkRule(t, "html.ids", input)) == 0 {
			t.Fatal(input)
		}
	}
	for _, input := range []string{`<p id=ümlaut></p>`, `<p id="123"></p>`} {
		if len(checkRule(t, "html.ids", input)) != 0 {
			t.Fatal(input)
		}
	}
	for _, input := range []string{`<label for=missing>Name</label>`, `<div aria-labelledby="a missing"><p id=a>A</p></div>`, `<a href="#missing">X</a>`} {
		if len(checkRule(t, "html.references", input)) == 0 {
			t.Fatal(input)
		}
	}
	for _, input := range []string{`<p id="a:b"></p><a href="#a%3Ab">X</a>`, `<a href="#">X</a>`, `<a href="#top">Top</a>`, `<label for="a b">Name</label><input id="a b">`} {
		if issues := checkRule(t, "html.references", input); len(issues) != 0 {
			t.Fatalf("%s: %+v", input, issues)
		}
	}
	if len(checkRule(t, "html.json", `<script type=application/ld+json>{"x":1,}</script>`)) != 1 {
		t.Fatal("invalid JSON missed")
	}
	if len(checkRule(t, "html.json", `<script type=application/json>{"x":"\u003c/script\u003e"}</script>`)) != 0 {
		t.Fatal("valid JSON rejected")
	}
}
