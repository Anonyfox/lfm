// Package aitext estimates whether text-detection tools would flag prose as
// written by a language model, and says why. It needs no model and no network:
// it measures what detectors measure in the open (burstiness, the punctuation
// profile, lexical diversity, compressibility against model-written text, how
// sentences open, repeated phrasing, participle clauses, abstract nouns) plus
// the habits of current models and the fillers of people writing, and
// combines them with weights fitted on labelled human and generated text in
// English, German, French, Spanish, Italian, Portuguese, Dutch and Polish.
// Other languages are read with the language-agnostic measurements only.
//
// A score is an estimate, not proof. Thresholds are set for about 1% false
// alarms on human writing from sources the model never saw, so a quiet
// result means little and a loud one means the text shares the measurable
// habits of generated prose.
package aitext

import (
	"compress/flate"
	"embed"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// MinWords is the shortest text the estimate is made for; below it every
// feature is noise.
const MinWords = 150

// Signal is one measured habit that moved the estimate towards generated.
type Signal struct {
	Feature string  `json:"feature"`
	Detail  string  `json:"detail"`
	Weight  float64 `json:"weight"` // its contribution to the score's log-odds
}

// Result is the estimate for one text.
type Result struct {
	Words    int       `json:"words"`
	Score    float64   `json:"score"` // 0 to 1: how strongly the text matches generated prose
	Likely   bool      `json:"likely"`
	Signals  []Signal  `json:"signals,omitempty"`
	Features []float64 `json:"-"`
	// Strongest is the index of the first paragraph of the window that
	// matches generated prose most: where a revision helps most.
	Strongest int        `json:"strongestParagraph"`
	logits    [2]float64 // per profile, for combining windows
	scored    bool
	lang      string
}

// FeatureNames name the features in the order they are computed; the weights
// follow it.
var FeatureNames = []string{
	"sentence-burstiness", "paragraph-burstiness", "mean-sentence", "commas", "semicolons", "parentheses",
	"colons", "questions", "dashes", "lexical-diversity", "word-length-spread", "compression",
	"model-likeness", "opener-diversity", "repeated-trigrams", "top-word-share", "habits", "asides",
	"fillers", "mid-length-sentences", "even-paragraphs", "claude-likeness",
	"participles", "nominals", "person", "short-sentences", "long-sentences", "punctuation-entropy", "one-sentence-paragraphs",
}

var (
	sentenceEnd = regexp.MustCompile(`[.!?…]+["”’»)]*(\s+|$)`)
)

func words(s string) []string { return Runs(s, -1, '\'', '’') }

// Runs are the first n (every one when n < 0) maximal runs of letters,
// digits and marks in s: what [\p{L}\p{N}<marks>]+ matches, without
// the regexp machinery a book's worth of words pays for.
func Runs(s string, n int, marks ...rune) []string {
	if n == 0 {
		return nil
	}
	var out []string
	start := -1
	for i, r := range s {
		in := unicode.IsLetter(r) || unicode.IsNumber(r) || slices.Contains(marks, r)
		switch {
		case in && start < 0:
			start = i
		case !in && start >= 0:
			if out = append(out, s[start:i]); len(out) == n {
				return out
			}
			start = -1
		}
	}
	if start >= 0 && len(out) != n {
		out = append(out, s[start:])
	}
	return out
}

func sentences(paragraph string) []string {
	var out []string
	last := 0
	for _, m := range sentenceEnd.FindAllStringIndex(paragraph, -1) {
		if t := strings.TrimSpace(paragraph[last:m[1]]); t != "" {
			out = append(out, t)
		}
		last = m[1]
	}
	if t := strings.TrimSpace(paragraph[last:]); t != "" {
		out = append(out, t)
	}
	return out
}

func meanCV(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	var sq float64
	for _, v := range values {
		sq += (v - mean) * (v - mean)
	}
	if mean == 0 {
		return 0, 0
	}
	return mean, math.Sqrt(sq/float64(len(values))) / mean
}

// mattr is the moving-average type-token ratio over windows of n words: the
// share of distinct words, independent of text length.
func mattr(tokens []string, n int) float64 {
	if len(tokens) < n {
		n = len(tokens)
	}
	if n == 0 {
		return 0
	}
	counts := map[string]int{}
	distinct := 0
	for i := 0; i < n; i++ {
		if counts[tokens[i]] == 0 {
			distinct++
		}
		counts[tokens[i]]++
	}
	sum, windows := float64(distinct)/float64(n), 1
	for i := n; i < len(tokens); i++ {
		out := tokens[i-n]
		counts[out]--
		if counts[out] == 0 {
			distinct--
		}
		if counts[tokens[i]] == 0 {
			distinct++
		}
		counts[tokens[i]]++
		sum += float64(distinct) / float64(n)
		windows++
	}
	return sum / float64(windows)
}

func deflated(b []byte) int { return compressed(&plainWriters, nil, b) }

// A compressor costs about a megabyte to set up, more than compressing a
// paragraph, so they are pooled: one pool for plain text, one per prelude
// used as a dictionary (Reset keeps a writer's dictionary).
var (
	plainWriters sync.Pool
	dictWriters  sync.Map // prelude name -> *sync.Pool
)

// compressed is the DEFLATE size of b at the best compression, after dict
// when it is given.
func compressed(pool *sync.Pool, dict []byte, b []byte) int {
	var n byteCount
	w, ok := pool.Get().(*flate.Writer)
	if ok {
		w.Reset(&n)
	} else if dict != nil {
		w, _ = flate.NewWriterDict(&n, flate.BestCompression, dict)
	} else {
		w, _ = flate.NewWriter(&n, flate.BestCompression)
	}
	_, _ = w.Write(b)
	_ = w.Close()
	pool.Put(w)
	return int(n)
}

type byteCount int

func (n *byteCount) Write(p []byte) (int, error) { *n += byteCount(len(p)); return len(p), nil }

// Model-written reference text per language, written for this package: a
// text that compresses well after it shares its phrasing (ZipPy's method).
// generic-<lang> is the stereotype of generated copy, current-<lang> is
// what current models write when asked plainly.
//
//go:embed preludes/*.txt
var preludes embed.FS

func prelude(kind, lang string) []byte {
	b, _ := preludes.ReadFile("preludes/" + kind + "-" + lang + ".txt")
	return b
}

// modelLikeness is how many fewer bytes the text costs after the model-written
// prelude than alone, per byte of text: phrasing shared with generated prose.
func modelLikeness(text []byte, kind, lang string) float64 {
	p := prelude(kind, lang)
	if len(p) == 0 || len(text) == 0 {
		return 0
	}
	// The prelude is a preset dictionary: the text costs what it costs after
	// the prelude, without compressing the prelude again for every text.
	alone := deflated(text)
	pool, _ := dictWriters.LoadOrStore(kind+"-"+lang, &sync.Pool{})
	return float64(alone-compressed(pool.(*sync.Pool), p, text)) / float64(len(text))
}

// Measure computes the features of a text given as paragraphs.
func Measure(paragraphs []string, language string) ([]float64, int) {
	lang := langOf(language)
	var sentLens, paraLens, wordLens []float64
	var tokens []string
	commas, semis, parens, colons, questions, dashes, exclaims := 0, 0, 0, 0, 0, 0, 0
	openers := map[string]bool{}
	var sentencesPer []int
	nSentences := 0
	var all strings.Builder
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		all.WriteString(p)
		all.WriteString("\n\n")
		pw := words(p)
		paraLens = append(paraLens, float64(len(pw)))
		sentencesPer = append(sentencesPer, len(sentences(p)))
		for _, s := range sentences(p) {
			sw := words(s)
			if len(sw) == 0 {
				continue
			}
			nSentences++
			sentLens = append(sentLens, float64(len(sw)))
			openers[strings.ToLower(sw[0])] = true
			if strings.HasSuffix(strings.TrimRight(s, "\"”’»)"), "?") {
				questions++
			}
		}
		for _, r := range p {
			switch r {
			case ',':
				commas++
			case ';':
				semis++
			case '(':
				parens++
			case ':':
				colons++
			case '—', '–':
				dashes++
			case '!':
				exclaims++
			}
		}
		for _, w := range pw {
			tokens = append(tokens, strings.ToLower(w))
			wordLens = append(wordLens, float64(utf8.RuneCountInString(w)))
		}
	}
	n := len(tokens)
	f := make([]float64, len(FeatureNames))
	if n == 0 || nSentences == 0 {
		return f, n
	}
	per := func(c int) float64 { return float64(c) / float64(nSentences) }
	_, f[0] = meanCV(sentLens)
	_, f[1] = meanCV(paraLens)
	f[2], _ = meanCV(sentLens)
	f[3] = per(commas)
	f[4] = per(semis)
	f[5] = per(parens)
	f[6] = per(colons)
	f[7] = per(questions)
	f[8] = per(dashes)
	f[9] = mattr(tokens, 100)
	_, f[10] = meanCV(wordLens)
	text := []byte(all.String())
	f[11] = float64(deflated(text)) / float64(len(text))
	f[12] = modelLikeness(text, "generic", lang)
	f[21] = modelLikeness(text, "current", lang)
	l := languages[lang]
	if l.participle != nil {
		f[22] = float64(CountWords(l.participle, all.String())) / float64(nSentences)
	}
	if l.nominal != nil || l.person != nil {
		nominal, person := 0, 0
		for _, t := range tokens {
			if l.nominal != nil && utf8.RuneCountInString(t) >= 7 && l.nominal.MatchString(t) {
				nominal++
			}
			if l.person[t] {
				person++
			}
		}
		f[23] = float64(nominal) / float64(n)
		f[24] = float64(person) * 1000 / float64(n)
	}
	short, long := 0, 0
	for _, x := range sentLens {
		if x < 6 {
			short++
		}
		if x > 30 {
			long++
		}
	}
	f[25] = float64(short) / float64(len(sentLens))
	f[26] = float64(long) / float64(len(sentLens))
	marks := []int{commas, semis, parens, colons, questions, dashes, exclaims, nSentences}
	total := 0
	for _, m := range marks {
		total += m
	}
	for _, m := range marks {
		if m > 0 {
			q := float64(m) / float64(total)
			f[27] -= q * math.Log2(q)
		}
	}
	single := 0
	for _, c := range sentencesPer {
		if c == 1 {
			single++
		}
	}
	if len(sentencesPer) > 0 {
		f[28] = float64(single) / float64(len(sentencesPer))
	}
	f[13] = float64(len(openers)) / float64(nSentences)
	f[14] = repeatedTrigrams(tokens)
	f[15] = topShare(tokens, 10)
	f[16] = float64(habits(paragraphs, lang)) * 1000 / float64(n)
	// the small irregularities of people writing: asides, doubts, exclamations
	f[17] = per(semis + parens + dashes + questions + exclaims)
	if list := languages[lang].fillers; list != nil {
		count := 0
		for _, t := range tokens {
			if list[t] {
				count++
			}
		}
		f[18] = float64(count) * 1000 / float64(n)
	}
	mid := 0
	for _, l := range sentLens {
		if l >= 8 && l <= 20 {
			mid++
		}
	}
	f[19] = float64(mid) / float64(len(sentLens))
	even := 0
	for _, s := range sentencesPer {
		if s >= 2 && s <= 4 {
			even++
		}
	}
	if len(sentencesPer) > 0 {
		f[20] = float64(even) / float64(len(sentencesPer))
	}
	return f, n
}

