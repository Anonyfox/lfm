package document

import (
	"testing"

	"github.com/Anonyfox/lfm/internal/golden"
)

// TestGoldenOutput holds every file a set builds, and every page it
// renders for Chrome, the guide, the preview and the listing images, to
// the bytes it built before.
func TestGoldenOutput(t *testing.T) {
	t.Parallel()
	for _, set := range []string{"club", "features"} {
		var r Report
		if set == "club" {
			r = Check(fixture(t, set, nil), testConfig, true, nil, "https://example.org/")
		} else {
			r = Check(features(t, "warm"), testConfig, true, nil, "https://example.org/")
		}
		built, err := r.Build(func(string) []byte { return nil })
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		var shots []Shot
		for _, b := range built {
			k := b.Edition.Key + "/" + b.Doc.Key
			files[k+".docx"] = b.Data
			files[k+".print.html"] = r.PrintDoc(b)
			files[k+".shot.html"] = r.ShotDoc(b)
			if b.Edition == r.Showcase() {
				shots = append(shots, Shot{Key: b.Key, Title: b.Doc.Title, PNG: []byte("png")})
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
		golden.Files(t, "document-"+set, files)
	}
}
