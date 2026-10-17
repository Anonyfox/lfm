package check

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ---------- copy.tells ----------

// A tell is a stock phrase of generated copy. Each pattern is narrow enough that
// a match is the phrase itself, not an ordinary use of one of its words: a
// knitting page may say "seamless", it will not say "seamless experience".
// Repeated constructions only count from their threshold on, because one use is
// ordinary language and the habit is the tell. Every pattern names literal
// keywords one of which any match contains, so the regular expression only
// runs on the few text blocks that contain one.
type tell struct {
	min  int      // occurrences on the page before the construction counts
	keys []string // lowercase literals, one of which every match contains
	re   *regexp.Regexp
}

type tellSource struct {
	min     int
	keys    []string
	pattern string
}

var copyTells = func() []tell {
	sources := []tellSource{
		{1, []string{"today"}, `in today'?s (?:fast[- ]paced|digital|modern|ever[- ](?:changing|evolving)|competitive|busy) (?:world|age|landscape|era|economy)`},
		{1, []string{"ever"}, `in (?:an|the|this) ever[- ](?:evolving|changing) (?:world|landscape|field|market)`},
		{1, []string{"unlock"}, `unlock (?:the (?:full )?(?:power|potential)|your (?:full |true )?potential)`},
		{1, []string{"unleash"}, `unleash (?:the|your) (?:full )?(?:power|potential|creativity|inner)`},
		{1, []string{"elevate"}, `elevate your (?:\w+ )?(?:game|experience|brand|business|skills|workflow|routine|style)`},
		{1, []string{"next level"}, `take (?:your|it) (?:\w+ ){0,3}to the next level`},
		{1, []string{"game"}, `game[- ]chang(?:er|ing)`},
		{1, []string{"look no further"}, `look no further`},
		{1, []string{"goodbye"}, `say goodbye to`},
		{1, []string{"delv"}, `delv(?:e|es|ing) (?:into|deeper)`},
		{1, []string{"testament"}, `(?:is|stands as|serves as) a testament to`},
		{1, []string{"seamless"}, `seamless(?:ly)? (?:experience|integration|integrat\w+|workflow|journey|solution|blend)`},
		{1, []string{"harness"}, `harness(?:es|ing)? the (?:full )?(?:power|potential)`},
		{1, []string{"embark"}, `embark (?:on|upon) (?:a|an|your) (?:\w+ )?(?:journey|adventure|quest)`},
		{1, []string{"navigat"}, `navigat(?:e|ing) the (?:complexities|intricacies|ins and outs)`},
		{1, []string{"realm"}, `in the realm of`},
		{1, []string{"note", "remember", "mention"}, `it'?s (?:important|worth|crucial|essential) to (?:note|remember|mention)`},
		{1, []string{"whether"}, `whether you'?re an? [^.,;:!?]{1,30} or an?`},
		{1, []string{"cutting"}, `cutting[- ]edge (?:technology|solutions?|tools?|platform|features?|ai)`},
		{1, []string{"state"}, `state[- ]of[- ]the[- ]art (?:technology|solutions?|platform)`},
		{1, []string{"the way you"}, `(?:revolutioni[sz]e|transform) the way you`},
		{1, []string{"supercharge"}, `supercharge your`},
		{1, []string{"empower"}, `empower(?:s|ing)? (?:you|your|businesses|teams|individuals) to`},
		{1, []string{"we understand"}, `we understand (?:how|that) [^.!?]{0,40}(?:overwhelming|frustrating|stressful|daunting|challenging)`},
		{1, []string{"nestled"}, `nestled (?:in|among|between) the`},
		{1, []string{"tapestry"}, `(?:rich|vibrant|intricate) tapestry`},
		{1, []string{"dive"}, `(?:let'?s|ready to) dive (?:in|into|deeper)`},
		{1, []string{". no "}, `no (?:\w+)\. no (?:\w+)\. (?:just|only)`},
		{1, []string{"heutigen"}, `in der heutigen (?:schnelllebigen |digitalen |modernen )?(?:welt|zeit)`},
		{1, []string{"zeitalter"}, `im (?:digitalen|heutigen) zeitalter`},
		{1, []string{"in der"}, `in einer (?:welt|zeit), in der`},
		{1, []string{"tauchen"}, `tauchen sie (?:tiefer )?ein`},
		{1, []string{"level", "stufe"}, `(?:auf das|aufs|auf ein|auf die) (?:nächste|neues|nächstes) (?:level|stufe)`},
		{1, []string{"nahtlos"}, `nahtlos(?:e|en|er)? (?:integration|integriert|erlebnis|zusammenarbeit|übergang|einfügen)`},
		{1, []string{"es ist"}, `es ist (?:wichtig|erwähnenswert|entscheidend) zu (?:beachten|erwähnen|wissen)`},
		{1, []string{"oder"}, `ob (?:anfänger|einsteiger|neuling) oder (?:profi|experte|fortgeschrittene)`},
		{1, []string{"entfesseln"}, `entfesseln sie`},
		{1, []string{"potenzial"}, `(?:schöpfen sie|entfalten sie) (?:das|ihr) (?:volle[sn]? )?potenzial`},
		{1, []string{". kein"}, `kein(?:e)? [a-zäöüß]+\. kein(?:e)? [a-zäöüß]+\. (?:nur|einfach|sondern)`},
		{2, []string{"not "}, `not (?:just|only|merely) (?:a |an |the )?[^.!?;:]{1,60}?[,;—–-]? but`},
		{2, []string{"not "}, `(?:it'?s|this is|that'?s) not (?:just )?(?:about )?[^.!?;:]{1,50}?[.,;—–]\s*(?:it'?s|this is|that'?s)`},
		{3, []string{"nicht nur"}, `nicht nur [^.!?;:]{1,60}?,? sondern (?:auch)?`},
	}
	out := make([]tell, len(sources))
	for i, src := range sources {
		out[i] = tell{src.min, src.keys, regexp.MustCompile(`(?i)\b(?:` + src.pattern + `)\b`)}
	}
	return out
}()

