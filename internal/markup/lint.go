package markup

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Issue is a problem with a piece of sample text and how to fix it.
type Issue struct{ Message, Hint string }

var (
	filler = regexp.MustCompile(`(?i)\b(lorem ipsum|dolor sit amet|john doe|jane doe|acme|123 main st|your name here|company name|test@test|foo bar)\b`)
	shout  = regexp.MustCompile(`\b[A-ZÄÖÜ]{3,}(?:\s+[A-ZÄÖÜ]{2,}){4,}\b`)
)

// Sample is what reads wrong in sample text a buyer judges a template by:
// filler, shouting capitals, emoji. marks are the symbols lfm itself sets.
func Sample(s, marks string) []Issue {
	var out []Issue
	if m := filler.FindString(s); m != "" {
		out = append(out, Issue{fmt.Sprintf("%q reads as filler.", m), "Write sample content a real buyer would have: real-sounding names, places and numbers they replace with their own."})
	}
	if shout.MatchString(s) {
		out = append(out, Issue{"A long run of capitals shouts.", "Write it in sentence case; the theme sets headings apart."})
	}
	for _, c := range s {
		if unicode.Is(unicode.So, c) && !strings.ContainsRune(marks, c) && c != '©' && c != '®' && c != '™' {
			out = append(out, Issue{fmt.Sprintf("%q is an emoji or symbol.", c), "Leave pictographs out; a template reads as made by hand without them."})
			break
		}
	}
	return out
}
