// Package check analyzes generated HTML without modifying it or fetching URLs.
// Parsing/tokenization are delegated to x/net/html; selectors use Cascadia.
package check

import (
	"strings"

	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/imaging"
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

// Rule is one final-HTML check. Its RuleInfo (ID, Summary, Help) is the same
// metadata the catalog and `lfm rules` show; there is no second description.
type Rule struct {
	diagnostic.RuleInfo
	Check func(*Document) []Diagnostic
}

// Options is also usable by small rule tests; no workbench/filesystem is needed.
type Options struct {
	File            string
	RobotsHeader    []string
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
	Budgets                           Budgets
	// CriticalCSSBytes are the inline copy of the stylesheet's first-screen
	// rules a release adds; a budget counts the stylesheet once.
	CriticalCSSBytes int
	Images           []imaging.Decision
	FontAssets       map[string]fontcheck.Info
	Stylesheets      map[string][]byte // exact emitted CSS keyed by final URL
	Assets           map[string][]byte // exact sidecar bytes keyed by emitted filename; no filesystem/network reads
	Commerce         commercecheck.Facts
	Concept          *ConceptFacts // decisions of concept.md, when the project has one
	// Starter texts and images (SHA-256) of lfm init a built page must have
	// replaced; empty while the page is still the starter page.
	StarterTexts  []string
	StarterImages [][32]byte
}

// Budgets are a page's advisory sizes for perf.budgets; zero keeps the
// default.
type Budgets struct{ CSSKiB, JSKiB, Elements, Depth int }

type Report struct {
	Stage       string       `json:"stage,omitempty"`
	Stats       Stats        `json:"stats,omitzero"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Errors      int          `json:"errors"`
	Warnings    int          `json:"warnings"`
}

// Analyze parses once for all rules. It does not serialize, minify, or fix HTML.
// Findings, errors included, are part of the report; the returned error only
// means the document could not be analyzed at all.
func Analyze(raw []byte, options Options) (Report, error) {
	doc, err := Parse(raw, options)
	if err != nil {
		return Report{}, err
	}
	return Run(doc, DefaultRules(), options.Rules), nil
}

// Run permits any subset/custom rules, including a single rule on a tiny fixture.
// Overrides are validated against the whole catalog by the compiler first.
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

func join(parts ...string) string { return strings.Join(parts, "; ") }
