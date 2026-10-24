package book

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/deliver"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/imaging"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/research"
)

// Summary describes the book for the report, the next step and --json.
type Summary struct {
	Concept  bool      `json:"concept"`
	Title    string    `json:"name,omitempty"`
	Chapters []Chapter `json:"chapters,omitempty"`
	Planned  int       `json:"planned"`
	Written  int       `json:"written"`
	Words    int       `json:"words"`
	Target   int       `json:"targetWords"`
	Complete bool      `json:"complete"`
	URL      string    `json:"url,omitempty"`
	Files    []string  `json:"files,omitempty"` // built formats, set by the build
	Listing  bool      `json:"listing"`
	Pending  bool      `json:"pending"`            // the PDF and paperback wait for a release to print
	Layout   string    `json:"layout,omitempty"`   // release: "fits", or the number of layout findings
	Channels []string  `json:"channels,omitempty"` // store packages written to distribution/
	Sell     string    `json:"sell"`               // page, shops or bundle
}

// Next returns the first unwritten active chapter, then the first planned chapter.
func (s Summary) Next() *Chapter {
	for i := range s.Chapters {
		if s.Chapters[i].Status != "planned" && !s.Chapters[i].Exists {
			return &s.Chapters[i]
		}
	}
	for i := range s.Chapters {
		if s.Chapters[i].Status == "planned" {
			return &s.Chapters[i]
		}
	}
	return nil
}

// Report is the result of checking the book.
type Report struct {
	Diagnostics []diagnostic.Diagnostic
	Summary     Summary
	Plan        Plan
	Config      Config
	Sources     map[string][]byte // book-relative path -> bytes of every chapter, image and style read
	Listing     Listing
	Buildable   bool // config valid and every chapter written; the build may run
}

