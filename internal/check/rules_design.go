package check

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Anonyfox/lfm/internal/color"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/stock"
	"golang.org/x/net/html"
)

// Generator-default heuristics. Each signal is either strong (a recognizable stock
// signature; reported individually) or weak (common in generated pages but also in
// deliberate design; only counted by design.defaults). Elements inside an ancestor
// with a non-empty data-design-reason are exempt: the reason documents the decision.
// See guides/ui-ux.md#generator-defaults.

const designGuide = "guides/ui-ux.md#generator-defaults"

var (
	reDesign6  = regexp.MustCompile(`(?i)^(window-?(dots|controls)|traffic-?lights|browser-?dots)$`)
	reDesign8  = regexp.MustCompile(`(?i)badge|pill|chip|tag`)
	reDesign10 = regexp.MustCompile(`(?i)icon`)
	reDesign17 = regexp.MustCompile(`^backdrop-blur`)
	reDesign18 = regexp.MustCompile(`^bg-(gradient|linear)-`)
	reDesign19 = regexp.MustCompile(`^blur-(2xl|3xl|\[[0-9]+px\])$`)
	reDesign20 = regexp.MustCompile(`^border-l-(2|4|8)$`)
	reDesign21 = regexp.MustCompile(`^from-`)
	reDesign22 = regexp.MustCompile(`^hover:-translate-y-`)
	reDesign23 = regexp.MustCompile(`^rounded-(lg|xl|2xl|3xl)$`)
	reDesign24 = regexp.MustCompile(`^shadow-(md|lg|xl|2xl)$`)
	reDesign25 = regexp.MustCompile(`^text-(xs|sm)$`)
	reDesign26 = regexp.MustCompile(`^tracking-(wide|wider|widest|\[)`)
)

type designSignal struct {
	rule, kind string
	strong     bool
	node       *html.Node
	css        *cssRule
	message    string
	hint       string
}

type designFacts struct {
	signals   []designSignal
	justified int
}

func designRule(id string) func(*Document) []Diagnostic {
	return func(d *Document) []Diagnostic {
		var out []Diagnostic
		for _, s := range d.design().signals {
			if s.rule == id && s.strong {
				out = append(out, d.designDiagnostic(s))
			}
		}
		return out
	}
}

func (d *Document) designDiagnostic(s designSignal) Diagnostic {
	x := d.At(s.node, Warning, s.message, s.hint+" Deliberate? Keep it and add data-design-reason=\"…\" on the element, or set build.validation.rules."+s.rule+".")
	if s.node == nil && s.css != nil {
		x = d.At(s.css.sheet, Warning, x.Message, x.Hint)
		if s.css.file != "" {
			x.Location = diagnostic.At(strings.TrimPrefix(s.css.file, "./"), s.css.raw, s.css.offset)
		}
	}
	x.Reference = designGuide
	return x
}

func (d *Document) design() *designFacts {
	if d.designFacts != nil {
		return d.designFacts
	}
	f := &designFacts{}
	a := &designAnalysis{d: d, m: d.styles(), facts: f}
	a.run()
	sort.SliceStable(f.signals, func(i, j int) bool { return d.positions[f.signals[i].node] < d.positions[f.signals[j].node] })
	d.designFacts = f
	return f
}

type designAnalysis struct {
	d     *Document
	m     *styleModel
	facts *designFacts
	seen  map[string]bool
}

func (a *designAnalysis) add(s designSignal) {
	if s.node != nil && reasoned(s.node) {
		a.facts.justified++
		return
	}
	if s.css != nil && len(s.css.matches) > 0 {
		all := true
		for _, n := range s.css.matches {
			if !reasoned(n) {
				all = false
				break
			}
		}
		if all {
			a.facts.justified++
			return
		}
	}
	a.facts.signals = append(a.facts.signals, s)
}

func (a *designAnalysis) once(key string) bool {
	if a.seen == nil {
		a.seen = map[string]bool{}
	}
	if a.seen[key] {
		return false
	}
	a.seen[key] = true
	return true
}

func reasoned(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && strings.TrimSpace(Attr(p, "data-design-reason")) != "" {
			return true
		}
	}
	return false
}

func (a *designAnalysis) run() {
	a.palette()
	a.gradientText()
	a.glows()
	a.eyebrows()
	a.typography()
	a.emoji()
	a.hero()
	a.grids()
	a.sideStripes()
	a.glass()
	a.shadows()
	a.motion()
	a.arrows()
	a.separators()
	a.mono()
	a.chrome()
	a.proof()
	a.sections()
	a.weakSurfaces()
	a.contrast()
}

// ---------- helpers ----------

func classes(n *html.Node) map[string]bool {
	out := map[string]bool{}
	for _, c := range strings.Fields(Attr(n, "class")) {
		out[c] = true
	}
	return out
}

func classMatch(n *html.Node, re *regexp.Regexp) bool {
	for _, c := range strings.Fields(Attr(n, "class")) {
		if re.MatchString(c) {
			return true
		}
	}
	return false
}

func designVisible(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p.Type != html.ElementNode {
			continue
		}
		switch p.Data {
		case "template", "script", "style", "noscript", "head", "title", "dialog":
			return false
		}
		if HasAttr(p, "hidden") {
			return false
		}
	}
	return true
}

func wordCount(s string) int { return len(strings.Fields(s)) }

func cleanText(n *html.Node) string { return strings.Join(strings.Fields(Text(n)), " ") }

// shownText omits visually hidden helper text (screen-reader-only spans).
func shownText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				b.WriteString(c.Data + " ")
			} else if c.Type == html.ElementNode && !classes(c)["visually-hidden"] && !classes(c)["sr-only"] && c.Data != "script" && c.Data != "style" {
				walk(c)
			}
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func (a *designAnalysis) cardLike(g []*html.Node) bool {
	for _, k := range g {
		surface := a.prop(k, "background") != "" || a.prop(k, "background-color") != "" || a.prop(k, "box-shadow") != "" || a.prop(k, "border") != "" || lengthPx(strings.Fields(a.prop(k, "border-radius") + " 0")[0]) >= 6
		heading := false
		for _, e := range descendants(k) {
			if isHeading(e) {
				heading = true
				break
			}
		}
		if !surface || !heading {
			return false
		}
	}
	return true
}