// minDashes and dashesPer100 bound em-dash density: four or more, and more than
// one per hundred words. Human sales copy rarely reaches both.
const minDashes, dashesPer100 = 4, 1.0

func copyTellsRule(d *Document) []Diagnostic {
	segments := copySegments(d)
	type hit struct {
		count int
		first *html.Node
		text  string
	}
	hits := make([]hit, len(copyTells))
	dashes, words := 0, 0
	var firstDash *html.Node
	for _, s := range segments {
		text := normalizeQuotes(s.text)
		lower := strings.ToLower(text)
		words += len(strings.Fields(text))
		if n := proseDashes(text); n > 0 {
			if firstDash == nil {
				firstDash = s.node
			}
			dashes += n
		}
		for i, t := range copyTells {
			if !containsAny(lower, t.keys) {
				continue
			}
			for _, m := range t.re.FindAllString(text, -1) {
				if hits[i].count == 0 {
					hits[i].first, hits[i].text = s.node, m
				}
				hits[i].count++
			}
		}
	}
	var issues []Diagnostic
	const hint = "Say the concrete thing instead: the fact, capability, situation or number it stands for. Quotes of real buyers inside <blockquote> or <q> are not checked."
	for i, h := range hits {
		if h.count == 0 || h.count < copyTells[i].min {
			continue
		}
		phrase := strings.Join(strings.Fields(h.text), " ")
		if r := []rune(phrase); len(r) > 60 {
			phrase = string(r[:59]) + "…"
		}
		var message string
		switch {
		case copyTells[i].min > 1:
			message = fmt.Sprintf("%q and %d more like it: a construction generated copy repeats.", phrase, h.count-1)
		case h.count > 1:
			message = fmt.Sprintf("%q appears %d times: a stock phrase of generated copy.", phrase, h.count)
		default:
			message = fmt.Sprintf("%q is a stock phrase of generated copy.", phrase)
		}
		issues = append(issues, d.At(h.first, Warning, message, hint))
	}
	if dashes >= minDashes && float64(dashes)*100 > dashesPer100*float64(words) {
		issues = append(issues, d.At(firstDash, Warning, fmt.Sprintf("%d em dashes in %d words of copy: a density that reads as generated text.", dashes, words), "Recast most of them as two sentences, a comma, a colon or parentheses; keep the few where the break is the point."))
	}
	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Location.Line < issues[j].Location.Line })
	return issues
}

