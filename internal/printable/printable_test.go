package printable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

const testConcept = `# Printable: Habit Tracker

## Buyer
People who want one calm page a month to see their habits.
- Not for: anyone who wants an app with reminders.

## Promise
Track up to eight habits a month on paper, with a yearly overview.

## Contents
| Part | Pages | Job |
| --- | --- | --- |
| Cover | 1 | shows the tracker and what is inside |
| Guide | 1 | explains printing at actual size and the licence |
| Months | 2 | one tracking grid per month to tick off |
| Puzzles | 1 | a word search for a break |
| Key | 1 | the solution to the word search |

## Editions
- Sizes: a4, letter
- Start: monday, sunday

## Look
Body: system-ui, sans-serif
Headings: Nunito, sans-serif
Accent: #2b6e5a
`

const testTemplate = `{{- $cal := calendar "2027-01" (.Edition.Option "Start") 2 -}}
{{- $ws := wordsearch .Data.words 10 "easy" 1 -}}
<section class="page" data-part="Cover" data-role="cover"><h1>{{.Name}}</h1><a href="#m2027-01">January</a></section>
<section class="page" data-part="Guide" data-role="guide"><p>Print at 100%. Months start on page <span class="page-ref" data-ref="m2027-01"></span>.</p></section>
{{range $cal.Months}}<section class="page" data-part="Months" id="{{.ID}}"><h2>{{.Name}}</h2>
<table>{{range .Rows}}<tr>{{range .Days}}<td>{{.Day}}</td>{{end}}</tr>{{end}}</table>
<span class="page-number"></span> / <span class="page-count"></span></section>{{end}}
<section class="page" data-part="Puzzles" data-role="puzzle"><table>{{range $ws.Grid}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</table></section>
<section class="page" data-part="Key" data-role="key"><p>{{range $ws.Found}}{{.Word}} {{end}}</p></section>
`

const testListing = `# Listing: Habit Tracker

## Title
Printable Habit Tracker: monthly pages, Monday or Sunday start, A4 and Letter

## Description
One calm page a month to see which habits stuck. Tick a box a day for up to eight habits, and look back at the year in one place.

- A4 and US Letter, Monday or Sunday start
- a word search for a break

Nothing is shipped: you download the PDFs after buying. For personal use.

## Tags
- habit tracker
- monthly tracker

## Price
5.00 EUR

## AI
Text: none
Images: none
Tools:
Statement:
`

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, replace map[string]string) project.Project {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{ConceptFile: testConcept, TemplateFile: testTemplate, ListingFile: testListing,
		Dir + "/data/words.json": `["habit", "water", "sleep", "steps", "read"]`}
	for k, v := range replace {
		files[k] = v
	}
	for rel, content := range files {
		if content != "" {
			write(t, root, rel, content)
		}
	}
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var testConfig = Config{Path: "k3x9m2q7w8e4r5t6y1u0", Language: "en", Shop: "Calm Paper"}

func check(t *testing.T, replace map[string]string) Report {
	t.Helper()
	price, _ := deliver.ParsePrice("5.00 EUR")
	return Check(fixture(t, replace), testConfig, true, map[string]*deliver.Price{"tracker": &price}, "https://example.org/")
}

func messages(ds []diagnostic.Diagnostic) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Rule+": "+d.Message)
	}
	return strings.Join(out, "\n")
}

func TestCompletePrintableChecksClean(t *testing.T) {
	r := check(t, map[string]string{Dir + "/printable.css": `@font-face { font-family: "Nunito"; src: url(fonts/nunito.woff2); }`, Dir + "/fonts/nunito.woff2": "wOF2"})
	if len(r.Diagnostics) > 0 || !r.Buildable {
		t.Fatalf("findings:\n%s", messages(r.Diagnostics))
	}
	if len(r.Editions) != 4 || r.Summary.Pages != 6 || r.Summary.Links != 1 || r.Summary.URL != "https://example.org/k3x9m2q7w8e4r5t6y1u0/" {
		t.Fatalf("%+v", r.Summary)
	}
	doc := string(r.Editions[0].Doc)
	for _, want := range []string{"@page { size: 210mm 297mm", `class="a4 monday"`, `data-ref="m2027-01">3</span>.`, "3</span> / <span class=\"page-count\">6"} {
		if !strings.Contains(doc, want) {
			t.Errorf("document lacks %q", want)
		}
	}
	if !strings.Contains(string(r.Editions[1].Doc), `class="a4 sunday"`) || r.FileName(r.Editions[3].Edition) != "habit-tracker-letter-sunday.pdf" {
		t.Error("editions")
	}
}

func TestPageDefectsAreReported(t *testing.T) {
	broken := strings.NewReplacer(
		`data-part="Cover" data-role="cover"`, `data-part="Cover"`,
		`<a href="#m2027-01">`, `<a href="#nowhere">`,
		`data-part="Key" data-role="key"`, `data-part="Answers"`,
		`data-role="guide"`, ``,
	).Replace(testTemplate) + "<p>stray</p>"
	r := check(t, map[string]string{TemplateFile: broken})
	got := messages(r.Diagnostics)
	for _, want := range []string{"outside a <section", "first page is not the cover", "points to #nowhere", `Part "Answers" is not in the concept`, `Part "Key" has 0 pages`, "how to print or import", "have no answer key"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// Four editions, each finding once.
	if strings.Count(got, "points to #nowhere") != 1 {
		t.Errorf("a finding repeats per edition:\n%s", got)
	}
}

func TestTemplateAndGeneratorErrorsBlock(t *testing.T) {
	r := check(t, map[string]string{TemplateFile: `{{ wordsearch .Data.words 4 "easy" 1 }}`})
	if r.Buildable || !strings.Contains(messages(r.Diagnostics), "No edition renders") || r.Diagnostics[0].Severity != diagnostic.Error {
		t.Fatalf("%s", messages(r.Diagnostics))
	}
	r = check(t, map[string]string{TemplateFile: "{{ if }}"})
	if r.Buildable || !strings.Contains(messages(r.Diagnostics), "does not parse") {
		t.Fatalf("%s", messages(r.Diagnostics))
	}
}

func TestUnreadableStyleAndDataBlock(t *testing.T) {
	for _, name := range []string{StyleName, DataDir + "/words.json"} {
		p := fixture(t, nil)
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte(`["habit"]`), 0o644); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(p.Root, filepath.FromSlash(Dir), filepath.FromSlash(name))
		os.Remove(file)
		if err := os.Symlink(outside, file); err != nil {
			t.Fatal(err)
		}
		r := Check(p, testConfig, true, nil, "")
		if r.Buildable || !strings.Contains(messages(r.Diagnostics), "symlink leaves the project") {
			t.Fatalf("%s was left out silently:\n%s", name, messages(r.Diagnostics))
		}
	}
}