func prevElement(n *html.Node) *html.Node {
	for p := n.PrevSibling; p != nil; p = p.PrevSibling {
		if p.Type == html.ElementNode {
			return p
		}
		if p.Type == html.TextNode && strings.TrimSpace(p.Data) != "" {
			return nil
		}
	}
	return nil
}

func elementChildren(n *html.Node) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data != "script" && c.Data != "template" {
			out = append(out, c)
		}
	}
	return out
}

func isHeading(n *html.Node) bool {
	return n != nil && n.Type == html.ElementNode && len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6'
}

func label(n *html.Node) string {
	t := cleanText(n)
	if len([]rune(t)) > 40 {
		t = string([]rune(t)[:39]) + "…"
	}
	return strconv.Quote(t)
}

func (a *designAnalysis) prop(n *html.Node, p string) string { return strings.ToLower(a.m.value(n, p)) }

// ---------- color ----------

var colorProps = regexp.MustCompile(`^(color|background(-color|-image)?|border(-[a-z-]+)?|outline(-color)?|fill|stroke|box-shadow|text-shadow|accent-color|caret-color|text-decoration(-color)?|--.*)$`)

func (a *designAnalysis) palette() {
	var cream, terra *cssRule
	for _, r := range a.m.rules {
		if r.alt || len(r.matches) == 0 && !strings.Contains(r.selector, ":root") {
			continue
		}
		for _, dcl := range r.decls {
			if !colorProps.MatchString(dcl.prop) {
				continue
			}
			cols := colorsIn(dcl.value)
			gradientHit := false
			if strings.Contains(strings.ToLower(dcl.value), "gradient(") {
				purple, foreign := 0, 0
				for _, c := range cols {
					switch {
					case stock.Purpleish(c):
						purple++
					case c.Chroma() < 0.06:
					case c.Hue() >= 200 && c.Hue() < 270, c.Hue() >= 330 || c.Hue() < 20:
					default:
						foreign++
					}
				}
				if purple > 0 && foreign == 0 && len(cols) >= 2 {
					gradientHit = true
					if a.once("purple gradient") {
						a.add(designSignal{rule: "design.palette", kind: "violet gradient", strong: true, css: r, message: fmt.Sprintf("Violet/purple gradient on `%s`: the most recognized generated-page color move.", r.selector), hint: "Use a flat, subject-derived color or a gradient that depicts something real (light, material, sky)."})
					}
				}
			}
			for _, c := range cols {
				if n, ok := stock.Nearest(c, stock.Accents, 0.02); ok && !gradientHit && a.once("accent "+n.Name) {
					a.add(designSignal{rule: "design.palette", kind: "stock accent color", strong: true, css: r, message: fmt.Sprintf("%s (%s) is the stock generator accent; `%s` uses it.", n.Name, dcl.prop, r.selector), hint: "Derive the accent from the subject's own world and name its role in the brief."})
				}
				if _, ok := stock.Nearest(c, stock.Creams, 0.012); ok && cream == nil && (strings.HasPrefix(dcl.prop, "background") || strings.HasPrefix(dcl.prop, "--")) {
					cream = r
				}
				if _, ok := stock.Nearest(c, stock.Terracottas, 0.02); ok && terra == nil {
					terra = r
				}
			}
		}
	}
	if cream != nil && terra != nil {
		a.add(designSignal{rule: "design.palette", kind: "assistant house palette", strong: true, css: cream, message: "Cream background with terracotta accent: the 2026 assistant house palette, now read as generated.", hint: "Take the palette from the product's real materials or place and state each role in the brief."})
	}
}

// ---------- gradient text ----------

func (a *designAnalysis) gradientText() {
	for _, r := range a.m.rules {
		clip, grad, transparent := false, false, false
		for _, dcl := range r.decls {
			v := strings.ToLower(dcl.value)
			switch {
			case strings.HasSuffix(dcl.prop, "background-clip") && strings.Contains(v, "text"):
				clip = true
			case (dcl.prop == "background" || dcl.prop == "background-image") && strings.Contains(v, "gradient("):
				grad = true
			case (dcl.prop == "color" || dcl.prop == "-webkit-text-fill-color") && (v == "transparent" || strings.HasSuffix(v, "/0)") || strings.HasSuffix(v, ",0)")):
				transparent = true
			}
		}
		if clip && grad && transparent {
			for _, n := range r.matches {
				if designVisible(n) && cleanText(n) != "" {
					a.add(designSignal{rule: "design.gradient-text", kind: "gradient text", strong: true, node: n, message: fmt.Sprintf("Gradient-filled text %s (background-clip:text): a top generated-page tell.", label(n)), hint: "Carry emphasis with size, weight, position or a solid color."})
				}
			}
		}
	}
	for _, n := range a.d.Elements {
		c := classes(n)
		if c["bg-clip-text"] && (classMatch(n, reDesign18) || classMatch(n, reDesign21)) && designVisible(n) {
			a.add(designSignal{rule: "design.gradient-text", kind: "gradient text", strong: true, node: n, message: fmt.Sprintf("Tailwind gradient text %s (bg-clip-text + gradient).", label(n)), hint: "Carry emphasis with size, weight, position or a solid color."})
		}
	}
}

// ---------- glows and orbs ----------

var blurPx = regexp.MustCompile(`blur\(\s*([0-9.]+)(px|rem)`)

