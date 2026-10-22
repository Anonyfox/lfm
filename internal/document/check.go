package document

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // image sizes
	_ "image/png"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/markup"
	"github.com/Anonyfox/lfm/internal/paper"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// Edition is one language on one paper: the unit buyers download.
type Edition struct {
	Key, Lang, Paper string
}

// Summary is what the compile report shows.
type Summary struct {
	Name      string   `json:"name,omitempty"`
	Concept   bool     `json:"concept"`
	Fonts     []string `json:"fonts,omitempty"` // the families the concept names
	Documents int      `json:"documents"`
	Languages []string `json:"languages,omitempty"`
	Editions  []string `json:"editions,omitempty"`
	Fields    int      `json:"fields"`
	Sell      string   `json:"sell"`
	URL       string   `json:"url,omitempty"`
	Files     []string `json:"files,omitempty"`
	Listing   bool     `json:"listing"`
	Linked    int      `json:"linked"`
	Channels  []string `json:"channels,omitempty"`
	Images    int      `json:"images"`
	Pending   bool     `json:"pending"`
	Layout    string   `json:"layout,omitempty"`
}

// Report is the checked document set.
type Report struct {
	Config      Config
	Summary     Summary
	Plan        Plan
	Theme       Theme
	Diagnostics []diagnostic.Diagnostic
	Buildable   bool
	Editions    []Edition
	Docs        map[string][]*Doc // by language, in the concept's order
	Listing     deliver.ShopListing
	fonts       map[string]*family
	text, head  *family
	images      map[string]*picture
	licences    map[string][]byte // licence files shipped with the fonts
	digest      []byte            // the hash of the theme, fonts, licences and images
	pack        *typeface.Pack    // the embedded fonts, compressed once
}

func at(file string, line int, rule string, sev diagnostic.Severity, message, hint string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Rule: rule, Severity: sev, Location: diagnostic.Location{File: file, Line: max(line, 1), Column: 1}, Message: message, Hint: hint}
}

