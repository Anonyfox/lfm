// Package compile turns an lfm project into a deployable output folder.
//
// One compile runs a fixed lifecycle. A stage starts only when the previous
// ones produced no errors, and every stage reports through the same
// diagnostics, so one run explains everything that needs fixing:
//
//  1. load      read lfm.json and data/
//  2. prepare   check template bindings and links statically, then render
//  3. assemble  resolve, bundle and pack every referenced resource
//  4. optimize  (release) split critical CSS after measuring it in Chrome
//  5. finalize  serialize and compact the document
//  6. verify    run every document check on the exact final bytes
//  7. audit     (release) runtime probe and Lighthouse in a headless browser
//  8. publish   replace output/ in one rename
//
// Nothing is written before verify passes; a failed compile keeps the
// previous output.
package compile

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/htmlcompact"
	"github.com/Anonyfox/lfm/internal/imaging"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"

	"golang.org/x/net/html"
)

// Options select the optional lifecycle stages.
type Options struct {
	Release bool // browser-backed optimize and audit stages
	Publish bool // replace output/ on success
}

// File is one emitted output file.
type File struct {
	Name  string `json:"name"`
	Bytes int    `json:"bytes"`
}

// Result describes a compile, filled as far as the lifecycle got.
type Result struct {
	Root       string              `json:"root"`
	Output     string              `json:"output"`
	PageURL    string              `json:"url,omitempty"`
	Files      []File              `json:"files"`
	Artifact   map[string][]byte   `json:"-"` // emitted bytes by file name
	External   []string            `json:"externalAssets,omitempty"`
	Validation *check.Report       `json:"validation,omitempty"`
	Published  bool                `json:"published"`
	Release    *release.Result     `json:"release,omitempty"`
	Packing    []AssetDecision     `json:"assets,omitempty"`
	Cost       *Cost               `json:"cost,omitempty"`
	Compaction *htmlcompact.Result `json:"htmlCompaction,omitempty"`
	Images     []imaging.Decision  `json:"images,omitempty"`
	Fonts      []fontcheck.Info    `json:"fonts,omitempty"`
	FontHints  int                 `json:"fontHintRepairs,omitempty"`
	Inputs     []string            `json:"-"` // resolved source files, for watchers
	Duration   time.Duration       `json:"duration"`
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
	project         project.Project
	root            string
	outDir          string // virtual esbuild output directory; nothing is written there
	config          project.Config
	mu              sync.Mutex
	files           map[string]emitted
	external        map[string]bool
	warnings        []string
	inlineSources   map[[32]byte][]string
	assetOrigins    map[string]string // esbuild virtual input + suffix -> real source
	helpers         map[string]helperEntry
	cssModuleGraphs int
	packed          map[string]packedAsset
	roles           map[string]assetRole
	notices         []diagnostic.Diagnostic
	references      map[string]string
	autoCode        []codeCandidate
	fileReasons     map[string]string
	fonts           map[string]fontcheck.Info
	fontHints       int
	inputs          map[string]bool
}

