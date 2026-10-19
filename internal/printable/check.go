package printable

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

// Summary is what the report and --json show about the printable.
type Summary struct {
	Name     string   `json:"name,omitempty"`
	Concept  bool     `json:"concept"`
	Template bool     `json:"template"`
	Editions []string `json:"editions,omitempty"`
	Pages    int      `json:"pages"` // per edition, as the first one renders
	Links    int      `json:"links"`
	Cut      int      `json:"cut,omitempty"` // pages to cut out (data-role="cut")
	URL      string   `json:"url,omitempty"`
	Files    []string `json:"files,omitempty"`
	Pending  bool     `json:"pending,omitempty"` // the PDFs print with --release
	Listing  bool     `json:"listing"`
	Channels []string `json:"channels,omitempty"`
	Images   int      `json:"images,omitempty"` // listing images rendered
	Layout   string   `json:"layout,omitempty"`
	Sell     string   `json:"sell"` // page, shops or bundle
}

// Report is the checked printable.
type Report struct {
	Config      Config
	Plan        Plan
	Summary     Summary
	Diagnostics []diagnostic.Diagnostic
	Editions    []Rendered
	Assets      map[string][]byte // flat name -> bytes, for printing
	Listing     deliver.ShopListing
	Buildable   bool // every edition rendered without errors
}

func at(file string, line int, rule string, sev diagnostic.Severity, message, hint string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Rule: rule, Severity: sev, Location: diagnostic.Location{File: file, Line: max(1, line), Column: 1}, Message: message, Hint: hint}
}

