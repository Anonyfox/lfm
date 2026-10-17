package fontcheck

import (
	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/urlcheck"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

type Source struct {
	URL    string
	Format string
	Local  bool
}
type Face struct {
	Offset                                       int
	Family, Display, Weight, Style, UnicodeRange string
	Sources                                      []Source
	Problems                                     []string
	Conditional                                  bool
}

type Stylesheet struct {
	Faces          []Face
	FamilyDefaults map[string]bool // a valid display policy may apply; no cascade claim
}

func Faces(raw []byte) ([]Face, error) { s, err := ReadCSS(raw); return s.Faces, err }

// Faces uses the shared CSS grammar/tokenizer. It observes every src declaration
// (fallback declarations can intentionally target different browser support),
// preserving order and nested media/supports/layer conditions. It never purges.
func ReadCSS(raw []byte) (Stylesheet, error) {
	in := parse.NewInputBytes(bytes.Clone(raw))
	defer in.Restore()
	p := css.NewParser(in, false)
	result := Stylesheet{FamilyDefaults: map[string]bool{}}
	var faces []Face
	var stack []string
	active, depth := -1, 0
	featureDepth := 0
	var featureFamilies []string
	var featureTokens []css.Token
	for {
		offset := in.Offset()
		kind, tokenType, data := p.Next()
		switch kind {
		case css.ErrorGrammar:
			result.Faces = faces
			if p.Err() == io.EOF {
				return result, nil
			}
			return result, p.Err()
		case css.BeginAtRuleGrammar, css.BeginRulesetGrammar:
			name := strings.ToLower(urlcheck.DecodeCSS(string(data)))
			if kind == css.BeginRulesetGrammar {
				name = "ruleset"
			}
			if name == "@font-face" && active < 0 {
				conditional := false
				for _, context := range stack {
					if context != "@layer" {
						conditional = true
					}
				}
				faces = append(faces, Face{Offset: offset, Conditional: conditional})
				active, depth = len(faces)-1, len(stack)+1
			}
			if name == "@font-feature-values" {
				featureDepth = len(stack) + 1
				featureFamilies = familyList(p.Values())
				featureTokens = nil
			}
			stack = append(stack, name)
		case css.EndAtRuleGrammar, css.EndRulesetGrammar:
			if len(stack) == featureDepth {
				if hasFamilyDisplay(featureTokens) {
					for _, family := range featureFamilies {
						result.FamilyDefaults[family] = true
					}
				}
				featureDepth = 0
				featureFamilies = nil
				featureTokens = nil
			}
			if active >= 0 && len(stack) == depth {
				active = -1
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case css.TokenGrammar:
			// parse/v2 intentionally returns token grammar for extensible
			// feature-value blocks. Reuse its token boundaries, retaining nesting.
			if featureDepth > 0 && len(stack) == featureDepth {
				featureTokens = append(featureTokens, css.Token{TokenType: tokenType, Data: bytes.Clone(data)})
			}
		case css.DeclarationGrammar:
			if active < 0 && featureDepth > 0 && len(stack) == featureDepth && strings.EqualFold(urlcheck.DecodeCSS(string(data)), "font-display") {
				var value strings.Builder
				for _, tok := range p.Values() {
					value.Write(tok.Data)
				}
				switch strings.ToLower(urlcheck.DecodeCSS(strings.TrimSpace(value.String()))) {
				case "auto", "block", "swap", "fallback", "optional":
					for _, family := range featureFamilies {
						result.FamilyDefaults[family] = true
					}
				}
			}
			if active < 0 || len(stack) != depth {
				continue
			}
			f := &faces[active]
			property := strings.ToLower(urlcheck.DecodeCSS(string(data)))
			var value strings.Builder
			for _, tok := range p.Values() {
				value.Write(tok.Data)
			}
			v := strings.TrimSpace(value.String())
			if strings.HasSuffix(strings.ToLower(v), "!important") {
				f.Problems = append(f.Problems, property+": !important is invalid in @font-face; descriptor ignored")
				continue
			}
			switch property {
			case "font-family":
				if validFamily(p.Values()) {
					f.Family = v
				} else {
					f.Problems = append(f.Problems, "invalid font-family descriptor; expected one custom family name")
				}
			case "font-display":
				display := strings.ToLower(urlcheck.DecodeCSS(v))
				switch display {
				case "auto", "block", "swap", "fallback", "optional":
					f.Display = display
				default:
					f.Problems = append(f.Problems, "invalid font-display value; descriptor ignored")
				}
			case "font-weight":
				f.Weight = v
			case "font-style":
				f.Style = v
			case "unicode-range":
				f.UnicodeRange = v
				if !ValidUnicodeRange(v) {
					f.Problems = append(f.Problems, "invalid unicode-range; browser may ignore the subset restriction")
				}
			case "src":
				f.Sources = append(f.Sources, sources(p.Values())...)
			}
		}
	}
}

func hasFamilyDisplay(tokens []css.Token) bool {
	var declaration []css.Token
	depth := 0
	valid := func() bool {
		if len(declaration) != 3 || declaration[0].TokenType != css.IdentToken || declaration[1].TokenType != css.ColonToken || declaration[2].TokenType != css.IdentToken {
			return false
		}
		if !strings.EqualFold(urlcheck.DecodeCSS(string(declaration[0].Data)), "font-display") {
			return false
		}
		switch strings.ToLower(urlcheck.DecodeCSS(string(declaration[2].Data))) {
		case "auto", "block", "swap", "fallback", "optional":
			return true
		}
		return false
	}
	for _, tok := range tokens {
		if tok.TokenType == css.LeftBraceToken {
			depth++
			declaration = nil
			continue
		}
		if tok.TokenType == css.RightBraceToken {
			if depth > 0 {
				depth--
			}
			declaration = nil
			continue
		}
		if depth > 0 {
			continue
		}
		if tok.TokenType == css.SemicolonToken {
			if valid() {
				return true
			}
			declaration = nil
			continue
		}
		if tok.TokenType != css.WhitespaceToken && tok.TokenType != css.CommentToken {
			declaration = append(declaration, tok)
		}
	}
	return valid()
}

func familyList(tokens []css.Token) []string {
	var groups [][]css.Token
	start := 0
	for j, t := range tokens {
		if t.TokenType == css.CommaToken {
			groups = append(groups, tokens[start:j])
			start = j + 1
		}
	}
	groups = append(groups, tokens[start:])
	var result []string
	for _, group := range groups {
		if !validFamily(group) {
			return nil
		}
		var v strings.Builder
		for _, tok := range group {
			v.Write(tok.Data)
		}
		result = append(result, FamilyKey(v.String()))
	}
	return result
}

func FamilyKey(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 1 && (v[0] == '\'' || v[0] == '"') {
		return strings.ToLower(decodeString(v))
	}
	return strings.ToLower(strings.Join(strings.Fields(urlcheck.DecodeCSS(v)), " "))
}

func validFamily(tokens []css.Token) bool {
	var names []css.Token
	for _, t := range tokens {
		if t.TokenType != css.WhitespaceToken && t.TokenType != css.CommentToken {
			names = append(names, t)
		}
	}
	if len(names) == 1 && names[0].TokenType == css.StringToken {
		return decodeString(string(names[0].Data)) != ""
	}
	if len(names) == 0 {
		return false
	}
	for _, t := range names {
		if t.TokenType != css.IdentToken {
			return false
		}
	}
	if len(names) == 1 {
		switch strings.ToLower(urlcheck.DecodeCSS(string(names[0].Data))) {
		case "inherit", "initial", "unset", "revert", "revert-layer", "default", "serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui", "emoji", "math", "fangsong", "ui-serif", "ui-sans-serif", "ui-monospace", "ui-rounded":
			return false
		}
	}
	return true
}

func sources(tokens []css.Token) []Source {
	var result []Source
	depth, last := 0, -1
	for j, tok := range tokens {
		switch tok.TokenType {
		case css.CommaToken:
			if depth == 0 {
				last = -1
			}
		case css.URLToken:
			if depth != 0 {
				continue
			}
			refs := urlcheck.CSSReferences(tok.Data, 0)
			if len(refs) == 1 {
				result = append(result, Source{URL: refs[0].Value})
				last = len(result) - 1
			}
		case css.FunctionToken:
			name := strings.ToLower(urlcheck.DecodeCSS(strings.TrimSuffix(string(tok.Data), "(")))
			if depth == 0 && name == "local" {
				result = append(result, Source{Local: true})
				last = len(result) - 1
			}
			if depth == 0 && name == "format" && last >= 0 && j+1 < len(tokens) {
				v := tokens[j+1]
				if v.TokenType == css.StringToken || v.TokenType == css.IdentToken {
					result[last].Format = decodeString(string(v.Data))
				}
			}
			depth++
		case css.LeftParenthesisToken:
			depth++
		case css.RightParenthesisToken:
			if depth > 0 {
				depth--
			}
		}
	}
	return result
}

func decodeString(v string) string {
	if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	return urlcheck.DecodeCSS(v)
}

// ValidUnicodeRange checks the descriptor's mechanical bounds, not glyph
// coverage or whether the page's future/dynamic text fits a subset.
func ValidUnicodeRange(value string) bool {
	for _, item := range strings.Split(strings.ToLower(value), ",") {
		s := strings.TrimSpace(item)
		if !strings.HasPrefix(s, "u+") {
			return false
		}
		s = s[2:]
		lo, hi, interval := strings.Cut(s, "-")
		if len(lo) == 0 || len(lo) > 6 {
			return false
		}
		if strings.Contains(lo, "?") {
			if interval {
				return false
			}
			first := strings.IndexByte(lo, '?')
			if strings.Trim(lo[first:], "?") != "" || len(lo)-first > 5 {
				return false
			}
			hi = strings.ReplaceAll(lo, "?", "f")
			lo = strings.ReplaceAll(lo, "?", "0")
		} else if !interval {
			hi = lo
		}
		if len(hi) == 0 || len(hi) > 6 {
			return false
		}
		// ParseUint accepts a leading +, which CSS hexadecimal codepoints don't.
		for _, c := range lo + hi {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
		a, e1 := strconv.ParseUint(lo, 16, 32)
		b, e2 := strconv.ParseUint(hi, 16, 32)
		if e1 != nil || e2 != nil || b < a || b > 0x10ffff {
			return false
		}
	}
	return true
}
