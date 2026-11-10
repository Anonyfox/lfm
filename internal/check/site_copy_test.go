package check

import (
	"fmt"
	"strings"
	"testing"
)

func TestSiteTriplets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		counts []int
		text   string
		want   int
	}{
		{"at limit", []int{5, 5}, "Sand, Wind und Meer.", 0},
		{"across pages", []int{5, 6}, "Sand, Wind und Meer.", 11},
		{"single page", []int{11}, "Tools, notes and maps.", 11},
		{"Oxford comma", []int{11}, "Tools, notes, and maps!", 11},
		{"two items", []int{11}, "Tools and maps.", 0},
		{"hidden", []int{11}, `<span hidden>Tools, notes and maps.</span>`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pages []*Document
			for i, n := range tc.counts {
				d, err := Parse([]byte("<main>"+strings.Repeat("<p>"+tc.text+"</p>\n", n)+"</main>"), Options{File: fmt.Sprintf("%d.html", i)})
				if err != nil {
					t.Fatal(err)
				}
				pages = append(pages, d)
			}
			got := SiteCopy(pages)
			count := 0
			for _, d := range got {
				if d.Rule == "copy.triplets" {
					count++
					if d.Severity != Warning || d.Location.File == "" || d.Location.Line < 1 || d.Hint == "" {
						t.Fatal(d)
					}
				}
			}
			if count != tc.want {
				t.Fatalf("got %d, want %d: %+v", count, tc.want, got)
			}
		})
	}
}

func TestSiteDuplicateGroups(t *testing.T) {
	const prose = "We ship 12 detailed maps for the northern coastal paths."
	for _, tc := range []struct {
		name, a, b string
		want       int
	}{
		{"normalized", "<main><p>" + prose + "</p><p>" + prose + "</p></main>", "<main><h2>WE SHIP 99 detailed maps for the northern coastal paths!</h2></main>", 1},
		{"without landmarks", "<p>" + prose + "</p><p>" + prose + "</p>", "<p>" + prose + "</p>", 1},
		{"short", "<main><h2>Find the map</h2></main>", "<main><h2>Find the map</h2></main>", 0},
		{"navigation", "<nav><p>" + prose + "</p></nav>", "<nav><p>" + prose + "</p></nav>", 0},
		{"footer", "<footer><p>" + prose + "</p></footer>", "<footer><p>" + prose + "</p></footer>", 0},
		{"shared chrome", "<header><p>" + prose + "</p></header>", "<header><p>" + prose + "</p></header>", 0},
		{"one page only", "<main><p>" + prose + "</p><p>" + prose + "</p></main>", "<main><p>A distinct sentence lives here with enough words for comparison.</p></main>", 0},
		{"quoted", "<blockquote>" + prose + "</blockquote>", "<main><p>" + prose + "</p></main>", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pages []*Document
			for i, raw := range []string{tc.a, tc.b} {
				d, err := Parse([]byte(raw), Options{File: fmt.Sprintf("%d.html", i)})
				if err != nil {
					t.Fatal(err)
				}
				pages = append(pages, d)
			}
			got := SiteCopy(pages)
			if len(got) != tc.want {
				t.Fatalf("%+v", got)
			}
			if tc.want > 0 {
				if !strings.Contains(got[0].Hint, "0.html:") || !strings.Contains(got[0].Hint, "1.html:") || strings.Count(got[0].Hint, "0.html:") != 2 {
					t.Fatal(got)
				}
			}
		})
	}
}

func FuzzSiteCopy(f *testing.F) {
	f.Add("<main><p>Tools, notes and maps.</p></main>")
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 32768 {
			t.Skip()
		}
		a, err := Parse([]byte(raw), Options{File: "a.html"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := Parse([]byte(raw), Options{File: "b.html"})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range SiteCopy([]*Document{a, b}) {
			if d.Location.Line < 1 || d.Rule == "" || d.Hint == "" {
				t.Fatal(d)
			}
		}
	})
}
