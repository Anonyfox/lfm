package deck

import (
	"encoding/base64"
	"fmt"
	"html"
	"maps"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/theme"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// words are what lfm writes around the decks, per language; English is
// the fallback.
type words struct {
	GuideTitle, PDFFolder, FontsFolder, Decks, Open, PowerPoint, Slides, SlidesCopy, SlidesLink, Keynote, LibreOffice string
	PresentTitle, Present, Prints, FillTitle, Fill, FillDone, Restyle, NewSlide, LicenseTitle, License, Version       string
	Included, SlideCount, Minutes, Notes, Quiz, Questions, AnswerKey, Answer, Name, Date                              string
	WorksWith, Languages, FontsIncluded, Zip                                                                          string
}

var wordings = map[string]words{
	"en": {
		GuideTitle: "Start here", PDFFolder: "PDF", FontsFolder: "Fonts", Decks: "Your decks", Open: "Open",
		PowerPoint:   "PowerPoint (Windows and Mac): open the .pptx file; the fonts are built in. Save under a new name to keep the original for next time.",
		Slides:       "Google Slides: upload the .pptx to Google Drive, then open it with Google Slides. The fonts are Google Fonts, so Slides shows them too.",
		SlidesCopy:   "Google Slides: open the link and click Make a copy; the copy is yours.",
		SlidesLink:   "Make my Google Slides copy",
		Keynote:      "Keynote (Mac, iPad): install the fonts first: open each file in the Fonts folder and click Install. Then open the .pptx in Keynote.",
		LibreOffice:  "LibreOffice Impress: open the .pptx file; the fonts are built in.",
		PresentTitle: "Present", Present: "Presenter view (PowerPoint: Slide Show > Use Presenter View; Keynote: Play > Presenter Display) shows the speaker notes on your screen only.",
		Prints:    "The PDF folder holds the slides, the speaker notes beside every slide to print and read from, and, where the deck asks questions, the quiz sheet to hand out with its answer key on the last page.",
		FillTitle: "Make it yours", Fill: "Highlighted text is sample text for you to replace: your company, dates, places. Click into it and type.",
		FillDone:     "Done? Select the text and set the highlight color to No Color.",
		Restyle:      "The look lives in the slide master (View > Slide Master): change a layout once and every slide on it follows.",
		NewSlide:     "New slide: Home > New Slide offers the deck's own layouts, so it matches the rest.",
		LicenseTitle: "Licence", License: "For presenting within your own organisation. Do not share or resell the files. The fonts are under the SIL Open Font License; its text is in the Fonts folder.",
		Version: "Version", Included: "%d decks", SlideCount: "%d slides", Minutes: "%d minutes", Notes: "speaker notes", Quiz: "quiz", Questions: "%d quiz questions",
		AnswerKey: "Answer key", Answer: "Answer", Name: "Name", Date: "Date",
		WorksWith: "Works with", Languages: "Languages: %s", FontsIncluded: "Fonts included", Zip: "one ZIP per language",
	},
	"de": {
		GuideTitle: "Hier starten", PDFFolder: "PDF", FontsFolder: "Schriften", Decks: "Deine Präsentationen", Open: "Öffnen",
		PowerPoint:   "PowerPoint (Windows und Mac): Öffne die .pptx-Datei; die Schriften sind eingebettet. Speichere unter neuem Namen, dann bleibt das Original für das nächste Mal.",
		Slides:       "Google Präsentationen: Lade die .pptx in Google Drive hoch und öffne sie mit Google Präsentationen. Die Schriften sind Google Fonts, sie werden also auch dort angezeigt.",
		SlidesCopy:   "Google Präsentationen: Öffne den Link und klicke auf Kopie erstellen; die Kopie gehört dir.",
		SlidesLink:   "Meine Google-Präsentationen-Kopie erstellen",
		Keynote:      "Keynote (Mac, iPad): Installiere zuerst die Schriften: Öffne jede Datei im Ordner Schriften und klicke auf Installieren. Öffne dann die .pptx in Keynote.",
		LibreOffice:  "LibreOffice Impress: Öffne die .pptx-Datei; die Schriften sind eingebettet.",
		PresentTitle: "Präsentieren", Present: "Die Referentenansicht (PowerPoint: Bildschirmpräsentation > Referentenansicht verwenden; Keynote: Wiedergabe > Moderatormonitor) zeigt die Sprechernotizen nur auf deinem Bildschirm.",
		Prints:    "Im Ordner PDF liegen die Folien, die Sprechernotizen neben jeder Folie zum Ausdrucken und Ablesen und, wo die Präsentation Fragen stellt, der Quizbogen zum Austeilen mit der Lösung auf der letzten Seite.",
		FillTitle: "Anpassen", Fill: "Markierter Text ist Beispieltext zum Ersetzen: dein Betrieb, Termine, Orte. Hineinklicken und tippen.",
		FillDone:     "Fertig? Text markieren und die Hervorhebungsfarbe auf Keine Farbe setzen.",
		Restyle:      "Das Aussehen steckt im Folienmaster (Ansicht > Folienmaster): Ändere ein Layout einmal, und alle Folien darauf folgen.",
		NewSlide:     "Neue Folie: Start > Neue Folie bietet die Layouts der Präsentation, so passt sie zum Rest.",
		LicenseTitle: "Lizenz", License: "Zum Präsentieren in deiner eigenen Organisation. Gib die Dateien nicht weiter und verkaufe sie nicht. Die Schriften stehen unter der SIL Open Font License; ihr Text liegt im Ordner Schriften.",
		Version: "Version", Included: "%d Präsentationen", SlideCount: "%d Folien", Minutes: "%d Minuten", Notes: "Sprechernotizen", Quiz: "Quiz", Questions: "%d Quizfragen",
		AnswerKey: "Lösung", Answer: "Antwort", Name: "Name", Date: "Datum",
		WorksWith: "Läuft mit", Languages: "Sprachen: %s", FontsIncluded: "Schriften inklusive", Zip: "ein ZIP je Sprache",
	},
}

func wording(lang string) words {
	base, _, _ := strings.Cut(strings.ToLower(lang), "-")
	if w, ok := wordings[base]; ok {
		return w
	}
	return wordings["en"]
}

// GuideName is an edition's start guide as a separate file.
func (r Report) GuideName(e Edition) string { return r.Slug() + "-" + e.Key + "-start.pdf" }

// Guide is an edition's start-here PDF as HTML to print: the decks, how to
// open them in every app, how to present and adapt them, the licence.
func (r Report) Guide(e Edition) []byte {
	w := wording(e.Lang)
	t := r.Theme
	paper := Papers[e.Papers[0]]
	var s strings.Builder
	fmt.Fprintf(&s, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>%s
@page{size:%.1fmm %.1fmm;margin:18mm}
body{font:11pt/1.45 %q,serif;color:%s;margin:0}
.kicker{margin:0;font:600 9pt/1 %q,sans-serif;letter-spacing:.08em;text-transform:uppercase;color:%s}
h1{font:26pt/1.1 %q,serif;color:%s;margin:2mm 0 6mm}
h2{font:600 13pt/1.2 %q,serif;color:%s;margin:7mm 0 2mm;break-after:avoid}
ul{margin:0;padding-left:5mm}li{margin:1.2mm 0}
.button{display:block;margin:2mm 0 1mm;padding:4mm 6mm;background:%s;color:#fff;text-decoration:none;border-radius:2mm;font-weight:600}
.url{font-size:8pt;color:%s;word-break:break-all;margin:0 0 3mm}
footer{margin-top:10mm;font-size:8.5pt;color:%s}
</style></head><body>`, html.EscapeString(e.Lang), html.EscapeString(r.Plan.Name), typeface.FontFaces("", r.text, r.head),
		paper.Width, paper.Height, t.Text, t.Ink, t.Heading, t.Muted, t.Heading, t.Accent, t.Heading, t.Ink, t.Accent, t.Muted, t.Muted)
	fmt.Fprintf(&s, `<p class="kicker">%s</p><h1>%s</h1><h2>%s</h2><ul>`, html.EscapeString(w.GuideTitle), html.EscapeString(r.Plan.Name), html.EscapeString(w.Decks))
	var links strings.Builder
	for _, d := range r.Decks[e.Lang] {
		fmt.Fprintf(&s, `<li>%s · %s</li>`, html.EscapeString(deliver.FileName(d.Title, "pptx")), html.EscapeString(deckFacts(w, d)))
		if link := CopyLink(r.Config.Slides[e.Lang+"/"+d.Key]); link != "" {
			fmt.Fprintf(&links, `<a class="button" href="%s">%s: %s</a><p class="url">%s</p>`, html.EscapeString(link), html.EscapeString(w.SlidesLink), html.EscapeString(d.Title), html.EscapeString(link))
		}
	}
	slides := w.Slides
	if links.Len() > 0 {
		slides = w.SlidesCopy
	}
	fmt.Fprintf(&s, `</ul><h2>%s</h2><ul><li>%s</li><li>%s</li><li>%s</li><li>%s</li></ul>%s`, html.EscapeString(w.Open), html.EscapeString(w.PowerPoint), html.EscapeString(slides), html.EscapeString(w.Keynote), html.EscapeString(w.LibreOffice), links.String())
	fmt.Fprintf(&s, `<h2>%s</h2><ul><li>%s</li><li>%s</li></ul>`, html.EscapeString(w.PresentTitle), html.EscapeString(w.Present), html.EscapeString(w.Prints))
	fmt.Fprintf(&s, `<h2>%s</h2><ul><li>%s</li><li>%s</li><li>%s</li><li>%s</li></ul>`, html.EscapeString(w.FillTitle), html.EscapeString(w.Fill), html.EscapeString(w.FillDone), html.EscapeString(w.Restyle), html.EscapeString(w.NewSlide))
	fmt.Fprintf(&s, `<h2>%s</h2><p>%s</p>`, html.EscapeString(w.LicenseTitle), html.EscapeString(w.License))
	footer := r.Config.Seller
	if r.Config.Version != "" {
		footer += " · " + w.Version + " " + r.Config.Version
	}
	fmt.Fprintf(&s, `<footer>%s</footer></body></html>`, html.EscapeString(strings.Trim(footer, " ·")))
	return []byte(s.String())
}

// Facts is a built deck in a line, in its language: slides, minutes,
// notes, quiz.
func Facts(b Built) string { return deckFacts(wording(b.Edition.Lang), b.Deck) }

// deckFacts is one deck in a line: slides, minutes, notes, quiz.
func deckFacts(w words, d *Deck) string {
	parts := []string{fmt.Sprintf(w.SlideCount, len(d.Slides))}
	if d.Minutes > 0 {
		parts = append(parts, fmt.Sprintf(w.Minutes, d.Minutes))
	}
	for _, s := range d.Slides {
		if len(s.Notes) > 0 {
			parts = append(parts, w.Notes)
			break
		}
	}
	if n := quizzes(d); n > 0 {
		parts = append(parts, fmt.Sprintf(w.Questions, n))
	}
	return strings.Join(parts, ", ")
}

// GuideAssets are the font files a guide, print or listing image loads, by
// bare file name as the print server serves them.
func (r Report) GuideAssets() map[string][]byte {
	out := map[string][]byte{}
	for _, f := range []*typeface.Family{r.text, r.head} {
		if f == nil {
			continue
		}
		for _, face := range f.Faces {
			out[baseName(face.File)] = face.Data
		}
	}
	return out
}

// Shot is a deck as a release renders it for the listing: the cover, the
// slides after it, a notes page and the quiz sheet.
type Shot struct {
	Key, Title  string
	Cover       []byte
	Slides      [][]byte
	Notes, Quiz []byte
	Facts       string
}

// ImagePlan is one listing image to render: 3000×2250 at twice the density.
type ImagePlan struct {
	Name string
	Doc  []byte
}

// Image is a rendered listing image.
type Image = deliver.Image

// Images plans the listing images from the first edition's decks: the
// first cover alone, every deck, each deck's slides, what the presenter
// gets on paper, then what the set works with.
func (r Report) Images(shots []Shot) []ImagePlan {
	if len(shots) == 0 {
		return nil
	}
	w := wording(r.Config.Language)
	src := func(png []byte) string { return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png) }
	doc := func(body string) []byte {
		return []byte(fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><style>%s
html,body{margin:0;width:1500px;height:1125px;overflow:hidden}
body{display:flex;align-items:center;justify-content:center;font:600 28px/1.3 %q,sans-serif;color:%s;background:radial-gradient(circle at 50%% 35%%,#fff 0,%s 85%%)}
.slide{background:#fff;box-shadow:0 2px 6px rgba(0,0,0,.10),0 22px 50px rgba(0,0,0,.16);display:block}
.col{display:flex;flex-direction:column;align-items:center;gap:26px}
.cap{font-size:30px;text-align:center}
.row{display:flex;gap:34px;align-items:flex-end}
.tile{display:flex;flex-direction:column;align-items:center;gap:12px;font-size:19px}
.grid{display:grid;grid-template-columns:repeat(3,420px);gap:24px}
.works{display:flex;flex-wrap:wrap;justify-content:center;gap:28px;font-size:28px;max-width:1300px}
.works div{background:#fff;border-radius:14px;padding:22px 30px;box-shadow:0 10px 28px rgba(0,0,0,.12)}
</style></head><body>%s</body></html>`, typeface.FontFaces("", r.head), r.Theme.Heading, r.Theme.Ink, theme.Tint(r.Theme.Accent, 0.86), body))
	}
	img := func(png []byte, w, h float64) string {
		return fmt.Sprintf(`<img class="slide" src="%s" style="width:%.0fpx;height:%.0fpx">`, src(png), w, h)
	}
	var out []ImagePlan
	first := shots[0]
	out = append(out, ImagePlan{Name: "01-" + deliver.Slug(first.Title, "deck") + ".jpg", Doc: doc(img(first.Cover, 1360, 765))})
	if len(shots) > 1 {
		var tiles strings.Builder
		n := min(len(shots), 4)
		tw := (1380 - 34*float64(n-1)) / float64(n)
		for _, s := range shots[:n] {
			fmt.Fprintf(&tiles, `<div class="tile">%s<span>%s</span></div>`, img(s.Cover, tw, tw*9/16), html.EscapeString(s.Title))
		}
		out = append(out, ImagePlan{Name: "02-decks.jpg", Doc: doc(fmt.Sprintf(`<div class="col"><div class="cap">%s</div><div class="row">%s</div></div>`, html.EscapeString(fmt.Sprintf(w.Included, len(r.Decks[r.Showcase().Lang]))), tiles.String()))})
	}
	add := func(name, body string) {
		if len(out) < deliver.EtsyImages-1 {
			out = append(out, ImagePlan{Name: fmt.Sprintf("%02d-%s.jpg", len(out)+1, name), Doc: doc(body)})
		}
	}
	for _, s := range shots {
		if len(s.Slides) == 0 {
			continue
		}
		var grid strings.Builder
		for _, png := range s.Slides {
			grid.WriteString(img(png, 420, 236))
		}
		add(deliver.Slug(s.Title, "deck")+"-slides", fmt.Sprintf(`<div class="col"><div class="cap">%s · %s</div><div class="grid">%s</div></div>`, html.EscapeString(s.Title), html.EscapeString(s.Facts), grid.String()))
	}
	for _, s := range shots {
		if s.Notes == nil {
			continue
		}
		pages := img(s.Notes, 560, 792)
		caption := w.Notes
		if s.Quiz != nil {
			pages += img(s.Quiz, 560, 792)
			caption += " · " + w.Quiz + " · " + w.AnswerKey
		}
		add(deliver.Slug(s.Title, "deck")+"-notes", fmt.Sprintf(`<div class="col"><div class="row">%s</div><div class="cap">%s</div></div>`, pages, html.EscapeString(caption)))
		break
	}
	var langs []string
	for _, e := range r.Editions {
		langs = append(langs, strings.ToUpper(e.Lang))
	}
	works := `<div class="col"><div class="cap">` + html.EscapeString(w.WorksWith) + `</div><div class="works">`
	for _, a := range []string{"PowerPoint", "Google Slides", "Keynote", "LibreOffice"} {
		works += `<div>` + a + ` ✓</div>`
	}
	works += fmt.Sprintf(`</div><div class="cap">%s<br>%s</div></div>`, html.EscapeString(fmt.Sprintf(w.Languages, strings.Join(langs, " · "))), html.EscapeString(w.FontsIncluded))
	out = append(out, ImagePlan{Name: fmt.Sprintf("%02d-works-with.jpg", len(out)+1), Doc: doc(works)})
	return out
}

