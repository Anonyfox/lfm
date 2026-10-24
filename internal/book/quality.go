package book

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/aitext"
	"github.com/Anonyfox/lfm/internal/check"
)

// The quality checks follow what readers punish in practical nonfiction and
// what still marks current generated prose. Readers' one-star reviews name thin
// value per page: one idea stretched to book length, the same point restated,
// generic advice without names, numbers or steps, claims nobody can check.
// Current models avoid the old giveaway words; what marks their prose is
// structural: correcting a claim nobody made ("It isn't X. It's Y."),
// announcing the chapter, closing with encouragement or a recap, bold labels,
// pet words. Each threshold sits beyond what edited human nonfiction shows.

// Language groups of the lexical checks; other languages get the structural
// and numeric checks only.
func langOf(language string) string {
	base, _, _ := strings.Cut(strings.ToLower(language), "-")
	if base == "" {
		return "en"
	}
	return base
}

// sentence is one sentence of a prose block.
type sentence struct {
	text  string
	words int
	block int // index into prose.blocks
	first bool
}

func sentencesOf(p prose) []sentence {
	var out []sentence
	for i, b := range p.blocks {
		if b.quote || b.list {
			continue
		}
		first := true
		for _, s := range sentenceSplit(b.text) {
			if n := len(strings.Fields(s)); n > 0 {
				out = append(out, sentence{s, n, i, first})
				first = false
			}
		}
	}
	return out
}

