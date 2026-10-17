package check

import (
	"bytes"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/urlcheck"
	"github.com/andybalholm/cascadia"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"golang.org/x/net/html"
)

// The style model is a deliberately small cascade for design heuristics. It reads
// every final stylesheet once, matches selectors with Cascadia and resolves the
// desktop/base context: specificity, source order, !important, inline style,
// inheritance of typographic properties and simple var() lookups. It is not a
// browser: layout, media evaluation and computed geometry stay out of scope.

type cssDecl struct {
	prop, value string
	important   bool
}

type cssRule struct {
	selector string       // authored complex selector
	state    string       // dynamic pseudo-class stripped from the selector (hover, focus…)
	pseudo   string       // pseudo-element stripped from the selector (before, after…)
	narrow   bool         // inside a max-width-only media query
	alt      bool         // print, reduced motion, dark scheme or other alternate context
	decls    []cssDecl
	order    int
	spec     cascadia.Specificity
	sel      cascadia.Sel
	sheet    *html.Node // inline <style> or <link> owning the rule
	file     string     // emitted stylesheet URL for linked sheets
	raw      []byte
	offset   int
	matches  []*html.Node
}

type styleModel struct {
	rules    []*cssRule
	base     map[*html.Node][]*cssRule
	computed map[*html.Node]map[string]string
	inline   map[*html.Node][]cssDecl
	unknown  bool // a linked stylesheet was not available to the validator
}

func (d *Document) styles() *styleModel {
	if d.styleModel != nil {
		return d.styleModel
	}
	m := &styleModel{base: map[*html.Node][]*cssRule{}, computed: map[*html.Node]map[string]string{}, inline: map[*html.Node][]cssDecl{}}
	order := 0
	for _, n := range d.activeNodes("style, link") {
		if n.Namespace != "" {
			continue
		}
		var raw []byte
		file := ""
		if n.Data == "style" {
			if kind := strings.ToLower(strings.TrimSpace(Attr(n, "type"))); kind != "" && kind != "text/css" {
				continue
			}
			raw = []byte(Text(n))
		} else {
			if !relHas(n, "stylesheet") {
				continue
			}
			file = Attr(n, "href")
			var ok bool
			if raw, ok = d.Options.Stylesheets[file]; !ok {
				m.unknown = true
				continue
			}
		}
		for _, r := range readRules(raw) {
			r.order, r.sheet, r.file, r.raw = order, n, file, raw
			order++
			m.rules = append(m.rules, r)
		}
	}
	for _, n := range d.Elements {
		if v := Attr(n, "style"); v != "" {
			m.inline[n] = inlineDecls(v)
		}
	}
	for _, r := range m.rules {
		sel, err := cascadia.Parse(r.selector)
		if err != nil {
			continue
		}
		r.sel, r.spec = sel, sel.Specificity()
		r.matches = cascadia.QueryAll(d.Root, sel)
		if r.state == "" && r.pseudo == "" && !r.alt && !r.narrow {
			for _, n := range r.matches {
				m.base[n] = append(m.base[n], r)
			}
		}
	}
	for n, rules := range m.base {
		sort.SliceStable(rules, func(i, j int) bool {
			if rules[i].spec.Less(rules[j].spec) != rules[j].spec.Less(rules[i].spec) {
				return rules[i].spec.Less(rules[j].spec)
			}
			return rules[i].order < rules[j].order
		})
		m.base[n] = rules
	}
	d.styleModel = m
	return m
}

