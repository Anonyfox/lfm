package document

import (
	"encoding/base64"
	"fmt"
	"html"
	"math"
	"strings"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// words are what lfm writes around the documents, per language; English
// is the fallback.
type words struct {
	GuideTitle, PDFFolder, FontsFolder, Documents, Open, Word, Docs, DocsCopy, DocsLink, Pages, LibreOffice string
	FillTitle, Fill, FillDone, Restyle, Rows, Print, PrintPaper, LicenseTitle, License, Version, Fonts      string
	Included, WorksWith, Editions, FontsIncluded, Paper, Zip                                                string
}

var wordings = map[string]words{
	"en": {
		GuideTitle: "Start here", PDFFolder: "PDF", FontsFolder: "Fonts", Documents: "Your documents", Open: "Open and edit",
		Word:        "Word (Windows and Mac): open the .docx file; the fonts are built in. Save under a new name to keep the original for next time.",
		Docs:        "Google Docs: upload the .docx to Google Drive, then open it with Google Docs. The fonts are Google Fonts, so Docs shows them too.",
		DocsCopy:    "Google Docs: open the link and click Make a copy; the copy is yours.",
		DocsLink:    "Make my Google Docs copy",
		Pages:       "Pages (Mac, iPad): install the fonts first: open each file in the Fonts folder and click Install. Then open the .docx in Pages.",
		LibreOffice: "LibreOffice: open the .docx file; the fonts are built in.",
		FillTitle:   "Fill in",
		Fill:        "Highlighted text is sample text for you to replace: click into it and type.",
		FillDone:    "Done? Select everything (Ctrl+A or Cmd+A) and set the highlight color to None.",
		Restyle:     "The look lives in styles (Normal, Title, Heading 1 to 3): change a style once and every heading or paragraph follows.",
		Rows:        "Tables grow: press Tab in the last cell to add a row.",
		Print:       "Print", PrintPaper: "Print at 100%% (actual size) on %s paper; the file is set to it.",
		LicenseTitle: "Licence", License: "For your own and your organisation's use. Do not share or resell the files. The fonts are under the SIL Open Font License; its text is in the Fonts folder.",
		Version: "Version", Fonts: "Fonts",
		Included: "%d documents", WorksWith: "Works with", Editions: "Editions: %s", FontsIncluded: "Fonts included", Paper: "%s paper", Zip: "one ZIP per edition",
	},
	"de": {
		GuideTitle: "Hier starten", PDFFolder: "PDF", FontsFolder: "Schriften", Documents: "Deine Dokumente", Open: "Öffnen und bearbeiten",
		Word:        "Word (Windows und Mac): Öffne die .docx-Datei; die Schriften sind eingebettet. Speichere unter neuem Namen, dann bleibt das Original für das nächste Mal.",
		Docs:        "Google Docs: Lade die .docx in Google Drive hoch und öffne sie mit Google Docs. Die Schriften sind Google Fonts, Docs zeigt sie also auch.",
		DocsCopy:    "Google Docs: Öffne den Link und klicke auf Kopie erstellen; die Kopie gehört dir.",
		DocsLink:    "Meine Google-Docs-Kopie erstellen",
		Pages:       "Pages (Mac, iPad): Installiere zuerst die Schriften: Öffne jede Datei im Ordner Schriften und klicke auf Installieren. Öffne dann die .docx in Pages.",
		LibreOffice: "LibreOffice: Öffne die .docx-Datei; die Schriften sind eingebettet.",
		FillTitle:   "Ausfüllen",
		Fill:        "Markierter Text ist Beispieltext zum Ersetzen: hineinklicken und tippen.",
		FillDone:    "Fertig? Alles markieren (Strg+A oder Cmd+A) und die Hervorhebungsfarbe auf Keine setzen.",
		Restyle:     "Das Aussehen steckt in Formatvorlagen (Standard, Titel, Überschrift 1 bis 3): Ändere eine Vorlage einmal, und alle Überschriften oder Absätze folgen.",
		Rows:        "Tabellen wachsen: Drücke Tab in der letzten Zelle für eine neue Zeile.",
		Print:       "Drucken", PrintPaper: "Drucke in 100 %% (Originalgröße) auf %s; die Datei ist darauf eingestellt.",
		LicenseTitle: "Lizenz", License: "Für deinen eigenen Gebrauch und den deiner Organisation. Gib die Dateien nicht weiter und verkaufe sie nicht. Die Schriften stehen unter der SIL Open Font License; ihr Text liegt im Ordner Schriften.",
		Version: "Version", Fonts: "Schriften",
		Included: "%d Dokumente", WorksWith: "Läuft mit", Editions: "Ausgaben: %s", FontsIncluded: "Schriften inklusive", Paper: "%s-Papier", Zip: "ein ZIP je Ausgabe",
	},
}

func wording(lang string) words {
	base, _, _ := strings.Cut(strings.ToLower(lang), "-")
	if w, ok := wordings[base]; ok {
		return w
	}
	return wordings["en"]
}

var paperNames = map[string]string{"a4": "A4", "letter": "US Letter"}

// GuideName is an edition's start guide as a separate file.
func (r Report) GuideName(e Edition) string { return r.Slug() + "-" + e.Key + "-start.pdf" }

// Guide is an edition's start-here PDF as HTML to print: the documents,
// how to open them in every app, how to fill and restyle them, printing,
// the licence.
func (r Report) Guide(e Edition) []byte {
	w := wording(e.Lang)
	m := layoutTheme(r.Theme, Papers[e.Paper], r.text, r.head)
	var s strings.Builder
	fmt.Fprintf(&s, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>%s
@page{size:%.1fmm %.1fmm;margin:18mm}
body{font:%.1fpt/1.45 %q,serif;color:%s;margin:0}
.kicker{margin:0;font:600 9pt/1 %q,sans-serif;letter-spacing:.08em;text-transform:uppercase;color:%s}
h1{font:%.0fpt/1.1 %q,serif;color:%s;margin:2mm 0 6mm}
h2{font:600 %.1fpt/1.2 %q,serif;color:%s;margin:7mm 0 2mm;break-after:avoid}
ul{margin:0;padding-left:5mm}li{margin:1.2mm 0}
.button{display:block;margin:2mm 0 1mm;padding:4mm 6mm;background:%s;color:#fff;text-decoration:none;border-radius:2mm;font-weight:600}
.url{font-size:8pt;color:%s;word-break:break-all;margin:0 0 3mm}
footer{margin-top:10mm;font-size:8.5pt;color:%s}
</style></head><body>`, html.EscapeString(e.Lang), html.EscapeString(r.Plan.Name), fontFaces(r.text, r.head, ""),
		m.Paper.Width, m.Paper.Height, m.Base, m.Text, m.Ink, m.Heading, m.Muted, m.H1*1.3, m.Heading, m.Accent, m.H3, m.Heading, m.Ink, m.Accent, m.Muted, m.Muted)
	fmt.Fprintf(&s, `<p class="kicker">%s</p><h1>%s</h1>`, html.EscapeString(w.GuideTitle), html.EscapeString(r.Plan.Name))
	fmt.Fprintf(&s, `<h2>%s</h2><ul>`, html.EscapeString(w.Documents))
	for _, d := range r.Docs[e.Lang] {
		fmt.Fprintf(&s, `<li>%s</li>`, html.EscapeString(FileName(d.Title, "docx")))
	}
	s.WriteString(`</ul>`)
	fmt.Fprintf(&s, `<h2>%s</h2>`, html.EscapeString(w.Open))
	var links strings.Builder
	for _, d := range r.Docs[e.Lang] {
		if link := CopyLink(r.Config.Docs[e.Lang+"/"+d.Key]); link != "" {
			fmt.Fprintf(&links, `<a class="button" href="%s">%s: %s</a><p class="url">%s</p>`, html.EscapeString(link), html.EscapeString(w.DocsLink), html.EscapeString(d.Title), html.EscapeString(link))
		}
	}
	docs := w.Docs
	if links.Len() > 0 {
		docs = w.DocsCopy
	}
	fmt.Fprintf(&s, `<ul><li>%s</li><li>%s</li><li>%s</li><li>%s</li></ul>%s`, html.EscapeString(w.Word), html.EscapeString(docs), html.EscapeString(w.Pages), html.EscapeString(w.LibreOffice), links.String())
	fmt.Fprintf(&s, `<h2>%s</h2><ul><li>%s</li><li>%s</li><li>%s</li><li>%s</li></ul>`, html.EscapeString(w.FillTitle), html.EscapeString(w.Fill), html.EscapeString(w.FillDone), html.EscapeString(w.Restyle), html.EscapeString(w.Rows))
	fmt.Fprintf(&s, `<h2>%s</h2><p>%s</p>`, html.EscapeString(w.Print), html.EscapeString(fmt.Sprintf(w.PrintPaper, paperNames[e.Paper])))
	fmt.Fprintf(&s, `<h2>%s</h2><p>%s</p>`, html.EscapeString(w.LicenseTitle), html.EscapeString(w.License))
	footer := r.Config.Seller
	if r.Config.Version != "" {
		footer += " · " + w.Version + " " + r.Config.Version
	}
	fmt.Fprintf(&s, `<footer>%s</footer></body></html>`, html.EscapeString(strings.Trim(footer, " ·")))
	return []byte(s.String())
}

// GuideAssets are the font files a guide or listing image loads, by bare
// file name as the print server serves them.
func (r Report) GuideAssets() map[string][]byte {
	out := map[string][]byte{}
	for _, f := range []*family{r.text, r.head} {
		if f == nil {
			continue
		}
		for _, face := range f.Faces {
			out[baseName(face.File)] = face.Data
		}
	}
	return out
}

func fontFaces(text, head *family, prefix string) string {
	return typeface.FontFaces(prefix, text, head)
}

// Shot is a document's first page as LibreOffice renders the real file.
type Shot struct {
	Key   string // the built file's key
	Title string
	PNG   []byte
}

// ImagePlan is one listing image to render: 3000×2250 at twice the density.
type ImagePlan struct {
	Name, What string
	Doc        []byte
}

// Image is a rendered listing image.
type Image struct {
	Name, What string
	Data       []byte
}

// Images plans the listing images from the first edition's first pages,
// rendered from the real files: the first document alone, every document,
// each one large, then what the set works with.
func (r Report) Images(shots []Shot) []ImagePlan {
	if len(shots) == 0 || len(r.Editions) == 0 {
		return nil
	}
	e := r.Showcase()
	w := wording(r.Config.Language)
	ratio := Papers[e.Paper].Height / Papers[e.Paper].Width
	src := func(s Shot) string { return "data:image/png;base64," + base64.StdEncoding.EncodeToString(s.PNG) }
	doc := func(body string) []byte {
		return []byte(fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><style>%s
html,body{margin:0;width:1500px;height:1125px;overflow:hidden}
body{display:flex;align-items:center;justify-content:center;font:600 28px/1.3 %q,sans-serif;color:%s;background:radial-gradient(circle at 50%% 35%%,#fff 0,%s 85%%)}
.page{background:#fff;box-shadow:0 2px 6px rgba(0,0,0,.10),0 22px 50px rgba(0,0,0,.16);display:block}
.col{display:flex;flex-direction:column;align-items:center;gap:26px}
.cap{font-size:30px;text-align:center}
.row{display:flex;gap:34px;align-items:flex-end}
.tile{display:flex;flex-direction:column;align-items:center;gap:12px;font-size:19px}
.works{display:flex;gap:28px;font-size:28px}
.works div{background:#fff;border-radius:14px;padding:22px 30px;box-shadow:0 10px 28px rgba(0,0,0,.12)}

</style></head><body>%s</body></html>`, fontFaces(nil, r.head, ""), r.Theme.Heading, r.Theme.Ink, tint(r.Theme.Accent, 0.86), body))
	}
	page := func(s Shot, height float64) string {
		return fmt.Sprintf(`<img class="page" src="%s" style="height:%.0fpx;width:%.0fpx">`, src(s), height, height/ratio)
	}
	var out []ImagePlan
	out = append(out, ImagePlan{Name: "01-" + deliver.Slug(shots[0].Title, "document") + ".jpg", What: shots[0].Title + ", the product alone", Doc: doc(page(shots[0], 1030))})
	var tiles strings.Builder
	n := min(len(shots), 5)
	// The tiles share the width, gaps included, and leave room for the caption.
	h := math.Min(780, (1380-34*float64(n-1))/float64(n)*ratio)
	for _, s := range shots[:n] {
		fmt.Fprintf(&tiles, `<div class="tile">%s<span>%s</span></div>`, page(s, h), html.EscapeString(s.Title))
	}
	out = append(out, ImagePlan{Name: "02-documents.jpg", What: "every document", Doc: doc(fmt.Sprintf(`<div class="col"><div class="cap">%s</div><div class="row">%s</div></div>`, html.EscapeString(fmt.Sprintf(w.Included, r.Summary.Documents)), tiles.String()))})
	i := 3
	for _, s := range shots[1:] {
		if i > deliver.EtsyImages-1 {
			break
		}
		out = append(out, ImagePlan{Name: fmt.Sprintf("%02d-%s.jpg", i, deliver.Slug(s.Title, "document")), What: s.Title, Doc: doc(fmt.Sprintf(`<div class="col">%s<div class="cap">%s</div></div>`, page(s, 930), html.EscapeString(s.Title)))})
		i++
	}
	var editions []string
	for _, e := range r.Editions {
		editions = append(editions, strings.ToUpper(e.Lang)+" "+paperNames[e.Paper])
	}
	works := fmt.Sprintf(`<div class="col"><div class="cap">%s</div><div class="works"><div>Word ✓</div><div>Google Docs ✓</div><div>Pages ✓</div><div>LibreOffice ✓</div></div><div class="cap">%s<br>%s</div></div>`,
		html.EscapeString(w.WorksWith), html.EscapeString(fmt.Sprintf(w.Editions, strings.Join(editions, " · "))), html.EscapeString(w.FontsIncluded))
	out = append(out, ImagePlan{Name: fmt.Sprintf("%02d-works-with.jpg", i), What: "the apps and editions", Doc: doc(works)})
	return out
}

