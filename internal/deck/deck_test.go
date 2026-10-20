package deck

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/ooxml"
	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/zipx"
)

// fonts are the fixture fonts the deck tests share with the document tests.
const fonts = "../document/testdata/club/fonts"

// fixture copies a testdata set and the shared fonts into a project's
// deliverables/deck.
func fixture(t testing.TB, name string, edit map[string]string) project.Project {
	t.Helper()
	root := t.TempDir()
	for src, under := range map[string]string{filepath.Join("testdata", name): "", fonts: "fonts"} {
		filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(src, path)
			dst := filepath.Join(root, filepath.FromSlash(Dir), under, rel)
			os.MkdirAll(filepath.Dir(dst), 0o755)
			data, _ := os.ReadFile(path)
			return os.WriteFile(dst, data, 0o644)
		})
	}
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

func briefing(t testing.TB) Report {
	return Check(fixture(t, "briefing", nil), testConfig, true, nil, "")
}

func TestBriefingChecksCleanAndBuildsTheSameBytes(t *testing.T) {
	t.Parallel()
	r := briefing(t)
	if len(r.Diagnostics) > 0 || !r.Buildable {
		t.Fatalf("findings:\n%s", messages(r.Diagnostics))
	}
	if r.Summary.Decks != 1 || r.Summary.Slides != 11 || r.Summary.Quizzes != 2 || r.Summary.Minutes != 15 || len(r.Editions) != 2 {
		t.Fatalf("summary %+v, editions %+v", r.Summary, r.Editions)
	}
	if e := r.Editions[1]; e.Key != "en" || strings.Join(e.Papers, " ") != "letter a4" {
		t.Fatalf("en edition %+v", e)
	}
	built, err := r.Build(func(string) []byte { return nil })
	if err != nil || len(built) != 2 {
		t.Fatal(err, len(built))
	}
	for _, b := range built {
		z, err := zip.NewReader(bytes.NewReader(b.Data), int64(len(b.Data)))
		if err != nil {
			t.Fatal(err)
		}
		parts := map[string]bool{}
		for _, f := range z.File {
			parts[f.Name] = true
		}
		for _, want := range []string{"ppt/presentation.xml", "ppt/slides/slide11.xml", "ppt/notesSlides/notesSlide11.xml", "ppt/slideLayouts/slideLayout6.xml", "ppt/fonts/font1.fntdata", "ppt/media/image1.png"} {
			if !parts[want] {
				t.Errorf("%s (%s) lacks %s", b.Name, b.Edition.Key, want)
			}
		}
		if zipx.Stamp(b.Data) != b.Key {
			t.Errorf("%s carries no stamp", b.Name)
		}
	}
	again, _ := r.Build(func(string) []byte { return nil })
	for i, b := range built {
		if !bytes.Equal(b.Data, again[i].Data) {
			t.Errorf("%s %s is not byte-identical on a rebuild", b.Name, b.Edition.Key)
		}
	}
}

func TestAnUnreadableDeckIsAnError(t *testing.T) {
	t.Parallel()
	p := fixture(t, "briefing", nil)
	outside := filepath.Join(t.TempDir(), "briefing.md")
	if err := os.WriteFile(outside, []byte("# Briefing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(p.Root, filepath.FromSlash(Dir), "en", "briefing.md")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, file); err != nil {
		t.Fatal(err)
	}
	r := Check(p, testConfig, true, nil, "")
	if !strings.Contains(messages(r.Diagnostics), "en/briefing.md: "+Dir+"/en/briefing.md: symlink leaves the project") {
		t.Fatalf("an unreadable deck was left out silently:\n%s", messages(r.Diagnostics))
	}
}

func TestSlidesTakeTheirLayoutFromTheContent(t *testing.T) {
	t.Parallel()
	r := briefing(t)
	d := r.Decks["en"][0]
	var got []string
	for _, s := range d.Slides {
		got = append(got, s.Layout)
	}
	if want := "cover content section two content picture content statement content content content"; strings.Join(got, " ") != want {
		t.Fatalf("layouts %v", got)
	}
	if q := d.Slides[8].Quiz; q == nil || answer(q) != "B" {
		t.Fatalf("quiz %+v", q)
	}
	if q := d.Slides[9].Quiz; q == nil || answer(q) != "C" {
		t.Fatalf("quiz %+v", q)
	}
}

func TestSlideChecksCatchWhatDoesNotFit(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Every member of staff checks the fryer thermostat before the first batch of the day. ", 14)
	src := "---\ntitle: Briefing\nminutes: 20\nheader: Shop\n---\n# Cover\n\n---\n## A slide that says too much\n\n" + long + "\n\n---\n\n---\nNo title here\n\n- [ ] one\n- [x] two\n---\n## Jeopardy night\n\n::: columns\nleft\n+++\nmiddle\n+++\nright\n:::\n---\n## Signed\n\n::: form\nName: Lena\n:::\n"
	r := Check(fixture(t, "briefing", map[string]string{Dir + "/en/briefing.md": src}), testConfig, true, nil, "")
	got := messages(r.Diagnostics)
	for _, want := range []string{
		`Front matter key "header" is unknown`,
		"Slide 2: the body needs",
		"Slide 2 shows",
		"Slide 3 is empty",
		"Slide 4 has no title",
		`"Jeopardy" is a trademark`,
		"Slide 5 has 3 columns",
		"Directive ::: form is unknown here",
		"Slide 2 has no speaker notes",
		"minutes of speech for a 20 minute deck",
		"en/briefing has 6 slides; de/briefing has 11",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no finding %q in:\n%s", want, got)
		}
	}
}

