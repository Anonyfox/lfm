package deck

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// Finding is a release finding about one source file.
type Finding struct{ File, Message string }

var (
	baseFont = regexp.MustCompile(`/BaseFont\s*/(?:[A-Z]{6}\+)?([A-Za-z0-9_-]+)`)
	mediaBox = regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([\d.]+)\s+([\d.]+)\s*\]`)
)

// PrintFormat names the slide print layout; changing slidehtml changes it,
// so PDFs an older lfm printed are printed again.
const PrintFormat = "lfm-slides-1"

// ShotPixels is a slide in CSS pixels: 960 pt at 96 dpi.
func ShotPixels() (int64, int64) { return 1280, 720 }

// PaperPixels is a paper in CSS pixels.
func PaperPixels(paper string) (int64, int64) {
	p := Papers[paper]
	return int64(p.Width / 25.4 * 96), int64(p.Height / 25.4 * 96)
}

// SlideShot is slide n of a built deck alone, for a listing image.
func (r Report) SlideShot(b Built, n int) []byte {
	return slideShot(r.render(b.Deck, b.Edition.Papers[0]), n)
}

// NotesShot is the notes page of slide n of a built deck on its first
// paper, for a listing image.
func (r Report) NotesShot(b Built, n int) []byte {
	return notesPages(r.render(b.Deck, b.Edition.Papers[0]), n)
}

// QuizShot is the first page of a built deck's quiz sheet; nil without a quiz.
func (r Report) QuizShot(b Built) []byte {
	if quizzes(b.Deck) == 0 {
		return nil
	}
	return quizPrint(r.render(b.Deck, b.Edition.Papers[0]))
}

// Inspect holds Chrome's print of a deck to its plan.
func (r Report) Inspect(b Built, p Print, pdf []byte) []Finding {
	return r.inspect(b, p, pdf, "Chrome's print")
}

// inspect holds one printed deck to its plan: a page a slide (or a notes
// page a slide), the size, the theme's fonts.
func (r Report) inspect(b Built, p Print, pdf []byte, app string) []Finding {
	var out []Finding
	add := func(format string, args ...any) {
		out = append(out, Finding{b.Deck.File, fmt.Sprintf("%s (%s): ", b.Deck.Title, b.Edition.Key) + fmt.Sprintf(format, args...)})
	}
	pages, n := pdfx.PageCount(pdf), len(b.Deck.Slides)
	if p.Kind != "quiz" && pages != n {
		add("%s prints %d pages for its %d slides.", app, pages, n)
	}
	if m := mediaBox.FindSubmatch(pdf); m != nil {
		w, _ := strconv.ParseFloat(string(m[1]), 64)
		h, _ := strconv.ParseFloat(string(m[2]), 64)
		switch p.Kind {
		case "slides":
			if math.Abs(w/h-slideW/slideH) > 0.01 {
				add("%s prints slides of %.0f × %.0f pt, not 16:9.", app, w, h)
			}
		default:
			if paper := Papers[p.Paper]; math.Abs(w-paper.Width*72/25.4) > 2 || math.Abs(h-paper.Height*72/25.4) > 2 {
				add("%s prints the %s on %.0f × %.0f mm, not %s.", app, p.Kind, w*25.4/72, h*25.4/72, paperNames[p.Paper])
			}
		}
	}
	allowed := []string{}
	for _, f := range []*typeface.Family{r.text, r.head} {
		if f != nil {
			allowed = append(allowed, strings.ReplaceAll(f.Name, " ", ""))
		}
	}
	if _, _, font := typeface.Checkboxes(r.text); font == "Arial" {
		allowed = append(allowed, "Arial", "LiberationSans")
	}
	seen := map[string]bool{}
	for _, m := range baseFont.FindAllSubmatch(pdf, -1) {
		name := string(m[1])
		ok := false
		for _, a := range allowed {
			if strings.HasPrefix(name, a) {
				ok = true
			}
		}
		if !ok && !seen[name] {
			seen[name] = true
			add("%s sets text in %s, so a character is missing from the theme's fonts or a font was not embedded.", app, name)
		}
	}
	return out
}

// Verify opens every built deck in LibreOffice, one run for all, and holds
// its print to the plan: another app's view of the real .pptx.
func (r Report) Verify(soffice string, built []Built) ([]Finding, error) {
	files := map[string][]byte{}
	for _, b := range built {
		files[b.Key+".pptx"] = b.Data
	}
	pdfs, err := office.Convert(soffice, files, "pdf")
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, b := range built {
		pdf := pdfs[b.Key+".pdf"]
		if pdf == nil {
			out = append(out, Finding{b.Deck.File, fmt.Sprintf("LibreOffice could not open %s (%s).", b.Name, b.Edition.Key)})
			continue
		}
		out = append(out, r.inspect(b, Print{Kind: "slides"}, pdf, "LibreOffice")...)
	}
	return out, nil
}

// PowerPoint opens every built deck in Microsoft PowerPoint for Mac and
// holds its PDF to the plan: a deck PowerPoint has to repair, other
// slides, other fonts.
func (r Report) PowerPoint(built []Built) ([]Finding, error) {
	files := map[string][]byte{}
	for _, b := range built {
		files[b.Key] = b.Data
	}
	opened, err := office.OpenInPowerPoint(files)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, b := range built {
		o := opened[b.Key]
		if o.Error != "" {
			out = append(out, Finding{b.Deck.File, fmt.Sprintf("%s (%s): %s.", b.Deck.Title, b.Edition.Key, o.Error)})
			continue
		}
		out = append(out, r.inspect(b, Print{Kind: "slides"}, o.Data, "PowerPoint")...)
	}
	return out, nil
}

// Preview is an edition's decks as one HTML page for lfm serve: every
// slide with its notes below.
func (r Report) Preview(e Edition) []byte {
	k := r.design()
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title><style>%s%s
*{box-sizing:border-box}
body{margin:0;background:#E9E7E3;font:14px/1.5 system-ui,sans-serif;color:#333}
nav{position:sticky;top:0;z-index:1;display:flex;gap:6px;flex-wrap:wrap;padding:10px 16px;background:#F7F6F4;border-bottom:1px solid #D9D6D0;font:13px system-ui,sans-serif}
nav a{color:#333;text-decoration:none;padding:4px 10px;border-radius:6px;background:#ECEAE6}
section{padding:24px 16px}
.label{font:12px system-ui,sans-serif;color:#666;margin:0 auto 8px;max-width:960px}
.deck{max-width:960px;margin:0 auto 28px}
.deck .frame{width:960px;height:540px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,.12),0 8px 24px rgba(0,0,0,.08);margin:0 auto}
.deck .frame .slide{zoom:.75}
.deck .notes{max-width:960px;margin:10px auto 0}
.deck .notes p{margin:0 0 6px}
.deck .notes .bu{display:inline-block;color:#888}
@media (max-width:1000px){.deck .frame{width:480px;height:270px}.deck .frame .slide{zoom:.375}}
</style></head><body><nav>`, html.EscapeString(e.Lang), html.EscapeString(r.Plan.Name), typeface.FontFaces("fonts/", r.text, r.head), slideCSS(k))
	for _, d := range r.Decks[e.Lang] {
		fmt.Fprintf(&b, `<a href="#%s">%s</a>`, html.EscapeString(d.Key), html.EscapeString(d.Title))
	}
	b.WriteString(`</nav>`)
	for _, d := range r.Decks[e.Lang] {
		fmt.Fprintf(&b, `<section id="%s"><h6 class="label">%s · %s</h6>`, html.EscapeString(d.Key), html.EscapeString(d.Title), html.EscapeString(deckFacts(wording(e.Lang), d)))
		previewSlides(&b, r.render(d, e.Papers[0]))
		b.WriteString(`</section>`)
	}
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

// PreviewAssets are the fonts and images a preview loads, by path below
// its folder.
func (r Report) PreviewAssets() map[string][]byte {
	out := map[string][]byte{}
	for _, f := range []*typeface.Family{r.text, r.head} {
		if f == nil {
			continue
		}
		for _, face := range f.Faces {
			out["fonts/"+baseName(face.File)] = face.Data
		}
	}
	for src, p := range r.images {
		out[src] = p.Data
	}
	return out
}
