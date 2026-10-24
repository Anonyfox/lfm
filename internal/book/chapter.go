package book

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/Anonyfox/lfm/internal/aitext"
	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// quoteStyles are the quotation marks of a language: left and right double,
// then left and right single. English marks are the default.
var quoteStyles = map[string][4]string{
	"en":    {"“", "”", "‘", "’"},
	"de":    {"„", "“", "‚", "‘"},
	"de-ch": {"«", "»", "‹", "›"},
	"fr":    {"«\u00a0", "\u00a0»", "‹\u00a0", "\u00a0›"},
	"es":    {"«", "»", "“", "”"},
	"it":    {"«", "»", "“", "”"},
	"nl":    {"“", "”", "‘", "’"},
	"pl":    {"„", "”", "‚", "’"},
	"cs":    {"„", "“", "‚", "‘"},
	"sv":    {"”", "”", "’", "’"},
	"da":    {"»", "«", "›", "‹"},
}

// typographerFor turns straight quotes, dashes and dots into the typographic
// characters of a language. The characters themselves, not HTML entities:
// XHTML in an EPUB knows only the five XML entities.
func typographerFor(language string) goldmark.Extender {
	lang := strings.ToLower(language)
	q, ok := quoteStyles[lang]
	if !ok {
		base, _, _ := strings.Cut(lang, "-")
		if q, ok = quoteStyles[base]; !ok {
			q = quoteStyles["en"]
		}
	}
	return extension.NewTypographer(extension.WithTypographicSubstitutions(extension.TypographicSubstitutions{
		extension.LeftDoubleQuote:  []byte(q[0]),
		extension.RightDoubleQuote: []byte(q[1]),
		extension.LeftSingleQuote:  []byte(q[2]),
		extension.RightSingleQuote: []byte(q[3]),
		extension.EnDash:           []byte("–"),
		extension.EmDash:           []byte("—"),
		extension.Ellipsis:         []byte("…"),
		extension.LeftAngleQuote:   []byte("«"),
		extension.RightAngleQuote:  []byte("»"),
		extension.Apostrophe:       []byte("’"),
	}))
}

var typographer = typographerFor("en")

// markdown is the one Markdown dialect of the book: CommonMark with tables,
// strikethrough, footnotes and typographic quotes and dashes.
var markdown = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Footnote, typographer))

type heading struct {
	level int
	text  string
	line  int
}

type block struct {
	text    string // with straight quotes, as the checks' patterns are written
	raw     string // as written, for the AI-text estimate
	line    int
	list    bool
	quote   bool
	link    bool // the block links a source
	at      int  // words of the chapter before the block
	section int  // index of the ## section the block is in; 0 before the first
	bold    int  // strong spans in the block
	label   bool // a list item that starts with a bold label ending in a colon
	allBold bool // the whole block is strong
}

type ref struct {
	dest, alt string
	line      int
}

// prose is what the checks need from one chapter.
type prose struct {
	headings     []heading
	blocks       []block // paragraphs and list item text, in order
	images       []ref
	links        []ref
	words        int // all words outside code
	listWords    int
	quoteWords   int
	sectionEmpty []heading // headings directly followed by a heading of the same or a higher level
	rawHTML      []int     // lines with HTML, which the book does not render
	tables       []int     // word offsets of tables
	external     int       // links to https sources
}

// lines maps byte offsets to 1-based line numbers.
type lines []int

