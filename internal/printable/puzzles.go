package printable

import (
	"fmt"
	"html/template"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// The puzzle generators are seeded, so a product renders the same puzzles on
// every compile, and each checks its own result: a puzzle a buyer cannot solve
// never reaches a page.

func rng(seed int) *rand.Rand { return rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15)) }

// BingoCard is one numbered card; Cells holds rows, the free centre as "".
type BingoCard struct {
	N     int
	Cells [][]string
}

// Bingo is a set of bingo cards with what the host needs to know.
type Bingo struct {
	Cards []BingoCard
	Calls []string // every item, for the caller's list
	// FirstWin is the median number of calls until the first card completes a
	// line when all cards play, from simulated games.
	FirstWin int
}

// NewBingo deals count unique cards of size×size items (a free centre when
// free and size is odd), each item on about as many cards as any other.
func NewBingo(items []string, count, size int, free bool, seed int) (Bingo, error) {
	items = uniqueItems(items)
	need := size * size
	if free && size%2 == 1 {
		need--
	}
	switch {
	case size < 3 || size > 7:
		return Bingo{}, fmt.Errorf("bingo: size %d; use 3 to 7", size)
	case count < 1 || count > 5000:
		return Bingo{}, fmt.Errorf("bingo: %d cards; use 1 to 5000", count)
	case len(items) < need+2:
		return Bingo{}, fmt.Errorf("bingo: %d distinct items for %d per card; give at least %d so cards differ", len(items), need, need+2)
	}
	r := rng(seed)
	uses := make([]int, len(items))
	seen := map[string]bool{}
	b := Bingo{Calls: items}
	for len(b.Cards) < count {
		var picked []int
		for attempt := 0; ; attempt++ {
			if attempt > 200 {
				return Bingo{}, fmt.Errorf("bingo: only %d unique cards fit %d items; add items or deal fewer cards", len(b.Cards), len(items))
			}
			// Least-used items first, ties broken at random, keeps the deal balanced.
			order := r.Perm(len(items))
			sort.SliceStable(order, func(i, j int) bool { return uses[order[i]] < uses[order[j]] })
			picked = append([]int(nil), order[:need]...)
			if attempt > 0 { // vary the choice when the balanced one repeats a card
				k := r.IntN(need)
				picked[k] = order[need+r.IntN(len(items)-need)]
			}
			key := slices.Clone(picked)
			slices.Sort(key)
			if !seen[fmt.Sprint(key)] {
				seen[fmt.Sprint(key)] = true
				break
			}
		}
		r.Shuffle(len(picked), func(i, j int) { picked[i], picked[j] = picked[j], picked[i] })
		card := BingoCard{N: len(b.Cards) + 1}
		k := 0
		for row := range size {
			var cells []string
			for col := range size {
				if free && size%2 == 1 && row == size/2 && col == size/2 {
					cells = append(cells, "")
					continue
				}
				uses[picked[k]]++
				cells = append(cells, items[picked[k]])
				k++
			}
			card.Cells = append(card.Cells, cells)
		}
		b.Cards = append(b.Cards, card)
	}
	b.FirstWin = firstWin(b, r)
	return b, nil
}

// firstWin simulates games and returns the median call that completes the
// first row, column or diagonal on any card.
func firstWin(b Bingo, r *rand.Rand) int {
	var results []int
	for range 101 {
		called := map[string]bool{"": true}
		for n, i := range r.Perm(len(b.Calls)) {
			called[b.Calls[i]] = true
			if slices.ContainsFunc(b.Cards, func(c BingoCard) bool { return hasLine(c, called) }) {
				results = append(results, n+1)
				break
			}
		}
	}
	slices.Sort(results)
	return results[len(results)/2]
}

func hasLine(c BingoCard, called map[string]bool) bool {
	n := len(c.Cells)
	diag, anti := true, true
	for i := range n {
		row, col := true, true
		for j := range n {
			row = row && called[c.Cells[i][j]]
			col = col && called[c.Cells[j][i]]
		}
		if row || col {
			return true
		}
		diag = diag && called[c.Cells[i][i]]
		anti = anti && called[c.Cells[i][n-1-i]]
	}
	return diag || anti
}

func uniqueItems(items []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" && !seen[strings.ToLower(it)] {
			seen[strings.ToLower(it)] = true
			out = append(out, it)
		}
	}
	return out
}

// WordSearch is a letter grid hiding every word exactly once.
type WordSearch struct {
	Grid  [][]string
	Words []string // as listed for the solver, in the order given
	Found []Placement
}

// Placement is where a word lies: its first letter and direction.
type Placement struct {
	Word           string
	Row, Col       int
	DRow, DCol     int
	Cells          [][2]int
	ReadsBackwards bool
}