func repeatedTrigrams(tokens []string) float64 {
	if len(tokens) < 3 {
		return 0
	}
	seen := map[string]int{}
	for i := 0; i+3 <= len(tokens); i++ {
		seen[tokens[i]+" "+tokens[i+1]+" "+tokens[i+2]]++
	}
	repeats := 0
	for _, c := range seen {
		if c > 1 {
			repeats += c - 1
		}
	}
	return float64(repeats) / float64(len(tokens)-2)
}

func topShare(tokens []string, k int) float64 {
	counts := map[string]int{}
	for _, t := range tokens {
		counts[t]++
	}
	vals := make([]int, 0, len(counts))
	for _, c := range counts {
		vals = append(vals, c)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(vals)))
	top := 0
	for i := 0; i < k && i < len(vals); i++ {
		top += vals[i]
	}
	return float64(top) / float64(len(tokens))
}

func langOf(language string) string {
	base, _, _ := strings.Cut(strings.ToLower(language), "-")
	if base == "" {
		return "en"
	}
	return base
}

// Window is the length the models were fitted on: longer texts are scored
// window by window, because repetition and compression grow with length.
const Window = 500

// windows groups paragraphs into runs of about Window words; a short rest
// joins the last run.
func windows(paragraphs []string) [][]string {
	var out [][]string
	var cur []string
	n := 0
	for _, p := range paragraphs {
		cur = append(cur, p)
		n += len(words(p))
		if n >= Window {
			out = append(out, cur)
			cur, n = nil, 0
		}
	}
	if len(cur) > 0 {
		if n < MinWords && len(out) > 0 {
			out[len(out)-1] = append(out[len(out)-1], cur...)
		} else {
			out = append(out, cur)
		}
	}
	return out
}