// sentenceSplit keeps each sentence's closing punctuation.
func sentenceSplit(text string) []string {
	var out []string
	last := 0
	for _, m := range sentenceEnd.FindAllStringIndex(text, -1) {
		out = append(out, strings.TrimSpace(text[last:m[1]]))
		last = m[1]
	}
	if rest := strings.TrimSpace(text[last:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

var (
	quotedSpeech = regexp.MustCompile(`[“„«»"][^“”„«»"]{3,}[”“«»"]`)
	unitPattern  = regexp.MustCompile(`(?i)\d\s?(?:%|°|g\b|kg\b|mg\b|ml\b|l\b|cm\b|mm\b|m\b|km\b|min\b|minutes?\b|minuten\b|h\b|hours?\b|stunden?\b|tage?n?\b|days?\b|weeks?\b|wochen?\b|€|\$|£|eur\b|usd\b|euro\b|dollars?\b)|[€$£]\s?\d`)
	acronym      = regexp.MustCompile(`(?:^|[^\p{L}])\p{Lu}{2,}(?:$|[^\p{L}])`)
	numberRun    = regexp.MustCompile(`\d+(?:[.,]\d+)?`)
)

// specifics counts what makes prose concrete: numbers, quoted speech and
// proper nouns (capitalized words inside a sentence). German capitalizes
// every noun, so there only numbers, quotes and acronyms count.
func specifics(s string, bl bookLang) int {
	n := len(numberRun.FindAllString(s, -1)) + len(quotedSpeech.FindAllString(s, -1))
	if !bl.properNouns {
		return n + len(acronym.FindAllString(s, -1))
	}
	words := strings.Fields(s)
	for i, w := range words {
		if i == 0 {
			continue
		}
		prev := words[i-1]
		if strings.HasSuffix(prev, ".") || strings.HasSuffix(prev, ":") || strings.HasSuffix(prev, "?") || strings.HasSuffix(prev, "!") {
			continue
		}
		w = strings.Trim(w, "\"“”‘’'()[],;.:!?")
		if w == "" || w == "I" || w == "I’m" || w == "I'm" {
			continue
		}
		if r := []rune(w); unicode.IsUpper(r[0]) {
			n++
		}
	}
	return n
}

// Leftovers of chat tools, the English participle tell, and emoji.
var (
	chatArtifact       = regexp.MustCompile(`(?i)oaicite|contentReference\[|\[cite:|utm_source=(?:chatgpt|openai|claude|perplexity)|turn\d+search\d+`)
	trailingParticiple = regexp.MustCompile(`, (?:highlighting|underscoring|emphasizing|emphasising|showcasing|fostering|reflecting|ensuring|contributing to|illustrating|demonstrating|signaling|signalling)\b[^.!?]{0,90}[.!?]$`)
	emoji              = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}\x{1F000}-\x{1F2FF}]`)
)

// quality is what the quality checks measure in one chapter.
type quality struct {
	words, sentences, paragraphs int
	corrective                   []int // lines
	signposts                    []found
	hard                         []found
	pets                         map[string]int
	petLine                      int
	stock                        []found
	vague, artifacts             []found
	hedges, transitions          []int
	participles                  []int
	emoji                        []int
	bold, labels                 int
	boldLine                     int
	allBold                      []int
	specifics                    int
	thinWindow                   int // line of the first 400-word stretch without any specific, or 0
	nominal                      int
	nominalTop                   []string
	oneLiners                    int
	fragments                    int
	paragraphMean, paragraphCV   float64
	sectionCV                    float64
	sections                     int
	firstValue                   int // words before the first value unit, -1 if none
	valueGap, gapLine            int
	opening                      string // why the opening is weak, or ""
	openingLine                  int
	ending                       string
	endingLine                   int
	readability                  float64 // the language's readability formula
	readLow, readHigh            float64
	readLabel                    string
	reading                      readingStats
	wstf                         float64 // Wiener Sachtextformel (de)
	meanSentence                 float64
}

type found struct {
	text string
	line int
}

// measure computes the quality profile of a parsed chapter.
func measure(p prose, language string) quality {
	lang := langOf(language)
	bl, _ := bookLangOf(language)
	q := quality{pets: map[string]int{}, firstValue: -1}
	sentences := sentencesOf(p)
	q.sentences = len(sentences)
	var lengths, paragraphLengths []int
	words, syllables, longWords, polysyllables, monosyllables, letters, veryLong := 0, 0, 0, 0, 0, 0, 0
	for i, s := range sentences {
		lengths = append(lengths, s.words)
		line := p.blocks[s.block].line
		if s.words <= 4 {
			q.fragments++
		}
		if bl.inline.MatchString(s.text) {
			q.corrective = append(q.corrective, line)
		} else if i > 0 && sentences[i-1].block == s.block && bl.correction.MatchString(s.text) && negated(sentences[i-1].text, bl) && sentences[i-1].words <= 25 {
			q.corrective = append(q.corrective, line)
		}
		if bl.transitions.MatchString(s.text) {
			q.transitions = append(q.transitions, line)
		}
		if lang == "en" && trailingParticiple.MatchString(s.text) {
			q.participles = append(q.participles, line)
		}
		for _, w := range aitext.Runs(s.text, -1, '\'') {
			n := syllablesOf(w, bl, lang)
			letters += utf8.RuneCountInString(w)
			if n >= 4 {
				veryLong++
			}
			words++
			syllables += n
			if n >= 3 {
				polysyllables++
			}
			if n == 1 {
				monosyllables++
			}
			if len([]rune(w)) > 6 {
				longWords++
			}
		}
	}
	nominals := map[string]int{}
	totalWords := 0
	for i, b := range p.blocks {
		if b.quote {
			continue
		}
		bw := len(strings.Fields(b.text))
		totalWords += bw
		if !b.list {
			q.paragraphs++
			paragraphLengths = append(paragraphLengths, bw)
			if bw < 15 && len(sentenceSplit(b.text)) == 1 {
				q.oneLiners++
			}
		}
		if m := aitext.Trim(bl.signposts.FindString(b.text)); m != "" {
			last := i == len(p.blocks)-1 || i == lastProse(p)
			if !(last && bl.forwardBridge.MatchString(b.text)) {
				q.signposts = append(q.signposts, found{m, b.line})
			}
		}
		for _, m := range bl.hardTells.FindAllString(b.text, -1) {
			q.hard = append(q.hard, found{aitext.Trim(m), b.line})
		}
		for _, m := range bl.petWords.FindAllString(b.text, -1) {
			// one key per word family: zuverlässig, zuverlässige, zuverlässigen …
			key := stem(strings.ToLower(aitext.Trim(m)))
			if q.pets[key] == 0 && q.petLine == 0 {
				q.petLine = b.line
			}
			q.pets[key]++
		}
		for _, m := range bl.stockNames.FindAllString(b.text, -1) {
			q.stock = append(q.stock, found{aitext.Trim(m), b.line})
		}
		if m := aitext.Trim(bl.vagueSource.FindString(b.text)); m != "" && !b.link {
			q.vague = append(q.vague, found{m, b.line})
		}
		if m := chatArtifact.FindString(b.text); m != "" {
			q.artifacts = append(q.artifacts, found{m, b.line})
		}
		for range aitext.CountWords(bl.hedges, b.text) {
			q.hedges = append(q.hedges, b.line)
		}
		if emoji.MatchString(b.text) {
			q.emoji = append(q.emoji, b.line)
		}
		q.bold += b.bold
		if b.bold > 0 && q.boldLine == 0 {
			q.boldLine = b.line
		}
		if b.label {
			q.labels++
		}
		if b.allBold {
			q.allBold = append(q.allBold, b.line)
		}
		q.specifics += specifics(b.text, bl)
		for _, w := range aitext.Runs(b.text, -1, '\'') {
			lw := w
			if bl.properNouns {
				lw = strings.ToLower(w)
			}
			if bl.nominalStop[lw] {
				continue
			}
			if utf8.RuneCountInString(lw) >= 7 && bl.nominal.MatchString(lw) {
				q.nominal++
				nominals[lw]++
			}
		}
	}
	q.words = totalWords
	q.nominalTop = topKeys(nominals, 3)
	q.paragraphMean, q.paragraphCV = meanCV(paragraphLengths)
	q.thinWindow = thinStretch(p, bl)

	// sections: the word count of each ## section
	if sections := sectionWords(p); len(sections) >= 4 {
		q.sections = len(sections)
		_, q.sectionCV = meanCV(sections)
	}

	// value units
	units := valueUnits(p, bl)
	if len(units) > 0 {
		q.firstValue = units[0].at
		prev := units[0].at
		for _, u := range append(units[1:], valueUnit{at: p.words}) {
			if gap := u.at - prev; gap > q.valueGap {
				q.valueGap, q.gapLine = gap, u.line
			}
			prev = u.at
		}
	}

	// opening
	if first := firstProse(p); first >= 0 {
		b := p.blocks[first]
		opening := strings.Join(strings.Fields(b.text), " ")
		head := opening
		for j := first + 1; j < len(p.blocks) && len(strings.Fields(head)) < 120; j++ {
			head += " " + p.blocks[j].text
		}
		head = strings.Join(strings.Fields(head)[:min(120, len(strings.Fields(head)))], " ")
		q.openingLine = b.line
		switch {
		case bl.imagine.MatchString(opening):
			q.opening = "opens with an invitation to imagine"
		case bl.generalOpener.MatchString(opening) && specifics(head, bl) < 2:
			q.opening = "opens on a generalization with no name, number or quote in its first 120 words"
		}
	}

	// ending
	if len(p.headings) > 1 {
		if last := p.headings[len(p.headings)-1]; last.level == 2 && bl.pepHeading.MatchString(strings.TrimSpace(strings.TrimRight(last.text, ".:!"))) {
			q.ending, q.endingLine = fmt.Sprintf("closes with the section %q", last.text), last.line
		}
	}
	if q.ending == "" {
		if last := lastProse(p); last >= 0 {
			b := p.blocks[last]
			switch {
			case bl.recapOpener.MatchString(b.text):
				q.ending, q.endingLine = "ends on a summary", b.line
			case bl.platitude.MatchString(b.text):
				q.ending, q.endingLine = fmt.Sprintf("ends on encouragement (%q)", aitext.Trim(bl.platitude.FindString(b.text))), b.line
			}
		}
	}

	// readability
	if words >= 200 && len(sentences) > 0 {
		asl := float64(words) / float64(len(sentences))
		asw := float64(syllables) / float64(words)
		_ = asw
		q.meanSentence = asl
		q.reading = readingStats{words, len(sentences), syllables, letters, longWords, polysyllables, monosyllables, veryLong}
		q.readability, q.readLow, q.readHigh, q.readLabel = bl.readability(q.reading)
		if lang == "de" {
			ms := 100 * float64(polysyllables) / float64(words)
			iw := 100 * float64(longWords) / float64(words)
			es := 100 * float64(monosyllables) / float64(words)
			q.wstf = 0.1935*ms + 0.1672*asl + 0.1297*iw - 0.0327*es - 0.875
		}
	}
	_ = lengths
	return q
}

// firstProse and lastProse return the index of the first and last prose
// paragraph, or -1.
func firstProse(p prose) int {
	for i, b := range p.blocks {
		if !b.list && !b.quote {
			return i
		}
	}
	return -1
}

func lastProse(p prose) int {
	for i := len(p.blocks) - 1; i >= 0; i-- {
		if !p.blocks[i].list && !p.blocks[i].quote {
			return i
		}
	}
	return -1
}

type valueUnit struct{ at, line int }

func valueUnits(p prose, bl bookLang) []valueUnit {
	var units []valueUnit
	for _, t := range p.tables {
		units = append(units, valueUnit{at: t})
	}
	for i, b := range p.blocks {
		switch {
		case b.quote:
		case b.list && (i == 0 || !p.blocks[i-1].list):
			units = append(units, valueUnit{b.at, b.line}) // a list starts
		case !b.list && len(unitPattern.FindAllString(b.text, -1)) >= 2:
			units = append(units, valueUnit{b.at, b.line}) // a worked example with amounts
		case !b.list:
			for _, s := range sentenceSplit(b.text) {
				if bl.imperative.MatchString(s) {
					units = append(units, valueUnit{b.at, b.line})
					break
				}
			}
		}
	}
	for _, h := range p.headings[min(1, len(p.headings)):] {
		if bl.valueHeading.MatchString(h.text) {
			// the heading's position: the first block after it
			for _, b := range p.blocks {
				if b.line > h.line {
					units = append(units, valueUnit{b.at, h.line})
					break
				}
			}
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].at < units[j].at })
	return units
}

// thinStretch returns the line where a run of at least 400 prose words
// without a single number, quote or (English) proper noun starts.
func thinStretch(p prose, bl bookLang) int {
	run, start := 0, 0
	for _, b := range p.blocks {
		if b.quote || b.list {
			continue
		}
		if specifics(b.text, bl) > 0 || b.link {
			run = 0
			continue
		}
		if run == 0 {
			start = b.line
		}
		run += len(strings.Fields(b.text))
		if run >= 400 {
			return start
		}
	}
	return 0
}

func sectionWords(p prose) []int {
	counts := map[int]int{}
	maxSection := 0
	for _, b := range p.blocks {
		if b.section == 0 {
			continue
		}
		counts[b.section] += len(strings.Fields(b.text))
		maxSection = max(maxSection, b.section)
	}
	var out []int
	for i := 1; i <= maxSection; i++ {
		out = append(out, counts[i])
	}
	return out
}

func meanCV(values []int) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0
	for _, v := range values {
		sum += v
	}
	mean := float64(sum) / float64(len(values))
	return mean, variation(values)
}

func topKeys(m map[string]int, n int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > n {
		keys = keys[:n]
	}
	return keys
}

// syllablesOf estimates a word's syllables by its vowel groups; English
// drops a silent final e. Good enough for readability bands.
func syllablesOf(w string, bl bookLang, lang string) int {
	lw := strings.ToLower(w)
	n := len(bl.vowels.FindAllString(lw, -1))
	if lang == "en" && n > 1 && strings.HasSuffix(lw, "e") && !strings.HasSuffix(lw, "le") {
		n--
	}
	return max(1, n)
}

// negated reports whether a sentence contains a negation word.
func negated(sentence string, bl bookLang) bool {
	for _, w := range aitext.Runs(sentence, -1, '\'') {
		if bl.negation[strings.ToLower(w)] {
			return true
		}
	}
	return false
}

// stem folds the endings of a word family for counting: zuverlässig,
// zuverlässige, zuverlässigen.
func stem(w string) string {
	return strings.TrimRight(w, "enrsm")
}

// Thresholds. Each sits beyond the human nonfiction lfm was calibrated on
// (public-domain books, modern essays, edited German and English guides).
const (
	correctivePer1000    = 2.5
	minCorrective        = 3
	petsPer1000          = 2.0
	minPets              = 3
	boldPer1000          = 5.0
	minBold              = 4
	maxLabels            = 1
	specificsPer1000     = 6.0
	specificsPer1000DE   = 3.0
	nominalShareEN       = 0.035
	nominalShareDE       = 0.05
	oneLinerShare        = 0.30
	minParagraphsShape   = 10
	fragmentsPer1000     = 20.0
	maxParagraphMean     = 120.0
	minParagraphMean     = 22.0
	uniformParagraphs    = 0.30
	uniformSections      = 0.20
	firstValueWords      = 800
	valueGapWords        = 1200
	maxHedges            = 2
	transitionsPer1000   = 2.0
	minTransitions       = 3
	participlesPer1000   = 3.0
	minParticiples       = 3
	minFleschEN          = 50.0
	maxFleschEN          = 85.0
	minAmstadDE          = 45.0
	maxWSTF              = 11.0
	maxSentenceEN        = 24.0
	maxSentenceDE        = 20.0
	minWordsForQuality   = 300
	repeatShingle        = 12 // words in a row two chapters share
	paragraphOverlap     = 0.35
	minParagraphShingles = 20
)

// qualityChecks reports what the profile shows. RuleCopy is for prose that
// reads generated, RuleValue for chapters that give the reader too little.
func qualityChecks(q quality, p prose, plan Plan, language, author string, warn func(rule string, line int, message, hint string)) {
	fiction := plan.Kind == "fiction"
	lang := langOf(language)
	bl, known := bookLangOf(language)
	if q.words < minWordsForQuality {
		return
	}
	per1000 := func(n int) float64 { return float64(n) * 1000 / float64(q.words) }

	if len(q.corrective) >= minCorrective && per1000(len(q.corrective)) > correctivePer1000 {
		warn(RuleCopy, q.corrective[0], fmt.Sprintf("%d sentences correct a claim nobody made (\"It isn't X. It's Y.\"): %.1f per 1000 words.", len(q.corrective), per1000(len(q.corrective))),
			"Say what it is. Keep the setup only where readers really believe the X, and name who believes it.")
	}
	for i, s := range q.signposts {
		if i == 3 {
			break
		}
		warn(RuleCopy, s.line, fmt.Sprintf("%q announces the chapter instead of doing its job.", s.text), "Start doing what the chapter promises; let a bridge at the very end raise the next question instead of listing what comes.")
	}
	for i, h := range q.hard {
		if i == 5 {
			break
		}
		warn(RuleCopy, h.line, fmt.Sprintf("%q is a known tell of generated prose.", h.text), "Use the plain word, or show the thing the phrase claims: the fact, the case, the consequence.")
	}
	for _, word := range topKeys(q.pets, len(q.pets)) {
		if n := q.pets[word]; n >= minPets && per1000(n) > petsPer1000 {
			warn(RuleCopy, q.petLine, fmt.Sprintf("%q and its forms appear %d times; current models repeat it where writers vary.", word, n), "Say what exactly holds and how often: the number, the condition, the result.")
		}
	}
	if q.bold >= minBold && per1000(q.bold) > boldPer1000 {
		warn(RuleCopy, q.boldLine, fmt.Sprintf("%d bold phrases in %d words.", q.bold, q.words), "Bold only the one phrase a skimmer must not miss; let sentences carry the emphasis.")
	}
	if q.labels > maxLabels {
		warn(RuleCopy, 1, fmt.Sprintf("%d list items open with a bold label and a colon.", q.labels), "Write labelled lists as sentences or a table; label-colon bullets read as generated.")
	}
	for _, line := range q.allBold {
		warn(RuleCopy, line, "A whole paragraph is bold.", "KDP rejects body text in bold; keep emphasis to a phrase.")
		break
	}
	if len(q.transitions) >= minTransitions && per1000(len(q.transitions)) > transitionsPer1000 {
		warn(RuleCopy, q.transitions[0], fmt.Sprintf("%d sentences open with a formal connector (Moreover, Furthermore, Darüber hinaus).", len(q.transitions)), "Drop the connector; when the logic needs one, use and, but or so.")
	}
	if len(q.participles) >= minParticiples && per1000(len(q.participles)) > participlesPer1000 {
		warn(RuleCopy, q.participles[0], fmt.Sprintf("%d sentences end in \", highlighting …\" or a similar clause.", len(q.participles)), "State the consequence as its own sentence, with the fact that shows it.")
	}
	if !fiction && len(q.hedges) > maxHedges {
		warn(RuleCopy, q.hedges[0], fmt.Sprintf("%d hedged promises (\"may help\", \"kann helfen\").", len(q.hedges)), "Commit: say when it works and when it does not.")
	}
	for _, line := range q.emoji {
		warn(RuleCopy, line, "An emoji in the text.", "Ebook readers render emoji unreliably and print drops them; use words.")
		break
	}
	declared := strings.ToLower(plan.Text + " " + author)
	if !fiction && !bl.composite.MatchString(strings.Join(blockTexts(p), " ")) {
		seen := map[string]bool{}
		for _, s := range q.stock {
			if seen[s.text] || strings.Contains(declared, strings.ToLower(s.text)) {
				continue
			}
			seen[s.text] = true
			warn(RuleCopy, s.line, fmt.Sprintf("%q is a stock name of invented examples.", s.text), "Use a real case with its source, name the person in the concept's Cases, or say the case is a composite.")
		}
	}
	var prose []string
	var proseLines []int
	for _, b := range p.blocks {
		if !b.quote {
			prose = append(prose, b.raw)
			proseLines = append(proseLines, b.line)
		}
	}
	if r := aitext.Analyze(prose, language); r.Likely {
		line := 1
		if r.Strongest < len(proseLines) {
			line = proseLines[r.Strongest]
		}
		warn(RuleMachine, line, check.MachineMessage("this chapter", r)+fmt.Sprintf(" The passage from line %d reads most generated.", line), check.MachineHint(r))
	}
	if !fiction && q.ending != "" {
		warn(RuleCopy, q.endingLine, "The chapter "+q.ending+".", "End on the last useful move or on the open question the next chapter answers; readers skip recaps and pep talks.")
	}

	// value
	if len(q.artifacts) > 0 {
		rule, a := RuleValue, q.artifacts[0]
		if fiction {
			rule = RuleCopy
		}
		warn(rule, a.line, fmt.Sprintf("%q is left over from a chat tool.", a.text), "Delete it, or replace it with the real source link.")
	}
	if !fiction {
		if q.opening != "" {
			warn(RuleValue, q.openingLine, "The chapter "+q.opening+".", "Open inside a moment: who, where, when and what went wrong, in the first two sentences; or with one surprising number and its source.")
		}
		if p.external == 0 {
			warn(RuleValue, 1, "The chapter names no source a reader can check.", "Link at least one real source next to the claim it supports: a study, a book, a practitioner, the research voice it comes from.")
		}
		for i, v := range q.vague {
			if i == 2 {
				break
			}
			warn(RuleValue, v.line, fmt.Sprintf("%q attributes a claim to nobody in particular.", v.text), "Name and link the source, or say it as your own claim and show why.")
		}
		floor := bl.specificsFloor
		if known {
			if per1000(q.specifics) < floor {
				warn(RuleValue, 1, fmt.Sprintf("%.1f names, numbers and quotes per 1000 words; generic advice starts below %.0f.", per1000(q.specifics), floor), "Add the case, the figure, the date, the words someone actually said.")
			} else if q.thinWindow > 0 {
				warn(RuleValue, q.thinWindow, "400 words in a row without a name, number or quote.", "Climb down the ladder: after each general claim, the concrete instance within two sentences.")
			}
		}
		if q.firstValue > firstValueWords {
			warn(RuleValue, q.openingLine, fmt.Sprintf("The reader waits %d words for the first thing to use: a step, a list, an amount, an instruction.", q.firstValue), "Move a concrete step, number or example up, or cut the preamble.")
		}
		if q.firstValue < 0 {
			warn(RuleValue, 1, "The chapter offers nothing to use: no step, list, table, amount or instruction.", "Give the reader one thing to do or decide with real numbers, even in a story chapter.")
		} else if q.valueGap > valueGapWords {
			warn(RuleValue, q.gapLine, fmt.Sprintf("%d words pass without anything to use.", q.valueGap), "Cut the stretch or put the step, example or number it is building up to inside it.")
		}
	}
	share := float64(q.nominal) / float64(q.words)
	limit := bl.nominalShare
	if !fiction && known && share > limit {
		warn(RuleCopy, 1, fmt.Sprintf("%.1f%% of the words are abstract nouns (%s).", share*100, strings.Join(q.nominalTop, ", ")), "Turn them back into verbs with someone doing them.")
	}
	if q.paragraphs >= minParagraphsShape {
		if !fiction && float64(q.oneLiners) > oneLinerShare*float64(q.paragraphs) {
			warn(RuleCopy, 1, fmt.Sprintf("%d of %d paragraphs are a single short sentence.", q.oneLiners, q.paragraphs), "Merge one-line punches into the paragraphs they conclude; a book is not a feed.")
		}
		switch {
		case q.paragraphMean > maxParagraphMean:
			warn(RuleCopy, 1, fmt.Sprintf("Paragraphs average %.0f words; on a phone that is a wall.", q.paragraphMean), "Break paragraphs where the thought turns.")
		case !fiction && q.paragraphMean < minParagraphMean:
			warn(RuleCopy, 1, fmt.Sprintf("Paragraphs average %.0f words.", q.paragraphMean), "Let paragraphs develop a thought; join the ones that only continue the previous one.")
		case q.paragraphCV < uniformParagraphs:
			warn(RuleCopy, 1, fmt.Sprintf("Paragraphs barely vary in length (variation %.2f).", q.paragraphCV), "Let a key case run long and a turn stand short.")
		}
	}
	if !fiction && q.sentences > 0 && per1000(q.fragments) > fragmentsPer1000 && q.fragments >= 15 {
		warn(RuleCopy, 1, fmt.Sprintf("%d sentences of four words or fewer.", q.fragments), "Fragments land when rare; join most of them to the sentence they belong to.")
	}
	if q.sections >= 4 && q.sectionCV < uniformSections {
		warn(RuleCopy, 1, fmt.Sprintf("All %d sections have nearly the same length (variation %.2f).", q.sections, q.sectionCV), "Give each section the length its material needs.")
	}
	if known && q.reading.words > 0 {
		if !fiction && (q.readability < q.readLow || q.readability > q.readHigh) {
			band := fmt.Sprintf("above %.0f", q.readLow)
			if q.readHigh < 1000 {
				band = fmt.Sprintf("between %.0f and %.0f", q.readLow, q.readHigh)
			}
			value := q.readability
			if strings.Contains(q.readLabel, "negated") {
				// FOG-PL: lower is easier; shown as the index itself
				value, band = -value, fmt.Sprintf("below %.0f", -q.readLow)
			}
			warn(RuleCopy, 1, fmt.Sprintf("Readability (%s) %.0f; mass-market nonfiction reads %s.", strings.TrimSuffix(q.readLabel, " (negated)"), value, band),
				"Split long sentences and prefer shorter words; keep the main point in the main clause.")
		}
		if !fiction && lang == "de" && q.wstf > maxWSTF {
			warn(RuleCopy, 1, fmt.Sprintf("Wiener Sachtextformel %.1f; above %.0f reads as hard.", q.wstf, maxWSTF), "Kürzere Sätze, weniger lange Wörter, Verben statt Substantive.")
		}
		if q.meanSentence > bl.maxSentence {
			warn(RuleCopy, 1, fmt.Sprintf("Sentences average %.0f words.", q.meanSentence), "Split the long ones; the main point in the main clause.")
		}
	}
}

func blockTexts(p prose) []string {
	out := make([]string, len(p.blocks))
	for i, b := range p.blocks {
		out[i] = b.text
	}
	return out
}

// repetition finds what two chapters say in the same words: a run of twelve
// words, or paragraphs that share a third of their five-word sequences.
type chapterText struct {
	file   string
	blocks []block
}

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`the a an and or but of to in on at for with by from as is are was were be been it its this that these those you your we our they their he she his her i me my not no so if then than there here what which who how when also just only very can will would should could may might do does did have has had
der die das ein eine einer eines einem einen und oder aber von zu im in am an auf für mit bei aus als ist sind war waren sein es sich du dein deine dir dich wir uns ihr sie er nicht kein keine so wenn dann als da hier was wer wie wann auch nur sehr kann wird würde soll könnte mag tun hat haben hatte den dem des`) {
		stopwords[w] = true
	}
}

func normWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

func repetition(chapters []chapterText, warn func(file string, line int, message, hint string)) {
	type place struct {
		chapter, line int
	}
	runs := map[string]place{}
	reported := map[[2]int]bool{}
	type para struct {
		chapter, line int
		shingles      map[string]bool
	}
	var paras []para
	for ci, ch := range chapters {
		for _, b := range ch.blocks {
			if b.quote {
				continue
			}
			w := normWords(b.text)
			for i := 0; i+repeatShingle <= len(w); i++ {
				key := strings.Join(w[i:i+repeatShingle], " ")
				prev, ok := runs[key]
				switch {
				case !ok:
					runs[key] = place{ci, b.line}
				case prev.chapter != ci && !reported[[2]int{prev.chapter, ci}]:
					reported[[2]int{prev.chapter, ci}] = true
					warn(ch.file, b.line, fmt.Sprintf("This passage repeats %s word for word: \"%s\".", chapters[prev.chapter].file, preview(key)),
						"Say it once. Where the point returns, add what is new: a complication, a counter-case, the next step.")
				}
			}
			var content []string
			for _, x := range w {
				if !stopwords[x] {
					content = append(content, x)
				}
			}
			if len(content) >= minParagraphShingles {
				set := map[string]bool{}
				for i := 0; i+3 <= len(content); i++ {
					set[strings.Join(content[i:i+3], " ")] = true
				}
				paras = append(paras, para{ci, b.line, set})
			}
		}
	}
	for i := range paras {
		for j := i + 1; j < len(paras); j++ {
			a, b := paras[i], paras[j]
			if a.chapter == b.chapter || reported[[2]int{a.chapter, b.chapter}] {
				continue
			}
			shared := 0
			for k := range a.shingles {
				if b.shingles[k] {
					shared++
				}
			}
			if union := len(a.shingles) + len(b.shingles) - shared; union > 0 && float64(shared)/float64(union) >= paragraphOverlap {
				reported[[2]int{a.chapter, b.chapter}] = true
				warn(chapters[b.chapter].file, b.line, fmt.Sprintf("This paragraph makes the point of %s line %d again.", chapters[a.chapter].file, a.line),
					"Say it once. Where the point returns, add what is new: a complication, a counter-case, the next step.")
			}
		}
	}
}