func readRules(raw []byte) []*cssRule {
	in := parse.NewInputBytes(bytes.Clone(raw))
	defer in.Restore()
	p := css.NewParser(in, false)
	type ctx struct {
		name, prelude string
		rule          []*cssRule
	}
	var stack []ctx
	var rules []*cssRule
	var pending strings.Builder
	skip := func() (narrow, alt, keyframes bool) {
		for _, c := range stack {
			switch c.name {
			case "@keyframes", "@-webkit-keyframes", "@font-face", "@font-feature-values", "@page", "@property", "@counter-style":
				keyframes = true
			case "@media":
				q := strings.ToLower(c.prelude)
				if strings.Contains(q, "print") || strings.Contains(q, "reduce") || strings.Contains(q, "dark") || strings.Contains(q, "hover:none") || strings.Contains(q, "pointer:coarse") {
					alt = true
				}
				if strings.Contains(q, "max-width") && !strings.Contains(q, "min-width") {
					narrow = true
				}
			}
		}
		return
	}
	for {
		offset := in.Offset()
		kind, _, data := p.Next()
		switch kind {
		case css.ErrorGrammar:
			if p.Err() == io.EOF {
				return rules
			}
			return rules
		case css.QualifiedRuleGrammar:
			pending.WriteString(tokensText(p.Values()))
			pending.WriteString(",")
		case css.BeginAtRuleGrammar:
			stack = append(stack, ctx{name: strings.ToLower(string(data)), prelude: tokensText(p.Values())})
		case css.BeginRulesetGrammar:
			pending.WriteString(tokensText(p.Values()))
			narrow, alt, keyframes := skip()
			var group []*cssRule
			if !keyframes {
				for _, s := range splitSelectors(pending.String()) {
					r := &cssRule{narrow: narrow, alt: alt, offset: offset}
					r.selector, r.state, r.pseudo = stripPseudo(s)
					if r.selector == "" {
						r.selector = "*"
					}
					group = append(group, r)
				}
			}
			pending.Reset()
			rules = append(rules, group...)
			stack = append(stack, ctx{name: "ruleset", rule: group})
		case css.EndAtRuleGrammar, css.EndRulesetGrammar:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case css.DeclarationGrammar, css.CustomPropertyGrammar:
			if len(stack) == 0 || stack[len(stack)-1].name != "ruleset" {
				continue
			}
			prop := strings.ToLower(urlcheck.DecodeCSS(string(data)))
			if kind == css.CustomPropertyGrammar {
				prop = string(data)
			}
			value := strings.TrimSpace(tokensText(p.Values()))
			important := false
			if lower := strings.ToLower(value); strings.HasSuffix(lower, "!important") {
				important, value = true, strings.TrimSpace(value[:len(value)-len("!important")])
			}
			for _, r := range stack[len(stack)-1].rule {
				r.decls = append(r.decls, cssDecl{prop, value, important})
			}
		}
	}
}

func tokensText(tokens []css.Token) string {
	var b strings.Builder
	for _, t := range tokens {
		b.Write(t.Data)
	}
	return b.String()
}

func splitSelectors(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				if part := strings.TrimSpace(s[start:i]); part != "" {
					out = append(out, part)
				}
				start = i + 1
			}
		}
	}
	if part := strings.TrimSpace(s[start:]); part != "" {
		out = append(out, part)
	}
	return out
}

var dynamicPseudo = []string{":hover", ":focus-visible", ":focus-within", ":focus", ":active", ":visited", ":target", ":checked"}

// stripPseudo removes trailing pseudo-elements and dynamic pseudo-classes so the
// remaining selector can be matched statically. Inner functional pseudo-classes stay.
func stripPseudo(s string) (selector, state, pseudo string) {
	selector = s
	if i := strings.Index(selector, "::"); i >= 0 {
		pseudo = strings.TrimLeft(selector[i:], ":")
		selector = selector[:i]
	}
	for _, p := range []string{":before", ":after", ":first-letter", ":first-line"} {
		if strings.HasSuffix(strings.ToLower(selector), p) {
			pseudo = strings.TrimPrefix(p, ":")
			selector = selector[:len(selector)-len(p)]
		}
	}
	if j := strings.IndexAny(pseudo, ":(["); j >= 0 {
		pseudo = pseudo[:j]
	}
	for _, p := range dynamicPseudo {
		if strings.Contains(selector, p) {
			if state == "" {
				state = strings.TrimPrefix(p, ":")
			}
			selector = strings.ReplaceAll(selector, p, "")
		}
	}
	return strings.TrimSpace(selector), state, pseudo
}

