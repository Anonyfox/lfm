// Package htmlx holds the small x/net/html node operations shared by the
// compiler, the document checks and the release audits.
package htmlx

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Attr returns the value of an attribute, or "" when absent or n is nil.
func Attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// HasAttr reports whether n carries the attribute, even when empty.
func HasAttr(n *html.Node, key string) bool {
	if n == nil {
		return false
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

// SetAttr sets or appends an attribute.
func SetAttr(n *html.Node, key, value string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
}

// DelAttr removes every listed attribute.
func DelAttr(n *html.Node, keys ...string) {
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		remove := false
		for _, key := range keys {
			if a.Key == key {
				remove = true
				break
			}
		}
		if !remove {
			kept = append(kept, a)
		}
	}
	n.Attr = kept
}

// Element creates an HTML element, optionally with one text child.
func Element(name, text string) *html.Node {
	n := &html.Node{Type: html.ElementNode, Data: name, DataAtom: atom.Lookup([]byte(name))}
	if text != "" {
		n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
	}
	return n
}

// RawText concatenates the direct children's data. It is meant for raw-text
// elements such as script and style, whose content is a single text node.
func RawText(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(c.Data)
	}
	return b.String()
}

// Text concatenates all descendant text nodes.
func Text(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}

// SetText replaces all children with one text node.
func SetText(n *html.Node, value string) {
	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}
	n.AppendChild(&html.Node{Type: html.TextNode, Data: value})
}

// Move reparents n as the last child of parent.
func Move(parent, n *html.Node) {
	if n.Parent != nil {
		n.Parent.RemoveChild(n)
	}
	parent.AppendChild(n)
}

// Walk visits every element in document order.
func Walk(n *html.Node, visit func(*html.Node)) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode {
		visit(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		Walk(c, visit)
	}
}

// HasToken reports whether a space-separated attribute (such as rel) contains
// token, case-insensitively.
func HasToken(n *html.Node, key, token string) bool {
	for _, value := range strings.Fields(Attr(n, key)) {
		if strings.EqualFold(value, token) {
			return true
		}
	}
	return false
}

// Rel reports whether n's rel attribute contains token.
func Rel(n *html.Node, token string) bool { return HasToken(n, "rel", token) }

// Dormant reports whether n sits inside an HTML <template>, whose content does
// not render or execute until a script instantiates it.
func Dormant(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "template" && p.Namespace == "" {
			return true
		}
	}
	return false
}
