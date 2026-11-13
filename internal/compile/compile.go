// Package compile turns an lfm project into a deployable output folder.
//
// One compile runs a fixed lifecycle. A stage starts only when the previous
// ones produced no errors, and every stage reports through the same
// diagnostics, so one run explains everything that needs fixing:
//
//  1. load      read lfm.json and data/
//  2. prepare   check template bindings and links statically, then render
//  3. assemble  resolve, bundle and pack every referenced resource
//  4. optimize  (release) reuse optimization or measure critical CSS in Chrome
//  5. finalize  serialize and compact the document
//  6. verify    run every document check on the exact final bytes
//  7. audit     (release) reuse audits; validate runs runtime and Lighthouse
//  8. publish   replace output/ in one rename
//
// Nothing is written before verify passes; a failed compile keeps the
// previous output.
package compile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/bundle"
	"github.com/Anonyfox/lfm/internal/cache"
	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/htmlcompact"
	"github.com/Anonyfox/lfm/internal/imaging"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/scaffold"
	"github.com/Anonyfox/lfm/internal/spreadsheet"

	"golang.org/x/net/html"
)

// Options select the optional lifecycle stages.
type Options struct {
	Release     bool   // build critical CSS and deliverable prints
	Validate    bool   // run browser audits and layout checks; implies Release
	BrowserJobs int    // maximum concurrent browser work
	Publish     bool   // replace output/ on success
	Waiting     func() // called when another compile of the project holds the lock
}