// Check reads deliverables/printable. configured tells whether lfm.json has
// deliverables.printable; site is the page URL the download path hangs off;
// offers are the page's offers by key.
func Check(p project.Project, cfg Config, configured bool, offers map[string]*deliver.Price, site string) Report {
	r := Report{Config: cfg, Assets: map[string][]byte{}}
	r.Summary.Sell = deliver.Sell(cfg.Sell)
	warn := func(file string, line int, rule, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at(file, line, rule, diagnostic.Warning, message, hint))
	}
	fail := func(file string, line int, rule, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at(file, line, rule, diagnostic.Error, message, hint))
	}
	switch {
	case !configured:
		fail("lfm.json", 1, RuleConfig, "deliverables/printable exists, but lfm.json has no deliverables.printable.",
			fmt.Sprintf(`Add "deliverables": {"printable": {"path": %q}} to lfm.json; the path is the unlisted folder buyers download from. 'lfm help config deliverables.printable' lists the options.`, deliver.NewPath()))
	case deliver.Sell(cfg.Sell) != deliver.SellPage:
		// no download page: shops or the bundle deliver the files
	case !deliver.ValidPath(cfg.Path):
		fail("lfm.json", 1, RuleConfig, fmt.Sprintf("deliverables.printable.path %q is not an unguessable folder name.", cfg.Path),
			fmt.Sprintf("Use at least %d lowercase letters, digits or single hyphens, for example %q. Anyone who knows the path can download the files.", deliver.MinPathLength, deliver.NewPath()))
	default:
		r.Summary.URL = strings.TrimSuffix(site, "/") + "/" + cfg.Path + "/"
	}
	var offer *deliver.Price
	if cfg.Offer != "" {
		o, ok := offers[cfg.Offer]
		if !ok {
			fail("lfm.json", 1, RuleConfig, fmt.Sprintf("deliverables.printable.offer %q names no offer in data.commerce.offers.", cfg.Offer), "Use the key of the offer that sells the printable.")
		}
		offer = o
	}

	raw, err := p.Read(ConceptFile)
	if err != nil {
		return r
	}
	r.Summary.Concept = true
	r.Plan = checkConcept(string(raw), func(line int, message, hint string) { warn(ConceptFile, line, RuleConcept, message, hint) })
	r.Summary.Name = r.Plan.Name

	src, err := p.Read(TemplateFile)
	if err != nil {
		return r
	}
	r.Summary.Template = true
	style, _ := p.Read(Dir + "/" + StyleName)
	data := map[string][]byte{}
	if dir, err := p.Path(Dir + "/" + DataDir); err == nil {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				if b, err := p.Read(Dir + "/" + DataDir + "/" + e.Name()); err == nil {
					data[e.Name()] = b
				}
			}
		}
	}
	values, err := loadData(data)
	if err != nil {
		fail(Dir+"/"+DataDir, 1, RuleBuild, "A data file is not valid JSON: "+err.Error(), "Fix the JSON; the template reads it as .Data.<name>.")
		return r
	}
	t, err := parseTemplate(src, cfg.Language)
	if err != nil {
		fail(TemplateFile, errorLine(err), RuleBuild, "The template does not parse: "+err.Error(), "Fix the template syntax.")
		return r
	}
	editions := r.Plan.Editions()
	if len(editions) == 0 {
		return r
	}
	r.Buildable = true
	failed := map[string][]string{} // error -> editions it occurs in
	var order []error
	// Editions render in parallel; the template and its memo are shared.
	results := make([]Rendered, len(editions))
	errs := make([]error, len(editions))
	var wg sync.WaitGroup
	for i, e := range editions {
		wg.Go(func() {
			view := View{Edition: e, Name: r.Plan.Name, Promise: r.Plan.Promise, Parts: r.Plan.Parts, Language: cfg.Language, Shop: cfg.Shop, Version: cfg.Version, Data: values}
			results[i], errs[i] = render(t, view, r.Plan, string(style))
		})
	}
	wg.Wait()
	for i, e := range editions {
		rendered, err := results[i], errs[i]
		if err != nil {
			if failed[err.Error()] == nil {
				order = append(order, err)
			}
			failed[err.Error()] = append(failed[err.Error()], e.Label)
			r.Buildable = false
			continue
		}
		r.Editions = append(r.Editions, rendered)
		r.Summary.Editions = append(r.Summary.Editions, e.Label)
	}
	for _, err := range order {
		which := "The " + strings.Join(failed[err.Error()], ", ") + " edition does not render"
		if len(failed[err.Error()]) == len(editions) {
			which = "No edition renders"
		}
		fail(TemplateFile, errorLine(err), RuleBuild, which+": "+err.Error(), "Fix the template or its data; a generator error names what it needs.")
	}
	if !r.Buildable {
		return r
	}
	r.Summary.Pages = len(r.Editions[0].Pages)
	r.Summary.Links = r.Editions[0].Links
	for _, pg := range r.Editions[0].Pages {
		if pg.Role == "cut" {
			r.Summary.Cut++
		}
	}
	r.checkPages(p, warn)
	r.collectAssets(p, string(style), warn)
	r.checkFonts(string(style), warn)

	if lraw, err := p.Read(ListingFile); err == nil {
		r.Listing = deliver.CheckShopListing(string(lraw), offer, func(line int, message, hint string) { warn(ListingFile, line, RuleListing, message, hint) })
		r.Summary.Listing = true
	}
	return r
}