// proseDashes counts em dashes used as sentence breaks; ranges between
// numbers ("13—14 Nov") are not counted.
func proseDashes(s string) int {
	n := 0
	for i := strings.Index(s, "—"); i >= 0; {
		before, after := strings.TrimRight(s[:i], " "), strings.TrimLeft(s[i+len("—"):], " ")
		if !(endsDigit(before) && startsDigit(after)) {
			n++
		}
		next := strings.Index(s[i+len("—"):], "—")
		if next < 0 {
			break
		}
		i += len("—") + next
	}
	return n
}

func endsDigit(s string) bool   { return s != "" && s[len(s)-1] >= '0' && s[len(s)-1] <= '9' }
func startsDigit(s string) bool { return s != "" && s[0] >= '0' && s[0] <= '9' }

func containsAny(s string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func normalizeQuotes(s string) string {
	return strings.NewReplacer("’", "'", "‘", "'", " ", " ").Replace(s)
}

type copySegment struct {
	node *html.Node
	text string
}

// copyBlocks start a new segment, so phrases never join across separate blocks.
var copyBlocks = words("address article aside button caption dd details dialog div dl dt fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 header label legend li main nav ol option p section summary table td th tr ul")

// copySegments returns the visible text of the body block by block. Quotations
// (blockquote, q) are someone else's words; decorative, hidden, inert and
// explicitly justified subtrees are skipped.
func copySegments(d *Document) []copySegment { return segments(d, false) }

// visibleSegments is copySegments with quotations included.
func visibleSegments(d *Document) []copySegment { return segments(d, true) }

func segments(d *Document, quotes bool) []copySegment {
	var body *html.Node
	for _, n := range d.Elements {
		if n.Data == "body" && n.Namespace == "" {
			body = n
			break
		}
	}
	if body == nil {
		return nil
	}
	var out []copySegment
	var b strings.Builder
	owner := body
	flush := func() {
		if t := strings.Join(strings.Fields(b.String()), " "); t != "" {
			out = append(out, copySegment{owner, t})
		}
		b.Reset()
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
			case html.ElementNode:
				quotation := quotes && (c.Data == "blockquote" || c.Data == "q") && !HasAttr(c, "hidden") && Attr(c, "aria-hidden") != "true"
				if c.Namespace != "" || skipCopy(c) && !quotation {
					continue
				}
				if copyBlocks[c.Data] {
					b.WriteString(" ")
					flush()
					parent := owner
					owner = c
					walk(c)
					flush()
					owner = parent
					continue
				}
				if c.Data == "br" {
					b.WriteString(" ")
				}
				walk(c)
			}
		}
	}
	walk(body)
	flush()
	return out
}

func skipCopy(n *html.Node) bool {
	switch n.Data {
	case "script", "style", "template", "noscript", "blockquote", "q", "code", "pre", "kbd", "samp", "textarea", "select":
		return true
	}
	return HasAttr(n, "hidden") || Attr(n, "aria-hidden") == "true" || HasAttr(n, "inert") || HasAttr(n, "data-design-reason")
}

// ---------- ux.reduced-motion ----------

var motionProps = words("transform translate rotate scale offset offset-path offset-distance top left right bottom inset inset-inline inset-block margin margin-top margin-left margin-right margin-bottom background-position")