// Analyze estimates how strongly a text matches generated prose. A text
// longer than a window is scored window by window and the scores averaged;
// the signals come from the window that matches most.
func Analyze(paragraphs []string, language string) Result {
	ws := windows(paragraphs)
	if len(ws) <= 1 {
		return analyzeWindow(paragraphs, language)
	}
	var total Result
	var sum float64
	likely := 0
	results := make([]Result, len(ws))
	first := 0
	for i, w := range ws {
		r := analyzeWindow(w, language)
		r.Strongest = first
		first += len(w)
		results[i] = r
		total.Words += r.Words
		if r.Words < MinWords {
			continue
		}
		sum += r.Score
		if r.Likely {
			likely++
		}
		if total.Signals == nil || r.Score > total.Score {
			total.Signals, total.Features, total.Strongest = r.Signals, r.Features, r.Strongest
			total.Score = r.Score
		}
	}
	total.Score = sum / float64(len(ws))
	// A text of several windows is judged on the mean of its windows'
	// log-odds against a threshold calibrated on whole human documents:
	// independent evidence adds up without more false alarms.
	st, ok := fittedModel.langs[langOf(language)]
	if !ok {
		st = fittedModel.langs["other"]
	}
	var mean [2]float64
	n := 0
	for _, r := range results {
		if r.scored {
			mean[0] += r.logits[0]
			mean[1] += r.logits[1]
			n++
		}
	}
	if n >= 2 && st.document != [2]float64{} {
		total.Likely = false
		best := math.Inf(-1)
		for k := range mean {
			m := mean[k] / float64(n)
			if st.document[k] == 0 {
				continue
			}
			if m >= st.document[k] {
				total.Likely = true
			}
			if margin := m - st.document[k]; margin > best {
				best = margin
				total.Score = math.Max(0, math.Min(1, 1/(1+math.Exp(-margin))))
			}
		}
		return total
	}
	// without a document threshold: likely when the average crosses the line
	// and at least half the windows do
	total.Likely = total.Score >= 0.5 && likely*2 >= len(ws)
	return total
}

