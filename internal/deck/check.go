package deck

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // image sizes
	_ "image/png"
	"maps"
	"math"
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
	"github.com/Anonyfox/lfm/internal/theme"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// Edition is one language: the unit buyers download. Its notes print on
// every paper the language uses.
type Edition struct {
	Key, Lang string
	Papers    []string
}

// Summary is what the compile report shows.
type Summary struct {
	Name      string   `json:"name,omitempty"`
	Concept   bool     `json:"concept"`
	Theme     string   `json:"theme,omitempty"`
	Decks     int      `json:"decks"`
	Slides    int      `json:"slides"`  // in the decks of one language
	Quizzes   int      `json:"quizzes"` // questions in them
	Minutes   int      `json:"minutes,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Editions  []string `json:"editions,omitempty"`
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

// Report is the checked deck set.
type Report struct {
	Config      Config
	Summary     Summary
	Plan        Plan
	Theme       theme.Theme
	Diagnostics []diagnostic.Diagnostic
	Buildable   bool
	Editions    []Edition
	Decks       map[string][]*Deck // by language, in the concept's order
	Listing     deliver.ShopListing
	text, head  *typeface.Family
	images      map[string]*picture
	licences    map[string][]byte
	digest      []byte
	pack        *typeface.Pack
}

// picture is an image a deck shows.
type picture struct {
	Data   []byte
	Ext    string // png or jpeg
	Px, Py int
}

// Paper is a page size in millimetres.
type Paper = paper.Size

// Papers are the sizes speaker notes print on.
var Papers = paper.Office

func at(file string, line int, rule string, sev diagnostic.Severity, message, hint string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Rule: rule, Severity: sev, Location: diagnostic.Location{File: file, Line: max(line, 1), Column: 1}, Message: message, Hint: hint}
}

var langDir = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})?$`)

// design is the report's theme laid out on a slide.
func (r Report) design() design { return newDesign(r.Theme, r.text, r.head) }