func inlineDecls(v string) []cssDecl {
	var out []cssDecl
	for _, part := range strings.Split(v, ";") {
		prop, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		prop = strings.TrimSpace(prop)
		if !strings.HasPrefix(prop, "--") {
			prop = strings.ToLower(prop)
		}
		out = append(out, cssDecl{prop: prop, value: strings.TrimSpace(value)})
	}
	return out
}

var inheritedProps = map[string]bool{"font-family": true, "font-style": true, "font-weight": true, "font-size": true, "text-transform": true, "letter-spacing": true, "color": true, "text-align": true, "font-variant": true}

// declared returns the winning base-context value an element sets itself.
func (m *styleModel) declared(n *html.Node, prop string) (string, bool) {
	value, ok, imp := "", false, false
	for _, r := range m.base[n] {
		for _, dcl := range r.decls {
			if dcl.prop == prop || shorthandFor(dcl.prop, prop) {
				if imp && !dcl.important {
					continue
				}
				value, ok, imp = longhand(dcl, prop), true, dcl.important
			}
		}
	}
	for _, dcl := range m.inline[n] {
		if (dcl.prop == prop || shorthandFor(dcl.prop, prop)) && !imp {
			value, ok = longhand(dcl, prop), true
		}
	}
	return value, ok
}

func shorthandFor(short, prop string) bool {
	switch short {
	case "font":
		return prop == "font-family" || prop == "font-size" || prop == "font-weight" || prop == "font-style"
	case "background":
		return prop == "background-image" || prop == "background-color"
	}
	return false
}

// longhand extracts a usable longhand from a few common shorthands.
func longhand(d cssDecl, prop string) string {
	if d.prop == prop {
		return d.value
	}
	switch d.prop {
	case "font":
		v := d.value
		if prop == "font-family" {
			if i := fontFamilyStart(v); i >= 0 {
				return strings.TrimSpace(v[i:])
			}
			return ""
		}
		if prop == "font-style" && strings.Contains(strings.ToLower(v), "italic") {
			return "italic"
		}
		if prop == "font-size" {
			for _, f := range strings.Fields(strings.Split(v, "/")[0]) {
				if px := lengthPx(f); px > 0 {
					return f
				}
			}
		}
		return ""
	case "background":
		return d.value
	}
	return d.value
}

func fontFamilyStart(v string) int {
	// The family list follows the size (and optional /line-height) in `font`.
	fields := strings.Fields(v)
	pos := 0
	for i, f := range fields {
		idx := strings.Index(v[pos:], f) + pos
		pos = idx + len(f)
		head := strings.Split(f, "/")[0]
		if lengthPx(head) > 0 || strings.HasPrefix(strings.ToLower(head), "clamp(") || strings.HasPrefix(head, "var(") {
			if i+1 < len(fields) {
				return strings.Index(v[pos:], fields[i+1]) + pos
			}
		}
	}
	return -1
}

// value resolves a property for an element: own declaration, then inheritance
// for typographic properties, then var() substitution from the ancestor chain.
func (m *styleModel) value(n *html.Node, prop string) string {
	if c, ok := m.computed[n]; ok {
		if v, ok := c[prop]; ok {
			return v
		}
	} else {
		m.computed[n] = map[string]string{}
	}
	v, ok := m.declared(n, prop)
	if !ok || strings.EqualFold(v, "inherit") {
		if inheritedProps[prop] && n.Parent != nil && n.Parent.Type == html.ElementNode {
			v = m.value(n.Parent, prop)
		} else if !ok {
			v = ""
		}
	}
	v = m.resolveVars(n, v, 0)
	m.computed[n][prop] = v
	return v
}

