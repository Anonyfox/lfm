package printable

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/golden"
)

// TestGoldenOutput holds every edition's print document, the listing image
// documents and the shop facts to the bytes they had before.
func TestGoldenOutput(t *testing.T) {
	r := Check(fixture(t, nil), testConfig, true, nil, "https://example.org/")
	files := map[string][]byte{}
	for _, ed := range r.Editions {
		files["editions/"+ed.Edition.Key+".html"] = ed.Doc
	}
	for _, plan := range r.Images() {
		files["images/"+plan.Name+".html"] = plan.Doc
	}
	files["facts.txt"] = []byte(strings.Join(Facts(r), "\n"))
	golden.Files(t, "printable-tracker", files)
}
