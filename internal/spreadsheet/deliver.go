package spreadsheet

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// wording is what lfm writes around a spreadsheet: the start PDF, the
// download page and the listing images, per language; English is the
// fallback.
type wording struct {
	StartTitle, CopyButton, CopyExample, NoLink, StepsTitle, ExcelTitle, Excel, SheetsNotDocs string
	Numbers, Phone, LicenseTitle, License, Version, Files, Blank, ExampleFile, StartFile      string
	WorksWith, Basic, Tabs, Formulas, EditionsLine                                            string
}

var wordings = map[string]wording{
	"en": {
		StartTitle: "Start here", CopyButton: "Make my Google Sheets copy", CopyExample: "Open the filled example",
		NoLink:        "Open the .xlsx file in Excel, or upload it to Google Drive and open it with Google Sheets.",
		StepsTitle:    "Start in three steps",
		ExcelTitle:    "Excel, Numbers, phones",
		Excel:         "Prefer Excel? Open the .xlsx file that came with this download (Excel 2016 or newer, Excel for the web, or LibreOffice).",
		SheetsNotDocs: "The copy link opens Google Sheets in your browser, signed in to your Google account; it makes your own private copy.",
		Numbers:       "On a Mac or iPhone, use Google Sheets or Excel: Apple Numbers drops dropdowns and highlights.",
		Phone:         "On a phone, the Google Sheets app works for adding entries; set it up on a computer first.",
		LicenseTitle:  "Licence", License: "For your personal use. Do not share or resell the files or the copy link.",
		Version: "Version", Files: "Your files", Blank: "blank template", ExampleFile: "filled example", StartFile: "start here guide with the Google Sheets copy link",
		WorksWith: "Works with", Basic: "basic", Tabs: "%d tabs", Formulas: "%d formulas, each computed and checked", EditionsLine: "Editions: %s",
	},
	"de": {
		StartTitle: "Hier starten", CopyButton: "Meine Google-Sheets-Kopie erstellen", CopyExample: "Ausgefülltes Beispiel öffnen",
		NoLink:        "Öffne die .xlsx-Datei in Excel, oder lade sie in Google Drive hoch und öffne sie mit Google Sheets.",
		StepsTitle:    "In drei Schritten loslegen",
		ExcelTitle:    "Excel, Numbers, Handy",
		Excel:         "Lieber Excel? Öffne die .xlsx-Datei aus diesem Download (Excel 2016 oder neuer, Excel im Browser oder LibreOffice).",
		SheetsNotDocs: "Der Kopier-Link öffnet Google Sheets im Browser, angemeldet mit deinem Google-Konto, und legt deine eigene private Kopie an.",
		Numbers:       "Auf Mac oder iPhone nimm Google Sheets oder Excel: Apple Numbers verliert Auswahllisten und Markierungen.",
		Phone:         "Auf dem Handy kannst du mit der Google-Sheets-App Buchungen eintragen; richte die Tabelle zuerst am Computer ein.",
		LicenseTitle:  "Lizenz", License: "Für deinen privaten Gebrauch. Gib die Dateien und den Kopier-Link nicht weiter und verkaufe sie nicht.",
		Version: "Version", Files: "Deine Dateien", Blank: "leere Vorlage", ExampleFile: "ausgefülltes Beispiel", StartFile: "Startanleitung mit dem Google-Sheets-Kopier-Link",
		WorksWith: "Läuft mit", Basic: "eingeschränkt", Tabs: "%d Blätter", Formulas: "%d Formeln, jede berechnet und geprüft", EditionsLine: "Ausgaben: %s",
	},
}

func wordsFor(language string) wording {
	base, _, _ := strings.Cut(strings.ToLower(language), "-")
	if w, ok := wordings[base]; ok {
		return w
	}
	return wordings["en"]
}

// StartFileName is the start PDF of an edition.
func (r Report) StartFileName(e Edition) string { return r.Slug() + "-" + e.Key + "-start.pdf" }