func (m *styleModel) custom(n *html.Node, name string) (string, bool) {
	for p := n; p != nil && p.Type == html.ElementNode; p = p.Parent {
		if v, ok := m.declared(p, name); ok {
			return v, true
		}
	}
	return "", false
}

func (m *styleModel) resolveVars(n *html.Node, v string, depth int) string {
	for depth < 8 {
		i := strings.Index(v, "var(")
		if i < 0 {
			return v
		}
		end, level := -1, 0
		for j := i + 4; j < len(v); j++ {
			if v[j] == '(' {
				level++
			} else if v[j] == ')' {
				if level == 0 {
					end = j
					break
				}
				level--
			}
		}
		if end < 0 {
			return v
		}
		inner := v[i+4 : end]
		name, fallback, _ := strings.Cut(inner, ",")
		name = strings.TrimSpace(name)
		sub, ok := m.custom(n, name)
		if !ok {
			sub = strings.TrimSpace(fallback)
		}
		v = v[:i] + sub + v[end+1:]
		depth++
	}
	return v
}

// lengthPx converts absolute-ish lengths to CSS pixels (rem/em assume 16px).
// clamp()/min()/max() report their largest absolute argument.
func lengthPx(v string) float64 {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return 0
	}
	for _, f := range []string{"clamp(", "min(", "max("} {
		if strings.HasPrefix(v, f) && strings.HasSuffix(v, ")") {
			best := 0.0
			for _, part := range splitSelectors(v[len(f) : len(v)-1]) {
				if px := lengthPx(part); px > best {
					best = px
				}
			}
			return best
		}
	}
	units := []struct {
		suffix string
		scale  float64
	}{{"px", 1}, {"rem", 16}, {"em", 16}, {"pt", 4.0 / 3}}
	for _, u := range units {
		if strings.HasSuffix(v, u.suffix) {
			if f, err := strconv.ParseFloat(strings.TrimSuffix(v, u.suffix), 64); err == nil {
				return f * u.scale
			}
		}
	}
	return 0
}

// letterSpacingEm reports tracking in em (px values assume a 13px label).
func letterSpacingEm(v string) float64 {
	v = strings.ToLower(strings.TrimSpace(v))
	if strings.HasSuffix(v, "em") && !strings.HasSuffix(v, "rem") {
		f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "em"), 64)
		return f
	}
	if px := lengthPx(v); px != 0 {
		return px / 13
	}
	return 0
}

func firstFamily(v string) string {
	family, _, _ := strings.Cut(v, ",")
	return strings.ToLower(strings.Trim(strings.TrimSpace(family), `"'`))
}

// ---- Color parsing (sRGB → OKLab/OKLCH) ----

type okColor struct{ L, a, b, alpha float64 }