func analyzeWindow(paragraphs []string, language string) Result {
	lang := langOf(language)
	f, n := Measure(paragraphs, lang)
	r := Result{Words: n, Features: f}
	if n < MinWords {
		return r
	}
	// Features are read against the human baseline of the language; a
	// language lfm has no baseline for is read against all of them, without
	// the features that depend on knowing the language.
	st, known := fittedModel.langs[lang]
	r.lang = lang
	weights := fittedModel.full
	if !known {
		st, weights = fittedModel.langs["other"], fittedModel.agnostic
	}
	if len(st.mean) != len(f) {
		return r // no fitted model
	}
	z := make([]float64, len(f))
	for i := range f {
		if !known && languageSpecific[i] {
			continue
		}
		z[i] = (f[i] - st.mean[i]) / st.scale[i]
		if capped[i] && z[i] < 0 {
			z[i] = 0 // only more of it than people use is evidence
		}
	}
	// Two profiles of generated prose: the repetitive, even prose of older
	// models, and the polished prose of current ones, which never repeats a
	// phrase, varies its words more than people do and drops the asides
	// people make. Whichever matches more strongly, relative to its own
	// threshold, decides.
	type contribution struct {
		i int
		v float64
	}
	var parts []contribution
	best := -1.0
	for k, w := range weights {
		if known && len(st.weights[k].weight) == len(f) {
			w = st.weights[k]
		}
		if len(w.weight) != len(f) {
			continue
		}
		logit := w.bias
		var ps []contribution
		for i := range z {
			c := w.weight[i] * z[i]
			logit += c
			ps = append(ps, contribution{i, c})
		}
		r.logits[k], r.scored = logit, true
		score := 1 / (1 + math.Exp(-logit))
		// the margin above or below the profile's threshold, on a 0..1 scale
		rel := 0.5 + (score-st.threshold[k])/2
		if rel > best {
			best, parts = rel, ps
			r.Score = math.Max(0, math.Min(1, rel))
			r.Likely = score >= st.threshold[k]
		}
	}
	sort.Slice(parts, func(a, b int) bool { return parts[a].v > parts[b].v })
	for _, p := range parts {
		if p.v < 0.4 || len(r.Signals) == 3 {
			break
		}
		r.Signals = append(r.Signals, Signal{FeatureNames[p.i], describe(p.i, f[p.i], st.human[p.i]), p.v})
	}
	return r
}