// directions per difficulty: easy reads right and down, medium adds the
// diagonals, hard adds every backwards direction.
var directions = map[string][][2]int{
	"easy":   {{0, 1}, {1, 0}},
	"medium": {{0, 1}, {1, 0}, {1, 1}, {-1, 1}},
	"hard":   {{0, 1}, {1, 0}, {1, 1}, {-1, 1}, {0, -1}, {-1, 0}, {-1, -1}, {1, -1}},
}

// NewWordSearch hides words in a size×size grid. Letters are uppercased and,
// for German, umlauts and ß spelled out as puzzle books do; the filler draws
// from the language's letters and never spells a listed word a second time.
func NewWordSearch(words []string, size int, difficulty, language string, seed int) (WordSearch, error) {
	dirs, ok := directions[difficulty]
	if !ok {
		return WordSearch{}, fmt.Errorf("word search: difficulty %q is not easy, medium or hard", difficulty)
	}
	if size < 6 || size > 30 {
		return WordSearch{}, fmt.Errorf("word search: size %d; use 6 to 30", size)
	}
	ws := WordSearch{Words: uniqueItems(words)}
	norm := make([]string, len(ws.Words))
	longest := 0
	for i, w := range ws.Words {
		norm[i] = puzzleLetters(w, language)
		if n := len([]rune(norm[i])); n < 3 || n > size {
			return WordSearch{}, fmt.Errorf("word search: %q has %d letters; words need 3 to %d", w, n, size)
		}
		longest = max(longest, len(norm[i]))
	}
	if len(ws.Words) == 0 {
		return WordSearch{}, fmt.Errorf("word search: no words")
	}
	r := rng(seed)
	for attempt := range 400 {
		grid := make([][]rune, size)
		for i := range grid {
			grid[i] = make([]rune, size)
		}
		var found []Placement
		order := r.Perm(len(norm))
		sort.SliceStable(order, func(i, j int) bool { return len(norm[order[i]]) > len(norm[order[j]]) })
		ok := true
		for _, wi := range order {
			p, placed := place(grid, []rune(norm[wi]), dirs, r)
			if !placed {
				ok = false
				break
			}
			p.Word = ws.Words[wi]
			found = append(found, p)
		}
		if !ok {
			continue
		}
		alphabet := []rune(fillerLetters(language))
		for i := range grid {
			for j := range grid[i] {
				if grid[i][j] == 0 {
					grid[i][j] = alphabet[r.IntN(len(alphabet))]
				}
			}
		}
		if !eachOnce(grid, norm) {
			_ = attempt
			continue
		}
		sort.Slice(found, func(i, j int) bool {
			return slices.Index(ws.Words, found[i].Word) < slices.Index(ws.Words, found[j].Word)
		})
		ws.Found = found
		for _, row := range grid {
			var cells []string
			for _, c := range row {
				cells = append(cells, string(c))
			}
			ws.Grid = append(ws.Grid, cells)
		}
		return ws, nil
	}
	return WordSearch{}, fmt.Errorf("word search: the words do not fit a %d×%d grid with each found once; enlarge it or use fewer words", size, size)
}

// Hit reports whether the cell at row and col belongs to a hidden word, for
// drawing the answer key: {{if $ws.Hit $r $c}}.
func (ws WordSearch) Hit(row, col int) bool {
	for _, p := range ws.Found {
		for _, c := range p.Cells {
			if c[0] == row && c[1] == col {
				return true
			}
		}
	}
	return false
}

func place(grid [][]rune, w []rune, dirs [][2]int, r *rand.Rand) (Placement, bool) {
	n := len(grid)
	for range 300 {
		d := dirs[r.IntN(len(dirs))]
		row, col := r.IntN(n), r.IntN(n)
		end := [2]int{row + d[0]*(len(w)-1), col + d[1]*(len(w)-1)}
		if end[0] < 0 || end[0] >= n || end[1] < 0 || end[1] >= n {
			continue
		}
		fits := true
		for k, c := range w {
			if g := grid[row+d[0]*k][col+d[1]*k]; g != 0 && g != c {
				fits = false
				break
			}
		}
		if !fits {
			continue
		}
		p := Placement{Row: row, Col: col, DRow: d[0], DCol: d[1], ReadsBackwards: d[1] < 0 || (d[1] == 0 && d[0] < 0)}
		for k, c := range w {
			grid[row+d[0]*k][col+d[1]*k] = c
			p.Cells = append(p.Cells, [2]int{row + d[0]*k, col + d[1]*k})
		}
		return p, true
	}
	return Placement{}, false
}