func (a *designAnalysis) glows() {
	var orbs []*html.Node
	var orbRule *cssRule
	for _, r := range a.m.rules {
		if r.alt {
			continue
		}
		blur, round, radial := 0.0, false, false
		for _, dcl := range r.decls {
			v := strings.ToLower(dcl.value)
			if dcl.prop == "filter" {
				if m := blurPx.FindStringSubmatch(v); m != nil {
					f, _ := strconv.ParseFloat(m[1], 64)
					if m[2] == "rem" {
						f *= 16
					}
					blur = f
				}
			}
			if dcl.prop == "border-radius" && (strings.Contains(v, "50%") || lengthPx(v) >= 300) {
				round = true
			}
			if (dcl.prop == "background" || dcl.prop == "background-image") && strings.Contains(v, "radial-gradient") {
				radial = true
			}
		}
		if blur >= 24 && (round || radial) {
			orbRule = r
			orbs = append(orbs, r.matches...)
		}
	}
	for _, n := range a.d.Elements {
		if classMatch(n, reDesign19) && classes(n)["rounded-full"] {
			orbs = append(orbs, n)
		}
	}
	var live []*html.Node
	for _, n := range orbs {
		if designVisible(n) && !reasoned(n) {
			live = append(live, n)
		}
	}
	if len(live) >= 2 {
		a.add(designSignal{rule: "design.glow", kind: "blur orbs", strong: true, node: live[0], message: fmt.Sprintf("%d heavily blurred decorative shapes (orbs/blobs): stock generated-page atmosphere.", len(live)), hint: "Use a real subject image, a flat field or light that belongs to the object."})
	} else if len(live) == 1 {
		a.add(designSignal{rule: "design.glow", kind: "blur orb", node: live[0], css: orbRule})
	}
	glowing := 0
	var glowNode *html.Node
	for _, r := range a.m.rules {
		for _, dcl := range r.decls {
			if dcl.prop != "box-shadow" && dcl.prop != "text-shadow" {
				continue
			}
			for _, layer := range splitSelectors(dcl.value) {
				f := strings.Fields(strings.ToLower(layer))
				if len(f) < 3 || f[0] != "0" && f[0] != "0px" || f[1] != "0" && f[1] != "0px" || lengthPx(f[2]) < 20 {
					continue
				}
				for _, c := range colorsIn(layer) {
					if c.Chroma() > 0.1 {
						glowing += len(r.matches)
						if glowNode == nil && len(r.matches) > 0 {
							glowNode = r.matches[0]
						}
					}
				}
			}
		}
	}
	if glowing >= 3 {
		a.add(designSignal{rule: "design.glow", kind: "colored glows", strong: true, node: glowNode, message: fmt.Sprintf("Colored zero-offset glows on %d elements: neon-glow default.", glowing), hint: "Reserve glow for a light source; separate surfaces by tone instead."})
	} else if glowing > 0 {
		a.add(designSignal{rule: "design.glow", kind: "colored glow", node: glowNode})
	}
}

// ---------- eyebrows ----------

var eyebrowClass = regexp.MustCompile(`(?i)^(eyebrow|kicker|overline|pre-?title|pretitle|tagline|section-label|label-top)$`)

func (a *designAnalysis) eyebrows() {
	for _, h := range a.d.Elements {
		if !isHeading(h) || h.Data > "h3" || !designVisible(h) {
			continue
		}
		e := prevElement(h)
		if e == nil {
			// a label wrapping the heading's group: <div><p class=eyebrow/>…</div><h2>
			continue
		}
		switch e.Data {
		case "p", "span", "div", "small", "strong", "em", "b":
		default:
			continue
		}
		t := cleanText(e)
		if t == "" || wordCount(t) > 9 || len([]rune(t)) > 64 || len(elementChildren(e)) > 3 {
			continue
		}
		upper := a.prop(e, "text-transform") == "uppercase" || classes(e)["uppercase"] || allCaps(t)
		tracked := letterSpacingEm(a.prop(e, "letter-spacing")) >= 0.05 || classMatch(e, reDesign26)
		small := lengthPx(a.prop(e, "font-size")) > 0 && lengthPx(a.prop(e, "font-size")) <= 15 || classMatch(e, reDesign25)
		named := classMatch(e, eyebrowClass)
		if !(upper && (tracked || small)) && !named {
			continue
		}
		informative := strings.ContainsAny(t, "0123456789€$£¥%") && !repeatsHeading(t, cleanText(h))
		if informative {
			a.add(designSignal{rule: "design.eyebrows", kind: "label above heading", node: e})
			continue
		}
		a.add(designSignal{rule: "design.eyebrows", kind: "eyebrow labels", strong: true, node: e, message: fmt.Sprintf("Eyebrow %s above heading %s: small tracked label restating or framing the heading.", label(e), label(h)), hint: "Delete it, or merge its information into the heading. Keep a label only for time, step, price or status."})
	}
}

// repeatsHeading reports whether a label restates a significant word of its heading.
func repeatsHeading(label, heading string) bool {
	hw := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(heading), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(w)) >= 4 {
			hw[w] = true
		}
	}
	for _, w := range strings.FieldsFunc(strings.ToLower(label), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if hw[w] {
			return true
		}
	}
	return false
}

func allCaps(s string) bool {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= 3 && upper == letters
}

// ---------- typography ----------

func (a *designAnalysis) typography() {
	var body, h1 *html.Node
	for _, n := range a.d.Elements {
		if n.Data == "body" && body == nil {
			body = n
		}
		if n.Data == "h1" && h1 == nil && designVisible(n) {
			h1 = n
		}
	}
	if body == nil || h1 == nil {
		return
	}
	fb, fh := firstFamily(a.m.value(body, "font-family")), firstFamily(a.m.value(h1, "font-family"))
	if fh == "" {
		fh = fb
	}
	if fb != "" && fb == fh && stock.GenericSans(fb) {
		distinct := false
		for _, n := range a.d.Elements {
			if isHeading(n) && designVisible(n) && firstFamily(a.m.value(n, "font-family")) != fb && firstFamily(a.m.value(n, "font-family")) != "" {
				distinct = true
				break
			}
		}
		if !distinct {
			a.add(designSignal{rule: "design.type", kind: "one stock sans", strong: true, node: h1, message: fmt.Sprintf("%q sets both the display headline and the body: the one-sans generator default.", fb), hint: "Give display a typeface chosen for this subject; keep a readable text face for copy."})
		}
	}
	if stock.Display(fh) {
		a.add(designSignal{rule: "design.type", kind: "LLM display roster face", node: h1})
	}
	// accent word: a short differently styled span inside a longer heading
	for _, h := range a.d.Elements {
		if (h.Data != "h1" && h.Data != "h2") || !designVisible(h) || wordCount(shownText(h)) < 4 {
			continue
		}
		for _, c := range elementChildren(h) {
			t := cleanText(c)
			if t == "" || wordCount(t) > 2 || c.Data == "br" || classes(c)["visually-hidden"] || classes(c)["sr-only"] {
				continue
			}
			italic := a.prop(c, "font-style") == "italic" && a.prop(h, "font-style") != "italic"
			family := firstFamily(a.m.value(c, "font-family")) != firstFamily(a.m.value(h, "font-family"))
			gradient := strings.Contains(a.prop(c, "background-clip"), "text") || strings.Contains(a.prop(c, "-webkit-background-clip"), "text") || classes(c)["bg-clip-text"]
			color := a.prop(c, "color") != a.prop(h, "color") && a.prop(c, "color") != ""
			if italic && family || gradient {
				a.add(designSignal{rule: "design.type", kind: "accent word", strong: true, node: c, message: fmt.Sprintf("Accent word %s styled apart inside heading %s: the italic-serif/gradient word formula.", label(c), label(h)), hint: "Use one face and one color for the headline; let the words carry the emphasis."})
			} else if italic || family || color {
				a.add(designSignal{rule: "design.type", kind: "accent word", node: c})
			}
		}
	}
}

