package book

import (
	"strings"
	"testing"
)

// Property: a concept.md and a listing.md of any shape parse without
// panicking, every finding on a line of the text.
func FuzzConceptAndListing(f *testing.F) {
	f.Add(testConcept, testListing)
	f.Add("# Book: X\n## Reader\n## Reader\n", "# Listing: X\n## Description\n")
	f.Add("", "## Price\n\n12,00 EUR\n")
	f.Fuzz(func(t *testing.T, concept, listing string) {
		within := func(text string) func(string, int, string, string) {
			lines := strings.Count(text, "\n") + 1
			return func(rule string, line int, message, hint string) {
				if line < 1 || line > lines+1 {
					t.Fatalf("%s finding on line %d of %d: %s", rule, line, lines, message)
				}
			}
		}
		plan := checkConcept(concept, nil, nil, within(concept), "nonfiction")
		checkListing(listing, plan, nil, "de", within(listing))
		fiction := checkConcept(concept, nil, nil, within(concept), "fiction")
		_, prose := parseChapter([]byte(listing))
		checkOutline(Chapter{Job: concept, Beats: concept}, prose, fiction, within(listing))
	})
}

// Property: a chapter of any Markdown parses without panicking.
func FuzzParseChapter(f *testing.F) {
	f.Add("# One\n\nText with a [link](https://example.org) and ![image](a.png).\n\n## Empty\n## Next\n\n- item\n> quote\n")
	f.Add("```\ncode\n```\n<div>raw</div>\n| a | b |\n|---|---|\n")
	f.Fuzz(func(t *testing.T, src string) {
		parseChapter([]byte(src))
	})
}
