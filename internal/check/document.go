package check

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/urlcheck"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

type Token struct {
	Type        html.TokenType
	Value       html.Token
	Offset, End int
}

type Document struct {
	Raw            []byte
	Root           *html.Node
	Elements       []*html.Node
	Tokens         []Token
	Options        Options
	Stats          Stats
	positions      map[*html.Node]int
	lines          []int
	linkRefs       []urlcheck.Reference
	linkAnalyzer   *urlcheck.Analyzer
	jsonldAnalyzed bool
	jsonldFindings []Diagnostic
	jsonldExpanded []ldBlock
	commerceDone   bool
	commerceIssues []Diagnostic
	fontAnalyzed   bool
	fontFindings   []Diagnostic
	fragmentIDs    map[string]*html.Node
	fragmentNames  map[string]*html.Node
	accessibility  *accessibilityFacts
	styleModel     *styleModel
	designFacts    *designFacts
	sheets         []styleSource
	keyframeIndex  map[string]keyframe
	ownText        []copySegment // visible text blocks without quotations
	allText        []copySegment // visible text blocks with quotations
}

func Parse(raw []byte, options Options) (*Document, error) {
	if options.File == "" {
		options.File = "index.html"
	}
	root, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("final HTML: %w", err)
	}
	tokens, err := tokenize(raw)
	if err != nil {
		return nil, err
	}
	d := &Document{Raw: raw, Root: root, Tokens: tokens, Options: options, positions: map[*html.Node]int{}, lines: lineStarts(raw)}
	// Final compiler output is serialized HTML; start tags and DOM preorder align.
	// For malformed standalone fixtures only matched tags receive a source position.
	starts := map[string][]int{}
	for _, token := range tokens {
		if token.Type == html.StartTagToken || token.Type == html.SelfClosingTagToken {
			starts[token.Value.Data] = append(starts[token.Value.Data], token.Offset)
		}
	}
	var visit func(*html.Node, int)
	visit = func(n *html.Node, depth int) {
		if n.Type == html.ElementNode {
			d.Elements = append(d.Elements, n)
			if offsets := starts[strings.ToLower(n.Data)]; len(offsets) > 0 {
				d.positions[n] = offsets[0]
				starts[strings.ToLower(n.Data)] = offsets[1:]
			}
			depth++
			if depth > d.Stats.MaxDepth {
				d.Stats.MaxDepth = depth
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, depth)
		}
	}
	visit(root, 0)
	d.Stats.HTMLBytes = len(raw)
	d.Stats.ExternalCSSBytes = options.ExternalCSSBytes
	d.Stats.ExternalJSBytes = options.ExternalJSBytes
	d.Stats.Elements = len(d.Elements)
	d.Stats.Embeds = scanEmbeds(raw, options.InlineSources)
	for _, e := range d.Stats.Embeds {
		d.Stats.EmbeddedBytes += e.TotalBytes
	}
	for _, n := range d.Elements {
		if n.Data == "style" {
			d.Stats.CSSBytes += len(Text(n))
		}
		if n.Data == "script" {
			kind := strings.ToLower(strings.TrimSpace(Attr(n, "type")))
			if executable(kind) {
				d.Stats.JSBytes += len(Text(n))
			}
		}
	}
	return d, nil
}

// Query supports Cascadia's CSS selector syntax and returns selector errors.
func (d *Document) Query(selector string) ([]*html.Node, error) {
	matcher, err := cascadia.Compile(selector)
	if err != nil {
		return nil, err
	}
	return cascadia.QueryAll(d.Root, matcher), nil
}

// selectNodes is only used with constant selectors owned/tested by this package.
func (d *Document) selectNodes(selector string) []*html.Node {
	nodes, err := d.Query(selector)
	if err != nil {
		panic(err)
	}
	return nodes
}

func inTemplate(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Data == "template" && p.Namespace == "" {
			return true
		}
	}
	return false
}

func (d *Document) activeNodes(selector string) []*html.Node {
	var active []*html.Node
	for _, n := range d.selectNodes(selector) {
		if !inTemplate(n) {
			active = append(active, n)
		}
	}
	return active
}

func (d *Document) At(n *html.Node, severity Severity, message, hint string) Diagnostic {
	offset := d.positions[n]
	return Diagnostic{Severity: severity, Location: location(d.Options.File, d.lines, offset), Element: elementLabel(n), Message: message, Hint: hint}
}

func (d *Document) Whole(severity Severity, message, hint string) Diagnostic {
	return d.At(nil, severity, message, hint)
}

func Attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func HasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

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

func elementLabel(n *html.Node) string {
	if n == nil {
		return ""
	}
	label := n.Data
	if id := Attr(n, "id"); id != "" {
		label += "#" + id
	} else if class := strings.Fields(Attr(n, "class")); len(class) > 0 {
		label += "." + class[0]
	}
	if len(label) > 120 {
		label = label[:120]
	}
	return label
}

func lineStarts(raw []byte) []int {
	starts := []int{0}
	for i, c := range raw {
		if c == '\n' || c == '\r' && (i+1 == len(raw) || raw[i+1] != '\n') {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func location(file string, lines []int, offset int) Location {
	line := sort.Search(len(lines), func(i int) bool { return lines[i] > offset }) - 1
	if line < 0 {
		line = 0
	}
	return Location{File: file, Line: line + 1, Column: offset - lines[line] + 1, Offset: offset}
}

// tokenize splits raw into tokens with their byte ranges. Only what the rules
// read is materialized: tag names (interned, they repeat), the text of text
// tokens that are not just whitespace, and complete doctype tokens. Copying
// every attribute and unescaping every text would dominate the check's cost.
func tokenize(raw []byte) ([]Token, error) {
	z := html.NewTokenizer(bytes.NewReader(raw))
	tokens := make([]Token, 0, len(raw)/32)
	names := map[string]string{}
	offset := 0
	for {
		kind := z.Next()
		end := offset + len(z.Raw())
		if kind == html.ErrorToken {
			if z.Err() != io.EOF {
				return nil, z.Err()
			}
			if end > offset {
				tokens = append(tokens, Token{Type: kind, Offset: offset, End: end})
			}
			return tokens, nil
		}
		t := Token{Type: kind, Offset: offset, End: end}
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken, html.EndTagToken:
			name, _ := z.TagName()
			interned, ok := names[string(name)]
			if !ok {
				interned = string(name)
				names[interned] = interned
			}
			t.Value = html.Token{Type: kind, Data: interned}
		case html.TextToken:
			if text := z.Text(); len(bytes.TrimSpace(text)) > 0 {
				t.Value = html.Token{Type: kind, Data: string(text)}
			}
		case html.DoctypeToken:
			t.Value = z.Token()
		}
		tokens = append(tokens, t)
		offset = end
	}
}

func executable(kind string) bool {
	return kind == "" || kind == "module" || kind == "text/javascript" || kind == "application/javascript"
}

func validUTF8(raw []byte) bool { return utf8.Valid(raw) && !bytes.ContainsRune(raw, 0) }