// describe says in words what a feature measured, against the human median:
// the two profiles of generated prose deviate in opposite directions, so the
// words follow the measurement.
func describe(i int, v, human float64) string {
	more := v > human
	pick := func(more bool, high, low string) string {
		if more {
			return high
		}
		return low
	}
	switch FeatureNames[i] {
	case "sentence-burstiness":
		return fmt.Sprintf("sentence lengths vary %s than in human writing (%.2f; human around %.2f)", pick(more, "more", "less"), v, human)
	case "paragraph-burstiness":
		return fmt.Sprintf("paragraph lengths vary %s than usual (%.2f; human around %.2f)", pick(more, "more", "less"), v, human)
	case "mean-sentence":
		return fmt.Sprintf("sentences average %.0f words (human around %.0f)", v, human)
	case "commas":
		return fmt.Sprintf("%.1f commas per sentence (human around %.1f)", v, human)
	case "semicolons":
		return fmt.Sprintf("%s semicolons than people use", pick(more, "more", "fewer"))
	case "parentheses":
		return fmt.Sprintf("%s parentheses and asides than people use", pick(more, "more", "fewer"))
	case "colons":
		return fmt.Sprintf("%.2f colons per sentence (human around %.2f)%s", v, human, pick(more, ": set-ups before a reveal", ""))
	case "questions":
		return fmt.Sprintf("%.2f questions per sentence (human around %.2f)", v, human)
	case "dashes":
		return fmt.Sprintf("%.2f dashes per sentence (human around %.2f)", v, human)
	case "lexical-diversity":
		return fmt.Sprintf("word choice is %s varied than in human writing (%.2f; human around %.2f)", pick(more, "more", "less"), v, human)
	case "word-length-spread":
		return pick(more, "word lengths are more mixed than usual", "word lengths are unusually even")
	case "compression":
		return pick(more, "the text compresses less than human prose", "the text compresses like repetitive prose")
	case "model-likeness":
		return "its phrasing compresses well against model-written text"
	case "claude-likeness":
		return "its phrasing compresses well against text current models write"
	case "opener-diversity":
		return pick(more, "sentences open more variedly than people's do", "sentences open with the same few words")
	case "repeated-trigrams":
		return pick(more, "the same three-word phrases return", "no phrase ever returns, where people repeat themselves")
	case "top-word-share":
		return pick(more, "a few words carry much of the text", "the most common words carry less of the text than usual")
	case "asides":
		return fmt.Sprintf("%.2f asides, doubts or exclamations per sentence (human around %.2f): parentheses, dashes, semicolons, questions", v, human)
	case "fillers":
		return fmt.Sprintf("%.0f hedges and fillers per 1000 words (human around %.0f): people write just, really, probably, doch, mal", v, human)
	case "mid-length-sentences":
		return fmt.Sprintf("%.0f%% of sentences run 8 to 20 words (human around %.0f%%)", v*100, human*100)
	case "even-paragraphs":
		return fmt.Sprintf("%.0f%% of paragraphs have two to four sentences (human around %.0f%%)", v*100, human*100)
	case "participles":
		return fmt.Sprintf("%.2f participle clauses after a comma per sentence (human around %.2f): \", highlighting …\", \", wobei …\"", v, human)
	case "nominals":
		return fmt.Sprintf("%.1f%% abstract nouns (human around %.1f%%)", v*100, human*100)
	case "person":
		return fmt.Sprintf("%.0f first- and second-person words per 1000 (human around %.0f)", v, human)
	case "short-sentences":
		return fmt.Sprintf("%.0f%% of sentences under six words (human around %.0f%%)", v*100, human*100)
	case "long-sentences":
		return fmt.Sprintf("%.0f%% of sentences over thirty words (human around %.0f%%)", v*100, human*100)
	case "punctuation-entropy":
		return pick(more, "punctuation is unusually varied", "punctuation keeps to a few marks")
	case "one-sentence-paragraphs":
		return fmt.Sprintf("%.0f%% one-sentence paragraphs (human around %.0f%%)", v*100, human*100)
	case "habits":
		return "habits of current models: corrective framing (\"It isn't X. It's Y.\"), signposting, pet words"
	}
	return FeatureNames[i]
}