// ---------- emoji ----------

func emojiRune(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1F64F, r >= 0x1F680 && r <= 0x1F6FF, r >= 0x1F900 && r <= 0x1F9FF, r >= 0x1FA70 && r <= 0x1FAFF, r >= 0x1F1E6 && r <= 0x1F1FF:
		return true
	}
	switch r {
	case 0x231A, 0x231B, 0x23E9, 0x23EA, 0x23EB, 0x23EC, 0x23F0, 0x23F3, 0x25FD, 0x25FE, 0x2614, 0x2615, 0x267F, 0x2693, 0x26A1, 0x26AA, 0x26AB, 0x26BD, 0x26BE, 0x26C4, 0x26C5, 0x26CE, 0x26D4, 0x26EA, 0x26F2, 0x26F3, 0x26F5, 0x26FA, 0x26FD, 0x2705, 0x270A, 0x270B, 0x2728, 0x274C, 0x274E, 0x2753, 0x2754, 0x2755, 0x2757, 0x2795, 0x2796, 0x2797, 0x27B0, 0x27BF, 0x2B1B, 0x2B1C, 0x2B50, 0x2B55:
		return true
	}
	return r >= 0x2648 && r <= 0x2653
}

func hasEmoji(s string) (rune, bool) {
	rs := []rune(s)
	for i, r := range rs {
		if emojiRune(r) || i+1 < len(rs) && rs[i+1] == 0xFE0F && r > 0xFF {
			return r, true
		}
	}
	return 0, false
}

func (a *designAnalysis) emoji() {
	for _, n := range a.d.Elements {
		switch n.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6", "button", "summary", "label", "legend", "th", "dt":
		case "a":
			if wordCount(cleanText(n)) > 8 {
				continue
			}
		case "li":
			t := cleanText(n)
			if r, ok := hasEmoji(firstRunes(t, 3)); ok && designVisible(n) {
				a.add(designSignal{rule: "design.emoji", kind: "emoji as interface", strong: true, node: n, message: fmt.Sprintf("Emoji %q used as a list bullet in %s.", string(r), label(n)), hint: "Use typographic bullets, numbers or a real glyph that names the item."})
			}
			continue
		default:
			continue
		}
		if !designVisible(n) {
			continue
		}
		if r, ok := hasEmoji(cleanText(n)); ok {
			msg := fmt.Sprintf("Emoji %q in <%s> %s: emoji as interface decoration reads as generated.", string(r), n.Data, label(n))
			if r == 0x2728 {
				msg = fmt.Sprintf("Sparkle ✨ in <%s> %s: the generic 'AI/new' glyph.", n.Data, label(n))
			}
			a.add(designSignal{rule: "design.emoji", kind: "emoji as interface", strong: true, node: n, message: msg, hint: "Remove it, or use a purpose-made glyph that names the thing."})
		}
	}
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// ---------- hero formula ----------

var badgeText = regexp.MustCompile(`(?i)^(✨|new\b|introducing|now (with|live|available)|beta\b|announcing|v\d|launching)`)

