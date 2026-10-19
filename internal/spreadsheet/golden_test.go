package spreadsheet

import (
	"testing"

	"github.com/Anonyfox/lfm/internal/golden"
)

// TestGoldenOutput holds every workbook, start guide, preview and listing
// image document to the bytes it built before.
func TestGoldenOutput(t *testing.T) {
	for _, set := range []string{"budget", "torture"} {
		r := Check(fixture(t, set, nil), testConfig, true, nil, "https://example.org/")
		built, err := r.Build(func(string) []byte { return nil })
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		for _, b := range built {
			files[b.Name] = b.Data
		}
		for name, doc := range r.StartDocs() {
			files["start/"+name+".html"] = doc
		}
		for _, e := range r.Workbook.Editions {
			files["preview/"+e.Key+".html"] = r.Preview(e.Key)
		}
		for _, plan := range r.Images() {
			files["images/"+plan.Name+".html"] = plan.Doc
		}
		golden.Files(t, "spreadsheet-"+set, files)
	}
}
