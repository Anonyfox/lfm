package document

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/Anonyfox/lfm/internal/release"
)

// TestPrintMatchesWord prints every document of the fixtures with Chrome,
// as a release does, and opens it in Word: the pages must agree, so the
// PDFs buyers get and the page checks speak for Word. LFM_WORD=1 runs it.
func TestPrintMatchesWord(t *testing.T) {
	if os.Getenv("LFM_WORD") == "" || !office.HasWord() {
		t.Skip("set LFM_WORD=1 with Word for Mac installed")
	}
	printer, err := release.NewPrinter()
	if err != nil {
		t.Skip(err)
	}
	defer printer.Close()
	type item struct {
		name string
		b    Built
		r    Report
	}
	var items []item
	add := func(prefix string, r Report) {
		built, _ := r.Build(func(string) []byte { return nil })
		for _, b := range built {
			items = append(items, item{prefix + "-" + b.Edition.Key + "-" + b.Doc.Key, b, r})
		}
	}
	add("club", Check(fixture(t, "club", nil), testConfig, true, nil, ""))
	for _, th := range ThemeNames() {
		add(th, Check(features(t, th), testConfig, true, nil, ""))
	}
	chrome := map[string]int{}
	files := map[string][]byte{}
	for _, it := range items {
		pdf, err := printer.Print(printHTML(it.r.render(it.b.Doc, it.b.Edition)), it.r.GuideAssets(), false)
		if err != nil {
			t.Fatal(err)
		}
		chrome[it.name] = pdfx.PageCount(pdf)
		files[it.name] = it.b.Data
		if os.Getenv("OUT") != "" {
			os.WriteFile(os.Getenv("OUT")+"/"+it.name+"-chrome.pdf", pdf, 0o644)
		}
	}
	word := map[string]string{}
	{
		opened, _ := office.OpenInWord(files)
		for n, o := range opened {
			word[n] = o.Result
			if os.Getenv("OUT") != "" {
				os.WriteFile(os.Getenv("OUT")+"/"+n+"-word.pdf", o.Data, 0o644)
			}
		}
	}
	names := make([]string, 0, len(chrome))
	for n := range chrome {
		names = append(names, n)
	}
	sort.Strings(names)
	same := 0
	for _, n := range names {
		mark := ""
		if fmt.Sprint(chrome[n]) == word[n] {
			same++
		} else {
			mark = "  <-- differs"
			t.Errorf("%s: Chrome prints %d pages, Word %s", n, chrome[n], word[n])
		}
		t.Logf("%-40s chrome %d  word %s%s", n, chrome[n], word[n], mark)
	}
	t.Logf("%d of %d page counts agree", same, len(names))
}