func TestQuizSheetAndGuide(t *testing.T) {
	t.Parallel()
	r := briefing(t)
	built, _ := r.Build(func(string) []byte { return nil })
	var de Built
	for _, b := range built {
		if b.Edition.Lang == "de" {
			de = b
		}
	}
	var kinds []string
	for _, p := range de.Prints() {
		kinds = append(kinds, p.Kind+"/"+p.Paper)
	}
	if strings.Join(kinds, " ") != "slides/ notes/a4 quiz/a4" {
		t.Fatalf("prints %v", kinds)
	}
	quiz := string(r.PrintDoc(de, Print{Kind: "quiz", Paper: "a4"}))
	for _, want := range []string{"Öl brennt in der Fritteuse", "Hitze abschalten", "Lösung", "<td>1</td><td>B</td>", "<td>2</td><td>C</td>"} {
		if !strings.Contains(quiz, want) {
			t.Errorf("quiz sheet lacks %q", want)
		}
	}
	guide := string(r.Guide(r.Editions[0]))
	for _, want := range []string{"Brandschutzunterweisung.pptx · 11 Folien, 15 Minuten, Sprechernotizen, 2 Quizfragen", "PowerPoint (Windows und Mac)", "Keynote", "Google Präsentationen", "Folienmaster", "Referentenansicht"} {
		if !strings.Contains(guide, want) {
			t.Errorf("guide lacks %q", want)
		}
	}
	if f := strings.Join(r.Facts(), "\n"); !strings.Contains(f, "Fire Safety Briefing (11 slides, 15 minutes, speaker notes, 2 quiz questions)") {
		t.Errorf("facts:\n%s", f)
	}
	if got := CopyLink("https://docs.google.com/presentation/d/1AbCdEfGhIjKlMnOpQrStUv/edit"); got != "https://docs.google.com/presentation/d/1AbCdEfGhIjKlMnOpQrStUv/copy" {
		t.Errorf("copy link %q", got)
	}
	if CopyLink("https://docs.google.com/document/d/1AbCdEfGhIjKlMnOpQrStUv/edit") != "" {
		t.Error("a Docs link passes as a Slides master")
	}
}

// TestStoredEntriesCarryTheirSizes keeps every stored zip entry free of a
// trailing data descriptor, which LibreOffice refuses in a .pptx.
func TestStoredEntriesCarryTheirSizes(t *testing.T) {
	t.Parallel()
	r := briefing(t)
	built, _ := r.Build(func(string) []byte { return nil })
	archives := map[string][]byte{}
	for _, b := range built {
		archives[b.Name+" "+b.Edition.Key] = b.Data
	}
	z, err := r.Zip(r.Editions[0], built, map[string][]byte{}, []byte("%PDF"), "")
	if err != nil {
		t.Fatal(err)
	}
	archives["edition zip"] = z
	for name, data := range archives {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(name, err)
		}
		for _, f := range zr.File {
			if f.Method == zip.Store && f.Flags&0x8 != 0 {
				t.Errorf("%s: %s is stored with a data descriptor", name, f.Name)
			}
		}
	}
}

// TestChromePrintsDecksAsPlanned prints every deck's slides, notes and quiz
// sheet with the Chrome a release needs: one 16:9 page a slide in the
// theme's fonts, nothing cut off, the notes and quiz on their paper.
func TestChromePrintsDecksAsPlanned(t *testing.T) {
	printer, err := release.NewPrinter()
	if err != nil || testing.Short() {
		t.Skip(err)
	}
	defer printer.Close()
	r := briefing(t)
	built, _ := r.Build(func(string) []byte { return nil })
	for _, b := range built {
		for _, p := range b.Prints() {
			pdf, err := printer.Print(r.PrintDoc(b, p), r.GuideAssets(), false)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range r.Inspect(b, p, pdf) {
				t.Errorf("%s: %s", f.File, f.Message)
			}
			if p.Kind == "quiz" && pdfx.PageCount(pdf) != 2 {
				t.Errorf("%s: the quiz sheet prints %d pages", b.Edition.Key, pdfx.PageCount(pdf))
			}
			if dir := os.Getenv("LFM_KEEP"); dir != "" {
				os.WriteFile(dir+"/"+b.Edition.Key+"-"+p.Kind+p.Paper+".pdf", pdf, 0o644)
			}
		}
		w, h := ShotPixels()
		if over, err := printer.Overflows(r.PrintDoc(b, Print{Kind: "slides"}), r.GuideAssets(), w, h); err != nil || len(over) > 0 {
			t.Errorf("%s: slides %v overflow (%v)", b.Edition.Key, over, err)
		}
	}
	long := strings.Repeat("Every member of staff checks the fryer thermostat before the first batch. ", 20)
	r = Check(fixture(t, "briefing", map[string]string{Dir + "/en/briefing.md": "---\ntitle: B\n---\n# Cover\n\n---\n## Too much\n\n" + long + "\n"}), testConfig, true, nil, "")
	built, _ = r.Build(func(string) []byte { return nil })
	for _, b := range built {
		if b.Edition.Lang == "en" {
			w, h := ShotPixels()
			if over, _ := printer.Overflows(r.PrintDoc(b, Print{Kind: "slides"}), r.GuideAssets(), w, h); len(over) != 1 || over[0] != "2" {
				t.Errorf("overflow not caught: %v", over)
			}
		}
	}
}