var langDir = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})?$`)

// Check reads and checks the document set: concept, fonts, every document
// of every language, the theme on every paper, and the listing.
func Check(p project.Project, cfg Config, configured bool, offers map[string]*deliver.Price, site string) Report {
	r := Report{Config: cfg, Docs: map[string][]*Doc{}, fonts: map[string]*family{}, images: map[string]*picture{}, licences: map[string][]byte{}, pack: typeface.NewPack()}
	r.Summary.Sell = deliver.Sell(cfg.Sell)
	fail := func(message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at("lfm.json", 1, RuleConfig, diagnostic.Error, message, hint))
	}
	switch {
	case !configured:
		fail("deliverables/document exists, but lfm.json has no deliverables.document.",
			fmt.Sprintf(`Add "deliverables": {"document": {"path": %q}} to lfm.json. 'lfm help config deliverables.document' lists the options.`, deliver.NewPath()))
	case r.Summary.Sell != deliver.SellPage:
	case !deliver.ValidPath(cfg.Path):
		fail(fmt.Sprintf("deliverables.document.path %q is not an unguessable folder name.", cfg.Path),
			fmt.Sprintf("Use at least %d lowercase letters, digits or single hyphens, for example %q. Anyone who knows the path can download the files.", deliver.MinPathLength, deliver.NewPath()))
	default:
		r.Summary.URL = strings.TrimSuffix(site, "/") + "/" + cfg.Path + "/"
	}
	for _, paper := range cfg.Papers {
		if _, ok := Papers[paper]; !ok {
			fail(fmt.Sprintf("deliverables.document.papers names %q.", paper), "Use a4 and letter.")
		}
	}
	var offer *deliver.Price
	if cfg.Offer != "" {
		o, ok := offers[cfg.Offer]
		if !ok {
			fail(fmt.Sprintf("deliverables.document.offer %q names no offer in data.commerce.offers.", cfg.Offer), "Use the key of the offer that sells the documents.")
		}
		offer = o
	}
	warn := func(file string, line int, rule, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at(file, line, rule, diagnostic.Warning, message, hint))
	}
	errorAt := func(file string, line int, rule, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at(file, line, rule, diagnostic.Error, message, hint))
	}

	if raw, err := p.Read(ConceptFile); err == nil {
		r.Summary.Concept = true
		r.Plan = checkConcept(string(raw), func(line int, message, hint string) { warn(ConceptFile, line, RuleConcept, message, hint) })
		r.Summary.Name = r.Plan.Name
	}
	r.Theme = r.Plan.theme()
	r.Summary.Fonts = families(r.Theme)
	// Until the concept is written there is nothing to hold to it: its Look
	// names the fonts, and the documents follow it.
	if !r.Summary.Concept {
		return r
	}

	r.readFonts(p, errorAt, warn)
	r.readDocs(p, errorAt, warn)
	r.Buildable = r.text != nil && r.head != nil && len(r.Editions) > 0
	for _, d := range r.Diagnostics {
		if d.Severity == diagnostic.Error && (d.Rule == RuleSource || d.Rule == RuleFonts || d.Rule == RuleConfig) {
			r.Buildable = false
		}
	}
	if r.text != nil && r.head != nil {
		seen := map[string]bool{}
		for _, e := range r.Editions {
			if seen[e.Paper] {
				continue
			}
			seen[e.Paper] = true
			for _, problem := range themeProblems(layoutTheme(r.Theme, Papers[e.Paper], r.text, r.head)) {
				errorAt(ConceptFile, r.Plan.Lines["Look"], RuleDesign, problem, "Change the accent or fonts in the concept's Look, or pick another theme.")
			}
		}
	}
	r.digest = r.inputs()
	r.checkLinks()
	if lraw, err := p.Read(ListingFile); err == nil {
		r.Listing = deliver.CheckShopListing(string(lraw), offer, func(line int, message, hint string) { warn(ListingFile, line, RuleListing, message, hint) })
		r.Summary.Listing = true
	}
	return r
}

// readFonts loads deliverables/document/fonts and finds the theme's two
// families in it.
func (r *Report) readFonts(p project.Project, errorAt, warn func(string, int, string, string, string)) {
	report := func(file string, line int, fatal bool, message, hint string) {
		if fatal {
			errorAt(file, line, RuleFonts, message, hint)
		} else {
			warn(file, line, RuleFonts, message, hint)
		}
	}
	folder := typeface.Load(p, FontsDir, report)
	r.fonts, r.licences = folder.Families, folder.Licences
	r.text = folder.Want(r.Theme.Text, "text", ConceptFile, r.Plan.Lines["Look"], report)
	r.head = r.text
	if r.Theme.Heading != r.Theme.Text {
		r.head = folder.Want(r.Theme.Heading, "heading", ConceptFile, r.Plan.Lines["Look"], report)
	}
}

// readDocs parses every language folder and checks each document.
func (r *Report) readDocs(p project.Project, errorAt, warn func(string, int, string, string, string)) {
	dir, err := p.Path(Dir)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		errorAt(Dir, 1, RuleSource, "deliverables/document cannot be read: "+err.Error()+".", "Make the folder readable.")
		return
	}
	var langs []string
	for _, e := range entries {
		if e.IsDir() && langDir.MatchString(e.Name()) {
			langs = append(langs, e.Name())
		}
	}
	slices.Sort(langs)
	if len(langs) == 0 {
		errorAt(Dir, 1, RuleSource, "deliverables/document has no language folder.", "Write each document as Markdown in a folder per language, such as en/invoice.md and de/invoice.md.")
		return
	}
	planned := map[string]bool{}
	for _, d := range r.Plan.Documents {
		planned[d.Key] = true
	}
	type parsed struct {
		doc      *Doc
		problems []problem
		missing  bool
		err      error
		key      string
	}
	results := map[string][]parsed{}
	var wg sync.WaitGroup
	for _, lang := range langs {
		names := map[string]bool{}
		files, err := os.ReadDir(filepath.Join(dir, lang))
		if err != nil {
			errorAt(Dir+"/"+lang, 1, RuleSource, lang+" cannot be read: "+err.Error()+".", "Make the folder readable.")
			continue
		}
		for _, f := range files {
			if key, ok := strings.CutSuffix(f.Name(), ".md"); ok && !f.IsDir() {
				names[key] = true
			}
		}
		keys := make([]string, 0, len(r.Plan.Documents))
		for _, d := range r.Plan.Documents {
			keys = append(keys, d.Key)
		}
		for _, k := range slices.Sorted(maps.Keys(names)) {
			if !planned[k] {
				warn(Dir+"/"+lang+"/"+k+".md", 1, RuleConcept, fmt.Sprintf("%s/%s.md is not in the concept's Documents table.", lang, k), "Plan every document in the concept, or remove the file.")
				keys = append(keys, k)
			}
		}
		// Each goroutine fills its own slot of this language's slice; the
		// map is only written here, before any of them starts.
		slots := make([]parsed, len(keys))
		results[lang] = slots
		for i, key := range keys {
			if !names[key] {
				slots[i] = parsed{missing: true, key: key}
				continue
			}
			wg.Go(func() {
				file := Dir + "/" + lang + "/" + key + ".md"
				src, err := p.Read(file)
				if err != nil {
					slots[i] = parsed{err: err, key: key}
					return
				}
				d, problems := parseDoc(key, lang, file, src)
				slots[i] = parsed{doc: d, problems: problems, key: key}
			})
		}
	}
	wg.Wait()
	for _, lang := range langs {
		for _, res := range results[lang] {
			if res.missing {
				errorAt(Dir+"/"+lang, 1, RuleSource, fmt.Sprintf("%s has no %s.md.", lang, res.key), "Every language needs every document of the concept; translate it, or plan it out of the concept.")
				continue
			}
			if res.err != nil {
				errorAt(Dir+"/"+lang+"/"+res.key+".md", 1, RuleSource, res.err.Error()+".", "Make the file readable inside the project.")
				continue
			}
			if res.doc == nil {
				continue
			}
			for _, pr := range res.problems {
				errorAt(res.doc.File, pr.Line, RuleSource, pr.Message, pr.Hint)
			}
			r.Docs[lang] = append(r.Docs[lang], res.doc)
			r.lint(p, res.doc, errorAt, warn)
		}
		for _, paper := range paper.For(lang, r.Config.Papers) {
			if _, ok := Papers[paper]; ok {
				r.Editions = append(r.Editions, Edition{Key: lang + "-" + paper, Lang: lang, Paper: paper})
				r.Summary.Editions = append(r.Summary.Editions, lang+"-"+paper)
			}
		}
		r.Summary.Languages = append(r.Summary.Languages, lang)
	}
	if len(langs) > 0 {
		r.Summary.Documents = len(r.Docs[langs[0]])
	}
}

// lint holds a document to the design and content rules.
func (r *Report) lint(p project.Project, d *Doc, errorAt, warn func(string, int, string, string, string)) {
	fields := 0
	missing := map[rune]int{}
	textCheck := func(line int, ins []Inline) {
		s := plain(ins)
		for _, in := range ins {
			if in.Fill {
				fields++
			}
		}
		for _, is := range markup.Sample(s, typeface.Symbols) {
			warn(d.File, line, RuleContent, is.Message, is.Hint)
		}
		for _, c := range s {
			if r.text != nil && c > ' ' && !r.text.Regular().Covers(c) {
				missing[c] = line
			}
		}
	}
	var walk func(bs []Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			textCheck(b.Line, b.Inlines)
			for _, it := range b.Items {
				textCheck(b.Line, it.Inlines)
			}
			for _, f := range b.Fields {
				textCheck(b.Line, f.Label)
				textCheck(b.Line, f.Value)
			}
			if t := b.Table; t != nil {
				for _, c := range t.Head {
					textCheck(b.Line, c)
				}
				for _, row := range t.Rows {
					for _, c := range row {
						textCheck(b.Line, c)
					}
				}
				if len(t.Align) > 7 {
					warn(d.File, b.Line, RuleDesign, fmt.Sprintf("A table with %d columns.", len(t.Align)), "Keep tables to seven columns on a page; split it or move details into a list.")
				}
				for i, a := range t.Align {
					if a == "left" && markup.Numeric(t, i) {
						warn(d.File, b.Line, RuleDesign, fmt.Sprintf("Column %d holds numbers but is aligned left.", i+1), "Leave the column's alignment out (---) and lfm aligns numbers right, so digits line up.")
					}
				}
			}
			if img := b.Image; img != nil {
				r.image(p, d, b.Line, img, errorAt, warn)
			}
			walk(b.Children)
			for _, c := range b.Cols {
				walk(c)
			}
		}
	}
	walk(d.Blocks)
	for _, c := range sortedRunes(missing) {
		warn(d.File, missing[c], RuleFonts, fmt.Sprintf("%s does not draw %q (U+%04X).", r.Theme.Text, c, c), "Apps fill it in from another font, which shows; write it differently or pick a font that has it.")
	}
	if fields == 0 {
		warn(d.File, 1, RuleContent, d.Key+" has no fields for the buyer to replace.", "Mark what the buyer changes as ==…==, such as ==Margaret Ellison==; lfm highlights it in every app.")
	}
	r.Summary.Fields += fields
}

func sortedRunes(m map[rune]int) []rune {
	out := make([]rune, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// image loads a document's picture once and checks it prints sharply.
func (r *Report) image(p project.Project, d *Doc, line int, img *Figure, errorAt, warn func(string, int, string, string, string)) {
	if strings.Contains(img.Src, "://") || strings.Contains(img.Src, "..") || !strings.HasPrefix(img.Src, "images/") {
		errorAt(d.File, line, RuleSource, fmt.Sprintf("Image %q is not in images/.", img.Src), "Put pictures in "+ImagesDir+" and reference them as images/name.png.")
		return
	}
	pic, ok := r.images[img.Src]
	if !ok {
		data, err := p.Read(Dir + "/" + img.Src)
		if err != nil {
			errorAt(d.File, line, RuleSource, fmt.Sprintf("Image %s does not exist.", img.Src), "Add the file to "+ImagesDir+".")
			return
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || (format != "png" && format != "jpeg") {
			errorAt(d.File, line, RuleSource, fmt.Sprintf("Image %s is not a PNG or JPEG.", img.Src), "Every app shows PNG and JPEG; convert it.")
			return
		}
		pic = &picture{Data: data, Ext: format, Px: cfg.Width, Py: cfg.Height}
		r.images[img.Src] = pic
	}
	if r.text == nil {
		return
	}
	width, _ := imageSize(img, pic, layoutTheme(r.Theme, Papers["a4"], r.text, r.head).TextWidth)
	if dpi := float64(pic.Px) / (width / 25.4); dpi < 150 {
		warn(d.File, line, RuleDesign, fmt.Sprintf("%s prints at %.0f dpi.", img.Src, dpi), "Use a picture of at least 150 dpi at its printed width, 300 for photos, or print it smaller.")
	}
}

// checkLinks counts the Google Docs masters and warns about links that
// are not Google Docs documents.
func (r *Report) checkLinks() {
	for _, key := range slices.Sorted(maps.Keys(r.Config.Docs)) {
		if CopyLink(r.Config.Docs[key]) == "" {
			r.Diagnostics = append(r.Diagnostics, at("lfm.json", 1, RuleConfig, diagnostic.Warning, fmt.Sprintf("deliverables.document.docs %q is not a Google Docs document link.", key), "Paste the address of the shared Google Docs master, https://docs.google.com/document/d/…/edit."))
			continue
		}
		r.Summary.Linked++
	}
}

// CopyLink turns a Google Docs master's address into the link that makes
// the buyer's own copy.
func CopyLink(url string) string { return deliver.CopyLink(url, "document") }

// Slug names the product's files.
func (r Report) Slug() string { return deliver.Slug(r.Plan.Name, "documents") }

// Showcase is the edition the listing shows: the first in the listing's
// language, else the first.
func (r Report) Showcase() Edition {
	for _, e := range r.Editions {
		if e.Lang == strings.ToLower(r.Config.Language) {
			return e
		}
	}
	for _, e := range r.Editions {
		if base, _, _ := strings.Cut(e.Lang, "-"); base == strings.ToLower(r.Config.Language) {
			return e
		}
	}
	if len(r.Editions) > 0 {
		return r.Editions[0]
	}
	return Edition{}
}
