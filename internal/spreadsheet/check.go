package spreadsheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/formula"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/project"
)

// Summary is what the report and --json show about the spreadsheet.
type Summary struct {
	Name     string   `json:"name,omitempty"`
	Concept  bool     `json:"concept"`
	Workbook bool     `json:"workbook"`
	Editions []string `json:"editions,omitempty"`
	Sheets   int      `json:"sheets"`
	Formulas int      `json:"formulas"` // formula cells per edition
	Checked  []string `json:"checked,omitempty"`
	Sell     string   `json:"sell"`
	URL      string   `json:"url,omitempty"`
	Files    []string `json:"files,omitempty"`
	Listing  bool     `json:"listing"`
	Linked   int      `json:"linked"` // Google Sheets masters linked
	Channels []string `json:"channels,omitempty"`
	Images   int      `json:"images,omitempty"`
	Pending  bool     `json:"pending,omitempty"`
	Compat   string   `json:"compat,omitempty"`
}

// Report is the checked spreadsheet.
type Report struct {
	Config      Config
	Plan        Plan
	Workbook    Workbook
	Editions    []*edition
	Books       map[string]map[string]*book // edition key -> dataset -> computed book
	Summary     Summary
	Diagnostics []diagnostic.Diagnostic
	Listing     deliver.ShopListing
	Buildable   bool
}

// Datasets the files are built from: the blank template and, when the
// workbook has one, the filled example.
const (
	Empty   = "empty"
	Example = "example"
)

func at(file string, line int, rule string, sev diagnostic.Severity, message, hint string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Rule: rule, Severity: sev, Location: diagnostic.Location{File: file, Line: max(1, line), Column: 1}, Message: message, Hint: hint}
}

var sheetName = regexp.MustCompile(`[:\\/?*\[\]]`)

