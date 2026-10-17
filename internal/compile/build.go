package compile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/imaging"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/check"

	"golang.org/x/net/html"
)

type File struct {
	Name  string `json:"name"`
	Bytes int    `json:"bytes"`
}

type Report struct {
	Output     string             `json:"output"`
	PageURL    string             `json:"url,omitempty"`
	Files      []File             `json:"files"`
	External   []string           `json:"externalAssets,omitempty"`
	Warnings   []string           `json:"warnings,omitempty"`
	Validation *check.Report   `json:"validation,omitempty"`
	Published  bool               `json:"published"`
	Release    *release.Result    `json:"release,omitempty"`
	Packing    []AssetDecision    `json:"assets,omitempty"`
	Cost       *Cost              `json:"cost,omitempty"`
	Compaction *Compaction        `json:"htmlCompaction,omitempty"`
	Images     []imaging.Decision `json:"images,omitempty"`
	Fonts      []fontcheck.Info   `json:"fonts,omitempty"`
	FontHints  int                `json:"fontHintRepairs,omitempty"`
	Inputs     []string           `json:"-"` // real resolved assets/code for preview dependency watching
	DurationMS int64              `json:"durationMs"`
}

type emitted struct {
	name, origin string
	contents     []byte
}

type builder struct {
	root, stage     string
	config          Config
	mu              sync.Mutex // esbuild plugin callbacks may run concurrently.
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

// Run validates and assembles a workbench. Check mode leaves output untouched.
func Run(directory string, publish bool) (Report, error) {
	return RunWithOptions(directory, RunOptions{Publish: publish})
}

type RunOptions struct{ Publish, Release bool }

func RunWithOptions(directory string, options RunOptions) (report Report, runErr error) {
	start := time.Now()
	defer func() {
		report.DurationMS = time.Since(start).Milliseconds()
		if runErr != nil && (report.Validation == nil || report.Validation.Errors == 0) {
			var issues []diagnostic.Diagnostic
			if report.Validation != nil {
				issues = report.Validation.Diagnostics
			}
			failure := templateReport(append(issues, diagnostic.Diagnostic{Rule: "build.failed", Severity: diagnostic.Error, Location: diagnostic.Location{File: entryTemplate, Line: 1, Column: 1}, Message: runErr.Error(), Hint: "Fix the named source/path or bundler error and rebuild; previous output is preserved."}))
			failure.Stage = "build"
			report.Validation = &failure
		}
	}()
	root, err := filepath.Abs(directory)
	if err != nil {
		return report, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return report, err
	}
	report.Output = filepath.Join(root, "output")
	c, values, sources, configErr := loadConfig(root)
	report.PageURL = c.URL
	prepared, preliminary, registry, err := prepareTemplate(root, c, values, sources, configErr)
	if err != nil {
		report.Validation = &preliminary
		return report, err
	}
	page := prepared.HTML
	cache := filepath.Join(root, ".micro")
	output := filepath.Join(root, "output")
	for _, path := range []string{cache, output} {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return report, err
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return report, fmt.Errorf("%s must be a real directory, not a file or symlink", path)
		}
	}
	if err := os.MkdirAll(cache, 0755); err != nil {
		return report, err
	}
	lock := filepath.Join(cache, "build.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		return report, fmt.Errorf("cannot acquire %s: %w; if a previous build was interrupted, remove the lock after confirming no build is running", lock, err)
	}
	defer os.Remove(lock)
	stage, err := os.MkdirTemp(cache, "build-")
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0755); err != nil {
		return report, err
	}
	b := &builder{root: root, stage: stage, config: c, files: map[string]emitted{}, external: map[string]bool{}, inlineSources: map[[32]byte][]string{}}
	defer func() {
		for path := range b.inputs {
			report.Inputs = append(report.Inputs, path)
		}
		sort.Strings(report.Inputs)
	}()
	b.helpers = prepared.Helpers

	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return report, fmt.Errorf("HTML: %w", err)
	}
	b.collectAssetRoles(doc)
	imageInputs := captureImages(doc)
	data, err := browserJSON(values, c.Build.BrowserData)
	if err != nil {
		return report, err
	}
	if err := b.processHTML(doc, data, prepared.JSONLD); err != nil {
		return report, err
	}
	if err := b.ensureFavicon(doc); err != nil {
		return report, err
	}
	report.Images = b.prepareImages(doc, imageInputs)
	report.Packing, err = b.planAssets(doc)
	if err != nil {
		return report, err
	}
	if err := b.packCode(); err != nil {
		return report, err
	}
	report.Packing = b.fileDecisions(report.Packing)
	b.fonts, report.Fonts = b.inspectFonts(doc)
	if err := b.resourceHints(doc); err != nil {
		return report, err
	}
	report.FontHints = b.fontHints
	if options.Release {
		report.Release, err = b.optimizeRelease(doc)
		if err != nil {
			if !optionalReleaseFailure(err) {
				return report, err
			}
			report.Release.Reason = "Critical CSS skipped: " + err.Error()
			if _, availability := release.BrowserPath(); availability == nil {
				b.notice("release.css", report.Release.Reason, "Full CSS was retained. Check Chrome sandbox/startup and page rendering; retry --release.")
			}
		}
	}
	var serialized bytes.Buffer
	if err := html.Render(&serialized, doc); err != nil {
		return report, err
	}
	result, compaction := compactHTML(serialized.Bytes())
	report.Compaction = &compaction
	if compaction.Skipped != "" {
		b.notice("html.compaction", "HTML syntax compaction skipped: "+compaction.Skipped, "The original serialized HTML was retained. Report this input as a compiler regression; the final validation gate still runs.")
	}
	report.Output = output
	for _, file := range b.files {
		report.Files = append(report.Files, File{file.name, len(file.contents)})
	}
	for name := range b.external {
		report.External = append(report.External, name)
	}
	report.Files = append(report.Files, File{"index.html", len(result)})
	sort.Slice(report.Files, func(i, j int) bool { return report.Files[i].Name < report.Files[j].Name })
	sort.Strings(report.External)
	for _, warning := range b.warnings {
		b.notice("bundle.warning", strings.TrimSpace(warning), "Review the bundler warning in the named source.")
	}
	report.Cost = measureCost(result, doc, b.files)
	// Analyze the exact buffer written below, without rewriting it. Sidecars may
	// exist in scratch, but index.html is never written before validation passes.
	generated := map[string]bool{}
	assets := map[string][]byte{}
	for _, file := range b.files {
		generated[file.name] = true
		assets[file.name] = file.contents
	}
	analysis, err := check.Analyze(result, check.Options{Registry: registry, GeneratedAssets: generated, Assets: assets, JSONLDGenerated: len(prepared.JSONLD) > 0, JSONLDSource: &prepared.JSONLDSource, URL: c.URL, RenderedSource: page, InlineSources: b.inlineSources, ExternalCSSBytes: report.Cost.ExternalCSSBytes, ExternalJSBytes: report.Cost.ExternalJSBytes, Rules: c.Build.Validation.Rules, Images: report.Images, FontAssets: b.fonts, Stylesheets: b.cssAssets(), Commerce: prepared.Commerce})
	issues := append(preliminary.Diagnostics, analysis.Diagnostics...)
	summary := diagnostic.Summarize(append(issues, diagnostic.Apply(b.notices, c.Build.Validation.Rules)...))
	analysis.Stage = "html"
	analysis.Diagnostics = summary.Diagnostics
	analysis.Errors = summary.Errors
	analysis.Warnings = summary.Warnings
	report.Validation = &analysis
	if err != nil {
		return report, err
	}
	if analysis.Errors > 0 {
		return report, &check.Failure{Count: analysis.Errors}
	}
	if options.Release {
		if report.Release == nil {
			report.Release = &release.Result{}
		}
		report.Release.Audit = release.Audit(result, b.previewAssets(), root, cache)
		summary := diagnostic.Summarize(append(report.Validation.Diagnostics, diagnostic.Apply(report.Release.Audit.Diagnostics, c.Build.Validation.Rules)...))
		report.Validation.Diagnostics, report.Validation.Errors, report.Validation.Warnings = summary.Diagnostics, summary.Errors, summary.Warnings
	}
	if report.Validation.Errors > 0 {
		return report, &check.Failure{Count: report.Validation.Errors}
	}
	if err := b.emit("index.html", "document", result); err != nil {
		return report, err
	}
	if options.Publish {
		if err := replaceOutput(stage, output, cache); err != nil {
			return report, err
		}
		report.Published = true
	}
	return report, nil
}