// eachOnce reports whether every word reads exactly once in all eight
// directions; a palindrome reads twice from the same cells and counts once.
func eachOnce(grid [][]rune, words []string) bool {
	n := len(grid)
	all := directions["hard"]
	for _, w := range words {
		rw := []rune(w)
		hits := map[string]bool{}
		for i := range n {
			for j := range n {
				for _, d := range all {
					matched := true
					var cells []string
					for k, c := range rw {
						y, x := i+d[0]*k, j+d[1]*k
						if y < 0 || y >= n || x < 0 || x >= n || grid[y][x] != c {
							matched = false
							break
						}
						cells = append(cells, fmt.Sprint(y, x))
					}
					if matched {
						slices.Sort(cells)
						hits[strings.Join(cells, " ")] = true
					}
				}
			}
		}
		if len(hits) != 1 {
			return false
		}
	}
	return true
}

func puzzleLetters(word, language string) string {
	w := strings.ToUpper(word)
	if base(language) == "de" {
		w = strings.NewReplacer("Ä", "AE", "Ö", "OE", "Ü", "UE", "ẞ", "SS", "ß", "SS").Replace(w)
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return r
		}
		return -1
	}, w)
}

// fillerLetters weights letters roughly by frequency so the filler looks like
// the language.
func fillerLetters(language string) string {
	switch base(language) {
	case "de":
		return "EEEEEENNNNIIIISSSSRRRRAAAATTTTDDDHHHUUULLLCCGGMMOOBBWFFKZPVJYXQ"
	case "fr":
		return "EEEEEEAAAASSSSIIIITTTTNNNNRRRRUUUULLLOOODDDCCMMPPVQFBGHJXYZK"
	case "es":
		return "EEEEEAAAAAOOOOSSSRRRNNNIIIDDDLLLCCTTUUMMPPBGVYQHFZJXK"
	default:
		return "EEEEEETTTTAAAAOOOIIINNNSSSHHHRRRDDLLCCUUMMWFGYPBVKJXQZ"
	}
}

// Sudoku is a 9×9 puzzle with exactly one solution.
type Sudoku struct {
	Puzzle   [9][9]int // 0 for an empty cell
	Solution [9][9]int
	Clues    int
}

// clueTarget is how many givens each difficulty keeps; every removal keeps
// the solution unique.
var clueTarget = map[string]int{"easy": 38, "medium": 32, "hard": 27}

// NewSudoku fills a grid and removes givens while the solution stays unique.
func NewSudoku(difficulty string, seed int) (Sudoku, error) {
	target, ok := clueTarget[difficulty]
	if !ok {
		return Sudoku{}, fmt.Errorf("sudoku: difficulty %q is not easy, medium or hard", difficulty)
	}
	r := rng(seed)
	var g [9][9]int
	fill(&g, r)
	s := Sudoku{Solution: g, Puzzle: g, Clues: 81}
	for _, cell := range r.Perm(81) {
		if s.Clues <= target {
			break
		}
		y, x := cell/9, cell%9
		keep := s.Puzzle[y][x]
		s.Puzzle[y][x] = 0
		if count(s.Puzzle, 2) != 1 {
			s.Puzzle[y][x] = keep
			continue
		}
		s.Clues--
	}
	if count(s.Puzzle, 2) != 1 {
		return Sudoku{}, fmt.Errorf("sudoku: no unique puzzle for seed %d", seed)
	}
	return s, nil
}

func fits(g *[9][9]int, y, x, v int) bool {
	for i := range 9 {
		if g[y][i] == v || g[i][x] == v || g[y/3*3+i/3][x/3*3+i%3] == v {
			return false
		}
	}
	return true
}

func fill(g *[9][9]int, r *rand.Rand) bool {
	for cell := range 81 {
		y, x := cell/9, cell%9
		if g[y][x] != 0 {
			continue
		}
		for _, v := range r.Perm(9) {
			if fits(g, y, x, v+1) {
				g[y][x] = v + 1
				if fill(g, r) {
					return true
				}
				g[y][x] = 0
			}
		}
		return false
	}
	return true
}

// count returns the number of solutions, stopping at limit.
func count(g [9][9]int, limit int) int {
	best, by, bx := 10, -1, -1
	for y := range 9 {
		for x := range 9 {
			if g[y][x] != 0 {
				continue
			}
			n := 0
			for v := 1; v <= 9; v++ {
				if fits(&g, y, x, v) {
					n++
				}
			}
			if n < best {
				best, by, bx = n, y, x
			}
		}
	}
	if by < 0 {
		return 1
	}
	total := 0
	for v := 1; v <= 9 && total < limit; v++ {
		if fits(&g, by, bx, v) {
			g[by][bx] = v
			total += count(g, limit-total)
			g[by][bx] = 0
		}
	}
	return total
}