func (a *designAnalysis) hero() {
	var h1 *html.Node
	for _, n := range a.d.Elements {
		if n.Data == "h1" && designVisible(n) {
			h1 = n
			break
		}
	}
	if h1 == nil {
		return
	}
	hero := h1
	for p := h1.Parent; p != nil && p.Type == html.ElementNode; p = p.Parent {
		hero = p
		if p.Data == "section" || p.Data == "header" || p.Parent != nil && (p.Parent.Data == "main" || p.Parent.Data == "body") {
			break
		}
	}
	before := true
	var badge *html.Node
	actions := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == h1 {
			before = false
			return
		}
		if n.Type == html.ElementNode && designVisible(n) {
			if before && badge == nil && n.Data != "a" && n.Data != "img" && n.Data != "svg" {
				t := cleanText(n)
				if t != "" && wordCount(t) <= 7 && len(elementChildren(n)) <= 3 {
					radius := a.prop(n, "border-radius")
					pill := classes(n)["rounded-full"] || strings.Contains(radius, "999") || strings.Contains(radius, "50%") || lengthPx(radius) >= 100
					framed := a.prop(n, "border") != "" || a.prop(n, "background") != "" || a.prop(n, "background-color") != "" || classes(n)["border"]
					if pill && framed || badgeText.MatchString(t) && (pill || framed || classMatch(n, reDesign8)) {
						badge = n
					}
				}
			}
			if !before && (n.Data == "a" || n.Data == "button") {
				actions++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(hero)
	if badge != nil {
		a.add(designSignal{rule: "design.hero", kind: "hero badge", strong: true, node: badge, message: fmt.Sprintf("Pill badge %s above the hero headline: stock 'New/Introducing' chip.", label(badge)), hint: "Delete it unless it states a real, current, dated status; then make it plain text."})
	}
	centered := a.prop(h1, "text-align") == "center" || classes(h1)["text-center"]
	if centered && actions >= 2 && badge != nil {
		a.add(designSignal{rule: "design.hero", kind: "centered hero formula", strong: true, node: h1, message: "Centered hero formula: badge, centered headline, subline and paired buttons.", hint: "Lead with the product, object or action; compose the hero around what this subject needs."})
	} else if centered {
		a.add(designSignal{rule: "design.hero", kind: "centered hero", node: h1})
	}
}

// ---------- repeated grids: icon cards, stats, numbers ----------

func shape(n *html.Node) string {
	cs := strings.Fields(Attr(n, "class"))
	sort.Strings(cs)
	var kids []string
	for _, c := range elementChildren(n) {
		kids = append(kids, c.Data)
	}
	return n.Data + "." + strings.Join(cs, ".") + ">" + strings.Join(kids, ",")
}

func leadingIcon(n *html.Node) bool {
	kids := elementChildren(n)
	if len(kids) < 2 {
		return false
	}
	first := kids[0]
	for depth := 0; depth < 2 && cleanText(first) == "" && len(elementChildren(first)) == 1; depth++ {
		first = elementChildren(first)[0]
	}
	icon := first.Data == "svg" || first.Data == "img" || first.Data == "i" || classMatch(first, reDesign10)
	if !icon || cleanText(kids[0]) != "" && wordCount(cleanText(kids[0])) > 0 && kids[0].Data != "i" {
		return false
	}
	head := false
	for _, k := range kids[1:] {
		if isHeading(k) || k.Data == "strong" || k.Data == "b" {
			head = true
		}
	}
	return head
}

var statNumber = regexp.MustCompile(`^[~≈<>+]?\d[\d.,]*\s?(%|\+|x|×|k|K|M|B|h|min|s|ms|/7|/5)?\+?$`)

func (a *designAnalysis) grids() {
	for _, p := range a.d.Elements {
		if !designVisible(p) || p.Data == "nav" || p.Data == "table" || p.Data == "tbody" || p.Data == "select" || p.Data == "form" || p.Data == "svg" {
			continue
		}
		kids := elementChildren(p)
		if len(kids) < 3 {
			continue
		}
		groups := map[string][]*html.Node{}
		for _, k := range kids {
			groups[shape(k)] = append(groups[shape(k)], k)
		}
		for _, g := range groups {
			if len(g) < 3 || len(g)*4 < len(kids)*3 {
				continue
			}
			icons := 0
			for _, k := range g {
				if leadingIcon(k) {
					icons++
				}
			}
			if icons == len(g) {
				a.add(designSignal{rule: "design.cards", kind: "icon-card grid", strong: true, node: p, message: fmt.Sprintf("%d identical icon-topped cards (icon, heading, text) in one row: the stock feature grid.", len(g)), hint: "Lay out by content weight: a lead item, a comparison table, a sequence or a list. Equal cards only for true peers."})
				continue
			}
			stats, round := 0, 0
			for _, k := range g {
				for _, e := range append([]*html.Node{k}, descendants(k)...) {
					t := cleanText(e)
					if len(elementChildren(e)) == 0 && len(t) <= 12 && statNumber.MatchString(t) && e.Data != "output" && e.Data != "time" && cleanText(k) != t {
						stats++
						if strings.ContainsAny(t, "+%kKMB") || strings.HasSuffix(strings.ReplaceAll(t, ",", ""), "000") || strings.HasPrefix(t, "99.9") {
							round++
						}
						break
					}
				}
			}
			if stats == len(g) && !insideTool(p) {
				if round >= 2 {
					a.add(designSignal{rule: "design.stats", kind: "stat strip", strong: true, node: p, message: fmt.Sprintf("Stat strip of %d big round numbers with small labels.", len(g)), hint: "Show sourced numbers where they prove a specific claim, next to their source."})
				} else {
					a.add(designSignal{rule: "design.stats", kind: "number strip", node: p})
				}
				continue
			}
			numbered := 0
			for i, k := range g {
				if strings.HasPrefix(cleanText(k), fmt.Sprintf("0%d", i+1)) {
					numbered++
				}
			}
			if numbered == len(g) {
				a.add(designSignal{rule: "design.cards", kind: "01/02/03 markers", node: p})
			} else if a.cardLike(g) {
				a.add(designSignal{rule: "design.cards", kind: "equal card grid", node: p})
			}
		}
	}
}

func descendants(n *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				out = append(out, c)
				walk(c)
			}
		}
	}
	walk(n)
	return out
}

func insideTool(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && (p.Data == "form" || p.Data == "fieldset" || p.Data == "output" || p.Data == "table" || p.Data == "dl") {
			return true
		}
	}
	return false
}

// ---------- side stripes ----------

func (a *designAnalysis) sideStripes() {
	for _, r := range a.m.rules {
		if r.alt || len(r.matches) < 2 {
			continue
		}
		stripe, other, surface := false, false, false
		for _, dcl := range r.decls {
			v := strings.ToLower(dcl.value)
			switch dcl.prop {
			case "border-left", "border-inline-start":
				w := lengthPx(strings.Fields(v + " x")[0])
				stripe = w >= 2 && w <= 8 && !strings.Contains(v, "transparent") && (strings.Contains(v, "solid") || strings.Contains(v, "#") || strings.Contains(v, "var("))
			case "border", "border-right", "border-top", "border-bottom", "border-width":
				if !strings.HasPrefix(v, "0") && v != "none" {
					other = true
				}
			case "background", "background-color", "border-radius", "padding", "box-shadow":
				surface = true
			}
		}
		if !stripe || other || !surface {
			continue
		}
		var hits []*html.Node
		for _, n := range r.matches {
			switch n.Data {
			case "blockquote", "li", "tr", "td", "th", "ins", "del", "hr", "q":
				continue
			}
			if designVisible(n) {
				hits = append(hits, n)
			}
		}
		if len(hits) >= 2 {
			a.add(designSignal{rule: "design.stripes", kind: "side-stripe accent", strong: true, node: hits[0], message: fmt.Sprintf("Colored left-border stripe on %d boxes (`%s`): a strong generated-card fingerprint.", len(hits), r.selector), hint: "Separate items by tone, spacing or a meaningful marker; keep stripes for quotations, timelines or diffs."})
		}
	}
	for _, n := range a.d.Elements {
		if classMatch(n, reDesign20) && designVisible(n) && n.Data != "blockquote" && a.once("tw stripe") {
			a.add(designSignal{rule: "design.stripes", kind: "side-stripe accent", strong: true, node: n, message: "Tailwind border-l-* accent stripe on a card.", hint: "Separate items by tone, spacing or a meaningful marker."})
		}
	}
}

// ---------- glass ----------

