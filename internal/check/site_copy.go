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
	type block struct {
		occurrence
		key string
	}
	var blocks []block
	primary := make([]bool, len(pages))
	shared := map[string]map[int]bool{}
	for i, d := range pages {
		for _, n := range d.Elements {
			if n.Data == "main" || n.Data == "article" {
				primary[i] = true
				break
			}
		}
		for _, s := range copySegments(d) {
			chrome := false
			for n := s.node; n != nil; n = n.Parent {
				if n.Data == "nav" || n.Data == "footer" || Attr(n, "role") == "navigation" || Attr(n, "role") == "contentinfo" {
					chrome = true
					break
				}
			}
			if chrome {
				continue
			}
			// Exact text and element type distinguish shared furniture from merely
			// similar content (numeric normalization must not hide repeated templates).
			key := s.node.Data + "\x00" + s.text
			if shared[key] == nil {
				shared[key] = map[int]bool{}
			}
			shared[key][i] = true
			blocks = append(blocks, block{occurrence{i, s.node, s.text}, key})
		}
	}
	var triplets []occurrence
	groups := map[string][]occurrence{}
	var order []string
	for _, b := range blocks {
		if len(pages) > 1 && len(shared[b.key]) == len(pages) {
			// Main/article content is authored copy even when repeated on every page.
			main := !primary[b.page]
			for n := b.node; n != nil; n = n.Parent {
				if n.Data == "header" || n.Data == "aside" || Attr(n, "role") == "banner" {
					main = false
					break
				}
				if n.Data == "main" || n.Data == "article" {
					main = true
					break
				}
			}
			if !main {
				continue
			}
		}
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
	if len(triplets) > 10 {
		for _, o := range triplets {
			d := pages[o.page].At(o.node, Warning, fmt.Sprintf("%d triplet endings across %d pages (limit 10): %q.", len(triplets), len(pages), o.text), "Rewrite repeated three-item endings as specific claims or vary the sentence structure.")
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
