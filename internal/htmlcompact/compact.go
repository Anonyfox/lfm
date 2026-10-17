// Package htmlcompact shrinks serialized HTML syntax without touching text,
// comments or raw content, and proves every result by comparing parsed trees.
package htmlcompact

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/tdewolff/parse/v2"
	htmllex "github.com/tdewolff/parse/v2/html"
	"golang.org/x/net/html"
)

// Result reports what compaction did. Skipped explains why the input was kept.
type Result struct {
	BeforeBytes int    `json:"beforeBytes"`
	AfterBytes  int    `json:"afterBytes"`
	SavedBytes  int    `json:"savedBytes"`
	Skipped     string `json:"skipped,omitempty"`
}

// Compact syntax, never text. Even indentation can become visible through CSS
// white-space, copied text, inline layout or later script changes. The general
// minify/html KeepWhitespace option still collapses it and changes DOM values.
// Reuse the existing lexer and attribute escaper, without those transformations.
// Comments, raw text and closing/document tags pass verbatim. Attribute
// names/case and namespaces are never normalized here.
func Compact(raw []byte) ([]byte, Result) {
	candidate, err := compactSyntax(raw)
	return acceptCompaction(raw, candidate, err)
}

func acceptCompaction(raw, candidate []byte, err error) ([]byte, Result) {
	report := Result{BeforeBytes: len(raw), AfterBytes: len(raw)}
	if err == nil && !bytes.Equal(raw, candidate) {
		// Guard every build, not just the regression corpus. Compare complete
		// reparsed trees including text, attributes, namespaces and comments.
		var before, after *html.Node
		before, err = html.Parse(bytes.NewReader(raw))
		if err == nil {
			after, err = html.Parse(bytes.NewReader(candidate))
		}
		if err == nil && !sameHTMLTree(before, after) {
			err = fmt.Errorf("HTML syntax compaction changed the parsed DOM")
		}
	}
	if err != nil {
		report.Skipped = err.Error()
		return raw, report
	}
	if len(candidate) >= len(raw) {
		return raw, report
	}
	report.AfterBytes = len(candidate)
	report.SavedBytes = len(raw) - len(candidate)
	return candidate, report
}

func compactSyntax(raw []byte) ([]byte, error) {
	// The HTML5 tokenizer knows every raw/RCDATA element, including noscript,
	// noembed and noframes. Never reinterpret their contents as HTML syntax.
	z := html.NewTokenizer(bytes.NewReader(raw))
	var out bytes.Buffer
	out.Grow(len(raw))
	foreign := 0 // depth inside SVG/MathML, where "/>" does close an element
	for {
		kind := z.Next()
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			tag, err := compactTag(z.Raw(), rawNameLength(z.Raw()))
			if err != nil {
				return nil, err
			}
			name, _ := z.TagName()
			if kind == html.SelfClosingTagToken && foreign == 0 && voidElements[string(name)] {
				tag = dropSelfClosing(tag)
			}
			if kind == html.StartTagToken && (string(name) == "svg" || string(name) == "math") {
				foreign++
			}
			out.Write(tag)
		} else {
			if kind == html.EndTagToken && foreign > 0 {
				if name, _ := z.TagName(); string(name) == "svg" || string(name) == "math" {
					foreign--
				}
			}
			out.Write(z.Raw())
		}
		if kind == html.ErrorToken {
			if z.Err() != io.EOF {
				return nil, z.Err()
			}
			return out.Bytes(), nil
		}
	}
}

// The slash on an HTML void element ("<br />") is ignored by parsers.
var voidElements = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}

// dropSelfClosing turns a compacted "…/>" into "…>". An unquoted value right
// before the slash stays intact: in "a=b/>" the slash would be part of it, so
// only the marker and the space that separated it go.
func dropSelfClosing(tag []byte) []byte {
	trimmed := bytes.TrimSuffix(tag, []byte("/>"))
	if len(trimmed) == len(tag) {
		return tag
	}
	return append(bytes.TrimRight(trimmed, " \t\n\r\f"), '>')
}

// rawNameLength measures the tag name as written. TagName is no substitute:
// it lowercases and replaces NUL with a longer U+FFFD.
func rawNameLength(tag []byte) int {
	n := 1
	for n < len(tag) && strings.IndexByte(" \t\n\f\r/>", tag[n]) < 0 {
		n++
	}
	return n - 1
}

func compactTag(raw []byte, nameLength int) ([]byte, error) {
	// Lexers may lowercase their backing input; keep the original untouched.
	part := bytes.Clone(raw)
	// Attribute lexing only: SVG/MathML must not activate the lexer's opaque
	// XML mode. The HTML5 tokenizer already delimited this exact opening tag.
	for i := 1; i <= nameLength; i++ {
		part[i] = 'x'
	}
	input := parse.NewInputBytes(part)
	defer input.Restore()
	lexer := htmllex.NewLexer(input)
	var out bytes.Buffer
	out.Grow(len(raw))
	var escaped []byte
	lastUnquoted := false
	for {
		offset := input.Offset()
		kind, data := lexer.Next()
		if kind == htmllex.ErrorToken {
			if lexer.Err() != io.EOF {
				return nil, lexer.Err()
			}
			// Preserve trailing bytes, including whitespace at end of document.
			out.Write(raw[offset:])
			return out.Bytes(), nil
		}
		// The lexer discards markup whitespace before '>' or '/>'; other
		// tokens retain it. Anchor to the end rather than assuming a partition.
		end := input.Offset()
		original := raw[end-len(data) : end]
		switch kind {
		case htmllex.AttributeToken:
			key := bytes.TrimLeft(original, " \t\n\r\f")[:len(lexer.AttrKey())]
			out.WriteByte(' ')
			out.Write(key)
			value := lexer.AttrVal()
			lastUnquoted = false
			if value == nil {
				continue
			}
			quote := byte(0)
			if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
				quote, value = value[0], value[1:len(value)-1]
			}
			if len(value) == 0 {
				continue // a present empty attribute still has the exact value "".
			}
			// These are the quote entities emitted by html.Render. Decode only
			// one level, then choose the cheaper delimiter. Keep all other
			// entities encoded: text-mode unescaping is wrong in attributes.
			if bytes.Contains(value, []byte("&#")) {
				value = bytes.ReplaceAll(value, []byte("&#34;"), []byte{'"'})
				value = bytes.ReplaceAll(value, []byte("&#39;"), []byte{'\''})
			}
			value = htmllex.EscapeAttrVal(&escaped, value, quote, false)
			out.WriteByte('=')
			out.Write(value)
			lastUnquoted = value[0] != '"' && value[0] != '\''
		case htmllex.StartTagVoidToken:
			// Retain the self-closing marker (also meaningful in foreign
			// content). Without this space an unquoted value consumes '/'.
			if lastUnquoted {
				out.WriteByte(' ')
			}
			out.Write(original)
			lastUnquoted = false
		default:
			out.Write(original)
			lastUnquoted = false
		}
	}
}

func sameHTMLTree(a, b *html.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type != b.Type || a.Namespace != b.Namespace || a.Data != b.Data || len(a.Attr) != len(b.Attr) {
		return false
	}
	for i := range a.Attr {
		if a.Attr[i] != b.Attr[i] {
			return false
		}
	}
	x, y := a.FirstChild, b.FirstChild
	for x != nil && y != nil {
		if !sameHTMLTree(x, y) {
			return false
		}
		x, y = x.NextSibling, y.NextSibling
	}
	return x == nil && y == nil
}