func (a *designAnalysis) glass() {
	set := map[*html.Node]bool{}
	for _, r := range a.m.rules {
		if r.alt {
			continue
		}
		for _, dcl := range r.decls {
			if strings.HasSuffix(dcl.prop, "backdrop-filter") && strings.Contains(strings.ToLower(dcl.value), "blur") {
				for _, n := range r.matches {
					if designVisible(n) && !reasoned(n) {
						set[n] = true
					}
				}
			}
		}
	}
	for _, n := range a.d.Elements {
		if classMatch(n, reDesign17) && designVisible(n) && !reasoned(n) {
			set[n] = true
		}
	}
	var nodes []*html.Node
	for n := range set {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return a.d.positions[nodes[i]] < a.d.positions[nodes[j]] })
	if len(nodes) >= 3 {
		a.add(designSignal{rule: "design.glass", kind: "glassmorphism", strong: true, node: nodes[0], message: fmt.Sprintf("Backdrop blur on %d separate elements: reflex glassmorphism.", len(nodes)), hint: "Use solid surfaces; keep blur for the one layer that truly floats over content."})
	} else if len(nodes) > 0 {
		a.add(designSignal{rule: "design.glass", kind: "backdrop blur", node: nodes[0]})
	}
}

// ---------- shadows and surfaces ----------

var tailwindShadow = regexp.MustCompile(`^0(px)? (1px 3px 0(px)?|4px 6px -1px|10px 15px -3px|20px 25px -5px|25px 50px -12px)`)

func (a *designAnalysis) shadows() {
	count := 0
	var first *html.Node
	for _, r := range a.m.rules {
		for _, dcl := range r.decls {
			if dcl.prop != "box-shadow" {
				continue
			}
			v := strings.Join(strings.Fields(strings.ToLower(dcl.value)), " ")
			if tailwindShadow.MatchString(v) {
				for _, c := range colorsIn(v) {
					if c.Chroma() < 0.01 && c.L < 0.1 {
						count += len(r.matches)
						if first == nil && len(r.matches) > 0 {
							first = r.matches[0]
						}
						break
					}
				}
			}
		}
	}
	tw := 0
	for _, n := range a.d.Elements {
		if classMatch(n, reDesign24) && classMatch(n, reDesign23) {
			tw++
			if first == nil {
				first = n
			}
		}
	}
	if count+tw >= 3 {
		a.add(designSignal{rule: "design.surfaces", kind: "stock shadow", strong: true, node: first, message: fmt.Sprintf("Tailwind default soft shadow on %d boxes: the uniform card-kit surface.", count+tw), hint: "Vary shape and elevation by role; shadows only for layers that float."})
	} else if count+tw > 0 {
		a.add(designSignal{rule: "design.surfaces", kind: "stock shadow", node: first})
	}
}

func (a *designAnalysis) weakSurfaces() {
	// identical radius + shadow on many boxes; pills everywhere; dot grids; hover lift
	type key struct{ radius, shadow string }
	same := map[key]int{}
	pills := 0
	for _, n := range a.d.Elements {
		if !designVisible(n) {
			continue
		}
		radius, shadow := a.prop(n, "border-radius"), a.prop(n, "box-shadow")
		if shadow != "" && shadow != "none" && lengthPx(strings.Fields(radius + " 0")[0]) >= 8 {
			same[key{radius, shadow}]++
		}
		if (n.Data == "a" || n.Data == "button") && (strings.Contains(radius, "999") || lengthPx(radius) >= 99 || classes(n)["rounded-full"]) {
			pills++
		}
	}
	for k, c := range same {
		if c >= 8 && a.once("uniform surfaces") {
			a.add(designSignal{rule: "design.surfaces", kind: "uniform radius+shadow", message: fmt.Sprintf("%d boxes share radius %s and one shadow", c, k.radius)})
		}
	}
	if pills >= 6 {
		a.add(designSignal{rule: "design.surfaces", kind: "pills everywhere"})
	}
	for _, r := range a.m.rules {
		for _, dcl := range r.decls {
			v := strings.ToLower(dcl.value)
			if dcl.prop == "background-size" && lengthPx(strings.Fields(v + " 0")[0]) > 0 && lengthPx(strings.Fields(v + " 0")[0]) <= 40 {
				for _, d2 := range r.decls {
					if strings.HasPrefix(d2.prop, "background") && strings.Contains(strings.ToLower(d2.value), "radial-gradient") && a.once("dot grid") {
						a.add(designSignal{rule: "design.surfaces", kind: "dot-grid backdrop"})
					}
				}
			}
		}
	}
}

// ---------- motion ----------

var revealClass = regexp.MustCompile(`(?i)^(reveal|fade-?(up|in|in-up)|animate-?(on-?scroll|in|fade)|slide-?up|aos|scroll-?reveal|js-reveal|motion-?safe:animate-.*)$`)
var liftTransform = regexp.MustCompile(`translatey\(\s*-([0-9.]+)px|translate\(\s*0(px)?\s*,\s*-([0-9.]+)px|translate3d\(\s*0(px)?\s*,\s*-([0-9.]+)px`)

func (a *designAnalysis) motion() {
	var reveal []*html.Node
	for _, n := range a.d.Elements {
		if !designVisible(n) {
			continue
		}
		if HasAttr(n, "data-aos") || HasAttr(n, "data-sal") || HasAttr(n, "data-animate") || HasAttr(n, "data-reveal") || classMatch(n, revealClass) {
			reveal = append(reveal, n)
		}
	}
	if len(reveal) >= 4 {
		a.add(designSignal{rule: "design.motion", kind: "uniform reveal", strong: true, node: reveal[0], message: fmt.Sprintf("Scroll-reveal hook on %d blocks: the uniform fade-and-rise default.", len(reveal)), hint: "Spend motion on one orchestrated moment tied to the concept and on feedback for interaction."})
	} else if len(reveal) > 0 {
		a.add(designSignal{rule: "design.motion", kind: "reveal hook", node: reveal[0]})
	}
	lifted := 0
	for _, r := range a.m.rules {
		if r.state != "hover" {
			continue
		}
		for _, dcl := range r.decls {
			v := strings.ToLower(dcl.value)
			if (dcl.prop == "transform" || dcl.prop == "translate") && (liftTransform.MatchString(v) || dcl.prop == "translate" && strings.Contains(v, "-")) {
				lifted += len(r.matches)
			}
		}
	}
	for _, n := range a.d.Elements {
		if classMatch(n, reDesign22) {
			lifted++
		}
	}
	if lifted >= 6 {
		a.add(designSignal{rule: "design.motion", kind: "hover lift everywhere"})
	}
}

