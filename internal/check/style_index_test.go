package check

import (
	"strings"
	"testing"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

const indexDoc = `<!doctype html><html><body class="page dark"><main id="top" class="wrap">
<h1 class="title big">T</h1><p class="lede">a <a href="#x" class="btn primary">b</a></p>
<ul class="list"><li class="item">1</li><li class="item on" data-x="1">2</li></ul>
<svg viewBox="0 0 10 10"><linearGradient id="g"></linearGradient><circle class="dot" r="1"></circle></svg>
<template><p class="lede">hidden</p></template><section id="Top" class="Wrap">c</section>
<div class="a-b_c ünï">d</div><input type="checkbox" checked class="t"></main></body></html>`

func indexFixture(t testing.TB) (*html.Node, []*html.Node) {
	root, err := html.Parse(strings.NewReader(indexDoc))
	if err != nil {
		t.Fatal(err)
	}
	var elements []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			elements = append(elements, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return root, elements
}

func sameMatches(t testing.TB, root *html.Node, x *elementIndex, selector string) {
	sel, err := cascadia.Parse(selector)
	if err != nil {
		return
	}
	want, got := cascadia.QueryAll(root, sel), x.match(root, selector, sel)
	if len(want) != len(got) {
		t.Fatalf("%q: want %d matches, got %d", selector, len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("%q: match %d differs", selector, i)
		}
	}
}

func TestElementIndexMatchesQueryAll(t *testing.T) {
	root, elements := indexFixture(t)
	x := newElementIndex(elements)
	for _, selector := range []string{
		"p", "P", ".lede", "#top", "#Top", ".Wrap", "main .item", "ul > li.on", "li.item:not(.on)", ".item[data-x]",
		"li:nth-child(2)", "*.btn", "a.btn.primary", "body.dark h1.title", ":is(.lede, .item)", "lineargradient",
		"linearGradient", "svg .dot", "circle", ".a-b_c", ".ünï", "input:checked", "h1 + p", "h1 ~ ul", "section#Top.Wrap",
		"[class~=item]", "p a", "main>*", "*", ":root", "html body", ".missing", "#missing", "div.ünï",
		"li:is(.on)", "p:has(a)", "main :where(.lede)", ".item:first-child", "a[href^='#']",
	} {
		sameMatches(t, root, x, selector)
	}
}

func FuzzElementIndexMatchesQueryAll(f *testing.F) {
	for _, s := range []string{".item.on", "ul>li", "#top .lede a", ":is(p, li).item", "a[href]:not(.x)"} {
		f.Add(s)
	}
	root, elements := indexFixture(f)
	x := newElementIndex(elements)
	f.Fuzz(func(t *testing.T, selector string) {
		sameMatches(t, root, x, selector)
	})
}