// Facts are the set's numbers for the shop descriptions, from the build.
func (r Report) Facts() []string {
	w := wording(r.Config.Language)
	var decks, langs []string
	for _, d := range r.Decks[r.Showcase().Lang] {
		decks = append(decks, d.Title+" ("+deckFacts(w, d)+")")
	}
	for _, e := range r.Editions {
		langs = append(langs, strings.ToUpper(e.Lang))
	}
	return []string{
		fmt.Sprintf(w.Included, len(decks)) + ": " + strings.Join(decks, "; "),
		w.WorksWith + ": PowerPoint, Google Slides, Keynote, LibreOffice",
		fmt.Sprintf(w.Languages, strings.Join(langs, ", ")) + "; " + w.Zip,
		w.FontsIncluded + ": " + strings.Join(r.familyNames(), ", ") + " (SIL Open Font License)",
	}
}

func (r Report) familyNames() []string {
	var out []string
	for _, f := range []*typeface.Family{r.text, r.head} {
		if f != nil && (len(out) == 0 || out[0] != f.Name) {
			out = append(out, f.Name)
		}
	}
	return out
}

// Distribution assembles the shop packages: every edition's ZIP once, the
// images, a guide per shop and the README.
func Distribution(r Report, files []deliver.File, data map[string][]byte, images []Image, pending bool, day string) (map[string][]byte, []string, []diagnostic.Diagnostic) {
	out := map[string][]byte{}
	pkg := deliver.ShopPackage{Name: r.Plan.Name, Source: Dir, Listing: r.Listing, Facts: r.Facts(), Pending: pending,
		Decide: []string{"**Google Slides.** Optional: upload each .pptx to Google Drive, open it with Google Slides, share it as anyone with the link: viewer, and paste its address into deliverables.deck.slides as \"<language>/<deck>\". The start guide then carries a copy link; never edit a master after selling."}}
	for _, f := range files {
		out[f.Name] = data[f.Name]
		pkg.Files = append(pkg.Files, f.Name)
	}
	for _, img := range images {
		out["images/"+img.Name] = img.Data
		pkg.Images = append(pkg.Images, img.Name)
	}
	channels, found := deliver.Package(pkg, out, day)
	var issues []diagnostic.Diagnostic
	for _, f := range found {
		issues = append(issues, at(ListingFile, 1, RuleChannels, diagnostic.Warning, f.Message, f.Hint))
	}
	return out, channels, issues
}

