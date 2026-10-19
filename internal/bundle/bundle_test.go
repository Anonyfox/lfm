package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

const goodPath = "k3n8q2w7x5r9t4m6p1"

func newProject(t *testing.T, files map[string]string) project.Project {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// present are the project's deliverables as the compiler passes them:
// every kind, the named ones present.
func present(kinds ...string) map[string]Unit {
	out := map[string]Unit{}
	for _, k := range Kinds {
		out[k] = Unit{Present: slices.Contains(kinds, k)}
	}
	return out
}

func sells(units map[string]Unit, kind, sell string) map[string]Unit {
	u := units[kind]
	u.Sell = sell
	units[kind] = u
	return units
}

func config(units ...string) Config {
	return Config{Name: "Fire Safety Kit", Path: goodPath, Units: units}
}

func errorsOf(r Report) []string {
	var out []string
	for _, d := range r.Diagnostics {
		if d.Rule == RuleConfig {
			out = append(out, d.Message)
		}
	}
	return out
}

func TestCheckCleanBundle(t *testing.T) {
	r := Check(newProject(t, nil), config("deck", "document"), true, present("deck", "document", "book"), nil, "https://example.org/")
	if len(r.Diagnostics) > 0 {
		t.Fatalf("diagnostics %+v", r.Diagnostics)
	}
	if !slices.Equal(r.Summary.Units, []string{"deck", "document"}) || r.Summary.Sell != deliver.SellPage || r.Summary.Name != "Fire Safety Kit" {
		t.Errorf("summary %+v", r.Summary)
	}
	if r.Summary.URL != "https://example.org/"+goodPath+"/" {
		t.Errorf("url %q", r.Summary.URL)
	}
	if !r.Holds("deck") || r.Holds("book") || r.Summary.Listing {
		t.Errorf("holds deck %v, book %v, listing %v", r.Holds("deck"), r.Holds("book"), r.Summary.Listing)
	}
}

func TestCheckDefaultUnitsAreEveryPresentDeliverableInOrder(t *testing.T) {
	r := Check(newProject(t, nil), config(), true, present("deck", "book", "spreadsheet"), nil, "https://example.org")
	if len(r.Diagnostics) > 0 {
		t.Fatalf("diagnostics %+v", r.Diagnostics)
	}
	if !slices.Equal(r.Summary.Units, []string{"book", "spreadsheet", "deck"}) {
		t.Errorf("units %v", r.Summary.Units)
	}
	if r.Summary.URL != "https://example.org/"+goodPath+"/" {
		t.Errorf("url %q", r.Summary.URL)
	}
}

func TestCheckNotConfigured(t *testing.T) {
	p := newProject(t, nil)
	if r := Check(p, Config{}, false, present("deck", "document"), nil, ""); len(r.Diagnostics) != 0 || r.Summary.Name != "" {
		t.Fatalf("an unconfigured bundle reports %+v", r)
	}
	units := sells(sells(present("deck", "document"), "deck", "bundle"), "book", "bundle")
	r := Check(p, Config{}, false, units, nil, "")
	if got := errorsOf(r); len(got) != 1 || !strings.Contains(got[0], "deliverables.deck.sell is bundle, but lfm.json has no deliverables.bundle.") {
		t.Fatalf("errors %q", got)
	}
}

func TestCheckConfigErrors(t *testing.T) {
	offers := map[string]*deliver.Price{"kit": {Text: "19 EUR"}}
	for _, tc := range []struct {
		name  string
		cfg   Config
		units map[string]Unit
		want  string
	}{
		{"no name", Config{Name: "  ", Path: goodPath}, present("deck", "document"), "deliverables.bundle has no name."},
		{"one unit", config("deck"), present("deck", "document"), "holds 1 deliverable; a bundle needs two or more."},
		{"one present", config(), present("deck"), "holds 1 deliverable; a bundle needs two or more."},
		{"none present", config(), present(), "holds 0 deliverables; a bundle needs two or more."},
		{"duplicate unit", config("deck", "deck"), present("deck", "document"), "holds 1 deliverable"},
		{"unknown unit", config("deck", "video"), present("deck", "document"), `deliverables.bundle.units names "video".`},
		{"absent unit", config("deck", "book"), present("deck", "document"), "names book, but the project has no deliverables/book."},
		{"bundle-only not held", config("deck", "document"), sells(present("deck", "document", "printable"), "printable", "bundle"), "deliverables.printable.sell is bundle, but the bundle does not hold it."},
		{"short path", Config{Name: "Kit", Path: "short"}, present("deck", "document"), `deliverables.bundle.path "short" is not an unguessable folder name.`},
		{"path with capitals", Config{Name: "Kit", Path: "K3N8Q2W7X5R9T4M6P1"}, present("deck", "document"), "is not an unguessable folder name."},
		{"no path", Config{Name: "Kit"}, present("deck", "document"), `deliverables.bundle.path "" is not`},
		{"unknown offer", Config{Name: "Kit", Path: goodPath, Offer: "bundle"}, present("deck", "document"), `deliverables.bundle.offer "bundle" names no offer`},
	} {
		r := Check(newProject(t, nil), tc.cfg, true, tc.units, offers, "https://example.org/")
		got := errorsOf(r)
		if len(got) != 1 || !strings.Contains(got[0], tc.want) {
			t.Errorf("%s: errors %q, want %q", tc.name, got, tc.want)
		}
		if r.Summary.URL != "" {
			t.Errorf("%s: a broken bundle has the url %q", tc.name, r.Summary.URL)
		}
		for _, d := range r.Diagnostics {
			if d.Severity != diagnostic.Error || d.Location.File != "lfm.json" || d.Location.Line != 1 {
				t.Errorf("%s: diagnostic %+v", tc.name, d)
			}
		}
	}
}

func TestCheckBundleOnlyUnitHeldIsFine(t *testing.T) {
	units := sells(present("deck", "document"), "deck", "bundle")
	if r := Check(newProject(t, nil), config(), true, units, nil, "https://x.org"); len(r.Diagnostics) != 0 || !r.Holds("deck") {
		t.Fatalf("report %+v", r)
	}
}

func TestCheckShopsBundleNeedsNoPath(t *testing.T) {
	cfg := Config{Name: "Kit", Sell: "shops"}
	r := Check(newProject(t, nil), cfg, true, present("deck", "document"), nil, "https://x.org")
	if len(r.Diagnostics) != 0 || r.Summary.Sell != "shops" || r.Summary.URL != "" {
		t.Fatalf("report %+v", r)
	}
}

func TestCheckKnownOfferIsFine(t *testing.T) {
	cfg := config()
	cfg.Offer = "kit"
	r := Check(newProject(t, nil), cfg, true, present("deck", "document"), map[string]*deliver.Price{"kit": {Text: "19 EUR"}}, "https://x.org")
	if len(r.Diagnostics) != 0 {
		t.Fatalf("diagnostics %+v", r.Diagnostics)
	}
}

func TestCheckReadsTheListing(t *testing.T) {
	p := newProject(t, map[string]string{ListingFile: "# Listing: Fire Safety Kit\n\n## Title\n\nFire Safety Kit\n"})
	r := Check(p, config(), true, present("deck", "document"), nil, "https://x.org")
	if !r.Summary.Listing || r.Listing.Title != "Fire Safety Kit" {
		t.Fatalf("listing %+v", r.Listing)
	}
	if len(errorsOf(r)) != 0 {
		t.Errorf("config errors %q", errorsOf(r))
	}
	var warnings int
	for _, d := range r.Diagnostics {
		if d.Rule == RuleListing {
			warnings++
			if d.Severity != diagnostic.Warning || d.Location.File != ListingFile || d.Location.Line < 1 {
				t.Errorf("listing diagnostic %+v", d)
			}
		}
	}
	if warnings == 0 {
		t.Error("the incomplete listing has no warnings")
	}
}

var imgSize = regexp.MustCompile(`<img src="([^"]*)" style="height:(\d+)px;width:(\d+)px"`)

func heroSizes(t *testing.T, doc []byte) (names []string, heights, widths []float64) {
	t.Helper()
	for _, m := range imgSize.FindAllSubmatch(doc, -1) {
		h, _ := strconv.ParseFloat(string(m[2]), 64)
		w, _ := strconv.ParseFloat(string(m[3]), 64)
		names, heights, widths = append(names, string(m[1])), append(heights, h), append(widths, w)
	}
	return
}

func TestHeroDocFitsCoversSideBySide(t *testing.T) {
	covers := []Cover{{"book.png", 0.7}, {"deck.png", 16.0 / 9}, {"sheet.png", 1 / 1.414}}
	doc := HeroDoc(covers, "#7A2E2E")
	names, heights, widths := heroSizes(t, doc)
	if len(names) != 3 || names[0] != "book.png" || names[2] != "sheet.png" {
		t.Fatalf("images %v in\n%s", names, doc)
	}
	total := 48.0 * 2
	for i, c := range covers {
		if heights[i] != heights[0] {
			t.Errorf("cover %d is %vpx high, cover 0 %vpx", i, heights[i], heights[0])
		}
		if want := heights[0] * c.Ratio; widths[i] < want-1 || widths[i] > want+1 {
			t.Errorf("cover %d is %vpx wide, want %v", i, widths[i], want)
		}
		total += widths[i]
	}
	if total > 1402 || total < 1390 {
		t.Errorf("the covers take %vpx, want 1400", total)
	}
	if !bytes.Contains(doc, []byte("color-mix(in srgb,#7A2E2E 10%,#f4f2ee)")) || !bytes.Contains(doc, []byte("gap:48px")) {
		t.Errorf("accent or gap missing:\n%s", doc)
	}
}

func TestHeroDocCapsTheHeight(t *testing.T) {
	_, heights, widths := heroSizes(t, HeroDoc([]Cover{{"a.png", 0.5}, {"b.png", 0.5}}, ""))
	if heights[0] != 860 || widths[0] != 430 {
		t.Fatalf("heights %v widths %v", heights, widths)
	}
}

func TestHeroDocDefaultsAndEscapes(t *testing.T) {
	doc := HeroDoc([]Cover{{`a"<b>.png`, 1}}, "")
	if !bytes.Contains(doc, []byte("#444444")) {
		t.Error("no default accent")
	}
	if bytes.Contains(doc, []byte(`a"<b>`)) || !bytes.Contains(doc, []byte(`a&#34;&lt;b&gt;.png`)) {
		t.Errorf("name not escaped:\n%s", doc)
	}
	if doc := HeroDoc(nil, ""); bytes.Contains(doc, []byte("<img")) || !bytes.Contains(doc, []byte("</body></html>")) {
		t.Errorf("no covers:\n%s", doc)
	}
}

func TestHoldsOnlyItsUnits(t *testing.T) {
	r := Report{Summary: Summary{Units: []string{"book", "deck"}}}
	for kind, want := range map[string]bool{"book": true, "deck": true, "document": false, "": false} {
		if r.Holds(kind) != want {
			t.Errorf("Holds(%q) = %v", kind, !want)
		}
	}
}

func TestDownloadPageListsEveryFile(t *testing.T) {
	r := Report{Config: Config{Name: "Fire Safety Kit", Language: "de"}}
	page := DownloadPage(r, "#7A2E2E", []deliver.File{{Name: "deck.pptx", Label: "Folien", Size: 1200}, {Name: "handout.pdf", Label: "Handout", Size: 300}})
	for _, want := range []string{"Fire Safety Kit", "deck.pptx", "handout.pdf", `lang="de"`} {
		if !bytes.Contains(page, []byte(want)) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func TestDistributionCarriesFilesAndImages(t *testing.T) {
	r := Report{Config: Config{Name: "Fire Safety Kit"}}
	files := []deliver.File{{Name: "deck.pptx"}, {Name: "handout.pdf"}}
	data := map[string][]byte{"deck.pptx": []byte("pptx"), "handout.pdf": []byte("%PDF"), "unused": []byte("x")}
	images := []Image{{Name: "01-cover.jpg", Data: []byte("jpg")}}
	out, _, issues := Distribution(r, files, data, nil, images, []string{"a deck"}, false, "2026-10-19")
	if string(out["deck.pptx"]) != "pptx" || string(out["images/01-cover.jpg"]) != "jpg" {
		t.Fatalf("out %v", keys(out))
	}
	if _, ok := out["unused"]; ok {
		t.Error("a file that is not the bundle's is in the package")
	}
	for _, d := range issues {
		if d.Rule != RuleChannels || d.Severity != diagnostic.Warning {
			t.Errorf("issue %+v", d)
		}
	}
}

func TestRulesAreDescribed(t *testing.T) {
	ids := map[string]bool{}
	for _, r := range Rules() {
		if r.ID == "" || r.Summary == "" || r.Help == "" || ids[r.ID] {
			t.Errorf("rule %+v", r)
		}
		ids[r.ID] = true
	}
	for _, id := range []string{RuleConfig, RuleDelivery, RuleListing, RuleChannels} {
		if !ids[id] {
			t.Errorf("rule %s is not described", id)
		}
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