// Present reports whether the project has a book.
func Present(p project.Project) bool {
	dir, err := p.Path(Dir)
	if err != nil {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// Check reads the book of the project. configured tells whether lfm.json has
// deliverables.book; site is the page URL the download path hangs off.
func Check(p project.Project, cfg Config, configured bool, offers map[string]*Price, site string, voices *research.Voices, competitors *research.Competitors) Report {
	if cfg.Kind == "" {
		cfg.Kind = "nonfiction"
	}
	r := Report{Config: cfg, Sources: map[string][]byte{}}
	// Config findings name the lfm.json key they are about; the compiler
	// resolves it to its line.
	errorAt := func(key string, rule, message, hint string) {
		d := at(project.ConfigFile, 1, rule, diagnostic.Error, message, hint)
		d.Path = key
		r.Diagnostics = append(r.Diagnostics, d)
	}
	configOK := true
	switch {
	case !configured:
		configOK = false
		errorAt("deliverables", RuleConfig, "deliverables/book exists but lfm.json has no deliverables.book.",
			fmt.Sprintf(`Add "deliverables": {"book": {"author": "<name on the cover>", "path": %q}} to lfm.json; the path is the unlisted folder buyers download from. 'lfm help config deliverables.book' lists the options.`, deliver.NewPath()))
	default:
		if deliver.Sell(cfg.Sell) == deliver.SellPage && !deliver.ValidPath(cfg.Path) {
			configOK = false
			errorAt("deliverables.book.path", RuleConfig, fmt.Sprintf("deliverables.book.path %q is not an unguessable folder name.", cfg.Path),
				fmt.Sprintf("Use at least %d lowercase letters, digits or single hyphens, for example %q. Anyone who knows the path can download the book.", deliver.MinPathLength, deliver.NewPath()))
		}
		if _, ok := offers[cfg.Offer]; cfg.Offer != "" && !ok {
			configOK = false
			errorAt("deliverables.book.offer", RuleConfig, fmt.Sprintf("deliverables.book.offer %q names no offer in data.commerce.offers.", cfg.Offer), "Use the key of the offer that sells the book.")
		}
		for _, isbn := range []struct{ field, value string }{{"isbn.ebook", cfg.ISBNEbook}, {"isbn.paperback", cfg.ISBNPaperback}} {
			if isbn.value != "" && !ValidISBN(isbn.value) {
				configOK = false
				errorAt("deliverables.book."+isbn.field, RuleConfig, fmt.Sprintf("deliverables.book.%s %q is not a valid ISBN-13.", isbn.field, isbn.value), "Copy the ISBN exactly as assigned, 13 digits starting with 978 or 979; an ebook and a paperback each need their own.")
			}
		}
		if digits := strings.NewReplacer("-", "", " ", ""); cfg.ISBNEbook != "" && digits.Replace(cfg.ISBNEbook) == digits.Replace(cfg.ISBNPaperback) {
			configOK = false
			errorAt("deliverables.book.isbn", RuleConfig, "The ebook and the paperback share one ISBN.", "Every edition needs its own ISBN.")
		}
	}
	if cfg.Kind != "fiction" && cfg.Kind != "nonfiction" {
		configOK = false
		errorAt("deliverables.book.kind", RuleConfig, "deliverables.book.kind must be nonfiction or fiction.", "Choose nonfiction or fiction.")
	}
	r.Summary.Sell = deliver.Sell(cfg.Sell)
	if configOK && r.Summary.Sell == deliver.SellPage {
		r.Summary.URL = strings.TrimSuffix(site, "/") + "/" + cfg.Path + "/"
	}

	raw, err := p.Read(ConceptFile)
	if err != nil {
		return r // the next step asks for the concept
	}
	r.Summary.Concept = true
	r.Plan = checkConcept(string(raw), voices, competitors, func(rule string, line int, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at(ConceptFile, line, rule, diagnostic.Warning, message, hint))
	}, cfg.Kind)
	r.Summary.Title = r.Plan.Title

	var sources sync.Mutex // chapters are checked side by side
	exists := func(rel string) bool {
		sources.Lock()
		defer sources.Unlock()
		if _, ok := r.Sources[rel]; ok {
			return true
		}
		if outside(rel) {
			return false
		}
		b, err := p.Read(Dir + "/" + rel)
		if err != nil {
			return false
		}
		r.Sources[rel] = b
		return true
	}
	// Each chapter is checked on its own; the report keeps the plan's order.
	type checked struct {
		src    []byte
		err    error
		pr     prose
		issues []diagnostic.Diagnostic
	}
	results := make([]checked, len(r.Plan.Chapters))
	var wg sync.WaitGroup
	for i, ch := range r.Plan.Chapters {
		wg.Go(func() {
			c := &results[i]
			if ch.Status == "planned" {
				return
			}
			if c.src, c.err = p.Read(Dir + "/" + ch.File); c.err == nil {
				ch.Exists = true
				c.pr, c.issues = checkChapter(ch, c.src, r.Plan, cfg, exists)
			}
		})
	}
	wg.Wait()
	var all []byte
	var texts []chapterText
	for i, ch := range r.Plan.Chapters {
		r.Summary.Target += ch.Words
		if ch.Status == "planned" {
			r.Summary.Planned++
			r.Summary.Chapters = append(r.Summary.Chapters, ch)
			continue
		}
		if c := results[i]; c.err == nil {
			ch.Exists = true
			r.Sources[ch.File] = c.src
			texts = append(texts, chapterText{Dir + "/" + ch.File, c.pr.blocks})
			r.Summary.Written++
			r.Summary.Words += c.pr.words
			r.Diagnostics = append(r.Diagnostics, c.issues...)
			all = append(append(all, c.src...), '\n')
		}
		r.Summary.Chapters = append(r.Summary.Chapters, ch)
	}
	r.Summary.Complete = len(r.Plan.Chapters) >= MinChapters && r.Summary.Planned == 0 && r.Summary.Written == len(r.Plan.Chapters)
	for _, ch := range r.Summary.Chapters {
		if ch.Status != "draft" && ch.Status != "done" {
			r.Summary.Complete = false
		}
	}
	repetition(texts, func(file string, line int, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at(file, line, repetitionRule(cfg.Kind), diagnostic.Warning, message, hint))
	})
	if r.Summary.Complete && cfg.Kind != "fiction" {
		for i, phrase := range r.Plan.Use {
			if !containsPhrase(string(all), phrase) {
				r.Diagnostics = append(r.Diagnostics, at(ConceptFile, r.Plan.UseLines[i], RuleCopy, diagnostic.Warning, fmt.Sprintf("The book never says %q, which the Voice uses.", phrase), "Use the readers' phrase where it fits, or change the Voice when the decision changed."))
			}
		}
	}
	r.checkMatter(exists)
	r.checkImages(exists)
	styleOK := true
	if raw, err := p.Read(Dir + "/" + StyleFile); err == nil {
		r.Sources[StyleFile] = raw
	} else if !errors.Is(err, fs.ErrNotExist) {
		styleOK = false
		r.Diagnostics = append(r.Diagnostics, at(Dir+"/"+StyleFile, 1, RuleBuild, diagnostic.Error, err.Error()+".", "Make the file readable inside the project."))
	}
	r.checkFonts()
	if raw, err := p.Read(ListingFile); err == nil {
		r.Listing = checkListing(string(raw), r.Plan, offers[cfg.Offer], cfg.Language, func(rule string, line int, message, hint string) {
			r.Diagnostics = append(r.Diagnostics, at(ListingFile, line, rule, diagnostic.Warning, message, hint))
		})
		if cfg.Kind == "fiction" {
			listingText := strings.Join(append([]string{r.Listing.Description, r.Listing.Short, r.Listing.BackCover}, append(r.Listing.Keywords, r.Listing.Tags...)...), "\n")
			for i, phrase := range r.Plan.Use {
				if !containsPhrase(listingText, phrase) {
					r.Diagnostics = append(r.Diagnostics, at(ConceptFile, r.Plan.UseLines[i], RuleCopy, diagnostic.Warning, fmt.Sprintf("The listing never says %q, which the Voice uses.", phrase), "Use the phrase in the listing or update the Voice."))
				}
			}
		}
	} else if r.Summary.Complete {
		r.Diagnostics = append(r.Diagnostics, at(ConceptFile, 1, RuleListing, diagnostic.Warning, "The book has no listing.md for the stores.",
			"Write deliverables/book/listing.md as the lfm-book skill shows: descriptions, keywords, categories, prices and AI use, which lfm renders for every channel."))
	}
	r.Summary.Listing = r.Listing.Present
	r.Buildable = configOK && styleOK && r.Summary.Complete
	return r
}