// StartDoc is the start-here PDF of an edition as HTML to print: the copy
// button first, the steps from the guide sheet, the notes for Excel,
// Numbers and phones, the licence.
func (r Report) StartDoc(ed *edition) []byte {
	w := wordsFor(ed.Language)
	pal := palette(r.Plan.Accent)
	name := r.Plan.Name
	var s strings.Builder
	fmt.Fprintf(&s, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>
@page{size:A4;margin:16mm 18mm}
body{font:11.5pt/1.45 Arial,Helvetica,sans-serif;color:%s}
.kicker{margin:0;font-size:10pt;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#6B7280}
h1{font-size:24pt;margin:1mm 0 4mm;color:%s}
h2{font-size:13.5pt;margin:6mm 0 1.5mm;color:%s;break-after:avoid}
ul,ol{margin:0;padding-left:6mm}
.button{display:block;margin:8mm 0 3mm;padding:7mm 8mm;background:%s;color:#fff;text-align:center;font-size:17pt;font-weight:700;text-decoration:none;border-radius:3mm}
.second{display:block;text-align:center;color:%s;font-weight:700}
.url{font-size:9pt;color:#6B7280;word-break:break-all;text-align:center}
li{margin:1.2mm 0}
footer{margin-top:8mm;font-size:9pt;color:#6B7280}
</style></head><body>`, html.EscapeString(ed.Language), html.EscapeString(name), pal.Ink, pal.Accent, pal.Accent, pal.Accent, pal.Accent)
	// The guide's title names the product in the edition's language.
	for _, l := range ed.Sheets {
		if l.Kind != "guide" {
			continue
		}
		for _, p := range l.Order {
			if sp := l.Cells[p]; sp.Style == "title" && sp.Text != "" {
				name = sp.Text
				break
			}
		}
		break
	}
	fmt.Fprintf(&s, `<p class="kicker">%s</p><h1>%s</h1>`, html.EscapeString(w.StartTitle), html.EscapeString(name))
	if link := CopyLink(r.Config.Sheets[ed.Key]); link != "" {
		fmt.Fprintf(&s, `<a class="button" href="%s">%s</a><p class="url">%s</p>`, html.EscapeString(link), html.EscapeString(w.CopyButton), html.EscapeString(link))
		if ex := CopyLink(r.Config.Sheets[ed.Key+"-example"]); ex != "" {
			fmt.Fprintf(&s, `<a class="second" href="%s">%s</a>`, html.EscapeString(ex), html.EscapeString(w.CopyExample))
		}
		fmt.Fprintf(&s, "<p>%s</p>", html.EscapeString(w.SheetsNotDocs))
	} else {
		fmt.Fprintf(&s, "<p><strong>%s</strong></p>", html.EscapeString(w.NoLink))
	}
	// The steps come from the guide sheet, so the PDF and the workbook agree.
	for _, l := range ed.Sheets {
		if l.Kind != "guide" {
			continue
		}
		var items []string
		heading := false
		flush := func() {
			if len(items) == 0 {
				return
			}
			if !heading { // the lines before the first heading introduce
				for _, it := range items {
					fmt.Fprintf(&s, "<p>%s</p>", html.EscapeString(it))
				}
				items = nil
				return
			}
			// Numbered lines ("1. Settings: …") become an ordered list.
			tag := "ol"
			for _, it := range items {
				if !numbered.MatchString(it) {
					tag = "ul"
				}
			}
			fmt.Fprintf(&s, "<%s>", tag)
			for _, it := range items {
				if tag == "ol" {
					it = numbered.ReplaceAllString(it, "")
				}
				fmt.Fprintf(&s, "<li>%s</li>", html.EscapeString(it))
			}
			fmt.Fprintf(&s, "</%s>", tag)
			items = nil
		}
		for _, p := range l.Order {
			sp := l.Cells[p]
			switch {
			case sp.Style == "title":
				continue
			case sp.Style == "heading":
				flush()
				heading = true
				fmt.Fprintf(&s, "<h2>%s</h2>", html.EscapeString(sp.Text))
			case sp.Text != "":
				items = append(items, sp.Text)
			}
		}
		flush()
		break
	}
	fmt.Fprintf(&s, "<h2>%s</h2><ul><li>%s</li><li>%s</li><li>%s</li></ul>", html.EscapeString(w.ExcelTitle), html.EscapeString(w.Excel), html.EscapeString(w.Numbers), html.EscapeString(w.Phone))
	fmt.Fprintf(&s, "<h2>%s</h2><p>%s</p>", html.EscapeString(w.LicenseTitle), html.EscapeString(w.License))
	footer := r.Config.Seller
	if r.Config.Version != "" {
		footer += " · " + w.Version + " " + r.Config.Version
	}
	fmt.Fprintf(&s, "<footer>%s</footer></body></html>", html.EscapeString(strings.Trim(footer, " ·")))
	return []byte(s.String())
}

var numbered = regexp.MustCompile(`^\d+[.)]\s+`)

// Image is one listing image.
type Image = deliver.Image

// ImagePlan is one listing image to render: 3000×2250 at twice the density.
type ImagePlan struct {
	Name, What string
	Doc        []byte
}

// Images plans the listing images from the first edition's example: the
// main report alone first (Etsy ranks it higher without text), every tab,
// each tab large, then what it works with.
func (r Report) Images() []ImagePlan {
	if len(r.Editions) == 0 {
		return nil
	}
	ed := r.Editions[0]
	b := r.Books[ed.Key][Example]
	if b == nil {
		b = r.Books[ed.Key][Empty]
	}
	pal := palette(r.Plan.Accent)
	w := wordsFor(ed.Language)
	doc := func(body string) []byte {
		return []byte(fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1500px;height:1125px;overflow:hidden}
body{display:flex;align-items:center;justify-content:center;font:600 28px/1.3 Arial,sans-serif;color:#1F2328;background:radial-gradient(circle at 50%% 35%%,#fff 0,%s 80%%)}
.window{background:#fff;border-radius:14px;box-shadow:0 4px 10px rgba(0,0,0,.12),0 24px 60px rgba(0,0,0,.16);overflow:hidden}
.bar{display:flex;gap:4px;padding:10px 14px;background:#F3F4F6;border-bottom:1px solid #E3E5E8;font:13px Arial}
.bar span{padding:5px 12px;border-radius:6px 6px 0 0;background:#E5E7EB}
.bar span.on{background:#fff;font-weight:700}
.body{padding:14px;width:max-content}
.col{display:flex;flex-direction:column;align-items:center;gap:28px}
.cap{font-size:30px;text-align:center}
.tiles{display:grid;grid-template-columns:repeat(3,auto);gap:26px}
.tile{width:400px;height:260px;overflow:hidden;background:#fff;border-radius:10px;box-shadow:0 10px 28px rgba(0,0,0,.14);display:flex;flex-direction:column}
.tile .view{flex:1;overflow:hidden}
.tile .inner{width:max-content;padding:10px}
.tile b{padding:8px 12px;font-size:18px;border-top:1px solid #E3E5E8}
.works{display:flex;gap:40px;font-size:30px}
.works div{background:#fff;border-radius:14px;padding:26px 34px;box-shadow:0 10px 28px rgba(0,0,0,.12)}
%s</style></head><body>%s</body></html>`, mix(pal.Accent, 0.88), gridCSS(pal), body))
	}
	window := func(on *laid, height int, scale float64) string {
		var bar strings.Builder
		for _, l := range ed.Sheets {
			if l.Hidden {
				continue
			}
			class := ""
			if l == on {
				class = ` class="on"`
			}
			fmt.Fprintf(&bar, "<span%s>%s</span>", class, html.EscapeString(l.Name))
		}
		return fmt.Sprintf(`<div class="window"><div class="bar">%s</div><div><div class="body" data-fit="1380 %d %.2f" data-fit-size>%s</div></div></div>`, bar.String(), height, scale, sheetHTML(ed, b, on, pal, 24))
	}
	var visible []*laid
	var report *laid
	for _, l := range ed.Sheets {
		if l.Hidden {
			continue
		}
		visible = append(visible, l)
		if report == nil && l.Kind == "report" {
			report = l
		}
	}
	if report == nil && len(visible) > 0 {
		report = visible[len(visible)-1]
	}
	var out []ImagePlan
	out = append(out, ImagePlan{Name: "01-" + deliver.Slug(report.Name, "sheet") + ".jpg", What: report.Name + ", the product alone", Doc: doc(window(report, 1000, 1.15))})
	var tiles strings.Builder
	for _, l := range visible {
		fmt.Fprintf(&tiles, `<div class="tile"><div class="view"><div class="inner" data-fit="400 100000 0.42">%s</div></div><b>%s</b></div>`, sheetHTML(ed, b, l, pal, 14), html.EscapeString(l.Name))
	}
	out = append(out, ImagePlan{Name: "02-tabs.jpg", What: "every tab", Doc: doc(fmt.Sprintf(`<div class="col"><div class="cap">%s</div><div class="tiles">%s</div></div>`, html.EscapeString(fmt.Sprintf(w.Tabs, len(visible))), tiles.String()))})
	n := 3
	for _, l := range visible {
		if l == report || l.Kind == "guide" || n > deliver.EtsyImages-1 {
			continue
		}
		out = append(out, ImagePlan{Name: fmt.Sprintf("%02d-%s.jpg", n, deliver.Slug(l.Name, "sheet")), What: l.Name, Doc: doc(fmt.Sprintf(`<div class="col">%s<div class="cap">%s</div></div>`, window(l, 900, 1), html.EscapeString(l.Name)))})
		n++
	}
	var editions []string
	for _, e := range r.Workbook.Editions {
		editions = append(editions, strings.ToUpper(e.Key)+" "+e.Currency)
	}
	works := fmt.Sprintf(`<div class="col"><div class="cap">%s</div><div class="works"><div>Google Sheets ✓</div><div>Excel 2016+ ✓</div><div>LibreOffice ✓</div><div>Numbers (%s)</div></div><div class="cap">%s<br>%s</div></div>`,
		html.EscapeString(w.WorksWith), html.EscapeString(w.Basic), html.EscapeString(fmt.Sprintf(w.Formulas, r.Summary.Formulas)), html.EscapeString(fmt.Sprintf(w.EditionsLine, strings.Join(editions, " · "))))
	out = append(out, ImagePlan{Name: fmt.Sprintf("%02d-works-with.jpg", n), What: "the apps and editions", Doc: doc(works)})
	return out
}

// Facts are the workbook's numbers for the shop descriptions, from the build.
func (r Report) Facts() []string {
	w := wordsFor(r.Config.Language)
	var visible []string
	if len(r.Editions) > 0 {
		for _, l := range r.Editions[0].Sheets {
			if !l.Hidden {
				visible = append(visible, l.Name)
			}
		}
	}
	var editions []string
	for _, e := range r.Workbook.Editions {
		editions = append(editions, strings.ToUpper(e.Key)+" ("+e.Currency+")")
	}
	return []string{
		fmt.Sprintf(w.Tabs, len(visible)) + ": " + strings.Join(visible, ", "),
		fmt.Sprintf(w.Formulas, r.Summary.Formulas),
		w.WorksWith + ": Google Sheets, Excel 2016+, LibreOffice",
		fmt.Sprintf(w.EditionsLine, strings.Join(editions, ", ")),
	}
}

// Distribution assembles the shop packages from the built files: per
// edition the blank and example .xlsx and the start PDF, a ZIP per edition
// when Etsy's five files would not hold them, images, guides and a README.
func Distribution(r Report, files []deliver.File, data map[string][]byte, images []Image, pending bool, day string) (map[string][]byte, []string, []diagnostic.Diagnostic) {
	out := map[string][]byte{}
	pkg := deliver.ShopPackage{Name: r.Plan.Name, Source: Dir, Listing: r.Listing, Facts: r.Facts(), Pending: pending, Groups: map[string][]string{},
		Decide: []string{"**Google Sheets.** Each edition's start PDF carries the copy link of its Google Sheets master; keep the master shared as anyone with the link: viewer, and never edit it after selling: buyers' copies are made from it."}}
	for _, f := range files {
		out[f.Name] = data[f.Name]
		pkg.Files = append(pkg.Files, f.Name)
		for _, e := range r.Workbook.Editions {
			if strings.HasPrefix(f.Name, r.Slug()+"-"+e.Key+"-") || f.Name == r.FileName(e, false) {
				group := r.Slug() + "-" + e.Key + ".zip"
				pkg.Groups[group] = append(pkg.Groups[group], f.Name)
			}
		}
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

// DownloadPage renders the unlisted download page: the Google Sheets copy
// links first, then the files.
func DownloadPage(r Report, files []deliver.File) []byte {
	w := wordsFor(r.Config.Language)
	dw := deliver.Words(r.Config.Language)
	var links []deliver.Link
	for _, e := range r.Workbook.Editions {
		if link := CopyLink(r.Config.Sheets[e.Key]); link != "" {
			label := w.CopyButton
			if len(r.Workbook.Editions) > 1 {
				label += " (" + strings.ToUpper(e.Key) + ")"
			}
			links = append(links, deliver.Link{Label: label, URL: link, Note: w.SheetsNotDocs})
		}
	}
	name := r.Plan.Name
	if name == "" {
		name = "Your download"
	}
	return deliver.Page(deliver.Download{Language: r.Config.Language, Title: name, Subtitle: r.Config.Seller, Accent: r.Plan.Accent,
		Heading: fmt.Sprintf(dw.Heading, name), Links: links, Files: files, Thanks: dw.Thanks, Pending: dw.Pending, Keep: dw.Keep})
}

// FileNote is the line under a file on the download page.
func (r Report) FileNote(name string) string {
	w := wordsFor(r.Config.Language)
	switch {
	case strings.HasSuffix(name, "-start.pdf"):
		return w.StartFile
	case strings.HasSuffix(name, "-example.xlsx"):
		return w.ExampleFile
	}
	return w.Blank
}