// Maze is a perfect maze: every cell reachable by exactly one path. Walls
// are kept per cell: right and bottom.
type Maze struct {
	Width, Height int
	Right, Bottom [][]bool
	Path          [][2]int // the solution from the top-left to the bottom-right cell
}

// NewMaze carves a maze by a randomized depth-first walk.
func NewMaze(width, height, seed int) (Maze, error) {
	if width < 4 || height < 4 || width > 80 || height > 80 {
		return Maze{}, fmt.Errorf("maze: %d×%d cells; use 4 to 80 each way", width, height)
	}
	m := Maze{Width: width, Height: height, Right: grid(height, width, true), Bottom: grid(height, width, true)}
	r := rng(seed)
	seen := grid(height, width, false)
	stack := [][2]int{{0, 0}}
	seen[0][0] = true
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		var next [][2]int
		for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
			y, x := c[0]+d[0], c[1]+d[1]
			if y >= 0 && y < height && x >= 0 && x < width && !seen[y][x] {
				next = append(next, [2]int{y, x})
			}
		}
		if len(next) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		n := next[r.IntN(len(next))]
		switch {
		case n[1] > c[1]:
			m.Right[c[0]][c[1]] = false
		case n[1] < c[1]:
			m.Right[n[0]][n[1]] = false
		case n[0] > c[0]:
			m.Bottom[c[0]][c[1]] = false
		default:
			m.Bottom[n[0]][n[1]] = false
		}
		seen[n[0]][n[1]] = true
		stack = append(stack, n)
	}
	m.Path = m.solve()
	if m.Path == nil {
		return Maze{}, fmt.Errorf("maze: no path for seed %d", seed)
	}
	return m, nil
}

func grid(h, w int, v bool) [][]bool {
	g := make([][]bool, h)
	for i := range g {
		g[i] = make([]bool, w)
		for j := range g[i] {
			g[i][j] = v
		}
	}
	return g
}

func (m Maze) open(a, b [2]int) bool {
	switch {
	case b[1] == a[1]+1:
		return !m.Right[a[0]][a[1]]
	case b[1] == a[1]-1:
		return !m.Right[b[0]][b[1]]
	case b[0] == a[0]+1:
		return !m.Bottom[a[0]][a[1]]
	default:
		return !m.Bottom[b[0]][b[1]]
	}
}

func (m Maze) solve() [][2]int {
	goal := [2]int{m.Height - 1, m.Width - 1}
	from := map[[2]int][2]int{{0, 0}: {-1, -1}}
	queue := [][2]int{{0, 0}}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == goal {
			var path [][2]int
			for p := c; p[0] >= 0; p = from[p] {
				path = append([][2]int{p}, path...)
			}
			return path
		}
		for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
			n := [2]int{c[0] + d[0], c[1] + d[1]}
			if n[0] < 0 || n[0] >= m.Height || n[1] < 0 || n[1] >= m.Width {
				continue
			}
			if _, ok := from[n]; !ok && m.open(c, n) {
				from[n] = c
				queue = append(queue, n)
			}
		}
	}
	return nil
}

// SVG draws the maze at cell px per cell, with the solution when solved.
func (m Maze) SVG(cell int, solved bool) template.HTML {
	w, h := m.Width*cell, m.Height*cell
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="maze" viewBox="-2 -2 %d %d" xmlns="http://www.w3.org/2000/svg"><path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square" d="`, w+4, h+4)
	// Outer walls with the entrance at the top left and the exit at the bottom right.
	fmt.Fprintf(&b, "M%d 0H%d V%d M%d %d H0 V0 ", cell, w, h, w-cell, h)
	for y := range m.Height {
		for x := range m.Width {
			if m.Right[y][x] && x < m.Width-1 {
				fmt.Fprintf(&b, "M%d %d V%d ", (x+1)*cell, y*cell, (y+1)*cell)
			}
			if m.Bottom[y][x] && y < m.Height-1 {
				fmt.Fprintf(&b, "M%d %d H%d ", x*cell, (y+1)*cell, (x+1)*cell)
			}
		}
	}
	b.WriteString(`"/>`)
	if solved {
		b.WriteString(`<path class="solution" fill="none" stroke="currentColor" stroke-width="2" stroke-dasharray="4 3" d="`)
		for i, p := range m.Path {
			op := "L"
			if i == 0 {
				op = "M"
			}
			fmt.Fprintf(&b, "%s%d %d ", op, p[1]*cell+cell/2, p[0]*cell+cell/2)
		}
		b.WriteString(`"/>`)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