// DownloadPage renders the unlisted download page: the Google Slides copy
// links first, then the files.
func DownloadPage(r Report, files []deliver.File) []byte {
	w := wording(r.Config.Language)
	dw := deliver.Words(r.Config.Language)
	var links []deliver.Link
	for _, lang := range slices.Sorted(maps.Keys(r.Decks)) {
		for _, d := range r.Decks[lang] {
			if link := CopyLink(r.Config.Slides[lang+"/"+d.Key]); link != "" {
				links = append(links, deliver.Link{Label: w.SlidesLink + ": " + d.Title, URL: link})
			}
		}
	}
	name := r.Plan.Name
	if name == "" {
		name = "Your download"
	}
	return deliver.Page(deliver.Download{Language: r.Config.Language, Title: name, Subtitle: r.Config.Seller, Accent: r.Theme.Accent,
		Heading: fmt.Sprintf(dw.Heading, name), Links: links, Files: files, Thanks: dw.Thanks, Pending: dw.Pending, Keep: dw.Keep})
}

// FileNote is the line under a file on the download page.
func (r Report) FileNote(e Edition) string {
	w := wording(e.Lang)
	return strings.ToUpper(e.Lang) + " · " + fmt.Sprintf(w.Included, len(r.Decks[e.Lang])) + " · .pptx, PDF, " + w.FontsFolder
}