func newLines(src []byte) lines {
	starts := lines{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func (l lines) at(offset int) int { return sort.SearchInts(l, offset+1) }

func blockLine(n ast.Node, l lines) int {
	for c := n; c != nil; c = c.Parent() {
		if c.Type() == ast.TypeBlock && c.Lines().Len() > 0 {
			return l.at(c.Lines().At(0).Start)
		}
		if c.Type() == ast.TypeBlock {
			if first := c.FirstChild(); first != nil && first.Type() == ast.TypeBlock && first.Lines().Len() > 0 {
				return l.at(first.Lines().At(0).Start)
			}
		}
	}
	return 1
}

// inlineText collects the readable text of an inline subtree.
func inlineText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := c.(type) {
		case *ast.Text:
			b.Write(v.Segment.Value(src))
			if v.SoftLineBreak() || v.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(v.Value)
		case *ast.Image:
			if c != n {
				return ast.WalkSkipChildren, nil // an image's alt text is not prose
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

func parseChapter(src []byte) (ast.Node, prose) {
	doc := markdown.Parser().Parse(text.NewReader(src))
	l := newLines(src)
	var p prose
	section := 0
	inside := func(n ast.Node, kind ast.NodeKind) bool {
		for c := n.Parent(); c != nil; c = c.Parent() {
			if c.Kind() == kind {
				return true
			}
		}
		return false
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Heading:
			p.headings = append(p.headings, heading{v.Level, check.Straight(inlineText(v, src)), blockLine(v, l)})
			if v.Level == 2 {
				section++
			}
			return ast.WalkSkipChildren, nil
		case *east.Table:
			p.tables = append(p.tables, p.words)
			return ast.WalkSkipChildren, nil
		case *ast.HTMLBlock:
			p.rawHTML = append(p.rawHTML, blockLine(v, l))
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			p.rawHTML = append(p.rawHTML, blockLine(v, l))
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph, *ast.TextBlock:
			raw := inlineText(v, src)
			t := check.Straight(raw)
			b := block{text: t, raw: raw, line: blockLine(v, l), list: inside(v, ast.KindListItem), quote: inside(v, ast.KindBlockquote), at: p.words, section: section}
			strongWords := 0
			_ = ast.Walk(v, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
				if !entering {
					return ast.WalkContinue, nil
				}
				if c.Kind() == ast.KindLink || c.Kind() == ast.KindAutoLink {
					b.link = true
				}
				if e, ok := c.(*ast.Emphasis); ok && e.Level == 2 {
					b.bold++
					strong := inlineText(e, src)
					strongWords += len(strings.Fields(strong))
					if c.Parent() == v && v.FirstChild() == c && b.list && strings.HasSuffix(strings.TrimSpace(strong), ":") {
						b.label = true
					}
					return ast.WalkSkipChildren, nil
				}
				return ast.WalkContinue, nil
			})
			b.allBold = strongWords > 0 && strongWords == len(strings.Fields(t)) && strongWords >= 8
			p.blocks = append(p.blocks, b)
			w := len(strings.Fields(t))
			p.words += w
			if b.list {
				p.listWords += w
			}
			if b.quote {
				p.quoteWords += w
			}
		case *ast.Image:
			p.images = append(p.images, ref{string(v.Destination), inlineText(v, src), blockLine(v, l)})
		case *ast.Link:
			p.links = append(p.links, ref{dest: string(v.Destination), line: blockLine(v, l)})
			if strings.HasPrefix(string(v.Destination), "https://") {
				p.external++
			}
		case *ast.AutoLink:
			p.links = append(p.links, ref{dest: string(v.URL(src)), line: blockLine(v, l)})
			if strings.HasPrefix(string(v.URL(src)), "https://") {
				p.external++
			}
		}
		return ast.WalkContinue, nil
	})
	// An empty section: a heading followed by a heading of the same or a
	// higher level with nothing between them.
	var prev ast.Node
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if h, ok := c.(*ast.Heading); ok {
			if ph, ok := prev.(*ast.Heading); ok && h.Level <= ph.Level {
				p.sectionEmpty = append(p.sectionEmpty, heading{ph.Level, check.Straight(inlineText(ph, src)), blockLine(ph, l)})
			}
		}
		prev = c
	}
	return doc, p
}

var sentenceEnd = regexp.MustCompile(`[.!?…]+["”’)]*(\s+|$)`)