// Compile runs the lifecycle on the project in dir.
func Compile(dir string, options Options) (result Result, err error) {
	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
		if err != nil && result.Errors() == 0 {
			// Operational failures (unreadable files, bundler errors) join the
			// same report as quality findings, as one required rule.
			result.Validation = summarize("build", append(diagnostics(result), diagnostic.Diagnostic{
				Rule: "build.failed", Severity: diagnostic.Error,
				Location: diagnostic.Location{File: project.TemplateFile, Line: 1, Column: 1},
				Message:  err.Error(),
				Hint:     "Fix the named source, path or bundler error and compile again; the previous output is kept.",
			}))
		}
	}()

	// 1. load
	p, err := project.Open(dir)
	if err != nil {
		return result, err
	}
	result.Root, result.Output = p.Root, p.Output()
	loaded := project.Load(p, ruleCheck)
	c := loaded.Config
	result.PageURL = c.URL

	// 2. prepare
	prepared, preliminary, registry, err := prepareTemplate(p, loaded)
	result.Validation = &preliminary
	if err != nil {
		return result, err
	}
	unlock, err := lock(p.Root)
	if err != nil {
		return result, err
	}
	defer unlock()

	// 3. assemble
	b := &builder{project: p, root: p.Root, outDir: p.Output(), config: c, files: map[string]emitted{}, external: map[string]bool{}, inlineSources: map[[32]byte][]string{}, helpers: prepared.Helpers}
	defer func() { result.Inputs = b.inputList() }()
	doc, err := html.Parse(bytes.NewReader(prepared.HTML))
	if err != nil {
		return result, fmt.Errorf("HTML: %w", err)
	}
	if err := b.assemble(doc, loaded.Values, prepared, &result); err != nil {
		return result, err
	}

	// 4. optimize
	if options.Release {
		if err := b.optimize(doc, &result); err != nil {
			return result, err
		}
	}

	// 5. finalize
	var serialized bytes.Buffer
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
	result.Cost = measureCost(page, doc, b.files)
	b.describe(&result, page)

	// 6. verify: analyze the exact bytes that publish writes.
	analysis, err := check.Analyze(page, check.Options{
		Registry: registry, GeneratedAssets: b.generatedNames(), Assets: result.Artifact,
		JSONLDGenerated: len(prepared.JSONLD) > 0, JSONLDSource: &prepared.JSONLDSource,
		URL: c.URL, RenderedSource: prepared.HTML, InlineSources: b.inlineSources,
		ExternalCSSBytes: result.Cost.ExternalCSSBytes, ExternalJSBytes: result.Cost.ExternalJSBytes,
		Rules: c.Build.Validation.Rules, Images: result.Images, FontAssets: b.fonts,
		Stylesheets: b.cssAssets(), Commerce: prepared.Commerce,
	})
	result.Validation = summarize("html", append(append(preliminary.Diagnostics, analysis.Diagnostics...), diagnostic.Apply(b.notices, c.Build.Validation.Rules)...))
	result.Validation.Stats = analysis.Stats
	var failure *check.Failure
	if err != nil && !errors.As(err, &failure) {
		return result, err
	}
	if result.Errors() > 0 {
		return result, &Failure{Errors: result.Errors()}
	}

	// 7. audit
	if options.Release {
		if result.Release == nil {
			result.Release = &release.Result{}
		}
		result.Release.Audit = release.Audit(page, b.sidecars(), p.Root)
		audited := append(result.Validation.Diagnostics, diagnostic.Apply(result.Release.Audit.Diagnostics, c.Build.Validation.Rules)...)
		stats := result.Validation.Stats
		result.Validation = summarize("audit", audited)
		result.Validation.Stats = stats
		if result.Errors() > 0 {
			return result, &Failure{Errors: result.Errors()}
		}
	}

	// 8. publish
	if options.Publish {
		if err := publish(p, result.Artifact); err != nil {
			return result, err
		}
		result.Published = true
	}
	return result, nil
}

// assemble rewrites the document so every local resource is bundled, packed
// and referenced by its final name.
func (b *builder) assemble(doc *html.Node, values map[string]any, prepared preparedTemplate, result *Result) error {
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

// optimize splits critical CSS. Missing browsers and failed measurements are
// warnings: the complete stylesheet is kept and the compile continues.
func (b *builder) optimize(doc *html.Node, result *Result) error {
	r, err := b.optimizeRelease(doc)
	result.Release = r
	if err == nil {
		return nil
	}
	if !optionalReleaseFailure(err) {
		return err
	}
	if result.Release == nil {
		result.Release = &release.Result{}
	}
	result.Release.Reason = "Critical CSS skipped: " + err.Error()
	if _, unavailable := release.BrowserPath(); unavailable == nil {
		b.notice("release.css", result.Release.Reason, "The complete CSS was kept. Check Chrome startup and page rendering, then compile with --release again.")
	}
	return nil
}

// describe fills the output listing from the assembled files plus the page.
func (b *builder) describe(result *Result, page []byte) {
	result.Artifact = map[string][]byte{"index.html": page}
	result.Files = []File{{"index.html", len(page)}}
	for _, file := range b.files {
		result.Artifact[file.name] = file.contents
		result.Files = append(result.Files, File{file.name, len(file.contents)})
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Name < result.Files[j].Name })
	for name := range b.external {
		result.External = append(result.External, name)
	}
	sort.Strings(result.External)
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

func ruleCheck(id string, level diagnostic.Severity) error {
	return check.ValidateOptions(map[string]diagnostic.Severity{id: level})
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