// Check reads and checks the decks: concept, fonts, every deck of every
// language, every slide, and the listing.
func Check(p project.Project, cfg Config, configured bool, offers map[string]*deliver.Price, site string) Report {
	r := Report{Config: cfg, Decks: map[string][]*Deck{}, images: map[string]*picture{}, licences: map[string][]byte{}, pack: typeface.NewPack()}
	r.Summary.Sell = deliver.Sell(cfg.Sell)
	fail := func(message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at("lfm.json", 1, RuleConfig, diagnostic.Error, message, hint))
	}
	switch {
	case !configured:
		fail("deliverables/deck exists, but lfm.json has no deliverables.deck.",
			fmt.Sprintf(`Add "deliverables": {"deck": {"path": %q}} to lfm.json. 'lfm help config deliverables.deck' lists the options.`, deliver.NewPath()))
	case r.Summary.Sell != deliver.SellPage:
	case !deliver.ValidPath(cfg.Path):
		fail(fmt.Sprintf("deliverables.deck.path %q is not an unguessable folder name.", cfg.Path),
			fmt.Sprintf("Use at least %d lowercase letters, digits or single hyphens, for example %q. Anyone who knows the path can download the files.", deliver.MinPathLength, deliver.NewPath()))
	default:
		r.Summary.URL = strings.TrimSuffix(site, "/") + "/" + cfg.Path + "/"
	}
	for _, paper := range cfg.Papers {
		if _, ok := Papers[paper]; !ok {
			fail(fmt.Sprintf("deliverables.deck.papers names %q.", paper), "Use a4 and letter.")
		}
	}
	var offer *deliver.Price
	if cfg.Offer != "" {
		o, ok := offers[cfg.Offer]
		if !ok {
			fail(fmt.Sprintf("deliverables.deck.offer %q names no offer in data.commerce.offers.", cfg.Offer), "Use the key of the offer that sells the decks.")
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
	r.Summary.Theme = r.Theme.Name

	// Until the concept is written there is nothing to hold to it: its Look
	// names the fonts, and the decks follow it.
	if !r.Summary.Concept {
		return r
	}
	report := func(file string, line int, fatal bool, message, hint string) {
		if fatal {
			errorAt(file, line, RuleFonts, message, hint)
		} else {
			warn(file, line, RuleFonts, message, hint)
		}
	}
	folder := typeface.Load(p, FontsDir, report)
	r.licences = folder.Licences
	r.text = folder.Want(r.Theme.Text, "text", ConceptFile, r.Plan.Lines["Look"], report)
	r.head = r.text
	if r.Theme.Heading != r.Theme.Text {
		r.head = folder.Want(r.Theme.Heading, "heading", ConceptFile, r.Plan.Lines["Look"], report)
	}
	for _, problem := range r.Theme.ColorProblems() {
		errorAt(ConceptFile, r.Plan.Lines["Look"], RuleDesign, problem, "Change the accent in the concept's Look, or pick another theme.")
	}
	// The cover sets the title in white on the accent.
	if c := theme.Contrast(r.Theme.Accent, "#FFFFFF"); c < 4.5 {
		errorAt(ConceptFile, r.Plan.Lines["Look"], RuleDesign, fmt.Sprintf("The cover's white title on the accent %s has a contrast of %.1f:1; it needs 4.5:1.", r.Theme.Accent, c), "Darken the accent.")
	}
	r.readDecks(p, errorAt, warn)
	r.Buildable = r.text != nil && r.head != nil && len(r.Editions) > 0
	for _, d := range r.Diagnostics {
		if d.Severity == diagnostic.Error && (d.Rule == RuleSource || d.Rule == RuleFonts || d.Rule == RuleConfig) {
			r.Buildable = false
		}
	}
	r.digest = r.inputs()
	for _, key := range slices.Sorted(maps.Keys(r.Config.Slides)) {
		if CopyLink(r.Config.Slides[key]) == "" {
			r.Diagnostics = append(r.Diagnostics, at("lfm.json", 1, RuleConfig, diagnostic.Warning, fmt.Sprintf("deliverables.deck.slides %q is not a Google Slides link.", key), "Paste the address of the shared Google Slides master, https://docs.google.com/presentation/d/…/edit."))
			continue
		}
		r.Summary.Linked++
	}
	if lraw, err := p.Read(ListingFile); err == nil {
		r.Listing = deliver.CheckShopListing(string(lraw), offer, func(line int, message, hint string) { warn(ListingFile, line, RuleListing, message, hint) })
		r.Summary.Listing = true
	}
	return r
}

// readDecks parses every language folder and checks each deck.
func (r *Report) readDecks(p project.Project, errorAt, warn func(string, int, string, string, string)) {
	dir, err := p.Path(Dir)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		errorAt(Dir, 1, RuleSource, "deliverables/deck cannot be read: "+err.Error()+".", "Make the folder readable.")
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
		errorAt(Dir, 1, RuleSource, "deliverables/deck has no language folder.", "Write each deck as Markdown in a folder per language, such as en/briefing.md and de/briefing.md.")
		return
	}
	planned := map[string]bool{}
	for _, d := range r.Plan.Decks {
		planned[d.Key] = true
	}
	type parsed struct {
		deck     *Deck
		problems []markup.Problem
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
		var keys []string
		for _, d := range r.Plan.Decks {
			keys = append(keys, d.Key)
		}
		for _, k := range slices.Sorted(maps.Keys(names)) {
			if !planned[k] {
				warn(Dir+"/"+lang+"/"+k+".md", 1, RuleConcept, fmt.Sprintf("%s/%s.md is not in the concept's Decks table.", lang, k), "Plan every deck in the concept, or remove the file.")
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
				d, problems := parse(key, lang, file, src)
				slots[i] = parsed{deck: d, problems: problems, key: key}
			})
		}
	}
	wg.Wait()
	for _, lang := range langs {
		for _, res := range results[lang] {
			if res.missing {
				errorAt(Dir+"/"+lang, 1, RuleSource, fmt.Sprintf("%s has no %s.md.", lang, res.key), "Every language needs every deck of the concept; translate it slide for slide, or plan it out of the concept.")
				continue
			}
			if res.err != nil {
				errorAt(Dir+"/"+lang+"/"+res.key+".md", 1, RuleSource, res.err.Error()+".", "Make the file readable inside the project.")
				continue
			}
			if res.deck == nil {
				continue
			}
			for _, pr := range res.problems {
				errorAt(res.deck.File, pr.Line, RuleSource, pr.Message, pr.Hint)
			}
			r.Decks[lang] = append(r.Decks[lang], res.deck)
			r.lint(p, res.deck, errorAt, warn)
		}
		r.Editions = append(r.Editions, Edition{Key: lang, Lang: lang, Papers: paper.For(lang, r.Config.Papers)})
		r.Summary.Editions = append(r.Summary.Editions, lang)
		r.Summary.Languages = append(r.Summary.Languages, lang)
	}
	first := map[string]int{}
	for _, d := range r.Decks[langs[0]] {
		r.Summary.Decks++
		r.Summary.Slides += len(d.Slides)
		r.Summary.Minutes += d.Minutes
		for _, s := range d.Slides {
			if s.Quiz != nil {
				r.Summary.Quizzes++
			}
		}
		first[d.Key] = len(d.Slides)
	}
	// A translated deck keeps its slides: a presenter switching languages,
	// the quiz sheet and the listing's slide count rely on it.
	for _, lang := range langs[1:] {
		for _, d := range r.Decks[lang] {
			if n, ok := first[d.Key]; ok && len(d.Slides) != n {
				warn(d.File, 1, RuleContent, fmt.Sprintf("%s/%s has %d slides; %s/%s has %d.", lang, d.Key, len(d.Slides), langs[0], d.Key, n), "Translate slide for slide, so every language's deck is the same.")
			}
		}
	}
}

