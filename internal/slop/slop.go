// Package slop owns the shared, embedded copy rules. It never rewrites input.
package slop

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Allow exempts occurrences of Phrase for Rule, or the whole rule when Phrase is empty.
type Allow struct {
	Rule   string `json:"rule"`
	Phrase string `json:"phrase,omitempty"`
	Reason string `json:"reason"`
}

type Rule struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Severity string `json:"severity"`
	Pattern  string `json:"pattern"`
	Reason   string `json:"reason"`
	Fix      string `json:"fix"`
	Example  string `json:"example"`
	Quiet    string `json:"quiet"`
	Min      int    `json:"min"`
	Kind     string `json:"kind"`
	re       *regexp.Regexp
}

type tell struct {
	Min     int      `json:"min"`
	Keys    []string `json:"keys"`
	Pattern string   `json:"pattern"`
	re      *regexp.Regexp
}

//go:embed rules.json
var ruleData []byte

//go:embed tells.json
var tellData []byte
var rules = loadRules()
var tells = loadTells()

func loadRules() []Rule {
	var out []Rule
	if err := json.Unmarshal(ruleData, &out); err != nil {
		panic(err)
	}
	for i := range out {
		if out[i].Pattern != "" {
			out[i].re = regexp.MustCompile("(?i)" + out[i].Pattern)
		}
	}
	return out
}
func loadTells() []tell {
	var out []tell
	if err := json.Unmarshal(tellData, &out); err != nil {
		panic(err)
	}
	for i := range out {
		out[i].re = regexp.MustCompile(`(?i)\b(?:` + out[i].Pattern + `)\b`)
	}
	return out
}

// Rules returns metadata by value; regular expressions are immutable.
func Rules() []Rule {
	return append([]Rule{{ID: "copy.tells", Language: "any", Severity: "warn", Reason: "Stock phrase of generated copy", Fix: "Say the concrete fact, capability or situation instead."}}, rules...)
}

func ValidateAllow(a Allow) error {
	if strings.TrimSpace(a.Reason) == "" {
		return fmt.Errorf("reason is required for every copy allowance")
	}
	for _, r := range Rules() {
		if a.Rule == r.ID {
			return nil
		}
	}
	return fmt.Errorf("unknown copy rule %q", a.Rule)
}

var straight = strings.NewReplacer("’", "'", "‘", "'", "ʼ", "'", "\u00a0", " ")

func normalize(s string) string { return strings.ToLower(straight.Replace(s)) }

