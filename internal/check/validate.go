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

func join(parts ...string) string { return strings.Join(parts, "; ") }