// TestPowerPointReadsEveryDeck holds every built deck to Microsoft's Open
// XML SDK, which reads them as PowerPoint does. LFM_OOXML_VALIDATOR runs it.
func TestPowerPointReadsEveryDeck(t *testing.T) {
	t.Parallel()
	dll := os.Getenv("LFM_OOXML_VALIDATOR")
	if dll == "" {
		t.Skip("LFM_OOXML_VALIDATOR is not set")
	}
	r := briefing(t)
	built, _ := r.Build(func(string) []byte { return nil })
	files := map[string][]byte{}
	for _, b := range built {
		files[b.Edition.Key+"-"+b.Deck.Key+".pptx"] = b.Data
	}
	if out, err := ooxml.Validate(dll, files); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// TestLibreOfficeOpensEveryDeck converts every deck in LibreOffice, when
// installed, and holds its print to the plan.
func TestLibreOfficeOpensEveryDeck(t *testing.T) {
	t.Parallel()
	soffice, err := office.Find()
	if err != nil || testing.Short() {
		t.Skip("no LibreOffice")
	}
	r := briefing(t)
	built, _ := r.Build(func(string) []byte { return nil })
	found, err := r.Verify(soffice, built)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		t.Error(f.File, f.Message)
	}
}

// TestPowerPointOpensEveryDeck opens every built deck in Microsoft
// PowerPoint for Mac: no repair, every slide, the theme's fonts.
// LFM_POWERPOINT=1 runs it; LFM_KEEP names a folder for its PDFs.
func TestPowerPointOpensEveryDeck(t *testing.T) {
	t.Parallel()
	if os.Getenv("LFM_POWERPOINT") == "" || !office.HasPowerPoint() {
		t.Skip("set LFM_POWERPOINT=1 with PowerPoint for Mac installed")
	}
	r := briefing(t)
	built, _ := r.Build(func(string) []byte { return nil })
	files := map[string][]byte{}
	for _, b := range built {
		files[b.Key] = b.Data
	}
	opened, err := office.OpenInPowerPoint(files)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range built {
		o := opened[b.Key]
		if o.Error != "" {
			t.Errorf("%s: %s", b.Edition.Key, o.Error)
			continue
		}
		for _, f := range r.inspect(b, Print{Kind: "slides"}, o.Data, "PowerPoint") {
			t.Error(f.File, f.Message)
		}
		if dir := os.Getenv("LFM_KEEP"); dir != "" {
			os.WriteFile(dir+"/"+b.Edition.Key+".pptx", b.Data, 0o644)
			os.WriteFile(dir+"/"+b.Edition.Key+"-powerpoint.pdf", o.Data, 0o644)
		}
	}
}

// BenchmarkCheck measures a compile's work on the decks: parse, measure
// every slide's lines, check.
func BenchmarkCheck(b *testing.B) {
	p := fixture(b, "briefing", nil)
	for b.Loop() {
		Check(p, testConfig, true, nil, "")
	}
}

// TestPrintsAreTheSameHTMLEveryTime builds every print of two separate
// checks: prints are keyed by their HTML, so unordered output would print
// every compile anew.
func TestPrintsAreTheSameHTMLEveryTime(t *testing.T) {
	t.Parallel()
	p := fixture(t, "briefing", nil)
	for range 5 {
		a, b := Check(p, testConfig, true, nil, ""), Check(p, testConfig, true, nil, "")
		ja, jb := a.Jobs(), b.Jobs()
		for i := range ja {
			for _, pp := range ja[i].Prints() {
				if !bytes.Equal(a.PrintDoc(ja[i], pp), b.PrintDoc(jb[i], pp)) {
					t.Fatalf("%s %s prints differently", ja[i].Name, pp.Kind)
				}
			}
		}
		for _, e := range a.Editions {
			if !bytes.Equal(a.Guide(e), b.Guide(e)) || !bytes.Equal(a.Preview(e), b.Preview(e)) {
				t.Fatalf("%s: the guide or preview differs", e.Key)
			}
		}
	}
}