// Check reads deliverables/spreadsheet: configured tells whether lfm.json
// has deliverables.spreadsheet; site is the page URL the download path hangs
// off.
func Check(p project.Project, cfg Config, configured bool, offers map[string]*deliver.Price, site string) Report {
	r := Report{Config: cfg, Books: map[string]map[string]*book{}}
	r.Summary.Sell = deliver.Sell(cfg.Sell)
	fail := func(message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at("lfm.json", 1, RuleConfig, diagnostic.Error, message, hint))
	}
	switch {
	case !configured:
		fail("deliverables/spreadsheet exists, but lfm.json has no deliverables.spreadsheet.",
			fmt.Sprintf(`Add "deliverables": {"spreadsheet": {"path": %q}} to lfm.json. 'lfm help config deliverables.spreadsheet' lists the options.`, deliver.NewPath()))
	case r.Summary.Sell != deliver.SellPage:
	case !deliver.ValidPath(cfg.Path):
		fail(fmt.Sprintf("deliverables.spreadsheet.path %q is not an unguessable folder name.", cfg.Path),
			fmt.Sprintf("Use at least %d lowercase letters, digits or single hyphens, for example %q. Anyone who knows the path can download the files.", deliver.MinPathLength, deliver.NewPath()))
	default:
		r.Summary.URL = strings.TrimSuffix(site, "/") + "/" + cfg.Path + "/"
	}
	var offer *deliver.Price
	if cfg.Offer != "" {
		o, ok := offers[cfg.Offer]
		if !ok {
			fail(fmt.Sprintf("deliverables.spreadsheet.offer %q names no offer in data.commerce.offers.", cfg.Offer), "Use the key of the offer that sells the spreadsheet.")
		}
		offer = o
	}

	if raw, err := p.Read(ConceptFile); err == nil {
		r.Summary.Concept = true
		r.Plan = checkConcept(string(raw), func(line int, message, hint string) {
			r.Diagnostics = append(r.Diagnostics, at(ConceptFile, line, RuleConcept, diagnostic.Warning, message, hint))
		})
		r.Summary.Name = r.Plan.Name
	}
	raw, err := p.Read(WorkbookFile)
	if err != nil {
		return r
	}
	r.Summary.Workbook = true
	// Only findings need locations, so the source is indexed on the first.
	var sources *jsonsrc.Sources
	locate := func(path string) diagnostic.Location {
		if sources == nil {
			index := jsonsrc.Index(map[string][]byte{WorkbookFile: raw})
			sources = &index
		}
		if loc, ok := sources.Nearest(path); ok {
			return loc
		}
		return diagnostic.Location{File: WorkbookFile, Line: 1, Column: 1}
	}
	report := func(rule string, sev diagnostic.Severity, path, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, diagnostic.Diagnostic{Rule: rule, Severity: sev, Location: locate(path), Path: path, Message: message, Hint: hint})
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&r.Workbook); err != nil {
		loc := diagnostic.At(WorkbookFile, raw, int(d.InputOffset()))
		r.Diagnostics = append(r.Diagnostics, diagnostic.Diagnostic{Rule: RuleWorkbook, Severity: diagnostic.Error, Location: loc, Message: "workbook.json cannot be read: " + err.Error(), Hint: "Fix the JSON; 'lfm skills spreadsheet' shows the format."})
		return r
	}
	w := r.Workbook
	if !validate(w, func(path, message, hint string) { report(RuleWorkbook, diagnostic.Error, path, message, hint) }) {
		return r
	}
	r.checkTabs()

	// Lay out and compute every edition with every dataset, side by side.
	datasets := []string{Empty}
	for name := range w.Datasets {
		if name != Empty {
			datasets = append(datasets, name)
		}
	}
	slices.Sort(datasets[1:])
	type result struct {
		ed       *edition
		books    map[string]*book
		problems []problem
		formula  []problem
	}
	results := make([]result, len(w.Editions))
	var wg sync.WaitGroup
	for i, e := range w.Editions {
		wg.Go(func() {
			ed, problems := layout(w, e)
			res := result{ed: ed, books: map[string]*book{}, problems: problems}
			if len(problems) == 0 {
				// Each dataset fills and computes its own book.
				books := make([]*book, len(datasets))
				found := make([][]problem, len(datasets))
				computed := make([][]problem, len(datasets))
				var each sync.WaitGroup
				for j, name := range datasets {
					each.Go(func() {
						books[j], found[j] = fill(ed, w, name)
						computed[j] = compute(ed, books[j], name, w.Expect[name])
					})
				}
				each.Wait()
				for j, name := range datasets {
					res.problems = append(res.problems, found[j]...)
					res.formula = append(res.formula, computed[j]...)
					res.books[name] = books[j]
				}
			}
			results[i] = res
		})
	}
	wg.Wait()
	seen := map[string]bool{}
	r.Buildable = true
	for i, res := range results {
		label := w.Editions[i].Key
		for _, pr := range res.problems {
			r.Buildable = false
			if !seen[pr.path+pr.message] {
				seen[pr.path+pr.message] = true
				report(RuleWorkbook, diagnostic.Error, pr.path, pr.message, pr.hint)
			}
		}
		for _, pr := range res.formula {
			r.Buildable = false
			if !seen[pr.message] {
				seen[pr.message] = true
				report(RuleFormulas, diagnostic.Error, pr.path, pr.message+" ("+label+")", pr.hint)
			}
		}
		r.Editions = append(r.Editions, res.ed)
		r.Books[label] = res.books
		r.Summary.Editions = append(r.Summary.Editions, label)
	}
	if len(r.Editions) > 0 {
		r.Summary.Sheets = len(r.Editions[0].Sheets)
		for _, l := range r.Editions[0].Sheets {
			for _, sp := range l.Cells {
				if sp.Formula != nil {
					r.Summary.Formulas++
				}
			}
		}
	}
	r.Summary.Checked = datasets
	r.checkLinks(func(path, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, at("lfm.json", 1, RuleSheets, diagnostic.Warning, message, hint))
	})
	if lraw, err := p.Read(ListingFile); err == nil {
		r.Listing = deliver.CheckShopListing(string(lraw), offer, func(line int, message, hint string) {
			r.Diagnostics = append(r.Diagnostics, at(ListingFile, line, RuleListing, diagnostic.Warning, message, hint))
		})
		r.Summary.Listing = true
	}
	return r
}