// Facts are the set's numbers for the shop descriptions, from the build.
func (r Report) Facts() []string {
	w := wording(r.Config.Language)
	var titles, editions []string
	if len(r.Editions) > 0 {
		for _, d := range r.Docs[r.Showcase().Lang] {
			titles = append(titles, d.Title)
		}
	}
	for _, e := range r.Editions {
		editions = append(editions, strings.ToUpper(e.Lang)+" "+paperNames[e.Paper])
	}
	return []string{
		fmt.Sprintf(w.Included, r.Summary.Documents) + ": " + strings.Join(titles, ", "),
		w.WorksWith + ": Word, Google Docs, Pages, LibreOffice",
		fmt.Sprintf(w.Editions, strings.Join(editions, ", ")) + "; " + w.Zip,
		w.FontsIncluded + ": " + strings.Join(r.familyNames(), ", ") + " (SIL Open Font License)",
	}
}

func (r Report) familyNames() []string {
	var out []string
	for _, f := range []*family{r.text, r.head} {
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
		Decide: []string{"**Google Docs.** Optional: upload each .docx to Google Drive, open it with Google Docs, share it as anyone with the link: viewer, and paste its address into deliverables.document.docs as \"<language>/<document>\". The start guide then carries a copy link; never edit a master after selling."}}
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

// DownloadPage renders the unlisted download page: the Google Docs copy
// links first, then the files.
func DownloadPage(r Report, files []deliver.File) []byte {
	w := wording(r.Config.Language)
	dw := deliver.Words(r.Config.Language)
	var links []deliver.Link
	for _, lang := range sortedKeys(r.Docs) {
		for _, d := range r.Docs[lang] {
			if link := CopyLink(r.Config.Docs[lang+"/"+d.Key]); link != "" {
				label := w.DocsLink + ": " + d.Title
				links = append(links, deliver.Link{Label: label, URL: link})
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
	return fmt.Sprintf(w.Paper, paperNames[e.Paper]) + " · " + fmt.Sprintf(w.Included, len(r.Docs[e.Lang])) + " · .docx, PDF, " + w.Fonts
}
