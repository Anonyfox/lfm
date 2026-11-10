package compile

import (
	_ "embed"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"golang.org/x/net/html"
)

//go:embed footer.css
var footerCSS string

func (b *builder) footer(doc *html.Node) {
	f := b.config.Footer
	if f.Disabled {
		return
	}
	var body, head *html.Node
	var own bool
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "body":
				body = n
			case "head":
				head = n
			case "footer":
				own = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if body == nil {
		return
	}
	if own {
		b.notice("footer.duplicate", "Page has an authored footer and the built-in footer is enabled.", "Remove the authored footer and configure footer in lfm.json, or set footer to false; see 'lfm help config footer'.")
	}
	element := func(parent *html.Node, tag, class, text string) *html.Node {
		n := htmlx.Element(tag, text)
		if class != "" {
			htmlx.SetAttr(n, "class", "fox-footer"+class)
		}
		parent.AppendChild(n)
		return n
	}
	footer := htmlx.Element("footer", "")
	htmlx.SetAttr(footer, "class", "fox-footer")
	var anchor *html.Node
	for c := body.LastChild; c != nil; c = c.PrevSibling {
		if c.Type == html.CommentNode || (c.Type == html.TextNode && strings.TrimSpace(c.Data) == "") {
			continue
		}
		if c.Type == html.ElementNode && c.Data == "script" {
			anchor = c
			continue
		}
		break
	}
	if anchor != nil {
		body.InsertBefore(footer, anchor)
	} else {
		body.AppendChild(footer)
	}
	htmlx.SetAttr(footer, "data-design-reason", "Built-in site navigation and attribution.")
	inner := element(footer, "div", "__inner", "")
	if f.Wordmark != "" || f.Tagline != "" || len(f.Groups) > 0 {
		top := element(inner, "div", "__top", "")
		if f.Wordmark != "" || f.Tagline != "" {
			brand := element(top, "div", "__brand", "")
			if f.Wordmark != "" {
				element(brand, "p", "__wordmark", f.Wordmark)
			}
			if f.Tagline != "" {
				element(brand, "p", "__tagline", f.Tagline)
			}
		}
		if len(f.Groups) > 0 {
			links := element(top, "nav", "__links", "")
			htmlx.SetAttr(links, "aria-label", "Footer")
			for _, g := range f.Groups {
				group := element(links, "div", "__group", "")
				element(group, "p", "__label", g.Label)
				for _, l := range g.Links {
					a := element(group, "a", "", l.Text)
					htmlx.SetAttr(a, "href", b.config.URLs[l.URLKey])
					if l.Lang != "" {
						htmlx.SetAttr(a, "lang", l.Lang)
					}
				}
			}
		}
	}
	if len(f.Badges) > 0 {
		badges := element(inner, "div", "__badges", "")
		for _, badge := range f.Badges {
			a := element(badges, "a", "", "")
			htmlx.SetAttr(a, "href", b.config.URLs[badge.URLKey])
			if badge.Target != "" {
				htmlx.SetAttr(a, "target", badge.Target)
			}
			if badge.Rel != "" {
				htmlx.SetAttr(a, "rel", badge.Rel)
			}
			img := element(a, "img", "", "")
			img.Attr = []html.Attribute{
				{Key: "src", Val: b.config.URLs[badge.SrcKey]},
				{Key: "alt", Val: badge.Alt},
				{Key: "width", Val: strconv.Itoa(badge.Width)},
				{Key: "height", Val: strconv.Itoa(badge.Height)},
			}
		}
	}
	bottom := element(inner, "div", "__bottom", "")
	craft := element(bottom, "p", "__craft", "crafted with ")
	htmlx.SetAttr(element(craft, "a", "", "lfm"), "href", "https://lfm.cash/")
	credit := element(bottom, "p", "__credit", "Made with ")
	htmlx.SetAttr(credit, "data-slop-skip", "Owner-required verbatim attribution: Made with ♥ by Anonyfox e. K.")
	heart := element(credit, "span", "__heart", "♥")
	htmlx.SetAttr(heart, "role", "img")
	htmlx.SetAttr(heart, "aria-label", "love")
	credit.AppendChild(&html.Node{Type: html.TextNode, Data: " by "})
	htmlx.SetAttr(element(credit, "a", "", "Anonyfox e. K."), "href", "https://anonyfox.com/")
	accent, typ, muted := f.Accent, f.Type, f.Muted
	if accent == "" {
		accent = "#fff"
	}
	if typ == "" {
		typ = "monospace"
	}
	if muted == "" {
		muted = "#aaa"
	}
	css := ".fox-footer{--footer-accent:" + accent + ";--footer-type:" + typ + ";--footer-muted:" + muted + ";--footer-columns:" + strconv.Itoa(max(1, len(f.Groups))) + "}" + footerCSS
	if head != nil {
		head.AppendChild(htmlx.Element("style", css))
	}
}