// ---------- arrows and separators ----------

var trailingArrow = regexp.MustCompile(`[→↗➜⟶➔»›↘]\s*$`)

func (a *designAnalysis) arrows() {
	total, arrowed := 0, 0
	var first *html.Node
	for _, n := range a.d.Elements {
		if n.Data != "a" && n.Data != "button" || !designVisible(n) {
			continue
		}
		if p := n.Parent; p != nil && (p.Data == "p" || p.Data == "li") && wordCount(cleanText(p)) > wordCount(cleanText(n))+3 {
			continue // links inside prose
		}
		t := cleanText(n)
		if t == "" {
			continue
		}
		total++
		if trailingArrow.MatchString(t) {
			external := Attr(n, "target") == "_blank" || strings.HasPrefix(Attr(n, "href"), "http")
			if strings.HasSuffix(t, "↗") && external {
				continue
			}
			arrowed++
			if first == nil {
				first = n
			}
		}
	}
	if total >= 4 && arrowed >= 3 && arrowed*2 >= total {
		a.add(designSignal{rule: "design.arrows", kind: "arrow on every action", strong: true, node: first, message: fmt.Sprintf("%d of %d buttons/links end in an arrow glyph: the appended-arrow default.", arrowed, total), hint: "Keep an arrow only where it states a direction (external, next step, scroll)."})
	} else if arrowed >= 2 {
		a.add(designSignal{rule: "design.arrows", kind: "appended arrows", node: first})
	}
}

func (a *designAnalysis) separators() {
	count := 0
	var first *html.Node
	for _, n := range a.d.Elements {
		if !designVisible(n) {
			continue
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.TextNode {
				continue
			}
			if strings.Count(c.Data, " · ")+strings.Count(c.Data, " • ") >= 1 && strings.Count(cleanText(n), "·")+strings.Count(cleanText(n), "•") >= 2 {
				count++
				if first == nil {
					first = n
				}
				break
			}
		}
	}
	if count >= 4 {
		a.add(designSignal{rule: "design.chrome", kind: "middot meta strings", strong: true, node: first, message: fmt.Sprintf("%d 'A · B · C' meta strings: template chrome repeated across the page.", count), hint: "Write the information as a sentence, a labelled value or a list."})
	} else if count >= 2 {
		a.add(designSignal{rule: "design.chrome", kind: "middot meta strings", node: first})
	}
}

// ---------- monospace microcopy ----------

func (a *designAnalysis) mono() {
	var hits []*html.Node
	for _, n := range a.d.Elements {
		if !designVisible(n) {
			continue
		}
		own := ""
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				own += c.Data
			}
		}
		own = strings.TrimSpace(own)
		letters := 0
		for _, r := range own {
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if letters < 3 || codeContext(n) {
			continue
		}
		if stock.Mono(firstFamily(a.m.value(n, "font-family"))) || classes(n)["font-mono"] {
			hits = append(hits, n)
		}
	}
	if len(hits) >= 8 {
		a.add(designSignal{rule: "design.mono", kind: "monospace microcopy", strong: true, node: hits[0], message: fmt.Sprintf("Monospace on %d non-code labels/captions: the 'console' microcopy default.", len(hits)), hint: "Set labels in the text face (tabular figures for numbers); keep mono for code and technical values."})
	} else if len(hits) >= 3 {
		a.add(designSignal{rule: "design.mono", kind: "monospace microcopy", node: hits[0]})
	}
}

func codeContext(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		switch p.Data {
		case "code", "pre", "kbd", "samp", "var", "output", "time", "data", "td", "th", "input", "select", "textarea", "svg", "button", "label":
			return true
		}
	}
	return false
}

// ---------- fake chrome, proof furniture, stock sections ----------

func (a *designAnalysis) chrome() {
	for _, p := range a.d.Elements {
		kids := elementChildren(p)
		if len(kids) < 3 || !designVisible(p) {
			continue
		}
		// red, yellow, green in this order, each a distinct traffic-light hue
		order := stock.WindowControls
		hits := 0
		for i, k := range kids[:3] {
			for _, c := range colorsIn(a.prop(k, "background") + " " + a.prop(k, "background-color")) {
				if _, ok := stock.Nearest(c, order[i], 0.04); ok {
					hits++
					break
				}
			}
		}
		empty := 0
		for _, k := range kids[:3] {
			if cleanText(k) == "" && len(elementChildren(k)) == 0 {
				empty++
			}
		}
		if hits == 3 || empty == 3 && classMatch(p, reDesign6) {
			a.add(designSignal{rule: "design.chrome", kind: "fake window chrome", strong: true, node: p, message: "Fake window dots (red/yellow/green): mock browser chrome around a product image.", hint: "Show the real interface or object at true scale, without fake frames."})
		}
	}
}

var proofClaim = regexp.MustCompile(`(?i)\b((trusted|loved|used|chosen) by|as (seen|featured) (on|in)|join(ed)? (over |more than )?[\d,.]+\s?k?\+?\s?(teams|companies|users|founders|creators|businesses|customers|developers))\b`)
var popularBadge = regexp.MustCompile(`(?i)^(most popular|best value|recommended|popular)$`)

var placeholderMedia = regexp.MustCompile(`(?i)(pravatar\.cc|randomuser\.me|ui-avatars\.com|picsum\.photos|placehold(\.co|\.it|er\.com)|via\.placeholder\.com|dummyimage\.com|unsplash\.it|source\.unsplash\.com|thispersondoesnotexist)`)

