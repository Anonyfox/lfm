// Package validate analyzes generated HTML without modifying it or fetching URLs.
// Parsing/tokenization are delegated to x/net/html; selectors use Cascadia.
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/imaging"
	"github.com/Anonyfox/lfm/internal/templatecheck"
	"github.com/Anonyfox/lfm/internal/urlcheck"
)

type Severity = diagnostic.Severity

const (
	Error   = diagnostic.Error
	Warning = diagnostic.Warning
	Off     = diagnostic.Off
)

type Location = diagnostic.Location
type Diagnostic = diagnostic.Diagnostic

type Rule struct {
	ID          string
	Description string
	Check       func(*Document) []Diagnostic
}

// Options is also usable by small rule tests; no workbench/filesystem is needed.
type Options struct {
	URL             string
	Registry        *urlcheck.Registry
	GeneratedAssets map[string]bool
	JSONLDGenerated bool
	JSONLDSource    *Location
	// RenderedSource is supplementary pre-repair evidence, never a replacement for
	// analyzing the exact final bytes. Locations explicitly name rendered-template.html.
	RenderedSource []byte
	// InlineSources maps a SHA-256 of a final data URL to its workbench asset paths.
	InlineSources                     map[[32]byte][]string
	ExternalCSSBytes, ExternalJSBytes int
	Rules                             map[string]Severity
	Images                            []imaging.Decision
	FontAssets                        map[string]fontcheck.Info
	Stylesheets                       map[string][]byte // exact emitted CSS keyed by final URL
	Assets                            map[string][]byte // exact sidecar bytes keyed by emitted filename; no filesystem/network reads
	Commerce                          commercecheck.Facts
}

