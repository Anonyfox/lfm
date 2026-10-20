package document

import (
	"os"
	"strings"
	"testing"
)

// Property: a concept.md and a source of any shape parse without panicking,
// every finding on a line of the text.
func FuzzConceptAndSource(f *testing.F) {
	for _, pair := range [][2]string{{"testdata/club/concept.md", "testdata/club/en/program.md"}, {"testdata/club/concept.md", "testdata/club/en/invoice.md"}, {"testdata/features/concept.md", "testdata/features/en/everything.md"}} {
		concept, _ := os.ReadFile(pair[0])
		source, _ := os.ReadFile(pair[1])
		f.Add(string(concept), string(source))
	}
	f.Add("# X\n## Look\nTheme: none\n", "---\nlanguage: de\n---\n")
	f.Fuzz(func(t *testing.T, concept, source string) {
		within := func(text string) func(int, string, string) {
			lines := strings.Count(text, "\n") + 1
			return func(line int, message, hint string) {
				if line < 1 || line > lines+1 {
					t.Fatalf("finding on line %d of %d: %s", line, lines, message)
				}
			}
		}
		checkConcept(concept, within(concept))
		_, problems := parseDoc("x", "en", "x.md", []byte(source))
		report := within(source)
		for _, p := range problems {
			report(p.Line, p.Message, p.Hint)
		}
	})
}
