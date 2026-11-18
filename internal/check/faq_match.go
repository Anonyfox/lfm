package check

import (
	"math/bits"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Twelve extra visible tokens fit a short code sample or link list while
// keeping even long accepted answers local to their rendered passage.
const faqAnswerExtras = 12

func faqTokens(text string) []string {
	return strings.FieldsFunc(cases.Fold().String(norm.NFKC.String(text)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func faqMatch(page, want []string, extras int) bool {
	if len(want) == 0 || len(want) > len(page) {
		return false
	}
	// Short queries need at most 64 comparisons plus the extras per start.
	// For long queries jump over equal runs, including on repetitive pages.
	var ranks [][]int
	if len(want) > 64 {
		ranks = faqRunRanks(page, want)
	}
	for start, token := range page[:len(page)-len(want)+1] {
		if token != want[0] {
			continue
		}
		pos, matched := start+1, 1
		for pos < len(page) && matched < len(want) {
			if ranks == nil {
				if page[pos] == want[matched] {
					pos++
					matched++
					continue
				}
			} else {
				for k := bits.Len(uint(min(len(page)-pos, len(want)-matched))) - 1; k >= 0; k-- {
					size := 1 << k
					if size <= len(page)-pos && size <= len(want)-matched && ranks[k][pos] == ranks[k][len(page)+matched] {
						pos += size
						matched += size
					}
				}
				if matched == len(want) {
					return true
				}
			}
			if pos-start-matched == extras {
				break
			}
			pos++
		}
		if matched == len(want) {
			return true
		}
	}
	return false
}

// Assign identical IDs to equal runs of 1, 2, 4, ... tokens. Two half-run
// IDs identify their concatenation exactly. With a fixed skip budget this
// bounds preprocessing and matching to O(page tokens * log(query tokens)).
func faqRunRanks(page, want []string) [][]int {
	tokens := slices.Concat(page, want)
	ids := map[string]int{}
	first := make([]int, len(tokens))
	for i, t := range tokens {
		if ids[t] == 0 {
			ids[t] = len(ids) + 1
		}
		first[i] = ids[t]
	}
	ranks := [][]int{first}
	for half := 1; half <= len(want)/2; half *= 2 {
		prev := ranks[len(ranks)-1]
		next := make([]int, len(prev)-half)
		pairs := map[[2]int]int{}
		for i := range next {
			pair := [2]int{prev[i], prev[i+half]}
			if pairs[pair] == 0 {
				pairs[pair] = len(pairs) + 1
			}
			next[i] = pairs[pair]
		}
		ranks = append(ranks, next)
	}
	return ranks
}