func (c okColor) chroma() float64 { return math.Hypot(c.a, c.b) }
func (c okColor) hue() float64 {
	h := math.Atan2(c.b, c.a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}
func (c okColor) dist(o okColor) float64 { return math.Sqrt((c.L-o.L)*(c.L-o.L) + (c.a-o.a)*(c.a-o.a) + (c.b-o.b)*(c.b-o.b)) }

func srgbToOK(r, g, b, alpha float64) okColor {
	lin := func(c float64) float64 {
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r, g, b = lin(r), lin(g), lin(b)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return okColor{0.2104542553*l + 0.7936177850*m - 0.0040720468*s, 1.9779984951*l - 2.4285922050*m + 0.4505937099*s, 0.0259040371*l + 0.7827717662*m - 0.8086757660*s, alpha}
}

func hexColor(h string) (okColor, bool) {
	h = strings.TrimPrefix(h, "#")
	if len(h) == 3 || len(h) == 4 {
		var b strings.Builder
		for _, c := range h {
			b.WriteRune(c)
			b.WriteRune(c)
		}
		h = b.String()
	}
	if len(h) != 6 && len(h) != 8 {
		return okColor{}, false
	}
	n, err := strconv.ParseUint(h, 16, 64)
	if err != nil {
		return okColor{}, false
	}
	alpha := 1.0
	if len(h) == 8 {
		alpha = float64(n&0xff) / 255
		n >>= 8
	}
	return srgbToOK(float64(n>>16&0xff)/255, float64(n>>8&0xff)/255, float64(n&0xff)/255, alpha), true
}

// colorsIn extracts every literal color from a CSS value with its byte offset.
func colorsIn(v string) []okColor {
	var out []okColor
	lower := strings.ToLower(v)
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if c == '#' {
			j := i + 1
			for j < len(lower) && strings.IndexByte("0123456789abcdef", lower[j]) >= 0 {
				j++
			}
			if col, ok := hexColor(lower[i:j]); ok {
				out = append(out, col)
			}
			i = j - 1
			continue
		}
		for _, fn := range []string{"rgba(", "rgb(", "hsla(", "hsl(", "oklch("} {
			if strings.HasPrefix(lower[i:], fn) && (i == 0 || !isIdentByte(lower[i-1])) {
				end := strings.IndexByte(lower[i:], ')')
				if end < 0 {
					break
				}
				if col, ok := functionColor(fn, lower[i+len(fn):i+end]); ok {
					out = append(out, col)
				}
				i += end
				break
			}
		}
	}
	return out
}

func isIdentByte(b byte) bool { return b == '-' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }

func functionColor(fn, args string) (okColor, bool) {
	args = strings.NewReplacer(",", " ", "/", " ").Replace(args)
	f := strings.Fields(args)
	if len(f) < 3 {
		return okColor{}, false
	}
	num := func(s string, scale float64) (float64, bool) {
		if strings.HasSuffix(s, "%") {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
			return v / 100 * scale, err == nil
		}
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "deg"), 64)
		return v, err == nil
	}
	alpha := 1.0
	if len(f) >= 4 {
		if a, ok := num(f[3], 1); ok {
			alpha = a
		}
	}
	switch fn {
	case "rgb(", "rgba(":
		r, ok1 := num(f[0], 255)
		g, ok2 := num(f[1], 255)
		b, ok3 := num(f[2], 255)
		if !ok1 || !ok2 || !ok3 {
			return okColor{}, false
		}
		return srgbToOK(r/255, g/255, b/255, alpha), true
	case "hsl(", "hsla(":
		h, ok1 := num(f[0], 1)
		s, ok2 := num(f[1], 1)
		l, ok3 := num(f[2], 1)
		if !ok1 || !ok2 || !ok3 {
			return okColor{}, false
		}
		if !strings.HasSuffix(f[1], "%") {
			s /= 100
		}
		if !strings.HasSuffix(f[2], "%") {
			l /= 100
		}
		k := func(n float64) float64 { return math.Mod(n+h/30, 12) }
		a := s * math.Min(l, 1-l)
		ch := func(n float64) float64 { return l - a*math.Max(-1, math.Min(math.Min(k(n)-3, 9-k(n)), 1)) }
		return srgbToOK(ch(0), ch(8), ch(4), alpha), true
	case "oklch(":
		L, ok1 := num(f[0], 1)
		C, ok2 := num(f[1], 0.4)
		H, ok3 := num(f[2], 1)
		if !ok1 || !ok2 || !ok3 {
			return okColor{}, false
		}
		if !strings.HasSuffix(f[0], "%") && L > 1 {
			L /= 100
		}
		return okColor{L, C * math.Cos(H*math.Pi/180), C * math.Sin(H*math.Pi/180), alpha}, true
	}
	return okColor{}, false
}

// relLuminance approximates WCAG relative luminance from OKLab lightness.
func contrastOK(a, b okColor) float64 {
	lum := func(c okColor) float64 { y := c.L * c.L * c.L; return y }
	x, y := lum(a), lum(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}