// reducedMotionRule warns when the page moves things but never asks for the
// visitor's reduced-motion preference. Opacity and color animations do not
// count as motion; transitions count only when they move something slowly
// enough to be a choreographed move rather than hover feedback.
func reducedMotionRule(d *Document) []Diagnostic {
	if d.mentions("prefers-reduced-motion") {
		return nil
	}
	m := d.styles()
	type use struct {
		node *html.Node
		what string
	}
	var uses []use
	visit := func(node *html.Node, selector string, decls []cssDecl) {
		for _, dcl := range decls {
			v := strings.ToLower(dcl.value)
			switch dcl.prop {
			case "animation", "animation-name":
				for _, name := range cssIdents(v) {
					if d.keyframes()[name].moves {
						uses = append(uses, use{node, fmt.Sprintf("@keyframes %s moves `%s`", name, selector)})
						break
					}
				}
			case "animation-timeline", "view-timeline-name", "scroll-timeline-name":
				if v != "auto" && v != "none" {
					uses = append(uses, use{node, fmt.Sprintf("scroll-driven animation on `%s`", selector)})
				}
			case "scroll-behavior":
				if v == "smooth" {
					uses = append(uses, use{node, "smooth scrolling on `" + selector + "`"})
				}
			case "transition", "transition-property":
				if slowMove(decls, v) {
					uses = append(uses, use{node, fmt.Sprintf("slow transform transition on `%s`", selector)})
				}
			}
		}
	}
	for _, r := range m.rules {
		visit(r.sheet, r.selector, r.decls)
	}
	for n, decls := range m.inline {
		visit(n, "<"+elementLabel(n)+" style>", decls)
	}
	for _, s := range d.scriptSources() {
		if strings.Contains(s, ".animate(") && (strings.Contains(s, "transform") || strings.Contains(s, "translate")) {
			uses = append(uses, use{nil, "element.animate() with transforms in a script"})
			break
		}
	}
	if len(uses) == 0 {
		return nil
	}
	sort.SliceStable(uses, func(i, j int) bool { return d.positions[uses[i].node] < d.positions[uses[j].node] })
	var list []string
	seen := map[string]bool{}
	for _, u := range uses {
		if !seen[u.what] && len(list) < 3 {
			seen[u.what] = true
			list = append(list, u.what)
		}
	}
	if len(uses) > len(list) {
		list = append(list, fmt.Sprintf("%d more", len(uses)-len(list)))
	}
	return []Diagnostic{d.At(uses[0].node, Warning,
		"The page moves things but never checks prefers-reduced-motion: "+strings.Join(list, "; ")+".",
		"Add @media (prefers-reduced-motion: reduce) { … } that shows the reduced state: stop or shorten the movement and keep the content and state changes; or test matchMedia(\"(prefers-reduced-motion: reduce)\") in the script that moves things.")}
}

// mentions reports whether any stylesheet or script contains text.
func (d *Document) mentions(text string) bool {
	for _, s := range d.styleSources() {
		if strings.Contains(strings.ToLower(s.text), text) {
			return true
		}
	}
	for _, s := range d.scriptSources() {
		if strings.Contains(s, text) {
			return true
		}
	}
	return false
}

// keyframes indexes the @keyframes of every stylesheet once per document.
func (d *Document) keyframes() map[string]keyframe {
	if d.keyframeIndex == nil {
		d.keyframeIndex = map[string]keyframe{}
		for _, s := range d.styleSources() {
			for name, k := range keyframes(s.text) {
				d.keyframeIndex[name] = k
			}
		}
	}
	return d.keyframeIndex
}

// slowMove reports whether a transition moves an element over at least 400 ms.
func slowMove(decls []cssDecl, v string) bool {
	props, durations := v, v
	for _, dcl := range decls {
		if dcl.prop == "transition-duration" {
			durations = dcl.value
		}
		if dcl.prop == "transition-property" {
			props = dcl.value
		}
	}
	moves := false
	for _, id := range cssIdents(strings.ToLower(props)) {
		if motionProps[id] {
			moves = true
		}
	}
	if !moves {
		return false
	}
	for _, f := range strings.FieldsFunc(strings.ToLower(durations), func(r rune) bool { return r == ' ' || r == ',' }) {
		if ms, ok := cssTime(f); ok && ms >= 400 {
			return true
		}
	}
	return false
}

