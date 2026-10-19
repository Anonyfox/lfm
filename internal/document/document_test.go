package document

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	pngenc "image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/markup"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/ooxml"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
)

// fixture copies a testdata set into a project's deliverables/document.
func fixture(t testing.TB, name string, edit map[string]string) project.Project {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("testdata", name)
	filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dst := filepath.Join(root, filepath.FromSlash(Dir), rel)
		os.MkdirAll(filepath.Dir(dst), 0o755)
		data, _ := os.ReadFile(path)
		return os.WriteFile(dst, data, 0o644)
	})
	for rel, content := range edit {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644)
	}
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var testConfig = Config{Path: "k3n8q2w7x5r9t4m6p1", Language: "en", Seller: "Riverside Press"}

func messages(ds []diagnostic.Diagnostic) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Rule+" "+d.Location.File+": "+d.Message)
	}
	return strings.Join(out, "\n")
}

func TestClubChecksCleanAndBuildsTheSameBytes(t *testing.T) {
	r := Check(fixture(t, "club", nil), testConfig, true, nil, "https://example.org/")
	if len(r.Diagnostics) > 0 || !r.Buildable {
		t.Fatalf("findings:\n%s", messages(r.Diagnostics))
	}
	if r.Summary.Documents != 3 || len(r.Editions) != 2 || r.Summary.Fields < 10 {
		t.Fatalf("summary %+v", r.Summary)
	}
	first, err := r.Build(func(string) []byte { return nil })
	if err != nil || len(first) != 6 {
		t.Fatal(err, len(first))
	}
	again, _ := r.Build(func(string) []byte { return nil })
	kept := map[string][]byte{}
	for i, b := range first {
		if !bytes.Equal(b.Data, again[i].Data) {
			t.Errorf("%s %s is not byte-identical on a rebuild", b.Name, b.Edition.Key)
		}
		if Stamp(b.Data) != b.Key || Stamp(b.Data[len(b.Data)-200:]) != b.Key {
			t.Errorf("%s carries no stamp", b.Name)
		}
		kept[b.Key] = b.Data
	}
	// Word refuses an entry that needs version 0.0 to extract, or has no date.
	for _, b := range first[:1] {
		z, _ := zip.NewReader(bytes.NewReader(b.Data), int64(len(b.Data)))
		for _, f := range z.File {
			if f.ReaderVersion < 20 || f.CreatorVersion&0xff < 20 || f.ModifiedDate == 0 {
				t.Errorf("%s: entry %s has versions %d/%d and date %d", b.Name, f.Name, f.ReaderVersion, f.CreatorVersion, f.ModifiedDate)
			}
		}
	}
	reused, _ := r.Build(func(key string) []byte { return kept[key] })
	for i, b := range reused {
		if &b.Data[0] != &first[i].Data[0] {
			t.Errorf("%s was written again", b.Name)
		}
	}
	zip, err := r.Zip(r.Editions[0], first, nil, nil, "")
	if err != nil || !bytes.Contains(zip, []byte("Plot Fee Invoice.docx")) || !bytes.Contains(zip, []byte("Fonts/OFL.txt")) {
		t.Fatalf("zip: %v", err)
	}
}

func TestDocumentsPrintAsPlannedInLibreOffice(t *testing.T) {
	soffice, err := office.Find()
	if err != nil || testing.Short() {
		t.Skip(err)
	}
	r := Check(fixture(t, "club", nil), testConfig, true, nil, "")
	built, _ := r.Build(func(string) []byte { return nil })
	findings, err := r.Verify(soffice, built)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		t.Errorf("%s: %s", f.File, f.Message)
	}
}

// TestChromePrintsAsPlanned prints every document's print layout with the
// Chrome a release needs anyway: the planned pages, paper and fonts,
// without any office suite installed.
func TestChromePrintsAsPlanned(t *testing.T) {
	printer, err := release.NewPrinter()
	if err != nil || testing.Short() {
		t.Skip(err)
	}
	defer printer.Close()
	for _, name := range []string{"club", "features"} {
		var r Report
		if name == "club" {
			r = Check(fixture(t, name, nil), testConfig, true, nil, "")
		} else {
			r = Check(features(t, "warm"), testConfig, true, nil, "")
		}
		built, _ := r.Build(func(string) []byte { return nil })
		for _, b := range built {
			pdf, err := printer.Print(r.PrintDoc(b), r.GuideAssets(), false)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range r.Inspect(b, pdf) {
				t.Errorf("%s: %s", f.File, f.Message)
			}
			w, h := PagePixels(b.Edition)
			if png, err := printer.Picture(r.ShotDoc(b), r.GuideAssets(), w, h, 1); err != nil || len(png) < 1000 {
				t.Errorf("%s: no first-page shot: %v", b.Name, err)
			}
		}
	}
}

