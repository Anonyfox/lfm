package check

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

var siteSentenceRE = regexp.MustCompile(`[^.!?\n]+[.!?]?`)
var tripletEndingRE = regexp.MustCompile(`(?:^|[^\pL\pN_-])[\pL\pN_-]+,\s+[\pL\pN_-]+,?\s+(?:and|und)\s+[\pL\pN_-]+[.!?]?$`)

// SiteCopy applies aggregate copy rules to the supplied pages in source order.
// A group is one diagnostic; its hint lists every occurrence, including repeats
// on the same page. The caller applies the usual severity overrides.
func SiteCopy(pages []*Document) []Diagnostic {
	type occurrence struct {
		page int
		node *html.Node
		text string
	}
	var blocks []occurrence
	for i, d := range pages {
		for _, s := range textSegments(d, false, false, true) {
			blocks = append(blocks, occurrence{i, s.node, s.text})
		}
	}
	var triplets []occurrence
	groups := map[string][]occurrence{}
	var order []string
	for _, b := range blocks {
		for _, sentence := range siteSentenceRE.FindAllString(b.text, -1) {
			sentence = strings.Join(strings.Fields(sentence), " ")
			o := occurrence{b.page, b.node, sentence}
			if tripletEndingRE.MatchString(sentence) {
				triplets = append(triplets, o)
			}
			if len(pages) < 2 {
				continue
			}
			words := strings.FieldsFunc(strings.ToLower(sentence), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
			if len(words) < 9 {
				continue
			}
			for i, w := range words {
				if strings.ContainsFunc(w, unicode.IsDigit) {
					words[i] = "#"
				}
			}
			key := strings.Join(words, " ")
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], o)
		}
	}
	var out []Diagnostic
	limit := 10 * len(pages)
	if len(triplets) > limit {
		for _, o := range triplets {
			d := pages[o.page].At(o.node, Warning, fmt.Sprintf("%d triplet endings across %d pages (limit %d): %q.", len(triplets), len(pages), limit, o.text), "Rewrite repeated three-item endings as specific claims or vary the sentence structure.")
			d.Rule = "copy.triplets"
			out = append(out, d)
		}
	}
	for _, key := range order {
		occurrences := groups[key]
		seen := map[int]bool{}
		var locations []string
		for _, o := range occurrences {
			seen[o.page] = true
			loc := pages[o.page].At(o.node, Warning, "", "").Location
			locations = append(locations, fmt.Sprintf("%s:%d:%d", loc.File, loc.Line, loc.Column))
		}
		if len(seen) < 2 {
			continue
		}
		first := occurrences[0]
		d := pages[first.page].At(first.node, Warning, fmt.Sprintf("Normalized copy repeated on %d pages: %q.", len(seen), key), "Replace template copy with page-specific facts. Occurrences: "+strings.Join(locations, ", "))
		d.Rule = "copy.cross-page"
		out = append(out, d)
	}
	return out
}