// trademark are game-show and quiz brands whose owners enforce their marks.
var trademark = regexp.MustCompile(`(?i)\b(jeopardy|family feud|wheel of fortune|kahoot|who wants to be a millionaire|wer wird millionär|pictionary|scrabble|monopoly|trivial pursuit|bingo blitz|price is right)\b`)

// lint holds a deck to the content and design rules.
func (r *Report) lint(p project.Project, d *Deck, errorAt, warn func(string, int, string, string, string)) {
	missing := map[rune]int{}
	textCheck := func(line int, ins []Inline) {
		s := plain(ins)
		for _, is := range markup.Sample(s, typeface.Symbols) {
			warn(d.File, line, RuleContent, is.Message, is.Hint)
		}
		if m := trademark.FindString(s); m != "" {
			warn(d.File, line, RuleContent, fmt.Sprintf("%q is a trademark.", m), "Name the game generically, such as quiz board or family quiz; shops take listings down for brand names.")
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
			if t := b.Table; t != nil {
				for _, c := range t.Head {
					textCheck(b.Line, c)
				}
				for _, row := range t.Rows {
					for _, c := range row {
						textCheck(b.Line, c)
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
	textCheck(1, []Inline{{Text: d.Title}})
	r.slideProblems(d, errorAt, warn)
	runes := make([]rune, 0, len(missing))
	for c := range missing {
		runes = append(runes, c)
	}
	slices.Sort(runes)
	for _, c := range runes {
		warn(d.File, missing[c], RuleFonts, fmt.Sprintf("%s does not draw %q (U+%04X).", r.Theme.Text, c, c), "Apps fill it in from another font, which shows; write it differently or pick a font that has it.")
	}
}

// image loads a deck's picture once and checks it stays sharp on a
// projector.
func (r *Report) image(p project.Project, d *Deck, line int, img *Figure, errorAt, warn func(string, int, string, string, string)) {
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
	if strings.TrimSpace(img.Alt) == "" {
		warn(d.File, line, RuleDesign, fmt.Sprintf("Image %s has no description.", img.Src), "Describe it in the brackets: ![Escape plan with both exits](images/plan.png); screen readers and PowerPoint's accessibility check read it.")
	}
	// A 1080p projector shows the slide's 13.3 inches in 1920 pixels.
	b := pictureBox(r.design(), pic)
	if ppi := float64(pic.Px) / (b.W / 72); ppi < 144 {
		warn(d.File, line, RuleDesign, fmt.Sprintf("%s shows at %.0f pixels per inch on its slide.", img.Src, ppi), fmt.Sprintf("Use a picture at least %.0f pixels wide, so it stays sharp on a 1080p projector.", math.Ceil(b.W/72*144)))
	}
}

// CopyLink turns a Google Slides master's address into the link that
// makes the buyer's own copy.
func CopyLink(url string) string { return deliver.CopyLink(url, "presentation") }

// Slug names the product's files.
func (r Report) Slug() string { return deliver.Slug(r.Plan.Name, "decks") }

// Showcase is the edition the listing shows: the listing's language, else
// the first.
func (r Report) Showcase() Edition {
	for _, e := range r.Editions {
		if base, _, _ := strings.Cut(e.Lang, "-"); e.Lang == strings.ToLower(r.Config.Language) || base == strings.ToLower(r.Config.Language) {
			return e
		}
	}
	if len(r.Editions) > 0 {
		return r.Editions[0]
	}
	return Edition{}
}