// checkImages verifies the planned images and the cover.
func (r *Report) checkImages(exists func(string) bool) {
	used := map[string]bool{}
	for _, ch := range r.Plan.Chapters {
		if src, ok := r.Sources[ch.File]; ok {
			_, p := parseChapter(src)
			for _, img := range p.images {
				used[path.Clean(path.Join(path.Dir(ch.File), img.dest))] = true
			}
		}
	}
	for _, img := range r.Plan.Images {
		if img.Open {
			if imageExt(img.File) {
				exists(img.File)
			}
			continue
		}
		if img.File == "" || img.File == "." || !imageExt(img.File) {
			continue
		}
		if !exists(img.File) {
			r.Diagnostics = append(r.Diagnostics, at(ConceptFile, img.Line, RuleImages, diagnostic.Warning, fmt.Sprintf("Planned image %s does not exist yet.", img.File), "Produce it for its job, or mark the cell Open: when it waits for the owner."))
			continue
		}
		if img.Job == "cover" {
			info := imaging.Inspect(r.Sources[img.File])
			switch {
			case info.Width == 0:
				r.Diagnostics = append(r.Diagnostics, at(ConceptFile, img.Line, RuleImages, diagnostic.Warning, "The cover's dimensions cannot be read.", "Save it as a standard JPEG, PNG or WebP."))
			case info.Width < CoverWidth || info.Height < CoverHeight:
				r.Diagnostics = append(r.Diagnostics, at(ConceptFile, img.Line, RuleImages, diagnostic.Warning, fmt.Sprintf("The cover is %d×%d; stores want at least %d×%d.", info.Width, info.Height, CoverWidth, CoverHeight), "Render or export the cover at 1600×2560 or larger."))
			default:
				if ratio := float64(info.Height) / float64(info.Width); ratio < 1.5 || ratio > 1.7 {
					r.Diagnostics = append(r.Diagnostics, at(ConceptFile, img.Line, RuleImages, diagnostic.Warning, fmt.Sprintf("The cover's ratio is 1:%.2f; ebook covers are 1:1.6.", ratio), "Crop or extend it to 1600×2560."))
				}
			}
			continue
		}
		if r.Summary.Complete && !used[img.File] {
			r.Diagnostics = append(r.Diagnostics, at(ConceptFile, img.Line, RuleImages, diagnostic.Warning, fmt.Sprintf("Planned image %s is used in no chapter.", img.File), "Place it where the concept gives it a job, or remove the row."))
		}
	}
}

