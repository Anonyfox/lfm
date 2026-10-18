package book

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/research"
)

const testConcept = `# Book: Sourdough rescue

## Reader
Home bakers whose starter died twice and who bake on weekends. V001 V002 V003
Not for: professional bakers running a bakery.

## Promise
After three weekends the reader keeps a starter alive and bakes a loaf with an open crumb. V004 V005
Won't cover: enriched doughs, gluten-free baking.
Before: throws out a third dead starter and buys bread. V004
After: feeds on two fixed slots a week and bakes one open loaf every weekend. V005

## Big idea
Feed the schedule, not the jar: every book times the starter to the baker's week instead of the baker to the starter. P001 P002 P003

## Objections
- "I work all week, I can't feed it twice a day." V002 → 2
- "Mine died even though I followed the recipe." V001 → 1
- "My loaves always come out dense." V003 → 3

## Hooks
- Title: "The Weekend Starter"
- Subtitle: "Keep sourdough alive on a working week"
- Opening: the jar that smelled like nail polish on a Sunday morning. V006

## Voice
- Use: "smells like nail polish" V006
- Avoid: "voyage", "game-changer"
- First person, plain, one joke per chapter at most. V007

## Devices
- A fridge calendar at the end of every chapter. V008

## Chapters
| # | Title | Job | Beats | Words | File |
| --- | --- | --- | --- | --- | --- |
| 1 | Why starters die | Diagnose why the last starter died from three signs | hook: the Sunday jar | 400 | [01](chapters/01-why.md) |
| 2 | A week that feeds itself | Plan feedings around a working week with two fixed slots | calendar | 400 | [02](chapters/02-week.md) |
| 3 | The first loaf | Bake one loaf and judge the crumb against four photos | crumb | 400 | [03](chapters/03-loaf.md) |

## Imagery
| Image | Chapter | Job | Art direction | File |
| --- | --- | --- | --- | --- |
| Cover | — | cover | the jar on a fridge shelf | [cover](images/cover.png) |
| Crumb | 3 | proof | cut loaf, side light | [crumb](images/crumb.png) |

## Look
Trim: 6x9in
Body: Literata, Georgia, serif
Headings: Literata, Georgia, serif
Accent: #8a3b12
`

const testListing = `# Listing: The Weekend Starter

## Description
Your starter died again, and the jar smelled like nail polish on a Sunday morning. This book fixes the schedule instead of the jar.

**Inside:**

- three signs that tell you why the last starter died
- a feeding plan with two fixed slots on a working week
- one loaf, judged against four photos of the crumb

For home bakers who bake on weekends and work in between.

## Short description
Keep a sourdough starter alive on a working week and bake your first open-crumb loaf in three weekends.

## Back cover
The jar smelled like nail polish. Again.

This book times the starter to your week: two fixed feedings, a fridge calendar, and one loaf you can judge by four photos.

## Author
Ada Baker has kept the same starter alive since 2019 next to a full-time job.

## Keywords
- sourdough for beginners
- sourdough starter schedule
- bread baking at home

## Tags
- sourdough
- starter guide
- bread ebook

## Categories
- BISAC: CKB009000 Cooking / Methods / Baking
- Thema: WBVD Bread
- KDP: Cookbooks, Food & Wine › Baking › Bread

## Prices
- Ebook: 7.99 EUR, 8.99 USD
- Paperback: 14.99 EUR

## AI
Text: assisted
Images: generated
Tools: Claude, Imagen
Statement: Edited with Claude; the cover was made with Imagen.
`

// pools give each fixture chapter its own words, so chapters never repeat
// one another.
var pools = []string{
	"the starter rose overnight while the kitchen stayed cool and the jar sat near the window where morning light warmed the glass slowly and evenly",
	"on tuesday the baker fed rye flour at seven before work then marked the rubber band and checked it again after the commute home that evening",
	"a dense loaf usually means the dough fermented too briefly so the crumb stays tight and gummy even though the crust browns nicely in the oven",
	"cold fridges slow everything down which suits a weekday schedule because a starter fed on sunday can wait until thursday without much fuss",
}