// compute evaluates every formula cell of a book and reports errors, circular
// references, array results and values that differ from what the workbook
// expects. A formula shared by a table column is reported once.
func compute(ed *edition, b *book, dataset string, expect map[string]any) []problem {
	var out []problem
	reported := map[string]bool{}
	for _, l := range ed.Sheets {
		for _, p := range l.Order {
			sp := l.Cells[p]
			if sp.Formula == nil {
				continue
			}
			v := b.at(l, p)
			if b.cycle == "" && v.Kind != formula.Error && v.Kind != formula.Array {
				continue // the common case names no cell
			}
			cell := l.Name + "!" + formula.CellName(p.Col, p.Row)
			if b.cycle != "" && !reported["cycle"] {
				reported["cycle"] = true
				out = append(out, problem{sp.Path, fmt.Sprintf("%s refers to itself through %s.", cell, b.cycle), "Break the circle: a total must not count itself."})
			}
			switch {
			case v.Kind == formula.Error && !reported[sp.Source]:
				reported[sp.Source] = true
				out = append(out, problem{sp.Path + ".formula", fmt.Sprintf("%s computes %s with the %s data: %s", cell, v.Str, dataset, sp.Source), errorHint(v.Str)})
			case v.Kind == formula.Array && !reported[sp.Source]:
				reported[sp.Source] = true
				out = append(out, problem{sp.Path + ".formula", fmt.Sprintf("%s returns a list of values; only Excel 365 spills them: %s", cell, sp.Source), "Wrap the list in SUM, SUMPRODUCT, INDEX or another function that returns one value."})
			}
		}
	}
	for _, l := range ed.Sheets {
		for _, p := range l.Order {
			sp := l.Cells[p]
			if why, ok := b.unportable[sp]; ok && !reported["portable "+sp.Source] {
				reported["portable "+sp.Source] = true
				out = append(out, problem{sp.Path + ".formula", fmt.Sprintf("%s!%s %s: %s", l.Name, formula.CellName(p.Col, p.Row), why, sp.Source), "Every app must compute the template alike; rewrite this part of the formula."})
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(expect)) {
		want := expect[key]
		got, ok := ed.lookup(b, key)
		if !ok {
			out = append(out, problem{"expect." + dataset + "." + key, fmt.Sprintf("Expected value %q names no cell.", key), "Name a cell (income), a table cell (log.amount[3]) or sheet!A1 by sheet key."})
			continue
		}
		if !matches(got, want) {
			out = append(out, problem{"expect." + dataset + "." + key, fmt.Sprintf("%s computes %s with the %s data, but %v is expected.", key, got, dataset, want), "Fix the formula, or the expectation when the template changed on purpose."})
		}
	}
	return out
}

func errorHint(code string) string {
	switch code {
	case formula.ErrDiv0:
		return "Guard the division: IFERROR(a/b, 0), or IF(b=0, 0, a/b). The blank template must show no errors."
	case formula.ErrNA:
		return "A lookup found nothing; wrap it in IFERROR with what the cell should show instead."
	case formula.ErrValue:
		return "A value has the wrong type: text where a number or date belongs, or a range where one value belongs."
	}
	return "Fix the formula so it computes a value for every dataset, the blank template included."
}

// lookup finds an expected cell: a named cell, sheet.column[row], or
// sheetkey!A1.
func (ed *edition) lookup(b *book, key string) (formula.Value, bool) {
	if sheet, cell, ok := strings.Cut(key, "!"); ok {
		l := ed.sheets[sheet]
		p, err := parsePos(cell)
		if l == nil || err != nil {
			return formula.Value{}, false
		}
		return b.at(l, p), true
	}
	if base, rest, ok := strings.Cut(key, "["); ok {
		var n int
		if _, err := fmt.Sscanf(rest, "%d]", &n); err != nil {
			return formula.Value{}, false
		}
		t, ok := ed.names[base]
		if !ok || t.area == nil {
			return formula.Value{}, false
		}
		sheet, _, _ := strings.Cut(base, ".")
		return b.at(ed.sheets[sheet], Pos{t.area.From.Col, t.area.From.Row + n - 1}), true
	}
	t, ok := ed.names[key]
	if !ok || t.area != nil {
		return formula.Value{}, false
	}
	for _, l := range ed.Sheets {
		if l.Name == t.sheet {
			return b.at(l, t.pos), true
		}
	}
	return formula.Value{}, false
}

func matches(got formula.Value, want any) bool {
	switch w := want.(type) {
	case float64:
		return got.Kind == formula.Number && math.Abs(got.Num-w) < 0.005
	case string:
		return got.String() == w
	case bool:
		return got.Kind == formula.Logical && (got.Num != 0) == w
	}
	return false
}

// checkTabs holds the concept's Tabs to the workbook's sheets.
func (r *Report) checkTabs() {
	if len(r.Plan.Tabs) == 0 {
		return
	}
	planned := map[string]bool{}
	for _, t := range r.Plan.Tabs {
		planned[t.Key] = true
	}
	for _, s := range r.Workbook.Sheets {
		if !planned[s.Key] {
			r.Diagnostics = append(r.Diagnostics, at(ConceptFile, r.Plan.Lines["Tabs"], RuleConcept, diagnostic.Warning, fmt.Sprintf("Sheet %q is not in the concept's Tabs.", s.Key), "Plan every sheet with its job, or remove it."))
		}
	}
	for _, t := range r.Plan.Tabs {
		if !slices.ContainsFunc(r.Workbook.Sheets, func(s Sheet) bool { return s.Key == t.Key }) {
			r.Diagnostics = append(r.Diagnostics, at(ConceptFile, t.Line, RuleConcept, diagnostic.Warning, fmt.Sprintf("Tab %q has no sheet in workbook.json.", t.Key), "Build the sheet, or take it out of the plan."))
		}
	}
}

var sheetsURL = regexp.MustCompile(`^https://docs\.google\.com/spreadsheets/d/([A-Za-z0-9_-]{20,})(?:/|$)`)

// CopyLink turns a Google Sheets address into the link that makes the buyer
// a copy, or "".
func CopyLink(raw string) string {
	m := sheetsURL.FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	return "https://docs.google.com/spreadsheets/d/" + m[1] + "/copy"
}

// checkLinks wants a Google Sheets master per edition and per example.
func (r *Report) checkLinks(warn func(path, message, hint string)) {
	_, example := r.Workbook.Datasets[Example]
	for _, e := range r.Workbook.Editions {
		keys := []string{e.Key}
		if example {
			keys = append(keys, e.Key+"-example")
		}
		for _, key := range keys {
			link, ok := r.Config.Sheets[key]
			switch {
			case !ok:
				warn("deliverables.spreadsheet.sheets."+key, fmt.Sprintf("No Google Sheets master is linked for %s.", key),
					fmt.Sprintf(`Upload %s to Google Drive, open it with Google Sheets (File > Save as Google Sheets), share it as "anyone with the link: viewer", and add "sheets": {"%s": "<its address>"} to deliverables.spreadsheet. Buyers get a copy link to it.`, r.FileName(e, strings.HasSuffix(key, "-example")), key))
			case CopyLink(link) == "":
				warn("deliverables.spreadsheet.sheets."+key, fmt.Sprintf("The %s link is not a Google Sheets document: %s", key, link), "Paste the address of the Google Sheet itself, https://docs.google.com/spreadsheets/d/…")
			default:
				r.Summary.Linked++
			}
		}
	}
}

// Slug is the file name stem of the product.
func (r Report) Slug() string { return deliver.Slug(r.Plan.Name, "spreadsheet") }

// FileName is the .xlsx name of an edition, blank or example.
func (r Report) FileName(e Edition, example bool) string {
	name := r.Slug() + "-" + e.Key
	if example {
		name += "-example"
	}
	return name + ".xlsx"
}
