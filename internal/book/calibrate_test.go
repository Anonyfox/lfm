package book

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestCalibrate prints the quality profile and findings of every text in
// LFM_CALIBRATE (Markdown or plain text, split into chapters of about 2000
// words), so thresholds can be set against human and generated prose.
// LFM_CALIBRATE_LANG sets the language (default en).
func TestCalibrate(t *testing.T) {
	dir := os.Getenv("LFM_CALIBRATE")
	if dir == "" {
		t.Skip("set LFM_CALIBRATE to a folder of texts")
	}
	language := os.Getenv("LFM_CALIBRATE_LANG")
	if language == "" {
		language = "en"
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	sort.Strings(files)
	totals := map[string]int{}
	chunks := 0
	for _, f := range files {
		if !strings.HasSuffix(f, ".md") && !strings.HasSuffix(f, ".txt") {
			continue
		}
		raw, _ := os.ReadFile(f)
		for i, chunk := range chunksOf(string(raw), 2000) {
			chunks++
			_, p := parseChapter([]byte(chunk))
			q := measure(p, language)
			var msgs []string
			qualityChecks(q, p, Plan{}, language, "", func(rule string, line int, message, hint string) {
				key := strings.SplitN(message, " ", 4)
				_ = key
				msgs = append(msgs, rule+": "+message)
				totals[classify(message)]++
			})
			fmt.Printf("%-28s #%d w=%d corr=%.1f spec=%.1f nom=%.1f%% bold=%d read=%.0f wstf=%.1f sl=%.1f para=%.0f/%.2f one=%d/%d first=%d gap=%d pets=%v\n",
				filepath.Base(f), i+1, q.words, float64(len(q.corrective))*1000/float64(max(1, q.words)), float64(q.specifics)*1000/float64(max(1, q.words)),
				100*float64(q.nominal)/float64(max(1, q.words)), q.bold, q.readability, q.wstf, q.meanSentence, q.paragraphMean, q.paragraphCV, q.oneLiners, q.paragraphs, q.firstValue, q.valueGap, q.pets)
			for _, m := range msgs {
				fmt.Println("    " + m)
			}
		}
	}
	fmt.Printf("\n%d chunks; findings by kind:\n", chunks)
	for k, n := range totals {
		fmt.Printf("  %3d %s\n", n, k)
	}
}

// classify shortens a finding to its kind for the totals.
func classify(message string) string {
	for _, k := range []string{"correct a claim", "announces", "known tell", "appear", "bold", "label", "formal connector", "highlighting", "hedged", "emoji", "stock name", "The chapter closes", "The chapter ends", "opens", "no source", "attributes", "names, numbers", "400 words in a row", "waits", "nothing to use", "pass without", "abstract nouns", "single short sentence", "Paragraphs", "four words", "sections", "Reading ease", "Lesbarkeit", "Wiener", "Sentences average"} {
		if strings.Contains(message, k) {
			return k
		}
	}
	return message
}

func chunksOf(text string, size int) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	paras := strings.Split(text, "\n\n")
	var out []string
	var cur []string
	n := 0
	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "#") && !strings.HasPrefix(p, "-") && !strings.HasPrefix(p, "|") && !strings.HasPrefix(p, ">") && !strings.HasPrefix(p, "*") && !(len(p) > 2 && p[0] >= '0' && p[0] <= '9' && p[1] == '.') {
			p = strings.Join(strings.Fields(p), " ") // unwrap hard-wrapped plain text
		}
		cur = append(cur, p)
		n += len(strings.Fields(p))
		if n >= size {
			out = append(out, "# Chunk\n\n"+strings.Join(cur, "\n\n"))
			cur, n = nil, 0
		}
	}
	if n >= 600 {
		out = append(out, "# Chunk\n\n"+strings.Join(cur, "\n\n"))
	}
	return out
}