func sentences(seed int, n int) string {
	// Varied, ordinary sentences: lengths cycle so the rhythm is not uniform.
	lengths := []int{6, 14, 9, 21, 4, 12, 17, 8}
	words := strings.Fields(pools[seed%len(pools)])
	var b strings.Builder
	for i := 0; i < n; i++ {
		l := lengths[(i+seed)%len(lengths)]
		for w := 0; w < l; w++ {
			if w > 0 {
				b.WriteByte(' ')
			}
			word := words[(i*7+w*3+seed*5)%len(words)]
			if w == 0 {
				word = strings.ToUpper(word[:1]) + word[1:]
			}
			b.WriteString(word)
		}
		if i%3 == 1 {
			fmt.Fprintf(&b, " at %d °C for %d hours", 20+(i+seed)%8, 4+(i+seed)%9)
		}
		b.WriteString(". ")
		if i%5 == 4 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

var openings = []string{
	"On a Sunday in March, Ada Baker found her jar at 24 °C with a grey layer on top.",
	"Tom Reyes in Leeds feeds his starter at 7:10 every weekday and bakes on Saturdays.",
	"The first loaf Ada cut open in 2019 had a crumb as tight as sandwich bread.",
}

// humanProse is public-domain human writing (Arnold Bennett, How to Live on
// 24 Hours a Day, 1908): the fixture chapters read as a person wrote them.
func humanProse(seed, words int) string {
	raw, err := os.ReadFile(fmt.Sprintf("../aitext/testdata/human/en-gutenberg-bennett-%d.txt", seed%3+1))
	if err != nil {
		panic(err)
	}
	var out []string
	n := 0
	for _, p := range strings.Split(string(raw), "\n\n") {
		out = append(out, p)
		if n += len(strings.Fields(p)); n >= words {
			break
		}
	}
	return strings.Join(out, "\n\n")
}

func chapter(title string, seed int) string {
	return "# " + title + "\n\n" + openings[seed%len(openings)] + " " + humanProse(seed, 280) +
		"\n\n## What to watch\n\n1. Feed 50 g rye flour and 50 g water.\n2. Mark the level with a rubber band.\n3. Check again after 6 hours at 24 °C.\n\n" + sentences(seed+len(pools), 4) +
		"\n\nThe [King Arthur guide](https://www.kingarthurbaking.com/recipes/sourdough-starter-recipe) times feedings the same way.\n"
}

func writeFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func pngOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.Black)
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func fixture(t *testing.T) (project.Project, *research.Voices, *research.Competitors) {
	root := t.TempDir()
	writeFile(t, root, ConceptFile, []byte(testConcept))
	writeFile(t, root, Dir+"/chapters/01-why.md", []byte(chapter("Why starters die", 0)))
	writeFile(t, root, Dir+"/chapters/02-week.md", []byte(chapter("A week that feeds itself", 1)+"\nSee [the loaf](03-loaf.md) and the smell that smells like nail polish.\n"))
	writeFile(t, root, Dir+"/chapters/03-loaf.md", []byte(chapter("The first loaf", 2)+"\n![A cut loaf with an open crumb](../images/crumb.png)\n"))
	writeFile(t, root, ListingFile, []byte(testListing))
	writeFile(t, root, Dir+"/images/cover.png", pngOf(1600, 2560))
	writeFile(t, root, Dir+"/images/crumb.png", pngOf(800, 600))
	font, err := os.ReadFile("../../site/assets/fonts/martian-mono.woff2")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, Dir+"/fonts/literata.woff2", font)
	writeFile(t, root, Dir+"/book.css", []byte(`@font-face { font-family: "Literata"; src: url("fonts/literata.woff2") format("woff2"); }`+"\n"))
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	voices := &research.Voices{}
	for i := 1; i <= 10; i++ {
		voices.Entries = append(voices.Entries, research.Voice{ID: i, Platform: "Reddit"})
	}
	peers := &research.Competitors{}
	for i := 1; i <= 3; i++ {
		peers.Entries = append(peers.Entries, research.Peer{ID: i})
	}
	return p, voices, peers
}

var testConfig = Config{Author: "Ada Baker", Path: "k3x9m2q7w8e4r5t6y1u0", Language: "en"}