// Language honors explicit BCP-47 tags, otherwise uses conservative function-word detection.
func Language(explicit, text string) string {
	if explicit != "" {
		return strings.ToLower(strings.Split(strings.ReplaceAll(explicit, "_", "-"), "-")[0])
	}
	de, en := 0, 0
	for _, w := range strings.FieldsFunc(normalize(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		switch w {
		case "der", "die", "das", "und", "ist", "mit", "für", "nicht", "ein", "eine":
			de++
		case "the", "and", "is", "with", "for", "not", "this", "your":
			en++
		}
	}
	if de > en {
		return "de"
	}
	if en > 0 {
		return "en"
	}
	return "any"
}

// Hit identifies the original block, preserving the caller's source location.
type Hit struct {
	Block  int
	Text   string
	Count  int
	Phrase string
	Repeat bool
}

func masked(id, text string, allow []Allow) string {
	for _, a := range allow {
		if a.Rule != id {
			continue
		}
		if a.Phrase == "" {
			return ""
		}
		phrase := normalize(a.Phrase)
		// Replace just the allowed span, never the rest of a paragraph.
		for start := 0; start < len(text); {
			n := strings.Index(text[start:], phrase)
			if n < 0 {
				break
			}
			n += start
			before := n == 0 || !letterAtEnd(text[:n])
			after := n+len(phrase) == len(text) || !letterAtStart(text[n+len(phrase):])
			if before && after {
				text = text[:n] + strings.Repeat(" ", len(phrase)) + text[n+len(phrase):]
			}
			start = n + len(phrase)
		}
	}
	return text
}
func letterAtEnd(s string) bool {
	r := []rune(s)
	return len(r) > 0 && (unicode.IsLetter(r[len(r)-1]) || unicode.IsDigit(r[len(r)-1]))
}
func letterAtStart(s string) bool {
	for _, r := range s {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	return false
}

var sentenceRE = regexp.MustCompile(`[^.!?\n]+[.!?]?`)
var sourceRE = regexp.MustCompile(`(?i)quelle|laut |nach angaben|studie von|studie:|umfrage von|umfrage:|source|according to|https?://|\[[0-9]+\]`)
var sentenceWordRE = regexp.MustCompile(`[\p{L}\p{N}]+`)

// Match runs one idea across blocks. All callers use these exact thresholds.
func Match(id, lang string, blocks []string, allow []Allow) []Hit {
	texts := make([]string, len(blocks))
	for i, s := range blocks {
		texts[i] = masked(id, normalize(s), allow)
	}
	return match(id, lang, texts)
}

// Analyze normalizes the blocks once for every rule.
func Analyze(lang string, blocks []string, allow []Allow) map[string][]Hit {
	texts := make([]string, len(blocks))
	for i, s := range blocks {
		texts[i] = normalize(s)
	}
	result := make(map[string][]Hit)
	for _, r := range Rules() {
		candidate := texts
		for _, a := range allow {
			if a.Rule == r.ID {
				candidate = make([]string, len(texts))
				for i, s := range texts {
					candidate[i] = masked(r.ID, s, allow)
				}
				break
			}
		}
		if hits := match(r.ID, lang, candidate); len(hits) > 0 {
			result[r.ID] = hits
		}
	}
	return result
}

func match(id, lang string, texts []string) []Hit {
	if id == "copy.tells" {
		return matchTells(texts, lang)
	}
	var rule Rule
	found := false
	for _, r := range rules {
		if r.ID == id {
			rule = r
			found = true
			break
		}
	}
	if !found || rule.Language != "any" && rule.Language != lang {
		return nil
	}
	var hits []Hit
	seen := map[string]bool{}
	lengths := []int{}
	for i, text := range texts {
		if text == "" {
			lengths = nil
			continue
		}
		switch rule.Kind {
		case "dash":
			if Dashes(text) > 0 {
				hits = append(hits, Hit{Block: i, Text: "em dash in German copy", Count: Dashes(text)})
			}
		case "duplicate", "rhythm":
			for _, sentence := range sentenceRE.FindAllString(text, -1) {
				// Labels and citation rows are not prose sentences.
				sentence = strings.TrimSpace(sentence)
				if !proseSentence(sentence) {
					lengths = nil
					continue
				}
				words := sentenceWordRE.FindAllString(sentence, -1)
				n := len(words)
				if rule.Kind == "duplicate" {
					key := strings.Join(words, " ")
					if n >= 9 && seen[key] {
						hits = append(hits, Hit{Block: i, Text: strings.TrimSpace(sentence), Count: 2})
					}
					if n >= 9 {
						seen[key] = true
					}
					continue
				}
				if n < 4 {
					lengths = nil
					continue
				}
				lengths = append(lengths, n)
				if len(lengths) >= 6 {
					last := lengths[len(lengths)-6:]
					lo, hi := last[0], last[0]
					for _, v := range last {
						lo = min(lo, v)
						hi = max(hi, v)
					}
					if hi-lo <= 1 {
						hits = append(hits, Hit{Block: i, Text: "six sentences of nearly equal length", Count: 6})
						lengths = nil
					}
				}
			}
		case "sentence":
			for _, sentence := range sentenceRE.FindAllString(text, -1) {
				if len(rule.re.FindAllString(sentence, -1)) >= rule.Min {
					hits = append(hits, Hit{Block: i, Text: strings.TrimSpace(sentence), Count: rule.Min})
				}
			}
		default:
			if rule.Kind == "source" && sourceRE.MatchString(text) {
				continue
			}
			for _, loc := range rule.re.FindAllStringIndex(text, -1) {
				// A single not-only construction remains legitimate, as in copy.tells.
				if id == "copy.reveal" && strings.HasPrefix(text[loc[0]:loc[1]], "nicht nur ") {
					continue
				}
				if loc[0] > 0 && letterAtStart(text[loc[0]:loc[1]]) && letterAtEnd(text[:loc[0]]) || loc[1] < len(text) && letterAtEnd(text[loc[0]:loc[1]]) && letterAtStart(text[loc[1]:]) {
					continue
				}
				hits = append(hits, Hit{Block: i, Text: text[loc[0]:loc[1]], Count: 1})
			}
		}
	}
	return hits
}

// proseSentence excludes unterminated labels, numeric data and citation rows.
func proseSentence(s string) bool {
	r := []rune(s)
	if len(r) < 2 || strings.ContainsRune(s, '·') {
		return false
	}
	last := r[len(r)-1]
	return (last == '.' || last == '!' || last == '?') && unicode.IsLetter(r[len(r)-2])
}

func matchTells(texts []string, lang string) []Hit {
	var hits []Hit
	for _, t := range tells {
		count, first, phrase := 0, 0, ""
		for i, text := range texts {
			keyed := false
			for _, key := range t.Keys {
				if strings.Contains(text, key) {
					keyed = true
					break
				}
			}
			if !keyed {
				continue
			}
			for _, loc := range t.re.FindAllStringIndex(text, -1) {
				if loc[0] > 0 && letterAtStart(text[loc[0]:loc[1]]) && letterAtEnd(text[:loc[0]]) || loc[1] < len(text) && letterAtEnd(text[loc[0]:loc[1]]) && letterAtStart(text[loc[1]:]) {
					continue
				}
				if count == 0 {
					first = i
					phrase = text[loc[0]:loc[1]]
				}
				count++
			}
		}
		if count >= t.Min {
			message := fmt.Sprintf("%q is a stock phrase of generated copy.", phrase)
			if t.Min > 1 {
				message = fmt.Sprintf("%q and %d more like it: a construction generated copy repeats.", phrase, count-1)
			} else if count > 1 {
				message = fmt.Sprintf("%q appears %d times: a stock phrase of generated copy.", phrase, count)
			}
			hits = append(hits, Hit{Block: first, Text: message, Count: count, Phrase: phrase, Repeat: t.Min > 1})
		}
	}
	dashes, words, first := 0, 0, 0
	for i, text := range texts {
		words += len(strings.Fields(text))
		if n := Dashes(text); n > 0 {
			if dashes == 0 {
				first = i
			}
			dashes += n
		}
	}
	if lang != "de" && dashes >= 4 && dashes*100 > words {
		hits = append(hits, Hit{Block: first, Text: fmt.Sprintf("%d em dashes in %d words of copy: a density that reads as generated text.", dashes, words), Count: dashes})
	}
	return hits
}

// Dashes counts prose em dashes, excluding numeric ranges.
func Dashes(s string) int {
	n := 0
	for i, r := range s {
		if r != '—' {
			continue
		}
		before := strings.TrimSpace(s[:i])
		after := strings.TrimSpace(s[i+len("—"):])
		if before != "" && after != "" && before[len(before)-1] >= '0' && before[len(before)-1] <= '9' && after[0] >= '0' && after[0] <= '9' {
			continue
		}
		n++
	}
	return n
}
