package printable

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCalendarComputesDatesAndWeeks(t *testing.T) {
	c, err := NewCalendar("2027", "monday", 0, "de")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Months) != 12 || len(c.Days) != 365 || c.Weekdays[0].Short != "Mo" || c.Months[2].Name != "März" {
		t.Fatalf("months %d, days %d, %v, %s", len(c.Months), len(c.Days), c.Weekdays[0], c.Months[2].Name)
	}
	// 1 January 2027 is a Friday in ISO week 53 of 2026.
	jan := c.Months[0]
	if first := jan.Rows[0]; first.ID != "w2026-53" || first.Number != 53 || first.Days[4].Day != 1 || first.Days[0].InRange {
		t.Fatalf("first row %+v", first)
	}
	for _, m := range c.Months {
		for _, row := range m.Rows {
			if row.Days[0].Date.Weekday() != time.Monday {
				t.Fatalf("%s row starts on %s", m.Name, row.Days[0].Date.Weekday())
			}
		}
	}
	leap, _ := NewCalendar("2028-02", "sunday", 1, "en")
	if len(leap.Days) != 29 || leap.Months[0].Rows[0].Days[0].Date.Weekday() != time.Sunday {
		t.Fatalf("leap February: %d days", len(leap.Days))
	}
	// Monday and Sunday weeks of the same span share their IDs.
	mon, _ := NewCalendar("2027", "monday", 0, "en")
	sun, _ := NewCalendar("2027", "sunday", 0, "en")
	for i, d := range mon.Days {
		if d.Date.Weekday() != time.Sunday && d.Week != sun.Days[i].Week {
			t.Fatalf("%s is in %s with Monday weeks but %s with Sunday weeks", d.IsoDate(), d.Week, sun.Days[i].Week)
		}
	}
	for _, bad := range [][2]string{{"27", "monday"}, {"2027-13", "monday"}, {"2027", "friday"}} {
		if _, err := NewCalendar(bad[0], bad[1], 0, "en"); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestBingoCardsAreUniqueAndBalanced(t *testing.T) {
	var items []string
	for i := range 40 {
		items = append(items, fmt.Sprint("item ", i))
	}
	b, err := NewBingo(items, 50, 5, true, 7)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	uses := map[string]int{}
	for _, c := range b.Cards {
		var cells []string
		for _, row := range c.Cells {
			for _, cell := range row {
				cells = append(cells, cell)
				uses[cell]++
			}
		}
		if c.Cells[2][2] != "" {
			t.Fatal("no free centre")
		}
		key := strings.Join(sorted(cells), "|")
		if seen[key] {
			t.Fatalf("card %d repeats another", c.N)
		}
		seen[key] = true
	}
	delete(uses, "")
	lo, hi := 1<<30, 0
	for _, n := range uses {
		lo, hi = min(lo, n), max(hi, n)
	}
	if hi-lo > 2 || b.FirstWin < 3 {
		t.Fatalf("balance %d..%d, first win %d", lo, hi, b.FirstWin)
	}
	again, _ := NewBingo(items, 50, 5, true, 7)
	if fmt.Sprint(again.Cards) != fmt.Sprint(b.Cards) {
		t.Fatal("same seed, different cards")
	}
	if _, err := NewBingo(items[:20], 5, 5, true, 1); err == nil {
		t.Fatal("too few items accepted")
	}
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestWordSearchHidesEachWordOnce(t *testing.T) {
	words := []string{"Lebkuchen", "Glühwein", "Schnee", "Kerze", "Stern", "Engel", "Plätzchen", "Rentier"}
	ws, err := NewWordSearch(words, 13, "hard", "de", 3)
	if err != nil {
		t.Fatal(err)
	}
	grid := make([][]rune, len(ws.Grid))
	for i, row := range ws.Grid {
		for _, c := range row {
			grid[i] = append(grid[i], []rune(c)...)
		}
	}
	var norm []string
	for _, w := range words {
		norm = append(norm, puzzleLetters(w, "de"))
	}
	if !eachOnce(grid, norm) || len(ws.Found) != len(words) || ws.Found[1].Word != "Glühwein" {
		t.Fatalf("placements %+v", ws.Found)
	}
	if puzzleLetters("Glühwein", "de") != "GLUEHWEIN" {
		t.Fatal("umlauts are not spelled out")
	}
	if _, err := NewWordSearch([]string{"Ab"}, 10, "easy", "en", 1); err == nil {
		t.Fatal("a two-letter word was accepted")
	}
}

func TestSudokuHasExactlyOneSolution(t *testing.T) {
	for _, level := range []string{"easy", "medium", "hard"} {
		s, err := NewSudoku(level, 11)
		if err != nil {
			t.Fatal(err)
		}
		if count(s.Puzzle, 3) != 1 || s.Clues > clueTarget[level]+6 {
			t.Fatalf("%s: %d clues, %d solutions", level, s.Clues, count(s.Puzzle, 3))
		}
		for y := range 9 {
			for x := range 9 {
				if v := s.Puzzle[y][x]; v != 0 && v != s.Solution[y][x] {
					t.Fatal("a clue contradicts the solution")
				}
			}
		}
	}
}

func TestMazeIsPerfect(t *testing.T) {
	m, err := NewMaze(12, 9, 5)
	if err != nil {
		t.Fatal(err)
	}
	// A perfect maze is a spanning tree: cells - 1 open passages.
	open := 0
	for y := range m.Height {
		for x := range m.Width {
			if !m.Right[y][x] && x < m.Width-1 {
				open++
			}
			if !m.Bottom[y][x] && y < m.Height-1 {
				open++
			}
		}
	}
	if open != m.Width*m.Height-1 || m.Path[0] != [2]int{0, 0} || m.Path[len(m.Path)-1] != [2]int{8, 11} {
		t.Fatalf("passages %d, path %v", open, m.Path)
	}
	if svg := string(m.SVG(10, true)); !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, `class="solution"`) {
		t.Fatal(svg)
	}
}
