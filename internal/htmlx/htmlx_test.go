package htmlx

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parse(t *testing.T, source string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func find(doc *html.Node, tag string) *html.Node {
	var found *html.Node
	Walk(doc, func(n *html.Node) {
		if found == nil && n.Data == tag {
			found = n
		}
	})
	return found
}

func render(t *testing.T, n *html.Node) string {
	t.Helper()
	var b bytes.Buffer
	if err := html.Render(&b, n); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestAttributes(t *testing.T) {
	a := find(parse(t, `<a href="/x" rel="Noopener  Icon" download>x</a>`), "a")
	if Attr(a, "href") != "/x" || Attr(a, "missing") != "" || Attr(nil, "href") != "" {
		t.Fatal("Attr")
	}
	if !HasAttr(a, "download") || HasAttr(a, "target") || HasAttr(nil, "href") {
		t.Fatal("HasAttr")
	}
	if !Rel(a, "icon") || !Rel(a, "noopener") || Rel(a, "no") {
		t.Fatal("Rel must match whole tokens case-insensitively")
	}
	SetAttr(a, "href", "/y")
	SetAttr(a, "title", "t")
	DelAttr(a, "download", "rel")
	if got := render(t, a); got != `<a href="/y" title="t">x</a>` {
		t.Fatal(got)
	}
}

func TestTextHelpers(t *testing.T) {
	doc := parse(t, `<p>One <b>two</b> three</p><script>let a = "<b>";</script><template><i>dormant</i></template>`)
	if got := Text(find(doc, "p")); got != "One two three" {
		t.Fatalf("Text %q", got)
	}
	if got := RawText(find(doc, "script")); got != `let a = "<b>";` {
		t.Fatalf("RawText %q", got)
	}
	p := find(doc, "p")
	SetText(p, "replaced")
	if render(t, p) != "<p>replaced</p>" {
		t.Fatal(render(t, p))
	}
	if !Dormant(find(doc, "i")) || Dormant(p) {
		t.Fatal("Dormant")
	}
}

func TestElementAndMove(t *testing.T) {
	doc := parse(t, `<div id=a></div><div id=b><span>s</span></div>`)
	var divs []*html.Node
	Walk(doc, func(n *html.Node) {
		if n.Data == "div" {
			divs = append(divs, n)
		}
	})
	Move(divs[0], find(doc, "span"))
	divs[0].AppendChild(Element("em", "new"))
	if got := render(t, divs[0]); got != `<div id="a"><span>s</span><em>new</em></div>` {
		t.Fatal(got)
	}
	if render(t, divs[1]) != `<div id="b"></div>` {
		t.Fatal(render(t, divs[1]))
	}
	if e := Element("br", ""); e.FirstChild != nil || e.DataAtom == 0 {
		t.Fatal("empty element")
	}
}