// sentenceLengths splits prose paragraphs (not lists or quotes) into sentences
// and returns their word counts.
func sentenceLengths(p prose) []int {
	var out []int
	for _, b := range p.blocks {
		if b.list || b.quote {
			continue
		}
		for _, s := range sentenceEnd.Split(b.text, -1) {
			if n := len(strings.Fields(s)); n > 0 {
				out = append(out, n)
			}
		}
	}
	return out
}

// variation is the coefficient of variation of the values.
func variation(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += float64(v)
	}
	mean := sum / float64(len(values))
	var sq float64
	for _, v := range values {
		sq += (float64(v) - mean) * (float64(v) - mean)
	}
	return math.Sqrt(sq/float64(len(values))) / mean
}

// Thresholds of the prose checks; each is far from what edited human prose shows.
const (
	minSentencesForRhythm = 40   // sentences before rhythm is judged
	uniformRhythm         = 0.32 // coefficient of variation of sentence length below this reads as generated
	dashesPer1000         = 6.0
	minDashesChapter      = 6
	wordsPerHeading       = 120 // fewer prose words per heading reads as a listicle
	minHeadingsDense      = 5
	listShare             = 0.5
	minWordsForShares     = 400
	openerShare           = 0.15
	minOpenerRepeats      = 4
)

// bookTells are the habits of generated nonfiction beyond the copy tells:
// announcing the chapter, travel metaphors, and claims of unnamed studies.
var bookTells = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bin this (?:chapter|section|book),? (?:we|you)(?:'ll| will| are going to)\b`),
	regexp.MustCompile(`(?i)\bin diesem (?:kapitel|abschnitt|buch) (?:werden wir|wirst du|lernst du|erfährst du|schauen wir)\b`),
	regexp.MustCompile(`(?i)\b(?:fascinating|exciting) world of\b`),
	regexp.MustCompile(`(?i)\b(?:faszinierende|spannende) welt\b`),
	regexp.MustCompile(`(?i)\b(?:ein)?tauch(?:en|t|e) (?:wir |du |sie )?(?:tief(?:er)? )?(?:ein|in)\b`),
	regexp.MustCompile(`(?i)\bjourney,? not (?:a|the) destination\b`),
	regexp.MustCompile(`(?i)\breise,? (?:kein|nicht das) ziel\b`),
	regexp.MustCompile(`(?i)\blet'?s (?:explore|dive|take a (?:closer|deeper) look)\b`),
	regexp.MustCompile(`(?i)\blass(?:t)? uns (?:gemeinsam )?(?:eintauchen|entdecken|einen (?:genaueren|tieferen) blick)\b`),
}

// unsourced matches claims that need a source in the same paragraph: studies,
// research and experts in general, and shares of people.
var unsourced = regexp.MustCompile(`(?i)\b(?:studies|research|experts|surveys?) (?:show|shows|have shown|has shown|suggest|suggests|agree|say|found)\b|\b(?:studien|untersuchungen|umfragen|experten|forscher) (?:zeigen|belegen|haben gezeigt|sind sich einig|sagen|fanden)\b|\blaut (?:einer |neuen |aktuellen )?(?:studie|umfrage|untersuchung)\b|\baccording to (?:a |one |recent )?(?:study|survey|research)\b|\b\d+(?:[.,]\d+)?\s?(?:%|percent|prozent) (?:of|aller|der|von den)\b`)

func opener(s string) string {
	words := aitext.Runs(strings.ToLower(s), 2, '\'')
	if len(words) < 2 {
		return ""
	}
	return words[0] + " " + words[1]
}

