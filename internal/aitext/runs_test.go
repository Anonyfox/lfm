package aitext

import (
	"regexp"
	"slices"
	"testing"
)

// Runs must find exactly what the word pattern it replaced matches.
func FuzzRunsMatchTheWordPattern(f *testing.F) {
	pattern := regexp.MustCompile(`[\p{L}\p{N}’']+`)
	for _, seed := range []string{"", "It's Größe 12 – l’été, ok?", "\xff\xfeab cd\x80", "a", "’'", "١٢٣ 中文 ﬁ"} {
		f.Add(seed, 2)
	}
	f.Fuzz(func(t *testing.T, s string, n int) {
		n = n%5 - 1
		if got, want := Runs(s, n, '\'', '’'), pattern.FindAllString(s, n); !slices.Equal(got, want) {
			t.Fatalf("Runs(%q, %d) = %q, the pattern %q", s, n, got, want)
		}
	})
}
