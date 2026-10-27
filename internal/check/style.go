package check

import (
	"bytes"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/color"
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
	selector string // authored complex selector
	state    string // dynamic pseudo-class stripped from the selector (hover, focus…)
	pseudo   string // pseudo-element stripped from the selector (before, after…)
	narrow   bool   // inside a max-width-only media query
	alt      bool   // print, reduced motion, dark scheme or other alternate context
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
	origin   *styleModel // originating element cascade for pseudo-element custom properties
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
	index := newElementIndex(d.Elements)
	parsed := map[string]cascadia.Sel{}
	for _, r := range m.rules {
		sel, seen := parsed[r.selector]
		if !seen {
			var err error
			if sel, err = cascadia.Parse(r.selector); err != nil {
				sel = nil
			}
			parsed[r.selector] = sel
		}
		if sel == nil {
			continue
		}
		r.sel, r.spec = sel, sel.Specificity()
		r.matches = index.match(d.Root, r.selector, sel)
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

// elementIndex finds the candidates of a selector the way browsers do: by the
// id, class or tag its rightmost compound requires. Only candidates are tested
// with the full selector, instead of every element for every rule.
type elementIndex struct {
	ids, classes, tags map[string][]*html.Node
	attrs              map[string][]*html.Node
	elements           []*html.Node
	nodes              []*html.Node
	keys               map[string]selectorKey
	ancestors          map[*html.Node]keySet
}

func newElementIndex(elements []*html.Node) *elementIndex {
	x := &elementIndex{ancestors: make(map[*html.Node]keySet, len(elements)), elements: elements, keys: map[string]selectorKey{}, ids: map[string][]*html.Node{}, attrs: map[string][]*html.Node{}, classes: map[string][]*html.Node{}, tags: map[string][]*html.Node{}}
	for _, n := range elements {
		x.ancestorBits(n)
		x.tags[n.Data] = append(x.tags[n.Data], n)
		seenID, seenClass, seenAttr := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, a := range n.Attr {
			if name := asciiLower(a.Key); !seenAttr[name] {
				seenAttr[name] = true
				x.attrs[name] = append(x.attrs[name], n)
			}
			switch a.Key {
			case "id":
				if !seenID[a.Val] {
					x.ids[a.Val] = append(x.ids[a.Val], n)
					seenID[a.Val] = true
				}
			case "class":
				for c := range strings.FieldsFuncSeq(a.Val, func(r rune) bool {
					return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\f'
				}) {
					if !seenClass[c] {
						seenClass[c] = true
						x.classes[c] = append(x.classes[c], n)
					}
				}
			}
		}
	}
	return x
}

// match returns the elements matching sel in document order, like
// cascadia.QueryAll(root, sel).
func (x *elementIndex) match(root *html.Node, selector string, sel cascadia.Sel) []*html.Node {
	var candidates []*html.Node
	key, ok := x.keys[selector]
	if !ok {
		key.kind, key.name = keySelector(selector)
		key.ancestors = ancestorKeys(selector)
		x.keys[selector] = key
	}
	switch kind, name := key.kind, key.name; kind {
	case '#':
		candidates = x.ids[name]
	case '.':
		candidates = x.classes[name]
	case '[':
		candidates = x.attrs[name]
	case 't':
		candidates = x.tags[name]
	default:
		candidates = x.elements
		// Cascadia's nonstandard text pseudo-classes can also match text nodes.
		// Preserve QueryAll semantics for these selectors with one shared preorder.
		serialized := sel.String()
		if strings.Contains(serialized, ":contains") || strings.Contains(serialized, ":matches") {
			if x.nodes == nil {
				var visit func(*html.Node)
				visit = func(n *html.Node) {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						x.nodes = append(x.nodes, c)
						visit(c)
					}
				}
				visit(root)
			}
			candidates = x.nodes
		}
	}
	var out []*html.Node
	required := key.ancestors
	for _, n := range candidates {
		if required != (keySet{}) && !x.ancestorBits(n.Parent).has(required) {
			continue
		}
		if sel.Match(n) {
			out = append(out, n)
		}
	}
	return out
}

type selectorKey struct {
	ancestors keySet
	kind      byte
	name      string
}

// keySelector reads only top-level simple selectors in the rightmost compound.
// The CSS lexer keeps escaped delimiters and quoted attribute values intact;
// requirements inside functional pseudo-classes never become mandatory keys.
func keySelector(selector string) (byte, string) {
	lexer := css.NewLexer(parse.NewInputString(selector))
	var id, class, attr, tag string
	depth := 0
	var previous css.TokenType
	namespace := false
	reset := func() { id, class, attr, tag = "", "", "", ""; previous = css.ErrorToken; namespace = false }
	for {
		typ, raw := lexer.Next()
		if typ == css.ErrorToken {
			break
		}
		if depth > 0 {
			// The first identifier of a top-level [attribute] names the attribute.
			if depth == 1 && previous == css.LeftBracketToken && typ == css.IdentToken && attr == "" {
				attr = asciiLower(selectorIdent(string(raw)))
			}
			if typ == css.DelimToken && string(raw) == "|" && depth == 1 {
				attr = "-" // namespaced attribute: no key
			}
			if typ == css.DelimToken && string(raw) == "!" && depth == 1 {
				attr = "-" // Cascadia's [attr!=value] also matches elements without attr: no key
			}
			previous = typ
			switch typ {
			case css.FunctionToken, css.LeftParenthesisToken, css.LeftBracketToken:
				depth++
			case css.RightParenthesisToken, css.RightBracketToken:
				depth--
			}
			continue
		}
		switch typ {
		case css.FunctionToken, css.LeftParenthesisToken, css.LeftBracketToken:
			depth++
		case css.WhitespaceToken:
			reset()
			continue
		case css.DelimToken:
			switch string(raw) {
			case ">", "+", "~":
				reset()
				continue
			case "|":
				namespace = true
			}
		case css.CommaToken:
			return 0, ""
		case css.HashToken:
			if id == "" {
				id = selectorIdent(string(raw[1:]))
			}
		case css.IdentToken:
			if previous == css.DelimToken {
				if class == "" {
					class = selectorIdent(string(raw))
				}
			} else if previous == css.ErrorToken {
				tag = selectorIdent(string(raw))
			}
		}
		// Only a dot can introduce a class identifier. Other delimiters suppress
		// tag extraction without making the next identifier a class.
		if typ == css.DelimToken && string(raw) != "." {
			previous = css.ColonToken
		} else {
			previous = typ
		}
	}
	if namespace {
		tag = ""
	}
	switch {
	case id != "":
		return '#', id
	case class != "":
		return '.', class
	case attr != "" && attr != "-":
		return '[', attr
	case tag != "":
		return 't', asciiLower(tag)
	}
	return 0, ""
}

// selectorIdent decodes CSS escapes with the same code point and whitespace
// handling as Cascadia. It is called only for lexer-recognized identifiers.
func selectorIdent(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			out.WriteByte(s[i])
			i++
			continue
		}
		i++
		start := i
		for i < len(s) && i < start+6 && (s[i] >= '0' && s[i] <= '9' || s[i] >= 'a' && s[i] <= 'f' || s[i] >= 'A' && s[i] <= 'F') {
			i++
		}
		if i > start {
			v, _ := strconv.ParseUint(s[start:i], 16, 32)
			out.WriteRune(rune(v))
			if i < len(s) {
				switch s[i] {
				case '\r':
					i++
					if i < len(s) && s[i] == '\n' {
						i++
					}
				case ' ', '\t', '\n', '\f':
					i++
				}
			}
		} else if i < len(s) {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
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
			dcl := styleDeclaration(kind, data, p.Values())
			for _, r := range stack[len(stack)-1].rule {
				r.decls = append(r.decls, dcl)
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

func styleDeclaration(kind css.GrammarType, data []byte, values []css.Token) cssDecl {
	prop := strings.ToLower(urlcheck.DecodeCSS(string(data)))
	if kind == css.CustomPropertyGrammar {
		prop = string(data)
	}
	value := strings.TrimSpace(tokensText(values))
	important := strings.HasSuffix(strings.ToLower(value), "!important")
	if important {
		value = strings.TrimSpace(value[:len(value)-len("!important")])
	}
	return cssDecl{prop: prop, value: value, important: important}
}

func inlineDecls(v string) []cssDecl {
	input := parse.NewInputString(v)
	defer input.Restore()
	parser := css.NewParser(input, true)
	var out []cssDecl
	for {
		kind, _, data := parser.Next()
		if kind == css.ErrorGrammar {
			return out
		}
		if kind == css.DeclarationGrammar || kind == css.CustomPropertyGrammar {
			out = append(out, styleDeclaration(kind, data, parser.Values()))
		}
	}
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
		if (dcl.prop == prop || shorthandFor(dcl.prop, prop)) && (!imp || dcl.important) {
			value, ok, imp = longhand(dcl, prop), true, dcl.important
		}
	}
	return value, ok
}

func shorthandFor(short, prop string) bool {
	switch short {
	case "padding":
		return strings.HasPrefix(prop, "padding-")
	case "font":
		return prop == "font-family" || prop == "font-size" || prop == "font-weight" || prop == "font-style"
	case "background":
		return prop == "background-image" || prop == "background-color"
	case "mask", "-webkit-mask":
		return prop == short+"-image"
	}
	return false
}

// longhand extracts a usable longhand from a few common shorthands.
func longhand(d cssDecl, prop string) string {
	if d.prop == prop {
		return d.value
	}
	switch d.prop {
	case "padding":
		fields := strings.Fields(d.value)
		if len(fields) < 1 || len(fields) > 4 {
			return ""
		}
		top, right, bottom, left := fields[0], fields[0], fields[0], fields[0]
		if len(fields) > 1 {
			right, left = fields[1], fields[1]
		}
		if len(fields) > 2 {
			bottom = fields[2]
		}
		if len(fields) > 3 {
			left = fields[3]
		}
		switch prop {
		case "padding-top":
			return top
		case "padding-right":
			return right
		case "padding-bottom":
			return bottom
		case "padding-left":
			return left
		}
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
	if m.origin != nil {
		if v, ok := m.declared(n, name); ok {
			return v, true
		}
		return m.origin.custom(n, name)
	}
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

// Colors are parsed and compared by the color package; the cascade above only
// decides which values apply.
type okColor = color.Color

func colorsIn(v string) []okColor { return color.Parse(v) }

// ancestorBits is the key filter of n and all its ancestors. Nodes outside the
// indexed elements are computed on demand, so a partial index never drops keys.
func (x *elementIndex) ancestorBits(n *html.Node) keySet {
	if n == nil {
		return keySet{}
	}
	if bits, ok := x.ancestors[n]; ok {
		return bits
	}
	bits := x.ancestorBits(n.Parent).union(nodeKeys(n))
	x.ancestors[n] = bits
	return bits
}

// keyBits is a compact membership filter: collisions allow extra matches,
// while a missing bit proves a required ancestor cannot be present.
type keySet [4]uint64

func (k keySet) union(o keySet) keySet {
	for i := range k {
		k[i] |= o[i]
	}
	return k
}

// has reports whether every key bit of want is present in k.
func (k keySet) has(want keySet) bool {
	for i := range k {
		if k[i]&want[i] != want[i] {
			return false
		}
	}
	return true
}

func keyBits(kind byte, name string) keySet {
	h := uint64(14695981039346656037) ^ uint64(kind)
	for i := 0; i < len(name); i++ {
		h = (h ^ uint64(name[i])) * 1099511628211
	}
	var k keySet
	k[h>>62] = uint64(1) << (h & 63)
	return k
}
func nodeKeys(n *html.Node) keySet {
	if n.Type != html.ElementNode {
		return keySet{}
	}
	bits := keyBits('t', asciiLower(n.Data))
	for _, a := range n.Attr {
		if a.Key == "id" {
			bits = bits.union(keyBits('#', a.Val))
		}
		if a.Key == "class" {
			for c := range strings.FieldsFuncSeq(a.Val, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' }) {
				bits = bits.union(keyBits('.', c))
			}
		}
	}
	return bits
}

// ancestorKeys returns the id, class and tag keys that every match must have
// on some ancestor: the compounds left of a descendant or child combinator.
// The subject compound and compounds before + or ~ apply to the element or a
// sibling, so they are not required. Attribute selectors and pseudo-class
// arguments are skipped; they only narrow a compound further. Selector lists
// and namespaces disable the filter.
func ancestorKeys(selector string) keySet {
	lexer := css.NewLexer(parse.NewInputString(strings.TrimSpace(selector)))
	var required, compound keySet
	combinator := byte(0) // pending combinator between compounds
	dot, pseudo := false, false
	depth := 0
	for {
		typ, raw := lexer.Next()
		if typ == css.ErrorToken {
			return required
		}
		if depth > 0 {
			switch typ {
			case css.FunctionToken, css.LeftParenthesisToken, css.LeftBracketToken:
				depth++
			case css.RightParenthesisToken, css.RightBracketToken:
				depth--
			}
			continue
		}
		if typ == css.WhitespaceToken {
			if combinator == 0 {
				combinator = ' '
			}
			continue
		}
		if typ == css.DelimToken && (string(raw) == ">" || string(raw) == "+" || string(raw) == "~") {
			combinator = raw[0]
			continue
		}
		if combinator != 0 {
			if combinator == ' ' || combinator == '>' {
				required = required.union(compound)
			}
			compound, combinator = keySet{}, 0
		}
		switch typ {
		case css.FunctionToken, css.LeftParenthesisToken, css.LeftBracketToken:
			depth++
		case css.ColonToken:
			pseudo = true
			continue
		case css.DelimToken:
			switch string(raw) {
			case ".":
				dot = true
				continue
			case "*":
			default:
				return keySet{}
			}
		case css.HashToken:
			compound = compound.union(keyBits('#', selectorIdent(string(raw[1:]))))
		case css.IdentToken:
			switch {
			case pseudo:
			case dot:
				compound = compound.union(keyBits('.', selectorIdent(string(raw))))
			default:
				compound = compound.union(keyBits('t', asciiLower(selectorIdent(string(raw)))))
			}
		default:
			return keySet{}
		}
		dot, pseudo = false, false
	}
}