func TestConfigListingAndConceptGaps(t *testing.T) {
	r := Check(fixture(t, map[string]string{
		ConceptFile: strings.Replace(strings.Replace(testConcept, "- Sizes: a4, letter", "- Sizes: a3", 1), "- Not for: anyone who wants an app with reminders.\n", "", 1),
		ListingFile: strings.Replace(testListing, "5.00 EUR", "6.00 EUR", 1),
	}), Config{Path: "short"}, true, nil, "https://example.org/")
	got := messages(r.Diagnostics)
	for _, want := range []string{"not an unguessable folder name", `Size "a3" is unknown`, "no paper size", `"Not for:"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	r = check(t, map[string]string{ListingFile: strings.Replace(testListing, "5.00 EUR", "6.00 EUR", 1) + "\n"})
	if !strings.Contains(messages(r.Diagnostics), `"nunito" is not loaded from fonts/`) {
		t.Errorf("unembedded font not reported:\n%s", messages(r.Diagnostics))
	}
	if !strings.Contains(messages(r.Diagnostics), "differs from the page's offer") {
		// the offer is only compared when the config names it
		r2 := Check(fixture(t, map[string]string{ListingFile: strings.Replace(testListing, "5.00 EUR", "6.00 EUR", 1)}), Config{Path: testConfig.Path, Offer: "tracker"}, true, map[string]*deliver.Price{"tracker": mustPrice("5.00 EUR")}, "https://example.org/")
		if !strings.Contains(messages(r2.Diagnostics), "differs from the page's offer") {
			t.Errorf("price mismatch not reported:\n%s", messages(r2.Diagnostics))
		}
	}
}

func mustPrice(s string) *deliver.Price {
	p, _ := deliver.ParsePrice(s)
	return &p
}

func TestDistributionPackagesEveryChannel(t *testing.T) {
	r := check(t, nil)
	pdfs := map[string][]byte{}
	for _, ed := range r.Editions {
		pdfs[ed.Edition.Key] = []byte("%PDF-" + ed.Edition.Key)
	}
	files, channels, issues := Distribution(r, Printed{PDFs: pdfs, Images: []Image{{Name: "01-cover.jpg", Data: []byte{0xff, 0xd8}}}}, "2026-10-19")
	if len(issues) > 0 || len(channels) != len(deliver.Shops()) {
		t.Fatalf("%v %v", channels, messages(issues))
	}
	for _, name := range []string{"README.md", "habit-tracker-a4-monday.pdf", "images/01-cover.jpg", "stores/etsy.md", "stores/gumroad.md"} {
		if files[name] == nil {
			t.Errorf("missing %s", name)
		}
	}
	etsy := string(files["stores/etsy.md"])
	for _, want := range []string{"[`habit-tracker-letter-sunday.pdf`](../habit-tracker-letter-sunday.pdf)", "`habit tracker`", "6 pages per file, in A4, US Letter", "Months: 2 pages", "1 internal links"} {
		if !strings.Contains(etsy, want) {
			t.Errorf("etsy guide lacks %q:\n%s", want, etsy)
		}
	}
	// Six PDFs exceed Etsy's five files: they are zipped by paper size.
	r.Plan.Axes = append(r.Plan.Axes, Axis{Name: "Ink", Values: []string{"color", "ink-saver"}})
	r2 := check(t, map[string]string{ConceptFile: strings.Replace(testConcept, "- Start: monday, sunday", "- Start: monday, sunday\n- Ink: color, ink-saver", 1)})
	for _, ed := range r2.Editions {
		pdfs[ed.Edition.Key] = []byte("%PDF-" + ed.Edition.Key)
	}
	files, _, _ = Distribution(r2, Printed{PDFs: pdfs}, "2026-10-19")
	if files["habit-tracker-a4.zip"] == nil || !strings.Contains(string(files["stores/etsy.md"]), "habit-tracker-letter.zip") || strings.Contains(string(files["stores/etsy.md"]), "-ink-saver.pdf") {
		t.Errorf("etsy zips:\n%s", files["stores/etsy.md"])
	}
}

func TestListingImagesPlan(t *testing.T) {
	r := check(t, nil)
	shots := r.Shots()
	images := r.Images()
	if len(shots) == 0 || shots[0].Page != 1 || images[0].Name != "01-cover.jpg" || !strings.HasSuffix(images[len(images)-1].Name, "-sizes.jpg") {
		t.Fatalf("%v %v", shots, images)
	}
	for _, img := range images {
		if !strings.Contains(string(img.Doc), "page-1.png") && img.Name == "01-cover.jpg" {
			t.Error("the cover image does not show the cover")
		}
	}
}