func (b *builder) notice(rule, message, hint string) {
	b.notices = append(b.notices, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: "index.html", Line: 1, Column: 1}, Message: message, Hint: hint})
}

func (b *builder) previewAssets() map[string][]byte {
	assets := map[string][]byte{}
	for _, file := range b.files {
		assets[file.name] = file.contents
	}
	return assets
}

func (b *builder) emit(name, origin string, content []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if name == "." || name != filepath.Base(name) || strings.ContainsAny(name, `/\\`) {
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
	if err := os.WriteFile(filepath.Join(b.stage, name), content, 0644); err != nil {
		return err
	}
	b.files[key] = emitted{name, origin, content}
	return nil
}

func replaceOutput(stage, output, cache string) error {
	backup, err := os.MkdirTemp(cache, "previous-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	hadOutput := false
	if _, err := os.Lstat(output); err == nil {
		if err := os.Rename(output, backup); err != nil {
			return err
		}
		hadOutput = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, output); err != nil {
		if hadOutput {
			if restore := os.Rename(backup, output); restore != nil {
				return fmt.Errorf("publish: %v; restore failed: %v; previous output is at %s", err, restore, backup)
			}
		}
		return err
	}
	if hadOutput {
		return os.RemoveAll(backup)
	}
	return nil
}