func cssTime(s string) (float64, bool) {
	var unit float64
	switch {
	case strings.HasSuffix(s, "ms"):
		s, unit = s[:len(s)-2], 1
	case strings.HasSuffix(s, "s"):
		s, unit = s[:len(s)-1], 1000
	default:
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v * unit, true
}

var cssIdent = regexp.MustCompile(`-?[a-z_][a-z0-9_-]*`)

func cssIdents(v string) []string { return cssIdent.FindAllString(v, -1) }

// keyframe is what one @keyframes block does.
type keyframe struct {
	moves  bool // changes a motion property in any frame
	hidden bool // its first frame (from, 0%) is invisible
}

// keyframes reads the @keyframes blocks of a raw sheet with a small
// brace-depth scanner, because the style model skips keyframes.
func keyframes(css string) map[string]keyframe {
	out := map[string]keyframe{}
	lower := strings.ToLower(css)
	for i := 0; i < len(lower); {
		at := strings.Index(lower[i:], "keyframes")
		if at < 0 {
			break
		}
		at += i
		open := strings.IndexByte(lower[at:], '{')
		if open < 0 {
			break
		}
		open += at
		name := strings.Trim(strings.TrimSpace(lower[at+len("keyframes"):open]), `"'`)
		var k keyframe
		depth, selector, end := 0, open+1, len(lower)
		for j := open; j < len(lower) && end == len(lower); j++ {
			switch lower[j] {
			case '{':
				depth++
				if depth == 2 {
					frame, body := strings.TrimSpace(lower[selector:j]), lower[j+1:]
					if close := strings.IndexByte(body, '}'); close >= 0 {
						body = body[:close]
					}
					first := strings.Contains(","+strings.ReplaceAll(frame, " ", "")+",", ",from,") || strings.Contains(","+strings.ReplaceAll(frame, " ", "")+",", ",0%,")
					for _, decl := range strings.Split(body, ";") {
						prop, value, ok := strings.Cut(decl, ":")
						if !ok {
							continue
						}
						prop, value = strings.TrimSpace(prop), strings.TrimSpace(value)
						if motionProps[prop] {
							k.moves = true
						}
						if first && (prop == "opacity" && zero(value) || prop == "visibility" && value == "hidden") {
							k.hidden = true
						}
					}
				}
			case '}':
				depth--
				if depth == 1 {
					selector = j + 1
				}
				if depth == 0 {
					end = j
				}
			}
		}
		out[name] = k
		i = end + 1
	}
	return out
}

// zero reports whether a CSS number or percentage is zero.
func zero(v string) bool {
	v = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(v, "!important")), "%")
	f, err := strconv.ParseFloat(v, 64)
	return err == nil && f == 0
}

type styleSource struct {
	node *html.Node
	text string
}

// styleSources returns the stylesheets of the page: inline <style> and emitted
// linked sheets. Style attributes hold no at-rules and are read from the style
// model instead.
func (d *Document) styleSources() []styleSource {
	if d.sheets != nil {
		return d.sheets
	}
	out := []styleSource{}
	for _, n := range d.activeNodes("style, link") {
		if n.Namespace != "" {
			continue
		}
		if n.Data == "style" {
			out = append(out, styleSource{n, Text(n)})
		} else if relHas(n, "stylesheet") {
			if raw, ok := d.Options.Stylesheets[Attr(n, "href")]; ok {
				out = append(out, styleSource{n, string(raw)})
			}
		}
	}
	d.sheets = out
	return out
}

