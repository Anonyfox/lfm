package spreadsheet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/project"
)

const smallWorkbook = `{"editions":[{"key":"en","language":"en","currency":"USD"}],"today":"2026-10-19","sheets":[` +
	`{"key":"data","name":"Data","kind":"table","rows":3,"columns":[{"key":"n","title":"N","type":"number"},{"key":"t","title":"T","type":"number","formula":"=[.n]*2"}],"seed":[{"n":1}]},` +
	`{"key":"calc","name":"Calc","kind":"report","cells":[{"at":"B2","formula":"=SUM([data.n])","name":"total"},{"at":"B3","formula":"=B3+1"}]}],` +
	`"expect":{"empty":{"total":1}}}`

// Property: Check reads any concept.md, workbook.json and listing.md
// without panicking, and computes every formula they hold in bounded time.
func FuzzCheck(f *testing.F) {
	budget, _ := os.ReadFile("testdata/budget/workbook.json")
	torture, _ := os.ReadFile("testdata/torture/workbook.json")
	concept, _ := os.ReadFile("testdata/budget/concept.md")
	f.Add(string(concept), smallWorkbook, "")
	f.Add("# Spreadsheet: X\n## Tabs\n| Tab | Job |\n| data | x |\n", `{"editions":[{"key":"en","language":"en","currency":"USD"}],"sheets":[{"key":"r","name":"R","kind":"report","cells":[{"at":"A1","formula":"=SUM(A2:XFD1048576)"}]}]}`, "# Listing: X\n## Price\n1 EUR\n")
	f.Add("", string(budget), "")
	f.Add("", string(torture), "")
	f.Fuzz(func(t *testing.T, concept, workbook, listing string) {
		if len(concept)+len(workbook)+len(listing) > 64<<10 {
			t.Skip()
		}
		root := t.TempDir()
		dir := filepath.Join(root, filepath.FromSlash(Dir))
		if os.MkdirAll(dir, 0o755) != nil {
			t.Fatal("cannot write fixture")
		}
		for name, content := range map[string]string{ConceptFile: concept, WorkbookFile: workbook, ListingFile: listing} {
			if os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644) != nil {
				t.Fatal("cannot write fixture")
			}
		}
		p, err := project.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		Check(p, testConfig, true, nil, "https://example.org/")
	})
}