// checkChapter checks one written chapter against its plan.
func checkChapter(ch Chapter, src []byte, plan Plan, cfg Config, exists func(bookRel string) bool) (prose, []diagnostic.Diagnostic) {
	file := Dir + "/" + ch.File
	var issues []diagnostic.Diagnostic
	warn := func(rule string, line int, message, hint string) {
		issues = append(issues, at(file, line, rule, diagnostic.Warning, message, hint))
	}
	_, p := parseChapter(src)
	if ch.Status == "planned" {
		return p, nil
	}
	if plan.Kind == "fiction" {
		checkOutline(ch, p, plan, warn)
	}

	// structure
	var tops []heading
	for _, h := range p.headings {
		if h.level == 1 {
			tops = append(tops, h)
		}
	}
	switch {
	case len(p.headings) == 0 || p.headings[0].level != 1:
		warn(RuleChapter, 1, fmt.Sprintf("The chapter does not start with \"# %s\".", ch.Title), "Open every chapter with its planned title as the one top heading.")
	case !strings.EqualFold(strings.TrimSpace(p.headings[0].text), strings.TrimSpace(check.Straight(ch.Title))):
		warn(RuleChapter, p.headings[0].line, fmt.Sprintf("The title %q differs from the plan's %q.", p.headings[0].text, ch.Title), "Change the title in the concept first when it changed, then here.")
	}
	if len(tops) > 1 {
		warn(RuleChapter, tops[1].line, fmt.Sprintf("%d top headings; a chapter has one.", len(tops)), "Use ## for the chapter's sections.")
	}
	for i := 1; i < len(p.headings); i++ {
		if p.headings[i].level > p.headings[i-1].level+1 {
			warn(RuleChapter, p.headings[i].line, fmt.Sprintf("Heading level jumps from %d to %d.", p.headings[i-1].level, p.headings[i].level), "Nest headings one level at a time; readers' tables of contents depend on it.")
		}
	}
	for _, h := range p.sectionEmpty {
		warn(RuleChapter, h.line, fmt.Sprintf("Section %q is empty.", h.text), "Write the section or remove its heading.")
	}
	if ch.Words > 0 {
		low, high := int(float64(ch.Words)*(1-WordTolerance)), int(float64(ch.Words)*(1+WordTolerance))
		if p.words < low || p.words > high {
			warn(RuleChapter, 1, fmt.Sprintf("%d words against a target of %d.", p.words, ch.Words), "Bring the chapter into its range, or change its target in the concept when the plan changed.")
		}
	}
	chapterDir := path.Dir(ch.File)
	for _, img := range p.images {
		rel := path.Clean(path.Join(chapterDir, img.dest))
		switch {
		case strings.Contains(img.dest, "://"):
			warn(RuleChapter, img.line, "Image "+img.dest+" is remote.", "Store the image in deliverables/book/images; books are read offline.")
		case outside(rel):
			warn(RuleChapter, img.line, "Image "+img.dest+" lies outside deliverables/book.", "Copy it into deliverables/book/images and link it as ../images/name.jpg; the book carries only its own files.")
		case !exists(rel):
			warn(RuleChapter, img.line, "Image "+img.dest+" does not exist.", "Link images relative to the chapter, e.g. ../images/name.jpg.")
		}
		if strings.TrimSpace(img.alt) == "" {
			warn(RuleChapter, img.line, "Image "+img.dest+" has no alternative text.", "Describe what the image shows for readers who cannot see it.")
		}
	}
	chapters := map[string]bool{}
	for _, c := range plan.Chapters {
		chapters[c.File] = true
	}
	for _, link := range p.links {
		dest, _, _ := strings.Cut(link.dest, "#")
		switch {
		case strings.HasPrefix(link.dest, "http://"):
			warn(RuleChapter, link.line, "Link "+preview(link.dest)+" uses plain http.", "Link the https version.")
		case strings.HasPrefix(link.dest, "https://"), strings.HasPrefix(link.dest, "mailto:"), strings.HasPrefix(link.dest, "#"):
		case dest != "" && !chapters[path.Clean(path.Join(chapterDir, dest))]:
			warn(RuleChapter, link.line, "Link "+preview(link.dest)+" points at no chapter.", "Link other chapters by their file, e.g. 02-name.md.")
		}
	}
	if len(p.rawHTML) > 0 {
		warn(RuleChapter, p.rawHTML[0], fmt.Sprintf("%d places use HTML, which the book leaves out.", len(p.rawHTML)), "Write it in Markdown: images as ![alt](../images/x.jpg), emphasis with * or _, tables with pipes.")
	}
	for _, b := range p.blocks {
		if m := check.Placeholder(b.text); m != "" {
			warn(RuleChapter, b.line, fmt.Sprintf("Filler left in the chapter: %q.", m), "Replace it with the real content.")
			break
		}
	}

	// prose
	var segments []string
	var segLines []int
	dashes := 0
	firstDash := 0
	for _, b := range p.blocks {
		if b.quote {
			continue
		}
		segments = append(segments, b.text)
		segLines = append(segLines, b.line)
		if n := check.ProseDashes(b.text); n > 0 {
			if firstDash == 0 {
				firstDash = b.line
			}
			dashes += n
		}
	}
	for _, h := range check.FindTells(segments) {
		message := fmt.Sprintf("%q is a stock phrase of generated prose.", h.Phrase)
		if h.Repeat {
			message = fmt.Sprintf("%q and %d more like it: a construction generated prose repeats.", h.Phrase, h.Count-1)
		} else if h.Count > 1 {
			message = fmt.Sprintf("%q appears %d times: a stock phrase of generated prose.", h.Phrase, h.Count)
		}
		warn(RuleCopy, segLines[h.Segment], message, "Say the concrete thing: the fact, case, number or step it stands for.")
	}
	prose := p.words - p.quoteWords
	if dashes >= minDashesChapter && float64(dashes)*1000 > dashesPer1000*float64(prose) {
		warn(RuleCopy, firstDash, fmt.Sprintf("%d em dashes in %d words.", dashes, prose), "Recast most of them as two sentences, a comma, a colon or parentheses.")
	}
	if lengths := sentenceLengths(p); len(lengths) >= minSentencesForRhythm {
		if v := variation(lengths); v < uniformRhythm {
			warn(RuleCopy, 1, fmt.Sprintf("Sentences barely vary in length (variation %.2f over %d sentences).", v, len(lengths)), "Mix short and long sentences the way the thought moves; uniform rhythm reads as generated.")
		}
	}
	if sub := len(p.headings) - 1; sub >= minHeadingsDense && prose/sub < wordsPerHeading {
		warn(RuleCopy, 1, fmt.Sprintf("%d headings for %d words: a heading every %d words.", sub, prose, prose/sub), "Let sections develop; merge headings that only announce a paragraph.")
	}
	if p.words >= minWordsForShares && float64(p.listWords) > listShare*float64(p.words) {
		warn(RuleCopy, 1, fmt.Sprintf("%d of %d words are in lists.", p.listWords, p.words), "Write the reasoning as prose; keep lists for steps and true enumerations.")
	}
	for _, b := range p.blocks {
		if b.quote {
			continue
		}
		if plan.Kind != "fiction" {
			for _, re := range bookTells {
				if m := re.FindString(b.text); m != "" {
					warn(RuleCopy, b.line, fmt.Sprintf("%q is a stock line of generated nonfiction.", m), "Start with the thing itself; the reader does not need it announced or framed.")
					break
				}
			}
			if m := unsourced.FindString(b.text); m != "" && !b.link {
				warn(RuleCopy, b.line, fmt.Sprintf("%q makes a claim without a source in the paragraph.", m), "Link the study, survey or source where the claim is made, name it, or cut the claim.")
			}
		}
	}
	questions, firstSentences := 0, 0
	for _, b := range p.blocks {
		if b.list || b.quote {
			continue
		}
		firstSentences++
		if first := sentenceEnd.FindStringIndex(b.text); first != nil && strings.Contains(b.text[first[0]:first[1]], "?") {
			questions++
		}
	}
	if plan.Kind != "fiction" && questions >= 3 && float64(questions) > 0.2*float64(firstSentences) {
		warn(RuleCopy, 1, fmt.Sprintf("%d paragraphs open with a question.", questions), "Answer instead of asking; keep a question where the reader would really ask it.")
	}
	openers := map[string][]int{}
	paragraphs := 0
	for _, b := range p.blocks {
		if b.list || b.quote {
			continue
		}
		paragraphs++
		if o := opener(b.text); o != "" {
			openers[o] = append(openers[o], b.line)
		}
	}
	for o, at := range openers {
		if len(at) >= minOpenerRepeats && float64(len(at)) > openerShare*float64(paragraphs) {
			warn(RuleCopy, at[0], fmt.Sprintf("%d paragraphs open with %q.", len(at), o), "Vary how paragraphs begin; start from what the paragraph is about.")
		}
	}
	lower := strings.ToLower(check.Straight(string(src)))
	for _, phrase := range plan.Avoid {
		if i := strings.Index(lower, strings.ToLower(check.Straight(phrase))); i >= 0 {
			warn(RuleCopy, newLines(src).at(i), fmt.Sprintf("The chapter says %q, which the Voice avoids.", phrase), "Replace it, or change the Voice in the concept when the decision changed.")
		}
	}
	qualityChecks(measure(p, cfg.Language), p, plan, cfg.Language, cfg.Author, warn)
	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Location.Line < issues[j].Location.Line })
	return p, issues
}