func rules(ds []diagnostic.Diagnostic) string {
	var out []string
	for _, d := range ds {
		out = append(out, fmt.Sprintf("%s %s:%d %s", d.Rule, d.Location.File, d.Location.Line, d.Message))
	}
	return strings.Join(out, "\n")
}

func TestCompleteBookChecksCleanAndBuilds(t *testing.T) {
	p, voices, peers := fixture(t)
	r := Check(p, testConfig, true, nil, "https://example.org/sourdough/", voices, peers)
	if len(r.Diagnostics) > 0 {
		t.Fatalf("unexpected findings:\n%s", rules(r.Diagnostics))
	}
	s := r.Summary
	if !s.Complete || !r.Buildable || s.Written != 3 || s.Title != "The Weekend Starter" || s.URL != "https://example.org/sourdough/k3x9m2q7w8e4r5t6y1u0/" {
		t.Fatalf("summary %+v", s)
	}
	built, err := Build(r, func(rel string) ([]byte, error) { return p.Read(Dir + "/" + rel) }, time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(built.EPUB), int64(len(built.EPUB)))
	if err != nil {
		t.Fatal(err)
	}
	if z.File[0].Name != "mimetype" || z.File[0].Method != zip.Store {
		t.Fatalf("first entry %s method %d", z.File[0].Name, z.File[0].Method)
	}
	names := map[string]string{}
	for _, f := range z.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		names[f.Name] = string(data)
		if strings.HasSuffix(f.Name, ".xhtml") || strings.HasSuffix(f.Name, ".opf") {
			if err := wellFormed(f.Name, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	opf := names["OEBPS/content.opf"]
	for _, want := range []string{`properties="nav"`, `properties="cover-image"`, `dcterms:modified">2026-10-18T12:00:00Z`, "<dc:title>The Weekend Starter</dc:title>", "<dc:creator>Ada Baker</dc:creator>", "<dc:language>en</dc:language>"} {
		if !strings.Contains(opf, want) {
			t.Errorf("content.opf lacks %s", want)
		}
	}
	for _, href := range []string{"text/ch01.xhtml", "text/ch03.xhtml", "text/nav.xhtml", "images/crumb.png", "css/book.css", "toc.ncx", "fonts/literata.woff2"} {
		if _, ok := names["OEBPS/"+href]; !ok {
			t.Errorf("missing %s", href)
		}
	}
	ch2 := names["OEBPS/text/ch02.xhtml"]
	if !strings.Contains(ch2, `href="ch03.xhtml"`) || strings.Contains(ch2, "&ldquo;") {
		t.Errorf("chapter link not rewritten or entity used: %s", ch2[:300])
	}
	if !strings.Contains(names["OEBPS/text/ch03.xhtml"], `src="../images/crumb.png"`) {
		t.Error("image path not rewritten")
	}
	if !strings.Contains(names["OEBPS/css/book.css"], `url("../fonts/literata.woff2")`) || !strings.Contains(opf, `media-type="font/woff2"`) {
		t.Error("font not embedded in the EPUB")
	}
	if !strings.Contains(opf, `<meta property="schema:accessMode">visual</meta>`) || !strings.Contains(opf, `dcterms:conformsTo`) || !strings.Contains(opf, "<dc:subject>CKB009000") {
		t.Error("accessibility or subject metadata missing")
	}
	if !strings.Contains(names["OEBPS/text/copyright.xhtml"], "made with Imagen") {
		t.Error("the AI statement is not on the copyright page")
	}
	paper := built.Document(PrintOptions{Paper: true, Inside: Gutter(40), Pages: map[int]int{1: 5, 2: 7, 3: 9}, Blanks: map[int]bool{1: true}, PadEnd: 1})
	if bytes.Contains(paper, []byte("<a ")) || !bytes.Contains(paper, []byte(`@page :left { margin-left: 0.600in; margin-right: 0.750in;`)) || bytes.Count(paper, []byte(`class="blank"`)) != 2 || !bytes.Contains(paper, []byte(`<span class="pg">7</span>`)) {
		t.Errorf("paperback interior wrong:\n%s", paper)
	}
	cover := built.CoverDocument(KDPSpine(120, "white"), false)
	if !bytes.Contains(cover, []byte("size: 12.5202in 9.2500in")) || !bytes.Contains(cover, []byte("This book times the starter")) {
		t.Errorf("cover wrap wrong:\n%s", cover[:600])
	}
	if !bytes.Contains(built.Print, []byte(`size: 6.000in 9.000in`)) || built.PrintAssets["img-images-crumb.png"] == nil || built.PrintAssets["font-fonts-literata.woff2"] == nil {
		t.Error("print document misses trim or assets")
	}
	// Building twice gives the same bytes: buyers' files only change when the book does.
	if dir := os.Getenv("LFM_EPUB_OUT"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "fixture.epub"), built.EPUB, 0o644)
	}
	again, _ := Build(r, func(rel string) ([]byte, error) { return p.Read(Dir + "/" + rel) }, time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	if !bytes.Equal(again.EPUB, built.EPUB) {
		t.Error("EPUB build is not reproducible")
	}
}

func TestDefectsAreReported(t *testing.T) {
	p, voices, peers := fixture(t)
	bad := chapter("Why starters die", 0) +
		"\nIt's important to note that this voyage will unlock your full potential. Let's dive in.\n\n#### Skipped level\n\nTODO write this.\n\n![](../images/missing.png)\n\nSee [this](http://example.com) and [that](99-nothing.md).\n"
	writeFile(t, p.Root, Dir+"/chapters/01-why.md", []byte(bad))
	writeFile(t, p.Root, Dir+"/chapters/03-loaf.md", []byte("# Wrong title\n\nShort.\n"))
	r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	got := rules(r.Diagnostics)
	for _, want := range []string{
		`book.copy deliverables/book/chapters/01-why.md`, `stock phrase`, `"voyage", which the Voice avoids`,
		`Heading level jumps from 2 to 4`, `Filler left in the chapter: "TODO"`, `missing.png does not exist`, `has no alternative text`,
		`uses plain http`, `points at no chapter`, `The title "Wrong title" differs`, `words against a target of 400`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestUniformRhythmIsReported(t *testing.T) {
	p, voices, peers := fixture(t)
	var b strings.Builder
	b.WriteString("# Why starters die\n\n")
	for i := 0; i < 45; i++ {
		b.WriteString("The jar sat warm on the shelf all night. ")
		if i%4 == 3 {
			b.WriteString("\n\n")
		}
	}
	writeFile(t, p.Root, Dir+"/chapters/01-why.md", []byte(b.String()))
	r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	if !strings.Contains(rules(r.Diagnostics), "Sentences barely vary in length") {
		t.Fatal(rules(r.Diagnostics))
	}
}

func TestConfigAndConceptGaps(t *testing.T) {
	p, voices, peers := fixture(t)
	r := Check(p, Config{}, false, nil, "https://example.org/", voices, peers)
	if r.Buildable || !strings.Contains(rules(r.Diagnostics), "lfm.json has no deliverables.book") {
		t.Fatal(rules(r.Diagnostics))
	}
	r = Check(p, Config{Author: "A", Path: "short", Offer: "nope", Formats: []string{"mobi"}}, true, map[string]*Price{"book": nil}, "https://example.org/", voices, peers)
	got := rules(r.Diagnostics)
	for _, want := range []string{"not an unguessable folder name", `names no offer`, `Unknown book format "mobi"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	gapped := strings.NewReplacer("Not for: professional bakers running a bakery.\n", "", "- Avoid: \"voyage\", \"game-changer\"\n", "", "Trim: 6x9in", "Trim: huge", "| 2 | A week", "| 7 | A week", "V008", "V099").Replace(testConcept)
	writeFile(t, p.Root, ConceptFile, []byte(gapped))
	r = Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	got = rules(r.Diagnostics)
	for _, want := range []string{`no "Not for:" line`, `Voice has no Avoid`, `no valid "Trim:"`, `Chapter number "7" should be 2`, `V099 does not exist`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestIncompleteBookIsNotBuildable(t *testing.T) {
	p, voices, peers := fixture(t)
	os.Remove(filepath.Join(p.Root, Dir, "chapters", "02-week.md"))
	r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	if r.Buildable || r.Summary.Complete || r.Summary.Next() == nil || r.Summary.Next().File != "chapters/02-week.md" {
		t.Fatalf("%+v", r.Summary)
	}
}

func TestDownloadPage(t *testing.T) {
	plan := Plan{Title: "The Weekend Starter", Subtitle: "Keep it alive", Accent: "#8a3b12"}
	for _, files := range [][]File{nil, {{Name: "the-weekend-starter.epub", Label: "EPUB", Note: "for e-readers", Size: 123456}}} {
		page := DownloadPage(plan, testConfig, files)
		if _, err := html.Parse(bytes.NewReader(page)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(page, []byte(`content="noindex, nofollow"`)) || !bytes.Contains(page, []byte(`no-referrer`)) {
			t.Fatal("download page is indexable or leaks its address")
		}
	}
	if !bytes.Contains(DownloadPage(plan, testConfig, nil), []byte("being written")) {
		t.Fatal("presale state missing")
	}
}

func TestPaths(t *testing.T) {
	for i := 0; i < 50; i++ {
		if p := NewPath(); !ValidPath(p) {
			t.Fatalf("generated path %q is invalid", p)
		}
	}
	for _, bad := range []string{"", "book", "abc", "UPPERCASEUPPERCASE12", "has space in it 1234", "-leading-hyphen-1234", "double--hyphen-12345"} {
		if ValidPath(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
	if Slug("The Weekend Starter: Keep It Alive!") != "the-weekend-starter-keep-it-alive" {
		t.Error(Slug("The Weekend Starter: Keep It Alive!"))
	}
}

func TestVerifyCatchesBrokenPackages(t *testing.T) {
	p, voices, peers := fixture(t)
	r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	built, err := Build(r, func(rel string) ([]byte, error) { return p.Read(Dir + "/" + rel) }, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyEPUB(built.EPUB); err != nil {
		t.Fatal(err)
	}
	// Rewrite one chapter so it links a file that is not packaged.
	z, _ := zip.NewReader(bytes.NewReader(built.EPUB), int64(len(built.EPUB)))
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range z.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == "OEBPS/text/ch01.xhtml" {
			data = bytes.Replace(data, []byte("</section>"), []byte(`<a href="ch09.xhtml">x</a></section>`), 1)
		}
		fw, _ := w.CreateHeader(&zip.FileHeader{Name: f.Name, Method: f.Method})
		fw.Write(data)
	}
	w.Close()
	if err := verifyEPUB(out.Bytes()); err == nil || !strings.Contains(err.Error(), "ch09.xhtml") {
		t.Fatalf("broken reference not caught: %v", err)
	}
}

func TestQuotesFollowTheLanguage(t *testing.T) {
	plan := Plan{Chapters: []Chapter{{N: 1, File: "chapters/01.md"}}}
	src := []byte("# T\n\nSie sagte \"Hallo\" und 'tschüss' -- dann ging sie...\n")
	for lang, want := range map[string]string{"de": "„Hallo“", "de-AT": "„Hallo“", "en": "“Hallo”", "fr": "« Hallo »", "xx": "“Hallo”"} {
		out, err := renderChapter(plan.Chapters[0], src, plan, "epub", lang)
		if err != nil || !strings.Contains(string(out), want) {
			t.Errorf("%s: want %s in %s (%v)", lang, want, out, err)
		}
	}
}

func TestPageCountOfAChromePDF(t *testing.T) {
	pdf, err := os.ReadFile("testdata/print.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if n := PageCount(pdf); n != 9 {
		t.Fatalf("PageCount = %d, want 9", n)
	}
}

func TestGeneratedNonfictionHabits(t *testing.T) {
	p, voices, peers := fixture(t)
	text := chapter("Why starters die", 0) + `
In diesem Kapitel werden wir die faszinierende Welt des Sauerteigs erkunden.

Studien zeigen, dass 87 % aller Hobbybäcker ihren Starter verlieren.

Laut einer Studie hilft Roggenmehl, wie [Plötzblog](https://www.ploetzblog.de) zeigt.

Why does it smell? Because it starves.

Is it dead? Mostly not.

What now? Feed it.

When? Tonight.
`
	writeFile(t, p.Root, Dir+"/chapters/01-why.md", []byte(text))
	r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	got := rules(r.Diagnostics)
	for _, want := range []string{`"In diesem Kapitel werden wir" is a stock line`, `"Studien zeigen" makes a claim without a source`, `4 paragraphs open with a question`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, `"Laut einer Studie"`) {
		t.Errorf("a linked claim was reported:\n%s", got)
	}
	if !containsPhrase("Wenn er morgens nicht mehr aufgeht, geht er auch abends nicht mehr auf.", "geht nicht mehr auf") || containsPhrase("Er geht. Nicht mehr auf.", "geht nicht mehr auf") {
		t.Error("phrase matching by sentence words")
	}
	if !ValidISBN("978-3-16-148410-0") || ValidISBN("978-3-16-148410-1") || ValidISBN("123") {
		t.Error("ISBN check digit")
	}
}

func TestOutlinePages(t *testing.T) {
	pdf, err := os.ReadFile("testdata/print.pdf")
	if err != nil {
		t.Fatal(err)
	}
	got := OutlinePages(pdf)
	// The title page heading, then the three chapters.
	if fmt.Sprint(got) != "[2 4 6 8]" {
		t.Fatalf("outline pages = %v", got)
	}
}

func TestListingFindsWhatStoresRefuse(t *testing.T) {
	var found []string
	bad := strings.NewReplacer(
		"For home bakers who bake on weekends and work in between.", "Visit https://example.org for 9.99 €. A five-star bestseller.",
		"- bread baking at home", "- the weekend starter",
		"- bread ebook", "- a tag far longer than twenty characters",
		"CKB009000", "CKB9",
		"- Ebook: 7.99 EUR, 8.99 USD", "- Ebook: 13.99 EUR",
		"Statement: Edited with Claude; the cover was made with Imagen.", "Statement: Edited with Claude.",
	).Replace(testListing)
	l := checkListing(bad, Plan{Title: "The Weekend Starter", Subtitle: "Keep sourdough alive"}, &Price{Text: "12.00 EUR", Amount: big.NewRat(12, 1), Currency: "EUR"}, "de", func(rule string, line int, message, hint string) {
		found = append(found, message)
	})
	all := strings.Join(found, "\n")
	for _, want := range []string{"https://", "mentions a price", "five-star", "repeats the title", "Etsy takes 20", "not a BISAC code", "more than 80% of the paperback", "German fixed book price", "does not name Imagen"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if l.AIImages != "generated" || len(l.Tags) != 3 {
		t.Errorf("parsed %+v", l)
	}
	if got := KDPDescription("Hello **world**\n\n### Inside\n\n- one\n- [two](https://x.org)"); got != "<p>Hello <b>world</b></p><h4>Inside</h4><ul><li>one</li><li>two</li></ul>" {
		t.Errorf("KDP description %q", got)
	}
}

func TestDistributionPackagesEveryChannel(t *testing.T) {
	p, voices, peers := fixture(t)
	r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	built, err := Build(r, func(rel string) ([]byte, error) { return p.Read(Dir + "/" + rel) }, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	printed := Printed{PDF: []byte("%PDF-screen"), Interior: []byte("%PDF-interior"), InteriorPages: 120, Inside: Gutter(120),
		Wraps: map[string][]byte{"kdp": []byte("%PDF-kdp"), "lulu": []byte("%PDF-lulu")}, Spines: map[string]float64{"kdp": KDPSpine(120, "white"), "lulu": LuluSpine(120)}}
	files, channels, issues := Distribution(r, built, printed, "2026-10-18")
	if len(channels) != len(Channels()) {
		t.Errorf("channels %v", channels)
	}
	for _, name := range []string{"README.md", "the-weekend-starter.epub", "the-weekend-starter.pdf", "cover.jpg", "paperback-interior.pdf", "paperback-cover-kdp.pdf", "paperback-cover-lulu.pdf", "stores/kdp.md", "stores/etsy.md", "stores/ingramspark.md"} {
		if files[name] == nil {
			t.Errorf("missing %s", name)
		}
	}
	if len(files) != 6+len(Channels())+1 {
		t.Errorf("%d files: every book file once, one guide per channel and the README", len(files))
	}
	kdp := string(files["stores/kdp.md"])
	for _, want := range []string{"- [`paperback-cover-kdp.pdf`](../paperback-cover-kdp.pdf)", "- [`the-weekend-starter.epub`](../the-weekend-starter.epub)", "```html\n<p>Your starter died again", "- `sourdough for beginners`", "Ebook 7.99 EUR: in the 70% royalty band", "Paperback 14.99 EUR: 60% royalty", "AI: answer Yes to AI-generated content and name what was generated (images with Claude, Imagen)"} {
		if !strings.Contains(kdp, want) {
			t.Errorf("kdp guide misses %q:\n%s", want, kdp)
		}
	}
	if apple, bn := string(files["stores/apple.md"]), string(files["stores/barnes-noble.md"]); strings.Contains(apple, "paperback") || strings.Contains(bn, "paperback-cover-") || !strings.Contains(bn, "paperback-interior.pdf") {
		t.Errorf("a guide names files its channel does not take:\n%s\n%s", apple, bn)
	}
	if etsy := string(files["stores/etsy.md"]); !strings.Contains(etsy, "The Weekend Starter: Keep sourdough alive on a working week | Ebook (EPUB + PDF)") || !strings.Contains(etsy, "made with Imagen") {
		t.Errorf("etsy guide:\n%s", etsy)
	}
	if !bytes.HasPrefix(files["cover.jpg"], []byte{0xff, 0xd8}) {
		t.Error("the store cover is not a JPEG")
	}
	if got := Wraps(r, 28); !slices.Equal(got, []string{"kdp"}) {
		t.Errorf("a 28-page paperback needs the wraps %v, want only KDP's (Lulu prints from 32)", got)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "dpi") {
		t.Errorf("issues %s", rules(issues))
	}

	r.Listing.AIText = "generated"
	files, channels, _ = Distribution(r, built, printed, "2026-10-18")
	if files["stores/draft2digital.md"] != nil || len(channels) != len(Channels())-2 || !strings.Contains(string(files["README.md"]), "IngramSpark's catalog guidelines") {
		t.Error("channels that refuse AI-generated books were prepared")
	}
}

func TestBarcodeAndSpine(t *testing.T) {
	svg := barcodeSVG("978-3-16-148410-0")
	if strings.Count(svg, `height="55"`) != 6 || !strings.Contains(svg, ">783161<") {
		t.Errorf("barcode: %s", svg)
	}
	if barcodeSVG("978-3-16-148410-1") != "" {
		t.Error("barcode for an invalid ISBN")
	}
	if s := KDPSpine(200, "cream"); s != 0.5 {
		t.Errorf("KDP cream spine %v", s)
	}
	if Gutter(100) != 0.75 || Gutter(400) != 0.875 {
		t.Error("gutter")
	}
}

func TestTransparent(t *testing.T) {
	if !Transparent([]byte("<</ca .5>>")) || !Transparent([]byte("/SMask 12 0 R")) || Transparent([]byte("<</ca 1 /CA 1>>")) {
		t.Error("transparency detection")
	}
}

func TestStampKeepsThePDFValid(t *testing.T) {
	pdf, _ := os.ReadFile("testdata/print.pdf")
	stamped, err := Stamp(pdf, "0123456789abcdef0123")
	if err != nil {
		t.Fatal(err)
	}
	if StampOf(stamped) != "0123456789abcdef0123" || StampOf(pdf) != "" || PageCount(stamped) != PageCount(pdf) || fmt.Sprint(OutlinePages(stamped)) != "[2 4 6 8]" {
		t.Fatal("stamp lost the key or changed the document")
	}
	again, _ := Stamp(stamped, "fedcba9876543210")
	if StampOf(again) != "fedcba9876543210" {
		t.Fatal("restamp")
	}
	if dir := os.Getenv("LFM_STAMP_OUT"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "stamped.pdf"), again, 0o644)
	}
}

func TestQualityChecksCatchGeneratedHabits(t *testing.T) {
	en := `# Why starters die

Most people think a starter is fragile. In this chapter, we'll look at what really happens in the jar.

It isn't about the flour. It's about time. A starter isn't just a jar of paste, it's a living culture. It's not the temperature. It's the schedule. The jar is not your enemy. It's your partner.

Experts agree that feeding twice a day is best. This plays a crucial role in the rise, and it's important to note that Sarah fed hers at random times.

- **Smell:** sour and sharp.
- **Look:** bubbles on the side.
- **Rise:** double in eight hours.

` + strings.Repeat("The jar rests on the shelf and waits for the next feeding while the kitchen cools down overnight and the culture settles. ", 20) + `

## Final Thoughts

Remember, you've got this. Every baker starts somewhere, and your journey matters.
`
	_, p := parseChapter([]byte(en))
	var got []string
	qualityChecks(measure(p, "en"), p, Plan{}, "en", "", func(rule string, line int, message, hint string) {
		got = append(got, rule+" "+message)
	})
	all := strings.Join(got, "\n")
	for _, want := range []string{"correct a claim nobody made", "announces the chapter", "known tell", "bold label", "stock name", "attributes a claim", "closes with the section", "names no source", "generalization"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}

	de := `# Lebt er noch?

Stell dir vor, dein Sauerteig riecht nach Nagellack. In diesem Kapitel zeige ich dir, was du tun kannst.

Es geht nicht um das Mehl, es geht um die Zeit. Das spielt eine entscheidende Rolle, und es ist wichtig zu beachten, dass Anna ihren Teig zuverlässig fütterte. Zuverlässig heißt hier: jeden Tag. Ein zuverlässiger Rhythmus hilft, und zuverlässige Wärme auch.

` + strings.Repeat("Das Glas steht auf dem Regal und wartet auf die nächste Fütterung, während die Küche über Nacht abkühlt und die Kultur ruht. ", 20) + `

## Zum Schluss

Du schaffst das. Jeder fängt einmal an.
`
	_, p = parseChapter([]byte(de))
	got = nil
	qualityChecks(measure(p, "de"), p, Plan{}, "de", "", func(rule string, line int, message, hint string) {
		got = append(got, rule+" "+message)
	})
	all = strings.Join(got, "\n")
	for _, want := range []string{"announces the chapter", "known tell", "zuverlässig", "stock name", "closes with the section", "invitation to imagine"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
}

func TestRepetitionAcrossChapters(t *testing.T) {
	text := "The starter doubles in eight hours at twenty four degrees when you feed it rye flour and water in equal parts every morning."
	a, b := []block{{text: text, line: 3}}, []block{{text: "Something new happens here. " + text, line: 7}}
	var got []string
	repetition([]chapterText{{"a.md", a}, {"b.md", b}}, func(file string, line int, message, hint string) {
		got = append(got, fmt.Sprintf("%s:%d %s", file, line, message))
	})
	if len(got) != 1 || !strings.HasPrefix(got[0], "b.md:7") {
		t.Errorf("repetition: %v", got)
	}
}

func TestStoreCoverIsReusedWhileItsSourceIsUnchanged(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 160, 256))
	img.Set(3, 3, color.RGBA{200, 10, 10, 255})
	var src bytes.Buffer
	if err := png.Encode(&src, img); err != nil {
		t.Fatal(err)
	}
	first, err := StoreCover(src.Bytes(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, format, err := image.Decode(bytes.NewReader(first)); err != nil || format != "jpeg" {
		t.Fatalf("store cover: %s, %v", format, err)
	}
	if again, _ := StoreCover(src.Bytes(), first); &again[0] != &first[0] {
		t.Error("the published cover was encoded again")
	}
	img.Set(4, 4, color.RGBA{10, 200, 10, 255})
	src.Reset()
	png.Encode(&src, img)
	if changed, _ := StoreCover(src.Bytes(), first); bytes.Equal(changed, first) {
		t.Error("a changed cover reused the old one")
	}
}

func TestBookFilesStayInsideTheBook(t *testing.T) {
	for rel, want := range map[string]bool{"images/a.jpg": false, "..": true, "../assets/a.jpg": true, "/etc/a.jpg": true, "..a/b.jpg": false} {
		if outside(rel) != want {
			t.Errorf("outside(%q) = %v", rel, !want)
		}
	}
}

func TestSlugSpellsOutLetters(t *testing.T) {
	for title, want := range map[string]string{"Größer leben": "groesser-leben", "Café crème": "cafe-creme", "Łódź": "lodz", "???": "book"} {
		if got := Slug(title); got != want {
			t.Errorf("Slug(%q) = %q, want %q", title, got, want)
		}
	}
}
