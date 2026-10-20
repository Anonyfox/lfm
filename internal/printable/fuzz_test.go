package printable

import (
	"errors"
	"strings"
	"testing"
)

// Property: a concept.md of any shape parses without panicking, every
// finding on a line of the text.
func FuzzConcept(f *testing.F) {
	f.Add(testConcept)
	f.Add("# Printable: X\n## Parts\n| Part | Pages |\n| a | -1 |\n")
	f.Fuzz(func(t *testing.T, text string) {
		lines := strings.Count(text, "\n") + 1
		checkConcept(text, func(line int, message, hint string) {
			if line < 1 || line > lines+1 {
				t.Fatalf("finding on line %d of %d: %s", line, lines, message)
			}
		})
	})
}

// capped fails writes past a megabyte, so a template's output stays bounded.
type capped struct{ n int }

func (c *capped) Write(p []byte) (int, error) {
	if c.n += len(p); c.n > 1<<20 {
		return 0, errors.New("output too large")
	}
	return len(p), nil
}

// Property: a template of any shape parses, and its helpers execute without
// panicking or exhausting memory. Templates that range are left out: each
// loop is bounded, but loops nested in loops multiply.
func FuzzTemplate(f *testing.F) {
	f.Add(testTemplate)
	f.Add(`{{seq 12}}{{chunk (seq 10) 3}}{{dict "a" 1}}{{add 1 2}}{{mod 5 0}}`)
	f.Add(`{{seq 9999999999999}}`)
	f.Add(`{{bingo (seq 30) 5000 7 true 1}}{{maze 80 80 3}}{{sudoku "hard" 2}}`)
	f.Fuzz(func(t *testing.T, src string) {
		tmpl, err := parseTemplate([]byte(src), "en")
		if err != nil || strings.Contains(src, "range") {
			return
		}
		tmpl.Execute(&capped{}, View{Language: "en", Data: map[string]any{"words": []any{"apple", "pear", "plum"}}})
	})
}