// containsPhrase reports whether one sentence of text holds every word of
// phrase, in any order: German splits verbs ("geht er nicht mehr auf") and
// every language moves words around a phrase it uses.
func containsPhrase(text, phrase string) bool {
	norm := func(s string) []string {
		s = strings.ToLower(check.Straight(s))
		return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '\'' })
	}
	want := norm(phrase)
	if len(want) == 0 {
		return true
	}
	for _, sentence := range sentenceEnd.Split(text, -1) {
		have := map[string]bool{}
		for _, w := range norm(sentence) {
			have[w] = true
		}
		all := true
		for _, w := range want {
			all = all && have[w]
		}
		if all {
			return true
		}
	}
	return false
}

// outside reports whether a cleaned book-relative path leaves the book folder.
func outside(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel)
}

// hasCastName matches a name and its English or German possessive.
func hasCastName(text, name string) bool {
	re, err := regexp.Compile(`(?:^|[^\p{L}\p{N}_])` + regexp.QuoteMeta(check.Straight(name)) + `(?:s|'s|’s)?(?:$|[^\p{L}\p{N}_])`)
	return err == nil && re.MatchString(text)
}

var plannedHook = regexp.MustCompile(`Hook:\s*(?:"([^"]+)"|„([^“]+)“|“([^”]+)”)`)

func checkOutline(ch Chapter, p prose, plan Plan, warn func(string, int, string, string)) {
	text := strings.Join(blockTexts(p), " ")
	for _, name := range plan.Cast {
		if hasCastName(ch.Job+" "+ch.Beats, name) && !hasCastName(text, name) {
			warn(RuleChapter, 1, fmt.Sprintf("The outline names %s for chapter %d; the chapter never mentions %s.", name, ch.N, name), "Follow the outline or update the chapter row.")
		}
	}
	if m := plannedHook.FindStringSubmatch(ch.Beats); m != nil {
		hook := check.Straight(m[1] + m[2] + m[3])
		words := strings.Fields(text)
		ending := strings.Join(words[max(0, len(words)-300):], " ")
		if !strings.Contains(ending, strings.Join(strings.Fields(hook), " ")) {
			warn(RuleChapter, 1, fmt.Sprintf("The planned hook %q is not in the chapter's ending.", hook), "Use the planned hook in the last 300 words or update the outline.")
		}
	}
}