func TestSourcesAreHeldToTheFormat(t *testing.T) {
	bad := "---\ntitle: Broken\nfoo: bar\n---\n#### Too deep\n\n::: wobble\nx\n:::\n\n::: center\nNot ceremonial\n:::\n\n<div>raw</div>\n\n| A | Amount |\n| :-- | :-- |\n| x | 12.50 |\n\nLorem ipsum and \"a stray quote. 🎉\n"
	r := Check(fixture(t, "club", map[string]string{Dir + "/en/minutes.md": bad}), testConfig, true, nil, "")
	got := messages(r.Diagnostics)
	for _, want := range []string{"Front matter key \"foo\"", "Heading level 4", "Directive ::: wobble", "Centered text is for ceremonial", "Raw HTML", "holds numbers but is aligned left", "reads as filler", "emoji", "no fields"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if r.Buildable {
		t.Error("a document with source errors must not build")
	}
	missing := Check(fixture(t, "club", map[string]string{Dir + "/de/minutes.md": "---\ntitle: Protokoll\n---\nText ==x==\n"}), testConfig, true, nil, "")
	if !strings.Contains(messages(missing.Diagnostics), "de has no invoice.md") {
		t.Errorf("a language without every document must fail:\n%s", messages(missing.Diagnostics))
	}
}

func TestThemesReadWell(t *testing.T) {
	fam := &family{Name: "X", Faces: map[string]*Face{"Regular": {AvgChar: 0.48, Natural: 1.2}}}
	for _, name := range ThemeNames() {
		for _, paper := range []string{"a4", "letter"} {
			if probs := themeProblems(layoutTheme(Themes[name], Papers[paper], fam, fam)); len(probs) > 0 {
				t.Errorf("%s on %s: %v", name, paper, probs)
			}
		}
	}
}

func TestTiesKeepWhatBelongsTogether(t *testing.T) {
	for in, want := range map[string]string{
		"Kleingartenverein Am Mühlbach e. V.": "Kleingartenverein Am Mühlbach e.\u00a0V.",
		"zahlbar bis 1. Dezember, 75 € netto": "zahlbar bis 1.\u00a0Dezember, 75\u00a0€ netto",
		"nach § 14 UStG, z. B. Nr. 5":         "nach §\u00a014 UStG, z.\u00a0B. Nr.\u00a05",
		"12 kg of compost at 7:30 pm":         "12\u00a0kg of compost at 7:30\u00a0pm",
		"Plot 31 goes to Daniel":              "Plot 31 goes to Daniel",
	} {
		if got := markup.Tie(in); got != want {
			t.Errorf("markup.Tie(%q) = %q, want %q", in, got, want)
		}
	}
}

// BenchmarkCheck is what every compile of a document set pays to read the
// fonts and every document and hold them to the rules.
func BenchmarkCheck(b *testing.B) {
	p := fixture(b, "club", nil)
	for b.Loop() {
		Check(p, testConfig, true, nil, "")
	}
}

// TestWordReadsEveryDocument holds every built file to Microsoft's Open XML
// SDK, which reads them as Word does; Word refuses what LibreOffice and
// Pages forgive. Set LFM_OOXML_VALIDATOR to the built validator to run it.
func TestWordReadsEveryDocument(t *testing.T) {
	dll := os.Getenv("LFM_OOXML_VALIDATOR")
	if dll == "" {
		t.Skip("LFM_OOXML_VALIDATOR is not set")
	}
	r := Check(fixture(t, "club", nil), testConfig, true, nil, "")
	built, _ := r.Build(func(string) []byte { return nil })
	files := map[string][]byte{}
	for _, b := range built {
		files[b.Edition.Key+"-"+b.Doc.Key+".docx"] = b.Data
	}
	if out, err := ooxml.Validate(dll, files); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// TestWordOpensEveryDocument opens every built file in Microsoft Word for
// Mac: no repair, the planned pages, the theme's fonts. LFM_WORD=1 runs it.
func TestWordOpensEveryDocument(t *testing.T) {
	if os.Getenv("LFM_WORD") == "" || !office.HasWord() {
		t.Skip("set LFM_WORD=1 with Word for Mac installed")
	}
	r := Check(fixture(t, "club", nil), testConfig, true, nil, "")
	built, _ := r.Build(func(string) []byte { return nil })
	files := map[string][]byte{}
	byName := map[string]Built{}
	for _, b := range built {
		name := b.Edition.Key + "-" + b.Doc.Key + ".docx"
		files[name], byName[name] = b.Data, b
	}
	opened, err := office.OpenInWord(files)
	if err != nil {
		t.Fatal(err)
	}
	for name, w := range opened {
		b := byName[name]
		switch {
		case w.Error != "":
			t.Errorf("%s: %s", name, w.Error)
		case b.Doc.Pages > 0 && w.Result != strconv.Itoa(b.Doc.Pages):
			t.Errorf("%s: Word counts %s pages, the plan %d", name, w.Result, b.Doc.Pages)
		}
		for _, m := range baseFont.FindAllSubmatch(w.Data, -1) {
			// Word sets the gap after a list marker in Arial; it draws nothing.
			if f := string(m[1]); !strings.HasPrefix(f, "Lora") && f != "ArialMT" {
				t.Errorf("%s: Word prints in %s", name, f)
			}
		}
	}
}

// features is the feature fixture with the club's fonts, a test image and
// the given theme.
func features(t testing.TB, theme string) project.Project {
	t.Helper()
	edit := map[string]string{}
	fonts, _ := filepath.Glob("testdata/club/fonts/*")
	for _, f := range fonts {
		data, _ := os.ReadFile(f)
		edit[FontsDir+"/"+filepath.Base(f)] = string(data)
	}
	img := image.NewRGBA(image.Rect(0, 0, 900, 300))
	for y := range 300 {
		for x := range 900 {
			img.Set(x, y, color.RGBA{uint8(x / 4), uint8(y), 160, 255})
		}
	}
	var png bytes.Buffer
	pngenc.Encode(&png, img)
	edit[ImagesDir+"/pattern.png"] = png.String()
	concept, _ := os.ReadFile("testdata/features/concept.md")
	edit[ConceptFile] = strings.Replace(string(concept), "Theme: editorial", "Theme: "+theme, 1)
	return fixture(t, "features", edit)
}

// TestEveryFeatureInEveryTheme builds the feature fixture in every theme
// and holds the files to LibreOffice and, when set up, to Microsoft's
// validator (LFM_OOXML_VALIDATOR) and Word itself (LFM_WORD=1).
func TestEveryFeatureInEveryTheme(t *testing.T) {
	files := map[string][]byte{}
	var all []Built
	var last Report
	for _, theme := range ThemeNames() {
		r := Check(features(t, theme), testConfig, true, nil, "")
		for _, d := range r.Diagnostics {
			if d.Severity == diagnostic.Error {
				t.Fatalf("%s: %s %s", theme, d.Rule, d.Message)
			}
		}
		built, err := r.Build(func(string) []byte { return nil })
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range built {
			files[theme+"-"+b.Edition.Key+"-"+b.Doc.Key+".docx"] = b.Data
		}
		all, last = append(all, built...), r
	}
	if dll := os.Getenv("LFM_OOXML_VALIDATOR"); dll != "" {
		if out, err := ooxml.Validate(dll, files); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	if soffice, err := office.Find(); err == nil && !testing.Short() {
		findings, err := last.Verify(soffice, all)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range findings {
			t.Errorf("LibreOffice: %s", f.Message)
		}
	}
	if os.Getenv("LFM_WORD") != "" && office.HasWord() {
		opened, err := office.OpenInWord(files)
		if err != nil {
			t.Fatal(err)
		}
		for name, w := range opened {
			if w.Error != "" {
				t.Errorf("Word, %s: %s", name, w.Error)
			}
			for _, m := range baseFont.FindAllSubmatch(w.Data, -1) {
				if f := string(m[1]); !strings.HasPrefix(f, "Lora") && f != "ArialMT" {
					t.Errorf("Word, %s: prints in %s", name, f)
				}
			}
		}
	}
}

// TestPrintsAreTheSameHTMLEveryTime builds the print layouts of two
// separate checks of one set: prints are keyed by their HTML, so any
// unordered output (a map range) would print every compile anew.
func TestPrintsAreTheSameHTMLEveryTime(t *testing.T) {
	p := fixture(t, "club", nil)
	for range 5 {
		a, b := Check(p, testConfig, true, nil, ""), Check(p, testConfig, true, nil, "")
		ja, jb := a.Jobs(), b.Jobs()
		for i := range ja {
			if !bytes.Equal(a.PrintDoc(ja[i]), b.PrintDoc(jb[i])) || !bytes.Equal(a.ShotDoc(ja[i]), b.ShotDoc(jb[i])) {
				t.Fatalf("%s prints differently", ja[i].Name)
			}
		}
		for _, e := range a.Editions {
			if !bytes.Equal(a.Guide(e), b.Guide(e)) || !bytes.Equal(a.Preview(e), b.Preview(e)) {
				t.Fatalf("%s: the guide or preview differs", e.Key)
			}
		}
	}
}
