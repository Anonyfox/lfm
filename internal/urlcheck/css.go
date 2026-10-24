package urlcheck

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

// CSSReferences delegates token boundaries to parse/v2. String-valued @import
// and image-set candidates are URLs; content strings and namespace IDs are not.
func CSSReferences(raw []byte, offset int) []Reference {
	input := parse.NewInputBytes(bytes.Clone(raw))
	lexer := css.NewLexer(input)
	var refs []Reference
	importing, namespace := false, false
	depth, imageSet := 0, 0
	for {
		start := input.Offset()
		kind, data := lexer.Next()
		if kind == css.ErrorToken {
			break
		}
		switch kind {
		case css.AtKeywordToken:
			importing = strings.EqualFold(string(data), "@import")
			namespace = strings.EqualFold(string(data), "@namespace")
		case css.SemicolonToken:
			importing = false
			namespace = false
		case css.FunctionToken:
			depth++
			name := strings.ToLower(cssUnescape(strings.TrimSuffix(string(data), "(")))
			if name == "image-set" || name == "-webkit-image-set" || name == "src" {
				imageSet = depth
			}
		case css.LeftParenthesisToken:
			depth++
		case css.RightParenthesisToken:
			if imageSet == depth {
				imageSet = 0
			}
			if depth > 0 {
				depth--
			}
		}
		isImport := importing
		var value string
		position := start
		if kind == css.URLToken && !namespace {
			text := string(data)
			opening := strings.IndexByte(text, '(')
			if opening < 0 {
				continue
			}
			inner := strings.TrimSuffix(text[opening+1:], ")")
			value = strings.TrimLeft(inner, cssSpace)
			position += opening + 1 + len(inner) - len(value)
			value = strings.TrimRight(value, cssSpace)
			if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
				value = value[1 : len(value)-1]
				position++
			}
			importing = false
		} else if kind == css.StringToken && (importing || imageSet > 0) && !namespace {
			value = string(data)
			if len(value) >= 2 && value[len(value)-1] == value[0] {
				value = value[1 : len(value)-1]
			} else {
				value = value[1:] // unterminated at end of input
			}
			position++
			importing = false
		} else {
			continue
		}
		refs = append(refs, Reference{Tag: "style", Attribute: "CSS URL", Value: cssUnescape(value), Offset: offset + position, End: offset + position + len(value), Resource: true, Import: isImport})
	}
	return refs
}

const cssSpace = " \t\r\n\f"

// CSS escaped-code-point decoding, after the library has validated/tokenized
// the string or URL. HTML entity decoding does not apply inside <style> text.
func cssUnescape(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		if s[i] == '\r' || s[i] == '\n' || s[i] == '\f' {
			if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			continue
		}
		start := i
		for i < len(s) && i-start < 6 && (s[i] >= '0' && s[i] <= '9' || s[i] >= 'a' && s[i] <= 'f' || s[i] >= 'A' && s[i] <= 'F') {
			i++
		}
		if i > start {
			n, _ := strconv.ParseUint(s[start:i], 16, 32)
			r := rune(n)
			if r == 0 || r > utf8.MaxRune || r >= 0xd800 && r <= 0xdfff {
				r = utf8.RuneError
			}
			out.WriteRune(r)
			if i < len(s) && strings.ContainsRune(" \t\r\n\f", rune(s[i])) {
				if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
					i++
				}
			} else {
				i--
			}
		} else {
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

// DecodeCSS decodes an already-tokenized CSS identifier/string. Token boundary
// validation belongs to the CSS lexer; HTML entities do not apply here.
func DecodeCSS(s string) string { return cssUnescape(s) }