var (
	fontFace   = regexp.MustCompile(`(?is)@font-face\s*\{[^}]*font-family\s*:\s*['"]?([^;'"}]+)`)
	systemFace = map[string]bool{"serif": true, "sans-serif": true, "monospace": true, "system-ui": true, "ui-serif": true, "ui-sans-serif": true,
		"georgia": true, "palatino": true, "palatino linotype": true, "times new roman": true, "times": true, "helvetica": true, "arial": true}
)

// checkFonts warns when the concept's first face would be replaced by a fallback.
func (r *Report) checkFonts() {
	declared := map[string]bool{}
	for _, m := range fontFace.FindAllStringSubmatch(string(r.Sources[StyleFile]), -1) {
		declared[strings.ToLower(strings.TrimSpace(m[1]))] = true
	}
	for _, f := range []struct{ role, stack string }{{"Body", r.Plan.Body}, {"Headings", r.Plan.Headings}} {
		first := strings.ToLower(strings.Trim(strings.TrimSpace(strings.Split(f.stack, ",")[0]), "'\""))
		if first == "" || declared[first] || systemFace[first] {
			continue
		}
		r.Diagnostics = append(r.Diagnostics, at(ConceptFile, max(1, r.Plan.LookLine), RuleFonts, diagnostic.Warning,
			fmt.Sprintf("The %s face %q is not embedded; the book would print in a fallback.", f.role, strings.TrimSpace(strings.Split(f.stack, ",")[0])),
			"Add its font files under deliverables/book/fonts with an @font-face rule in deliverables/book/book.css, or name a face that is embedded."))
	}
}

func repetitionRule(kind string) string {
	if kind == "fiction" {
		return RuleCopy
	}
	return RuleValue
}

func (r *Report) checkMatter(exists func(string) bool) {
	for i := range r.Plan.Matter {
		m := &r.Plan.Matter[i]
		m.Exists = exists(m.File)
		if !m.Exists {
			r.Diagnostics = append(r.Diagnostics, at(ConceptFile, m.Line, RuleConcept, diagnostic.Warning, "Matter file "+m.File+" does not exist yet.", "Write the file or mark the row Open:."))
			continue
		}
		src := r.Sources[m.File]
		_, p := parseChapter(src)
		if len(p.headings) == 0 || p.headings[0].level != 1 || !strings.EqualFold(strings.TrimSpace(p.headings[0].text), strings.TrimSpace(check.Straight(m.Part))) {
			r.Diagnostics = append(r.Diagnostics, at(Dir+"/"+m.File, 1, RuleChapter, diagnostic.Warning, fmt.Sprintf("Matter does not start with %q.", "# "+m.Part), "Open the file with its planned part heading."))
		}
		for _, phrase := range r.Plan.Avoid {
			if i := strings.Index(strings.ToLower(check.Straight(string(src))), strings.ToLower(check.Straight(phrase))); i >= 0 {
				r.Diagnostics = append(r.Diagnostics, at(Dir+"/"+m.File, newLines(src).at(i), RuleCopy, diagnostic.Warning, fmt.Sprintf("The matter says %q, which the Voice avoids.", phrase), "Replace the phrase or update the Voice."))
			}
		}
	}
}
