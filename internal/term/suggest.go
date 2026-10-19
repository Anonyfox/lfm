package term

// Suggest is the candidate closest to a mistyped word, or "" when none is
// close enough to be what was meant: at most a third of its letters apart.
func Suggest(word string, candidates []string) string {
	best, bestDist := "", -1
	for _, c := range candidates {
		d := distance(word, c)
		if limit := max(1, len([]rune(c))/3); d <= limit && (bestDist < 0 || d < bestDist) {
			best, bestDist = c, d
		}
	}
	return best
}

// distance is the Levenshtein distance of a and b, adjacent swaps counting
// as one edit.
func distance(a, b string) int {
	x, y := []rune(a), []rune(b)
	d := make([][]int, len(x)+1)
	for i := range d {
		d[i] = make([]int, len(y)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(x); i++ {
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(x)][len(y)]
}

// DidYouMean is " Did you mean x?" for a close candidate, else "".
func DidYouMean(word string, candidates []string) string {
	if s := Suggest(word, candidates); s != "" {
		return " Did you mean " + s + "?"
	}
	return ""
}