// File is one emitted output file.
type File struct {
	Name   string `json:"name"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Result describes a compile, filled as far as the lifecycle got.
type Result struct {
	Root     string            `json:"root"`
	Output   string            `json:"output"`
	PageURL  string            `json:"url,omitempty"`
	Files    []File            `json:"files"`
	Artifact map[string][]byte `json:"-"` // emitted bytes by file name
	// Distribution holds the store packages per unit (each deliverable and
	// the bundle) by file name, published to distribution/<unit>/ and never
	// deployed. A unit without an entry is left as it is; an empty one is
	// removed.
	Distribution map[string]map[string][]byte `json:"-"`
	// Previews are rendered deliverable pages lfm serve shows and publish
	// never writes, such as each printable edition as HTML.
	Previews    map[string][]byte    `json:"-"`
	External    []string             `json:"externalAssets,omitempty"`
	Validation  *check.Report        `json:"validation,omitempty"`
	Validated   bool                 `json:"validated"`
	Published   bool                 `json:"published"`
	Release     *release.Result      `json:"release,omitempty"`
	Packing     []AssetDecision      `json:"assets,omitempty"`
	Cost        *Cost                `json:"cost,omitempty"`
	Compaction  *htmlcompact.Result  `json:"htmlCompaction,omitempty"`
	Images      []imaging.Decision   `json:"images,omitempty"`
	Fonts       []fontcheck.Info     `json:"fonts,omitempty"`
	Research    research.Summary     `json:"research"`
	Concept     concept.Summary      `json:"concept"`
	SocialMeta  []string             `json:"socialMetaRepairs,omitempty"`
	FontHints   int                  `json:"fontHintRepairs,omitempty"`
	PageOffers  bool                 `json:"pageOffers,omitempty"`
	Starter     bool                 `json:"starter"` // the template is still the starter page of lfm init
	Book        *book.Summary        `json:"book,omitempty"`
	Printable   *printable.Summary   `json:"printable,omitempty"`
	Bundle      *bundle.Summary      `json:"bundle,omitempty"`
	Spreadsheet *spreadsheet.Summary `json:"spreadsheet,omitempty"`
	Document    *document.Summary    `json:"document,omitempty"`
	Deck        *deck.Summary        `json:"deck,omitempty"`
	// Optional names the helpers a release found missing and what each
	// would add; no release needs them.
	Optional []string      `json:"optional,omitempty"`
	Inputs   []string      `json:"-"` // resolved source files, for watchers
	Duration time.Duration `json:"-"`
}

// Errors and Warnings count the reported diagnostics.
func (r Result) Errors() int {
	if r.Validation == nil {
		return 0
	}
	return r.Validation.Errors
}

func (r Result) Warnings() int {
	if r.Validation == nil {
		return 0
	}
	return r.Validation.Warnings
}

// Failure is returned when diagnostics block the compile.
type Failure struct{ Errors int }

func (f *Failure) Error() string {
	return fmt.Sprintf("compile failed with %d errors; output was not replaced", f.Errors)
}

type emitted struct {
	name, origin string
	contents     []byte
}

// builder carries the state of the assemble stage. esbuild plugin callbacks
// run concurrently, so shared maps are guarded by mu.
type builder struct {
	tools           *release.ToolIdentity
	cards           map[string]string
	ctx             context.Context // a cancelled compile stops its browsers
	project         project.Project
	root            string
	outDir          string // virtual esbuild output directory; nothing is written there
	config          project.Config
	mu              sync.Mutex
	files           map[string]emitted
	public          map[string][]byte // fixed URLs, copied verbatim after delivery
	external        map[string]bool
	warnings        []string
	inlineSources   map[[32]byte][]string
	assetOrigins    map[string]string // esbuild virtual input + suffix -> real source
	helpers         map[string]helperEntry
	cssModuleGraphs int
	packed          map[string]packedAsset
	roles           map[string]assetRole
	notices         []diagnostic.Diagnostic
	renderHint      int // expected serialized size, to size render buffers once
	references      map[string]string
	autoCode        []codeCandidate
	fileReasons     map[string]string
	fonts           map[string]fontcheck.Info
	fontHints       int
	inputs          map[string]bool
	nameMu          sync.Mutex
	names           map[string]string // source path -> emitted file name
}

// Compile runs the lifecycle on the project in dir. Cancelling ctx stops it
// between stages; a cancelled compile never publishes.
func Compile(ctx context.Context, dir string, options Options) (result Result, err error) {
	if options.Validate {
		options.Release = true
	}
	result.Validated = options.Validate
	ctx = release.WithBrowserJobs(ctx, options.BrowserJobs)
	defer cache.Prune()
	p, err := project.Open(dir)
	if err == nil {
		err = p.Expect(dir)
	}
	if err != nil {
		return result, err
	}
	result.Root, result.Output = p.Root, p.Output()
	unlock, err := lock(ctx, p.Root, options.Waiting)
	if err != nil {
		return result, err
	}
	defer unlock()
	start := time.Now()
	defer func() {
		result.listFiles()
		result.Duration = time.Since(start)
		if err != nil && !errors.Is(err, context.Canceled) && result.Errors() == 0 {
			// Operational failures (unreadable files, bundler errors) join the
			// same report as quality findings, as one required rule.
			result.Validation = summarize("build", append(diagnostics(result), diagnostic.Diagnostic{
				Rule: "build.failed", Severity: diagnostic.Error,
				Message: err.Error(),
				Hint:    "Fix the named source, path or bundler error and compile again; the previous output is kept.",
			}))
		}
	}()

	// 1. load
	loaded := project.Load(p, ruleCheck)
	c := loaded.Config
	result.PageURL = c.URL
	findings, facts, researched := checkResearch(p, &result, c.Build.Validation.Rules)
	var starterTexts []string
	var starterImages [][32]byte
	if page, err := p.Read(project.TemplateFile); err == nil && !scaffold.IsStarterPage(page) {
		starterTexts, starterImages = scaffold.Placeholders()
	} else if err == nil {
		result.Starter = true
	}

	// 2. prepare
	prepared, preliminary, registry, err := prepareTemplate(p, loaded)
	var units []unit
	var pack *bundle.Report
	if len(loaded.Problems) == 0 {
		var found []diagnostic.Diagnostic
		units, pack, found = checkDeliverables(checking{p: p, loaded: loaded, commerce: prepared.Commerce, researched: researched}, &result)
		findings = append(findings, found...)
	}
	result.PageOffers = len(prepared.Commerce.Offers) > 0
	preliminary = *summarize(preliminary.Stage, append(preliminary.Diagnostics, findings...))
	result.Validation = &preliminary
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 3. assemble
	b := &builder{ctx: ctx, project: p, root: p.Root, outDir: p.Output(), config: c, files: map[string]emitted{}, external: map[string]bool{}, inlineSources: map[[32]byte][]string{}, helpers: prepared.Helpers}
	defer func() { result.Inputs = b.inputList() }()
	doc, err := html.Parse(bytes.NewReader(prepared.HTML))
	if err != nil {
		return result, fmt.Errorf("HTML: %w", err)
	}
	if err := b.assemble(doc, loaded.Values, prepared, &result); err != nil {
		return result, err
	}

	for _, d := range b.notices {
		if d.Severity == diagnostic.Error {
			result.revalidate("assemble", b.notices, c.Build.Validation.Rules)
			return result, &Failure{Errors: result.Errors()}
		}
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	if b.public, err = b.publicFiles(); err != nil {
		return result, err
	}
	// 4. optimize
	if options.Release && result.PageNeeded() {
		b.tools = release.ResolveTools(ctx, p.Root)
		if err := b.optimizeWithCache(doc, &result, b.optimize); err != nil {
			return result, err
		}
	}

	// 5. finalize
	var serialized bytes.Buffer
	serialized.Grow(b.renderHint)
	if err := html.Render(&serialized, doc); err != nil {
		return result, err
	}
	page, compaction := htmlcompact.Compact(serialized.Bytes())
	result.Compaction = &compaction
	if compaction.Skipped != "" {
		b.notice("html.compaction", "HTML syntax compaction skipped: "+compaction.Skipped, "The serialized HTML was kept unchanged and still passes every check. Please report this input as an lfm bug.")
	}
	for _, warning := range b.warnings {
		b.notice("bundle.warning", strings.TrimSpace(warning), "Review the bundler warning in the named source.")
	}
	if err := b.discoveryFiles(doc); err != nil {
		return result, err
	}
	result.Cost = measureCost(page, doc, b.files)
	b.describe(&result, page)
	auditAssets := b.sidecars()

	// 6. verify: analyze the exact bytes that publish writes.
	analysis, err := check.Analyze(page, check.Options{
		Registry: registry, GeneratedAssets: b.generatedNames(), Assets: result.Artifact,
		JSONLDGenerated: len(prepared.JSONLD) > 0, JSONLDSource: &prepared.JSONLDSource,
		URL: c.URL, RenderedSource: prepared.HTML, InlineSources: b.inlineSources,
		ExternalCSSBytes: result.Cost.ExternalCSSBytes, ExternalJSBytes: result.Cost.ExternalJSBytes,
		Rules: c.Build.Validation.Rules, CopyAllow: c.Build.Validation.CopyAllow, CopyBans: c.Build.Validation.CopyBans, Images: result.Images, FontAssets: b.fonts,
		Budgets:          check.Budgets(c.Build.Validation.Budgets),
		CriticalCSSBytes: criticalCopy(result.Release),
		Stylesheets:      b.cssAssets(), Commerce: prepared.Commerce, Concept: facts,
		StarterTexts: starterTexts, StarterImages: starterImages,
	})
	result.Validation = summarize("html", append(append(preliminary.Diagnostics, analysis.Diagnostics...), diagnostic.Apply(b.notices, c.Build.Validation.Rules)...))
	result.Validation.Stats = analysis.Stats
	if err != nil {
		return result, err
	}
	if result.Errors() > 0 {
		return result, &Failure{Errors: result.Errors()}
	}
	// One stock for every deliverable: the last publish indexed once, one Chrome.
	st := newStock(ctx, p, options.Release)
	st.validate = options.Validate
	defer st.close()
	delivered := deliverAll(p, page, units, pack, st, c.Build.Validation.Rules, &result)
	if !delivered {
		return result, &Failure{Errors: result.Errors()}
	}
	if err := b.addPublic(&result); err != nil {
		return result, err
	}

	// 7. audit
	if options.Release && result.PageNeeded() && ctx.Err() == nil {
		if result.Release == nil {
			result.Release = &release.Result{}
		}
		target := release.Target{Page: page, Assets: auditAssets, Tools: b.tools, Printer: st.printer}
		var audit *release.AuditResult
		if options.Validate {
			audit = release.Audit(ctx, target, p.Root)
		} else {
			audit = release.CachedAudit(ctx, target, p.Root)
		}
		if audit == nil {
			audit = &release.AuditResult{}
		}
		result.Release.Audit = audit

		if !result.revalidate("audit", audit.Diagnostics, c.Build.Validation.Rules) {
			return result, &Failure{Errors: result.Errors()}
		}
	}

	st.close()
	if options.Release && ctx.Err() == nil {
		if result.Release == nil {
			result.Release = &release.Result{}
		}
		var opened []string
		for _, u := range units {
			if ext := u.opens(); ext != "" {
				opened = append(opened, ext)
			}
		}
		result.Optional = optionalHelpers(opened)
	}

	// 8. publish
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if options.Publish {
		if err := publish(p, result.Artifact); err != nil {
			return result, err
		}
		if err := publishDistribution(p, result.Distribution); err != nil {
			return result, err
		}
		result.Published = true
	}
	return result, nil
}

// checkDeliverables checks every deliverable in order, each reading only
// its own folder, then the bundle of them and how each sells on the page.
func checkDeliverables(in checking, result *Result) ([]unit, *bundle.Report, []diagnostic.Diagnostic) {
	rules := in.loaded.Config.Build.Validation.Rules
	var units []unit
	var findings []diagnostic.Diagnostic
	for _, k := range kinds {
		if !k.present(in.p) {
			continue
		}
		u, found := k.check(in)
		if u != nil {
			units = append(units, u)
			u.summarize(result)
		}
		findings = append(findings, diagnostic.Apply(found, rules)...)
	}
	pack, bundleFindings := checkBundle(in.p, in.loaded, in.commerce)
	if pack != nil {
		result.Bundle = &pack.Summary
	}
	findings = append(findings, diagnostic.Apply(bundleFindings, rules)...)
	d := in.loaded.Config.Deliverables
	var sale []diagnostic.Diagnostic
	for _, u := range units {
		if field, sell, offer, rule, ok := u.sale(d); ok {
			sale = append(sale, saleFindings(in.commerce, field, sell, offer, rule)...)
		}
	}
	if pack != nil {
		sale = append(sale, saleFindings(in.commerce, "deliverables.bundle", d.Bundle.Sell, d.Bundle.Offer, bundle.RuleConfig)...)
	}
	return units, pack, append(findings, diagnostic.Apply(sale, rules)...)
}

// deliverAll delivers each deliverable on its own, then the bundle of those
// it holds, and stops at the first that leaves errors.
func deliverAll(p project.Project, page []byte, units []unit, pack *bundle.Report, st *stock, rules map[string]diagnostic.Severity, result *Result) bool {
	selected := map[string]part{}
	if pack != nil && pack.Listing.Present {
		for _, u := range units {
			pt := u.part()
			selected[pt.kind] = pt
		}
		counts := map[string]int{}
		covers := 0
		for _, kind := range pack.Summary.Units {
			if selected[kind].cover != nil || len(selected[kind].plans) > 0 {
				covers++
			}
		}
		left := deliver.EtsyImages
		if covers >= 2 {
			left--
		}
		for round := 0; left > 0; round++ {
			added := false
			for _, kind := range pack.Summary.Units {
				if round < len(selected[kind].plans) && left > 0 {
					counts[kind]++
					left--
					added = true
				}
			}
			if !added {
				break
			}
		}
		st.demand = counts
	}
	for _, u := range units {
		if st.ctx.Err() != nil {
			return true // cancelled: the compile ends without publishing
		}
		if !result.revalidate(result.Validation.Stage, u.deliver(p, page, st, result), rules) {
			return false
		}
	}
	if pack != nil && len(pack.Summary.Units) >= 2 {
		var parts []part
		accent := ""
		for _, u := range units {
			pt := u.part()
			selected[pt.kind] = pt
		}
		for _, kind := range pack.Summary.Units {
			pt, ok := selected[kind]
			if !ok {
				continue
			}
			parts = append(parts, pt)
			if accent == "" {
				accent = pt.accent
			}
		}
		return result.revalidate(result.Validation.Stage, deliverBundle(p, pack, parts, accent, page, st, result), rules)
	}
	return true
}

// revalidate adds findings to the report at stage, keeping the page's
// stats, and reports whether it is still free of errors.
func (r *Result) revalidate(stage string, found []diagnostic.Diagnostic, rules map[string]diagnostic.Severity) bool {
	stats := r.Validation.Stats
	r.Validation = summarize(stage, append(r.Validation.Diagnostics, diagnostic.Apply(found, rules)...))
	r.Validation.Stats = stats
	return r.Errors() == 0
}

// assemble rewrites the document so every local resource is bundled, packed
// and referenced by its final name.
func (b *builder) assemble(doc *html.Node, values map[string]any, prepared preparedTemplate, result *Result) error {
	b.footer(doc)
	b.collectAssetRoles(doc)
	images := captureImages(doc)
	data, err := project.BrowserData(values, b.config.Build.BrowserData)
	if err != nil {
		return err
	}
	if err := b.processHTML(doc, data, prepared.JSONLD); err != nil {
		return err
	}
	if err := b.ensureFavicon(doc); err != nil {
		return err
	}
	b.socialImageSizes(doc)
	result.SocialMeta = b.socialHandles(doc)
	result.Images = b.prepareImages(doc, images)
	if result.Packing, err = b.planAssets(doc); err != nil {
		return err
	}
	if err := b.packCode(); err != nil {
		return err
	}
	result.Packing = b.fileDecisions(result.Packing)
	b.fonts, result.Fonts = b.inspectFonts(doc)
	if err := b.resourceHints(doc); err != nil {
		return err
	}
	result.FontHints = b.fontHints
	return nil
}

// optimize builds critical CSS and image hints; extraction failures block publishing.
func (b *builder) optimize(doc *html.Node, result *Result) error {
	r, err := b.optimizeRelease(doc)
	result.Release = r
	if err != nil {
		if !cssExtractionFailure(err) {
			return err
		}
		b.notices = append(b.notices, diagnostic.Diagnostic{Rule: "release.css", Severity: diagnostic.Error, Message: "Critical CSS extraction failed: " + err.Error(), Hint: "Check Chrome startup (LFM_CHROME) and page rendering, then run lfm compile --release again."})
		return nil
	}
	if !r.Applied && r.Reason != "" && !strings.HasPrefix(r.Reason, "Stylesheet is below 8 KiB") && !strings.HasPrefix(r.Reason, "Critical CSS does not remove at least") {
		b.notice("release.critical", r.Reason, "Use one ordinary generated local stylesheet, local fonts and sized media, and review authored CSP or the named rendering limitation; retry --release.")
	}
	b.imageHints(doc, result.Release)
	return nil
}

// describe collects the assembled bytes plus the page and external names.
func (b *builder) describe(result *Result, page []byte) {
	result.Artifact = map[string][]byte{"index.html": page}
	for _, file := range b.files {
		result.Artifact[file.name] = file.contents
	}
	for name := range b.external {
		result.External = append(result.External, name)
	}
	sort.Strings(result.External)
}

// listFiles derives the deployment manifest once, after every stage has had its
// chance to emit files. The artifact is the single source of paths and bytes.
func (r *Result) listFiles() {
	if r.Artifact == nil {
		return
	}
	r.Files = make([]File, 0, len(r.Artifact))
	for name, data := range r.Artifact {
		sum := sha256.Sum256(data)
		r.Files = append(r.Files, File{Name: name, Bytes: len(data), SHA256: hex.EncodeToString(sum[:])})
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Name < r.Files[j].Name })
}

func (b *builder) generatedNames() map[string]bool {
	names := map[string]bool{}
	for _, file := range b.files {
		names[file.name] = true
	}
	return names
}

func (b *builder) sidecars() map[string][]byte {
	assets := map[string][]byte{}
	for _, file := range b.files {
		assets[file.name] = file.contents
	}
	for name, contents := range b.public {
		assets[name] = contents
	}
	return assets
}

func (b *builder) notice(rule, message, hint string) {
	b.notices = append(b.notices, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: "index.html", Line: 1, Column: 1}, Message: message, Hint: hint})
}

// recordInput is called by concurrent esbuild loaders. Watchers use the
// resolved inputs instead of crawling node_modules on every edit.
func (b *builder) recordInput(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.inputs == nil {
		b.inputs = map[string]bool{}
	}
	b.inputs[path] = true
}

func (b *builder) inputList() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	inputs := make([]string, 0, len(b.inputs))
	for path := range b.inputs {
		inputs = append(inputs, path)
	}
	sort.Strings(inputs)
	return inputs
}

// emit registers an output file. Names are flat; collisions with a different
// origin or content fail instead of silently overwriting.
func (b *builder) emit(name, origin string, content []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid output filename %q", name)
	}
	key := strings.ToLower(name)
	if key == "index.html" && origin != "document" {
		return fmt.Errorf("%s: index.html is reserved", origin)
	}
	if previous, exists := b.files[key]; exists {
		if previous.origin == origin && bytes.Equal(previous.contents, content) {
			return nil
		}
		return fmt.Errorf("output filename collision %q: %s and %s; rename one source asset", name, previous.origin, origin)
	}
	b.files[key] = emitted{name, origin, content}
	return nil
}

func summarize(stage string, issues []diagnostic.Diagnostic) *check.Report {
	s := diagnostic.Summarize(issues)
	return &check.Report{Stage: stage, Diagnostics: s.Diagnostics, Errors: s.Errors, Warnings: s.Warnings}
}

func diagnostics(r Result) []diagnostic.Diagnostic {
	if r.Validation == nil {
		return nil
	}
	return r.Validation.Diagnostics
}

// checkResearch checks the research files and concept.md that exist. They are
// never build inputs; their findings join the report so both stay exact as
// they grow, and the concept's decisions become facts the page is held to.
func checkResearch(p project.Project, result *Result, rules map[string]diagnostic.Severity) ([]diagnostic.Diagnostic, *check.ConceptFacts, research.Report) {
	files := map[string][]byte{}
	for _, name := range []string{research.VoicesFile, research.CompetitorsFile} {
		if raw, err := p.Read(project.ResearchDir + "/" + name); err == nil {
			files[name] = raw
		}
	}
	r := research.Check(files, project.ResearchDir, time.Now())
	result.Research = r.Summary
	findings := r.Diagnostics
	raw, err := p.Read(concept.File)
	if err != nil {
		return diagnostic.Apply(findings, rules), nil, r
	}
	c := concept.CheckProject(string(raw), r.Voices, r.Competitors, p)
	result.Concept = c.Summary
	findings = append(findings, c.Diagnostics...)
	// The page is held to the concept once it is built; the untouched starter
	// page of lfm init has nothing to hold yet.
	if page, err := p.Read(project.TemplateFile); err == nil && scaffold.IsStarterPage(page) {
		return diagnostic.Apply(findings, rules), nil, r
	}
	facts := &check.ConceptFacts{Intensity: c.Summary.Intensity, Colors: c.Summary.DeclaredColors, Families: c.Summary.Families, Imagery: c.Summary.Imagery, Use: c.Summary.Use, Avoid: c.Summary.Avoid}
	return diagnostic.Apply(findings, rules), facts, r
}

// distribute sets the store package of a unit; empty removes it.
func (r *Result) distribute(unit string, files map[string][]byte) {
	if !units[unit] {
		// publishDistribution would clear the folder as one lfm no longer writes.
		panic("compile: distribution/" + unit + " is not a unit")
	}
	if r.Distribution == nil {
		r.Distribution = map[string]map[string][]byte{}
	}
	r.Distribution[unit] = files
}

// optionalHelpers are the helpers a release did without and what each
// would have added.
func optionalHelpers(kinds []string) []string {
	var out []string
	if _, err := office.Find(); len(kinds) > 0 && office.Missing(err) {
		out = append(out, "LibreOffice, a second opinion on how another app opens the "+strings.Join(kinds, " and ")+" files")
	}
	return out
}

// criticalCopy is the size of the first-screen rules a release inlined in
// front of the full stylesheet, which holds them too.
func criticalCopy(r *release.Result) int {
	if r == nil || !r.Applied {
		return 0
	}
	return r.CriticalBytes
}

// PageNeeded reports whether the current page participates in selling the deliverables.
func (r Result) PageNeeded() bool {
	if !r.Starter || r.PageOffers {
		return true
	}
	var sells []string
	if r.Book != nil {
		sells = append(sells, r.Book.Sell)
	}
	if r.Printable != nil {
		sells = append(sells, r.Printable.Sell)
	}
	if r.Spreadsheet != nil {
		sells = append(sells, r.Spreadsheet.Sell)
	}
	if r.Document != nil {
		sells = append(sells, r.Document.Sell)
	}
	if r.Deck != nil {
		sells = append(sells, r.Deck.Sell)
	}
	if r.Bundle != nil {
		sells = append(sells, r.Bundle.Sell)
	}
	if len(sells) == 0 {
		return true
	}
	for _, sell := range sells {
		if sell != "shops" && sell != "bundle" {
			return true
		}
	}
	return false
}
