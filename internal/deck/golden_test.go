package deck

import (
	"fmt"
	"testing"

	"github.com/Anonyfox/lfm/internal/golden"
)

// TestGoldenOutput holds every deck, print, guide, preview and listing
// image document the briefing builds to the bytes it built before.
func TestGoldenOutput(t *testing.T) {
	r := briefing(t)
	built, err := r.Build(func(string) []byte { return nil })
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	var shots []Shot
	for _, b := range built {
		k := b.Edition.Key + "/" + b.Deck.Key
		files[k+".pptx"] = b.Data
		for _, p := range b.Prints() {
			files[fmt.Sprintf("%s.%s%s.html", k, p.Kind, p.Paper)] = r.PrintDoc(b, p)
		}
		for n := 1; n <= len(b.Deck.Slides); n++ {
			files[fmt.Sprintf("%s.slide%02d.html", k, n)] = r.SlideShot(b, n)
		}
		files[k+".notes-shot.html"] = r.NotesShot(b, 2)
		files[k+".quiz-shot.html"] = r.QuizShot(b)
		if b.Edition.Key == r.Showcase().Key {
			shots = append(shots, Shot{Key: b.Key, Title: b.Deck.Title, Cover: []byte("c"), Slides: [][]byte{[]byte("s")}, Notes: []byte("n"), Quiz: []byte("q"), Facts: Facts(b)})
		}
	}
	for _, e := range r.Editions {
		files[e.Key+"/guide.html"] = r.Guide(e)
		files[e.Key+"/preview.html"] = r.Preview(e)
		z, err := r.Zip(e, built, map[string][]byte{}, []byte("%PDF guide"), "")
		if err != nil {
			t.Fatal(err)
		}
		files[e.Key+".zip"] = z
	}
	for _, plan := range r.Images(shots) {
		files["images/"+plan.Name+".html"] = plan.Doc
	}
	files["download.html"] = DownloadPage(r, nil)
	golden.Files(t, "deck-briefing", files)
}