// checkPages holds every edition's pages to the plan. A finding that repeats
// across editions is reported once, naming the first edition it occurs in.
func (r *Report) checkPages(p project.Project, warn func(file string, line int, rule, message, hint string)) {
	// A finding names its edition as {edition}; the same finding in several
	// editions is reported once, naming them all.
	type finding struct {
		file, hint string
		line       int
		editions   []string
	}
	found := map[string]*finding{}
	var order []string
	label := ""
	once := func(file string, line int, message, hint string) {
		f := found[message]
		if f == nil {
			f = &finding{file: file, line: line, hint: hint}
			found[message] = f
			order = append(order, message)
		}
		if len(f.editions) == 0 || f.editions[len(f.editions)-1] != label {
			f.editions = append(f.editions, label)
		}
	}
	defer func() {
		for _, message := range order {
			f := found[message]
			which := "the " + strings.Join(f.editions, ", ") + " edition"
			if len(f.editions) == len(r.Editions) {
				which = "every edition"
			}
			text := strings.ReplaceAll(message, "{edition}", which)
			warn(f.file, f.line, RulePages, strings.ToUpper(text[:1])+text[1:], f.hint)
		}
	}()
	planned := map[string]Part{}
	for _, part := range r.Plan.Parts {
		planned[strings.ToLower(part.Name)] = part
	}
	for _, ed := range r.Editions {
		label = ed.Edition.Label
		if ed.Stray > 0 {
			once(TemplateFile, 1, fmt.Sprintf("%d elements of {edition} sit outside a <section class=\"page\">.", ed.Stray), "Wrap everything in page sections: each section is one printed page.")
		}
		if len(ed.Pages) == 0 {
			once(TemplateFile, 1, "{edition} renders no pages.", "Render each page as <section class=\"page\" data-part=\"…\">.")
			continue
		}
		counts := map[string]int{}
		ids := map[string]int{}
		roles := map[string]bool{}
		for _, pg := range ed.Pages {
			roles[pg.Role] = true
			if pg.Role != "" && !knownRole(pg.Role) {
				once(TemplateFile, 1, fmt.Sprintf("Page %d has the unknown role %q.", pg.N, pg.Role), "Use "+strings.Join(Roles, ", ")+", or no role.")
			}
			switch _, ok := planned[strings.ToLower(pg.Part)]; {
			case pg.Part == "":
				once(TemplateFile, 1, fmt.Sprintf("Page %d of {edition} has no data-part.", pg.N), "Name the Contents part every page belongs to in data-part.")
			case !ok:
				once(TemplateFile, 1, fmt.Sprintf("Part %q is not in the concept's Contents.", pg.Part), "Add the part to Contents, or use the name planned there.")
			}
			counts[strings.ToLower(pg.Part)]++
			for _, id := range pg.IDs {
				if prev, dup := ids[id]; dup {
					once(TemplateFile, 1, fmt.Sprintf("id %q is used on page %d and page %d.", id, prev, pg.N), "Give every link target its own id; links jump to the first one.")
				}
				ids[id] = pg.N
			}
			for _, src := range pg.Images {
				rel := path.Clean(src)
				if rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) || strings.Contains(src, "://") {
					once(TemplateFile, 1, fmt.Sprintf("Image %s lies outside deliverables/printable.", src), "Copy it into deliverables/printable/images and reference it as images/name.png.")
				} else if _, err := p.Read(Dir + "/" + rel); err != nil {
					once(TemplateFile, 1, fmt.Sprintf("Image %s does not exist.", src), "Reference images relative to deliverables/printable, e.g. images/cover.png.")
				}
			}
		}
		for _, pg := range ed.Pages {
			for _, target := range pg.Links {
				if _, ok := ids[target]; !ok {
					once(TemplateFile, 1, fmt.Sprintf("A link on page %d of {edition} points to #%s, which no page has.", pg.N, target), "Link only to ids that exist; a dead link is the complaint planner buyers name most.")
				}
			}
		}
		for _, part := range r.Plan.Parts {
			if got := counts[strings.ToLower(part.Name)]; got != part.Pages {
				once(ConceptFile, part.Line, fmt.Sprintf("Part %q has %d pages in {edition}; Contents plans %d.", part.Name, got, part.Pages), "Render the planned pages, or change the plan when the product changed.")
			}
		}
		if ed.Pages[0].Role != "cover" {
			once(TemplateFile, 1, "The first page is not the cover (data-role=\"cover\").", "Open with a cover: the product name and what is inside, the page buyers see first.")
		}
		if !roles["guide"] {
			once(TemplateFile, 1, "No page tells buyers how to print or import the file (data-role=\"guide\").", "Add a guide page: print at 100% (actual size), which pages to print in colour or on card, how to import into a note app, the licence.")
		}
		if roles["puzzle"] && !roles["key"] {
			once(TemplateFile, 1, "Puzzles (data-role=\"puzzle\") have no answer key (data-role=\"key\").", "Add the solutions: buyers rate a missing key below a missing puzzle.")
		}
	}
	// Editions of one size should not differ in page count: Monday and Sunday,
	// colour and ink-saver are the same product.
	bySize := map[string]int{}
	for _, ed := range r.Editions {
		if n, ok := bySize[ed.Edition.Size.Key]; ok && n != len(ed.Pages) {
			once(TemplateFile, 1, fmt.Sprintf("Editions in %s differ in page count (%d and %d).", ed.Edition.Size.Label, n, len(ed.Pages)), "Variants of one size should hold the same pages so the listing's page count is true for all.")
		}
		bySize[ed.Edition.Size.Key] = len(ed.Pages)
	}
}