func (a *designAnalysis) proof() {
	for _, n := range a.d.Elements {
		if (n.Data == "img" || n.Data == "source") && placeholderMedia.MatchString(Attr(n, "src")+" "+Attr(n, "srcset")) && a.once("placeholder media") {
			a.add(designSignal{rule: "design.proof", kind: "placeholder people/images", strong: true, node: n, message: "Placeholder image service in the page (" + placeholderMedia.FindString(Attr(n, "src")+" "+Attr(n, "srcset")) + "): stand-in faces or photos read as fabricated proof.", hint: "Use real, permitted photos of the actual people/product, or no image."})
		}
		if n.Data == "img" && !reasoned(n) {
			for p := n; p != nil && p.Type == html.ElementNode && p.Data != "section" && p.Data != "main"; p = p.Parent {
				t := a.prop(p, "transform")
				if strings.Contains(t, "rotatex") || strings.Contains(t, "rotatey") || strings.Contains(t, "perspective") {
					if a.once("tilted mockup") {
						a.add(designSignal{rule: "design.proof", kind: "tilted product mockup", node: n})
					}
					break
				}
			}
		}
	}
	for _, n := range a.d.Elements {
		if !designVisible(n) || (n.Data != "h2" && n.Data != "h3" && n.Data != "p" && n.Data != "span" && n.Data != "div" && n.Data != "strong") || len(elementChildren(n)) > 4 {
			continue
		}
		t := cleanText(n)
		if wordCount(t) <= 14 && proofClaim.MatchString(t) && a.once("proof") {
			a.add(designSignal{rule: "design.proof", kind: "proof furniture", strong: true, node: n, message: fmt.Sprintf("Generic proof claim %s: stock 'trusted by' furniture.", label(n)), hint: "Show verifiable proof beside the claim it supports, or remove it (see guides/content-copy.md)."})
		}
		if popularBadge.MatchString(t) && a.once("popular") {
			a.add(designSignal{rule: "design.proof", kind: "'most popular' badge", node: n})
		}
	}
}

var (
	howItWorks   = regexp.MustCompile(`(?i)how it works|so funktioniert|in (three|3|four|4) (easy |simple )?steps`)
	testimonials = regexp.MustCompile(`(?i)testimonial|what (our )?(customers|clients|people|users) (say|think)|loved by|kundenstimmen`)
	pricingHead  = regexp.MustCompile(`(?i)^(pricing|plans|choose your plan|preise)`)
	faqHead      = regexp.MustCompile(`(?i)\bfaq\b|frequently asked|häufige fragen`)
)

func (a *designAnalysis) sections() {
	found := map[string]bool{}
	details := 0
	for _, n := range a.d.Elements {
		if !designVisible(n) {
			continue
		}
		if n.Data == "details" {
			details++
		}
		if !isHeading(n) {
			continue
		}
		t := cleanText(n)
		switch {
		case howItWorks.MatchString(t):
			found["how it works"] = true
		case testimonials.MatchString(t):
			found["testimonials"] = true
		case pricingHead.MatchString(t):
			found["pricing tiers"] = true
		case faqHead.MatchString(t):
			found["FAQ"] = true
		case proofClaim.MatchString(t):
			found["logo strip"] = true
		}
	}
	if details >= 4 {
		found["FAQ"] = true
	}
	for _, s := range a.facts.signals {
		switch s.kind {
		case "icon-card grid":
			found["feature grid"] = true
		case "proof furniture":
			found["logo strip"] = true
		case "stat strip":
			found["stat strip"] = true
		}
	}
	var list []string
	for k := range found {
		list = append(list, k)
	}
	sort.Strings(list)
	if len(list) >= 4 {
		a.add(designSignal{rule: "design.sections", kind: "stock section order", strong: true, message: "Stock landing-page sections present: " + strings.Join(list, ", ") + ".", hint: "Keep a section only when it makes its own claim or does its own job for this offer."})
	} else if len(list) == 3 {
		a.add(designSignal{rule: "design.sections", kind: "stock sections", message: strings.Join(list, ", ")})
	}
}

// ---------- contrast on dark pages ----------

func (a *designAnalysis) contrast() {
	// Only dark backdrops that are a single known color; gradients/images are unknowable statically.
	backdrop := func(n *html.Node) (okColor, bool) {
		for p := n; p != nil && p.Type == html.ElementNode; p = p.Parent {
			bg := a.prop(p, "background-color")
			if v := a.prop(p, "background"); v != "" {
				if strings.Contains(v, "gradient(") || strings.Contains(v, "url(") {
					return okColor{}, false
				}
				bg = v
			}
			if strings.Contains(a.prop(p, "background-image"), "gradient(") {
				return okColor{}, false
			}
			if cs := colorsIn(bg); len(cs) > 0 && cs[0].Alpha > 0.95 {
				return cs[0], true
			}
		}
		return okColor{}, false
	}
	for _, p := range a.d.Elements {
		if p.Data != "p" || !designVisible(p) || wordCount(cleanText(p)) < 8 {
			continue
		}
		bg, ok := backdrop(p)
		if !ok || bg.L > 0.35 {
			continue
		}
		cs := colorsIn(a.prop(p, "color"))
		if len(cs) == 0 {
			continue
		}
		fg := cs[0]
		if fg.Alpha > 0 && fg.Alpha < 1 {
			fg = fg.Over(bg)
		}
		if color.Contrast(fg, bg) < 7 && a.once("grey body") {
			a.add(designSignal{rule: "design.contrast", kind: "grey body text on dark", node: p})
		}
	}
}

// ---------- the combination ----------

func designDefaultsRule(d *Document) []Diagnostic {
	f := d.design()
	strongKinds, weakKinds := map[string]bool{}, map[string]bool{}
	for _, s := range f.signals {
		if d.Options.Rules[s.rule] == Off {
			continue // a rule the project turned off does not count toward the combination
		}
		if s.strong {
			strongKinds[s.kind] = true
		} else {
			weakKinds[s.kind] = true
		}
	}
	for k := range strongKinds {
		delete(weakKinds, k)
	}
	if len(strongKinds)+len(weakKinds) < 4 || len(strongKinds) == 0 && len(weakKinds) < 5 {
		return nil
	}
	list := func(m map[string]bool) string {
		var s []string
		for k := range m {
			s = append(s, k)
		}
		sort.Strings(s)
		return strings.Join(s, ", ")
	}
	msg := fmt.Sprintf("%d generator-default signals combine on this page", len(strongKinds)+len(weakKinds))
	if len(strongKinds) > 0 {
		msg += "; reported: " + list(strongKinds)
	}
	if len(weakKinds) > 0 {
		msg += "; weaker, review in context: " + list(weakKinds)
	}
	msg += ". The combination is what visitors recognize as generated."
	if f.justified > 0 {
		msg += fmt.Sprintf(" %d further signals are justified by data-design-reason.", f.justified)
	}
	x := d.Whole(Warning, msg, "Decide palette, type, shape, motion and layout from the subject in the brief; keep each listed pattern only with a reason. Weak signals alone never warn.")
	x.Reference = designGuide
	return []Diagnostic{x}
}