func (d *Document) scriptSources() []string {
	var out []string
	for _, n := range d.activeNodes("script") {
		if kind := strings.ToLower(strings.TrimSpace(Attr(n, "type"))); !executable(kind) {
			continue
		}
		if HasAttr(n, "src") {
			continue // emitted script files are read from Assets below
		}
		out = append(out, Text(n))
	}
	for name, raw := range d.Options.Assets {
		if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".mjs") {
			out = append(out, string(raw))
		}
	}
	return out
}

// ---------- a11y.link-purpose ----------

// linkPurposeRule warns when links with the same accessible name lead to
// different destinations: "Buy" twice for two offers, "Learn more" for three
// pages. Visitors scanning links, screen readers listing them and agents
// choosing one cannot tell them apart.
func linkPurposeRule(d *Document) []Diagnostic {
	type group struct {
		first *html.Node
		label string
		count int
		hrefs map[string]bool
	}
	groups := map[string]*group{}
	var order []string
	for _, n := range d.activeNodes("a[href], area[href]") {
		if hidden(n) {
			continue
		}
		name := strings.Join(strings.Fields(d.controlName(n)), " ")
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		g := groups[key]
		if g == nil {
			g = &group{first: n, label: name, hrefs: map[string]bool{}}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
		g.hrefs[strings.TrimSpace(Attr(n, "href"))] = true
	}
	var issues []Diagnostic
	for _, key := range order {
		g := groups[key]
		if len(g.hrefs) < 2 {
			continue
		}
		issues = append(issues, d.At(g.first, Warning,
			fmt.Sprintf("%d links named %q lead to %d different destinations.", g.count, g.label, len(g.hrefs)),
			"Name each link by where it goes or what it buys (\"Buy the starter kit\", \"Read the method\"); a visually short label can carry the full name in aria-label."))
	}
	return issues
}

// ---------- copy.placeholder ----------

// Template filler is matched case-sensitively where case carries the meaning
// (TODO, TBD, FIXME are markers; "todo" is a word).
var placeholderText = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\blorem ipsum\b`),
	regexp.MustCompile(`\b(?:TODO|TBD|FIXME)\b`),
	regexp.MustCompile(`(?i)\byour (?:headline|tagline|title|text|copy|name|company|product|description|logo) (?:goes )?here\b`),
	regexp.MustCompile(`(?i)\[(?:insert|add|your|placeholder)[^\]]{0,40}\]`),
}

// placeholderRule reports template filler in visible text, alternative text and
// metadata, and the starter description, alt text and image of lfm init once
// the starter page itself has been replaced.
func placeholderRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	seen := map[string]bool{}
	report := func(n *html.Node, what, found string) {
		if seen[found] {
			return
		}
		seen[found] = true
		issues = append(issues, d.At(n, Warning, fmt.Sprintf("Placeholder %s left in the page: %q.", what, preview(found)), "Replace it with the real content; a page with filler reads as unfinished, and search and share previews show it."))
	}
	check := func(n *html.Node, what, text string) {
		for _, re := range placeholderText {
			if m := re.FindString(text); m != "" {
				report(n, what, m)
				return
			}
		}
		for _, starter := range d.Options.StarterTexts {
			if starter != "" && strings.Contains(text, starter) {
				report(n, what, starter)
				return
			}
		}
	}
	for _, s := range copySegments(d) {
		check(s.node, "text", s.text)
	}
	for _, n := range d.Elements {
		if inTemplate(n) {
			continue
		}
		switch {
		case n.Data == "img" || n.Data == "area" || (n.Data == "input" && strings.EqualFold(Attr(n, "type"), "image")):
			check(n, "alternative text", Attr(n, "alt"))
		case n.Data == "meta":
			key := strings.ToLower(Attr(n, "name") + Attr(n, "property"))
			if key == "description" || strings.HasSuffix(key, ":description") || strings.HasSuffix(key, ":title") || strings.HasSuffix(key, ":alt") {
				check(n, "metadata", Attr(n, "content"))
			}
		case n.Data == "title" && n.Namespace == "":
			check(n, "title", Text(n))
		}
	}
	if len(d.Options.StarterImages) > 0 {
		names := sortedNames(d.Options.Assets)
		for _, name := range names {
			sum := sha256.Sum256(d.Options.Assets[name])
			for _, starter := range d.Options.StarterImages {
				if sum == starter {
					issues = append(issues, d.Whole(Warning, fmt.Sprintf("%s is still the starter placeholder image of lfm init.", name), "Make a social image that shows the offer at thumbnail size and point socialImage.src in lfm.json at it."))
				}
			}
		}
	}
	return issues
}

func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 60 {
		return string(r[:59]) + "…"
	}
	return s
}

// ---------- ux.hidden-start ----------

// hiddenStartRule warns when the main heading begins invisible: opacity 0 or
// hidden visibility in the base styles, or an animation whose first frame is
// invisible. Such content waits for scripts or animation, so slow phones,
// crawlers and no-JS readers see an empty first screen, and the largest paint
// comes late.
func hiddenStartRule(d *Document) []Diagnostic {
	var h1 *html.Node
	for _, n := range d.activeNodes("h1") {
		if designVisible(n) {
			h1 = n
			break
		}
	}
	if h1 == nil {
		return nil
	}
	m := d.styles()
	for n := h1; n != nil && n.Type == html.ElementNode && n.Data != "body" && n.Data != "html"; n = n.Parent {
		if HasAttr(n, "data-design-reason") {
			return nil
		}
		cause := ""
		switch {
		case zero(m.value(n, "opacity")):
			cause = "opacity: 0"
		case strings.EqualFold(m.value(n, "visibility"), "hidden"):
			cause = "visibility: hidden"
		default:
			for _, prop := range []string{"animation", "animation-name"} {
				for _, name := range cssIdents(strings.ToLower(m.value(n, prop))) {
					if d.keyframes()[name].hidden {
						cause = "the animation " + name + " starts invisible"
					}
				}
			}
		}
		if cause != "" {
			what := "The main heading"
			if n != h1 {
				what = "The main heading's container <" + elementLabel(n) + ">"
			}
			return []Diagnostic{d.At(h1, Warning, what+" starts invisible ("+cause+") and appears only when a script or animation runs.",
				"Render it in its final state; animate something that does not hide content, or start from a visible state. Slow phones, crawlers and no-JS readers otherwise meet an empty first screen.")}
		}
	}
	return nil
}

// ---------- copy.actions ----------

// genericActionNames are complete control names that say nothing about where
// they lead or what they do. Only the whole name is matched, so "Learn more
// about the rota" passes.
var genericActionNames = map[string]bool{
	"click here": true, "here": true, "learn more": true, "read more": true, "more": true, "more…": true, "submit": true,
	"send": true, "go": true, "get started": true, "start now": true, "start here": true, "details": true, "more info": true,
	"discover more": true, "mehr": true, "weiter": true, "weiterlesen": true, "mehr erfahren": true, "hier": true,
	"hier klicken": true, "klicken sie hier": true, "absenden": true, "senden": true, "jetzt starten": true,
	"los geht's": true, "loslegen": true, "mehr entdecken": true, "zum angebot": true,
}

// actionsRule warns for links and buttons named only by a generic label.
func actionsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	seen := map[string]bool{}
	for _, n := range d.activeNodes("a[href], button, input[type=submit], input[type=button], summary") {
		if hidden(n) || HasAttr(n, "data-design-reason") {
			continue
		}
		name := strings.ToLower(strings.Join(strings.Fields(normalizeQuotes(d.controlName(n))), " "))
		name = strings.TrimRight(name, " .!→›»>")
		if n.Data == "input" {
			name = strings.ToLower(strings.TrimSpace(Attr(n, "value")))
		}
		if !genericActionNames[name] || seen[name] {
			continue
		}
		seen[name] = true
		issues = append(issues, d.At(n, Warning, fmt.Sprintf("Action named %q says nothing about what happens.", name), "Name the next step and its outcome: \"See your sun plan\", \"Buy the guide for 12 €\", \"Send the request\"."))
	}
	return issues
}