// collectAssets reads the images the pages use and the files the CSS loads,
// keyed by the flat names the print document refers to.
func (r *Report) collectAssets(p project.Project, style string, warn func(file string, line int, rule, message, hint string)) {
	var refs []string
	for _, ed := range r.Editions {
		for _, pg := range ed.Pages {
			refs = append(refs, pg.Images...)
		}
	}
	for _, m := range cssURL.FindAllStringSubmatch(style, -1) {
		if ref := m[2]; !strings.Contains(ref, ":") && !strings.HasPrefix(ref, "#") {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	for _, ref := range slices.Compact(refs) {
		rel := path.Clean(ref)
		if rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
			continue // reported with the page
		}
		data, err := p.Read(Dir + "/" + rel)
		if err != nil {
			if strings.HasSuffix(ref, ".woff2") || strings.HasSuffix(ref, ".woff") || strings.HasSuffix(ref, ".ttf") || strings.HasSuffix(ref, ".otf") {
				warn(Dir+"/"+StyleName, 1, RulePages, fmt.Sprintf("%s loads %s, which does not exist.", StyleName, ref), "Put the font into deliverables/printable/fonts and reference it as fonts/name.woff2.")
			}
			continue
		}
		r.Assets[flat(rel)] = data
	}
}

// Slug is the file name stem of the product.
func (r Report) Slug() string { return deliver.Slug(r.Plan.Name, "printable") }

// FileName is the PDF name of an edition.
func (r Report) FileName(e Edition) string { return r.Slug() + "-" + e.Key + ".pdf" }

var fontFamily = regexp.MustCompile(`(?is)@font-face\s*\{[^}]*font-family\s*:\s*['"]?([^'";}]+)`)

// generic families are always present; any other first choice in a stack
// must come from fonts/, or the PDF embeds whatever the build machine has.
var generic = map[string]bool{"serif": true, "sans-serif": true, "system-ui": true, "monospace": true, "cursive": true, "fantasy": true, "ui-serif": true, "ui-sans-serif": true, "ui-rounded": true, "ui-monospace": true}

func (r *Report) checkFonts(style string, warn func(file string, line int, rule, message, hint string)) {
	faces := map[string]bool{}
	for _, m := range fontFamily.FindAllStringSubmatch(style, -1) {
		faces[strings.ToLower(strings.TrimSpace(m[1]))] = true
	}
	seen := map[string]bool{}
	for _, stack := range []string{r.Plan.Body, r.Plan.Headings} {
		first, _, _ := strings.Cut(stack, ",")
		first = strings.ToLower(strings.Trim(strings.TrimSpace(first), `"'`))
		if first == "" || generic[first] || faces[first] || seen[first] {
			continue
		}
		seen[first] = true
		warn(ConceptFile, r.Plan.Lines["Look"], RulePages, fmt.Sprintf("The font %q is not loaded from fonts/; the PDF would embed whatever the build machine has, if it has it at all.", first),
			"Put an OFL font (Google Fonts are) as WOFF2 into deliverables/printable/fonts and declare it with @font-face in printable.css; or start the stack with a generic family such as sans-serif.")
	}
}