type Report struct {
	Stage       string       `json:"stage,omitempty"`
	Stats       Stats        `json:"stats,omitzero"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Errors      int          `json:"errors"`
	Warnings    int          `json:"warnings"`
}

type Failure struct{ Count int }

func (f *Failure) Error() string {
	return fmt.Sprintf("HTML validation failed (%d errors); output was not replaced", f.Count)
}

// Analyze parses once for all rules. It does not serialize, minify, or fix HTML.
func Analyze(raw []byte, options Options) (Report, error) {
	if err := ValidateOptions(options.Rules); err != nil {
		return Report{}, err
	}
	doc, err := Parse(raw, options)
	if err != nil {
		return Report{}, err
	}
	report := Run(doc, DefaultRules(), options.Rules)
	if report.Errors > 0 {
		return report, &Failure{report.Errors}
	}
	return report, nil
}

// Run permits any subset/custom rules, including a single rule on a tiny fixture.
// Configuration overrides for the default catalog use ValidateOptions first.
// Custom Go catalogs/overrides are owned by the caller.
func Run(doc *Document, rules []Rule, overrides map[string]Severity) Report {
	report := Report{Stage: "html", Stats: doc.Stats, Diagnostics: []Diagnostic{}}
	for _, rule := range rules {
		if overrides[rule.ID] == Off {
			continue
		}
		for _, diagnostic := range rule.Check(doc) {
			diagnostic.Rule = rule.ID
			if level := overrides[rule.ID]; level != "" {
				diagnostic.Severity = level
			}
			report.Diagnostics = append(report.Diagnostics, diagnostic)
		}
	}
	summary := diagnostic.Summarize(report.Diagnostics)
	report.Diagnostics = summary.Diagnostics
	report.Errors = summary.Errors
	report.Warnings = summary.Warnings
	return report
}

func ValidateOptions(overrides map[string]Severity) error {
	known := map[string]diagnostic.RuleInfo{}
	for _, rule := range Catalog() {
		known[rule.ID] = rule
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, exists := known[key]; !exists && !diagnostic.DynamicRule(key) {
			return fmt.Errorf("build.validation.rules: unknown rule %q", key)
		}
		level := overrides[key]
		if known[key].Required && level != Error {
			return fmt.Errorf("build.validation.rules.%s: required safety check must remain error", key)
		}
		if level != Off && level != Warning && level != Error {
			return fmt.Errorf("build.validation.rules.%s: use off, warn, or error", key)
		}
	}
	return nil
}

// Catalog lists checks from every lifecycle stage. Required checks cannot be disabled.
func Catalog() []diagnostic.RuleInfo {
	result := templatecheck.Catalog()
	result = append(result, commercecheck.Catalog()...)
	result = append(result, diagnostic.OperationalRules()...)
	for _, rule := range urlcheck.Catalog() {
		if rule.ID == "links.registry" || rule.ID == "links.literal" {
			result = append(result, rule)
		}
	}
	for _, rule := range DefaultRules() {
		result = append(result, diagnostic.RuleInfo{ID: rule.ID, Description: rule.Description})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func DefaultRules() []Rule {
	return []Rule{
		{"links.syntax", "Validate browser URL syntax, protocols and normalization", linksSyntaxRule},
		{"links.declared", "Rendered destinations match named config URLs", linksDeclaredRule},
		{"links.behavior", "Crawlability, mixed content and download behavior", linksBehaviorRule},
		{"links.fragments", "Same-document fragment destinations with explicit hidden blockers", fragmentDiscoveryRule},
		{"html.syntax", "Detect parser repairs and malformed markup", syntaxRule},
		{"html.document", "HTML5 document, head and encoding structure", documentRule},
		{"html.content-model", "Restricted parents, children and interactive nesting", contentRule},
		{"html.ids", "Unique, nonempty IDs without ASCII whitespace", idsRule},
		{"html.references", "Local fragments, labels and ARIA ID references", referencesRule},
		{"html.json", "Well-formed embedded JSON and JSON-LD", jsonRule},
		{"jsonld.block", "One optional compiler-owned JSON-LD block", jsonldBlockRule},
		{"jsonld.structure", "JSON-LD structure checked by offline standard expansion", jsonldRule("jsonld.structure")},
		{"jsonld.context", "Report remote contexts unavailable to offline validation", jsonldRule("jsonld.context")},
		{"jsonld.dropped", "Report data discarded by JSON-LD expansion", jsonldRule("jsonld.dropped")},
		{"commerce.checkout", "Hosted checkout URLs, stable Payment Links and supported query parameters", commerceRule("commerce.checkout")},
		{"commerce.discovery", "Declared offers expose usable native purchase anchors and fixed-price structured facts", commerceRule("commerce.discovery")},
		{"commerce.structured-data", "Expanded Schema.org Offer/BuyAction facts and exact declared price agreement", commerceRule("commerce.structured-data")},
		{"commerce.secrets", "Stripe secret-key shapes in final HTML and emitted text assets, without exposing their values", commerceSecretsRule},
		{"seo.metadata", "Title/description placement, one exact parameter-free self-canonical and language tags", metadataRule},
		{"seo.indexability", "Explicit indexing and link-following restrictions", indexabilityRule},
		{"seo.snippets", "Explicit text-snippet and search content-reuse restrictions", snippetsRule},
		{"seo.social", "Preview fields, placement, scalar duplicates and public URL identity", socialRule},
		{"sharing.discovery", "Early declared platform metadata, media references and discovery contracts", sharingDiscoveryRule},
		{"sharing.media", "Social image alternatives, dimensions and inspected metadata agreement", sharingMediaRule},
		{"sharing.x-player", "Declared X Player Card targets, dimensions, poster and fallback text", xPlayerRule},
		{"sharing.discord", "Declared Discord component JSON, limits, supported structure and return actions", discordRule},
		{"sharing.farcaster", "Declared Farcaster mini-app JSON, launch actions and inspected image limits", farcasterRule},
		{"sharing.snapchat", "Declared Snapchat sticker URL and inspected format, byte and dimension limits", snapchatRule},
		{"seo.headings", "Visible main heading, readable labels and heading hierarchy", headingsRule},
		{"seo.content", "Main landmark, initially readable text and blanket snippet exclusion", contentDiscoveryRule},
		{"a11y.images", "Explicit alternative text, including decorative empty alt", imageAltRule},
		{"a11y.controls", "Names for native/custom controls, dialogs and informative roles", controlsRule},
		{"a11y.relationships", "Native label, form, list, table and popup target associations", relationshipsRule},
		{"a11y.aria", "Focused ARIA state/value and custom-widget contract checks", ariaRule},
		{"a11y.focus", "Static focus order, aria-hidden conflicts and custom focus entry", focusRule},
		{"ux.forms", "Form-button intent and numeric input constraints/defaults", formsRule},
		{"ux.disclosures", "Native details/summary structure and declarative command targets", disclosuresRule},
		{"ux.media", "Autoplay requests without an initial mute", mediaInteractionRule},
		{"perf.html-size", "Warn near 2 MB; reject HTML above the conservative limit", sizeRule},
		{"perf.inline-assets", "Large embedded resources and inlined responsive variants", inlineRule},
		{"perf.images", "Image layout reservation and loading hints", imagePerformanceRule},
		{"perf.image-assets", "Inspected image metadata, byte/pixel budgets and excessive source resolution", imageAssetsRule},
		{"perf.fonts", "Font-face descriptors, source formats, loading policy and preloads", fontsRule},
		{"perf.font-assets", "Bounded font container inspection and byte budgets", fontAssetsRule},
		{"perf.viewport", "Mobile viewport and zoom availability", viewportRule},
		{"perf.budgets", "CSS/JS/DOM advisory budgets", budgetsRule},
		{"design.palette", "Stock generator accents, violet gradients and the assistant cream/terracotta palette", designRule("design.palette")},
		{"design.gradient-text", "Gradient-filled text via background-clip:text", designRule("design.gradient-text")},
		{"design.glow", "Multiple blurred decorative orbs and colored zero-offset glows", designRule("design.glow")},
		{"design.eyebrows", "Small tracked uppercase labels directly above headings without information of their own", designRule("design.eyebrows")},
		{"design.type", "One stock sans for display and body; italic-serif or gradient accent words in headlines", designRule("design.type")},
		{"design.emoji", "Emoji used as headings, buttons, labels or list bullets", designRule("design.emoji")},
		{"design.hero", "Pill badge above the hero headline and the centered hero formula", designRule("design.hero")},
		{"design.cards", "Rows of identical icon-topped cards", designRule("design.cards")},
		{"design.stats", "Stat strips of big round numbers with small labels", designRule("design.stats")},
		{"design.stripes", "Colored left-border accent stripes on cards", designRule("design.stripes")},
		{"design.glass", "Backdrop blur on three or more elements", designRule("design.glass")},
		{"design.surfaces", "Tailwind default soft shadows repeated across boxes", designRule("design.surfaces")},
		{"design.motion", "Scroll-reveal hooks on many blocks", designRule("design.motion")},
		{"design.arrows", "Arrow glyphs appended to most buttons and links", designRule("design.arrows")},
		{"design.chrome", "Repeated middot meta strings and fake window chrome", designRule("design.chrome")},
		{"design.mono", "Monospace used for many non-code labels and captions", designRule("design.mono")},
		{"design.proof", "Generic 'trusted by' proof furniture", designRule("design.proof")},
		{"design.sections", "Four or more stock landing-page sections", designRule("design.sections")},
		{"design.defaults", "Combination of generator-default signals, including weak ones", designDefaultsRule},
	}
}

func join(parts ...string) string { return strings.Join(parts, "; ") }
