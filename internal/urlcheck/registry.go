package urlcheck

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

type Declaration struct {
	Path, Value string
	Location    diagnostic.Location
}
type Registry struct {
	Analyzer     *Analyzer
	Declarations []Declaration
	keys         map[string][]Declaration
	checked      bool
}

func NewRegistry(base string, declarations []Declaration) *Registry {
	r := &Registry{Analyzer: New(base), Declarations: append([]Declaration(nil), declarations...), keys: map[string][]Declaration{}}
	sort.Slice(r.Declarations, func(i, j int) bool { return r.Declarations[i].Path < r.Declarations[j].Path })
	for _, d := range r.Declarations {
		seen := map[string]bool{}
		for _, form := range renderedForms(d.Value) {
			if key := r.Analyzer.Key(form); key != "" && !seen[key] {
				r.keys[key] = append(r.keys[key], d)
				seen[key] = true
			}
		}
	}
	return r
}
func (r *Registry) Match(raw string) []Declaration { return r.keys[r.Analyzer.Key(raw)] }
func (r *Registry) Checked() bool                  { return r.checked }
func (r *Registry) Check() []diagnostic.Diagnostic {
	r.checked = true
	var issues []diagnostic.Diagnostic
	seen := map[string]Declaration{}
	for _, d := range r.Declarations {
		name := strings.TrimPrefix(d.Path, "urls.")
		if strings.HasPrefix(d.Path, "urls[") {
			name, _ = strconv.Unquote(strings.TrimSuffix(strings.TrimPrefix(d.Path, "urls["), "]"))
		}
		if strings.TrimSpace(name) == "" {
			issues = append(issues, diagnostic.Diagnostic{Rule: "links.registry", Severity: diagnostic.Error, Location: d.Location, Path: d.Path, Message: "URL declaration needs a nonempty unique name.", Hint: "Use a purpose-based name such as checkout, calculator or relatedHotels."})
		} else if (strings.HasPrefix(d.Path, "urls.") || strings.HasPrefix(d.Path, "urls[")) && !naturalName(name) {
			issues = append(issues, diagnostic.Diagnostic{Rule: "links.registry", Severity: diagnostic.Warning, Location: d.Location, Path: d.Path, Message: "URL name is awkward to use as a template path.", Hint: "Prefer a descriptive camelCase/snake_case identifier, e.g. urls.checkout or urls.relatedHotels; arbitrary JSON keys still work with index."})
		}
		if LocalAsset(d.Value) || Embedded(d.Value) {
			continue
		}
		result := r.Analyzer.Inspect(d.Value)
		for _, finding := range result.Findings {
			issues = append(issues, diagnostic.Diagnostic{Rule: "links.syntax", Severity: finding.Severity, Location: d.Location, Path: d.Path, Message: finding.Message, Hint: finding.Hint, Reference: "https://url.spec.whatwg.org/"})
		}
		if !result.Valid() {
			continue
		}
		for _, form := range renderedForms(d.Value)[1:] {
			rendered, err := r.Analyzer.Resolve(form)
			if err != nil || (result.URL.Scheme() == "http" || result.URL.Scheme() == "https") && (rendered.Scheme() != result.URL.Scheme() || rendered.Host() != result.URL.Host()) || d.Value != strings.TrimSpace(d.Value) {
				issues = append(issues, diagnostic.Diagnostic{Rule: "links.syntax", Severity: diagnostic.Error, Location: d.Location, Path: d.Path, Message: "Contextual URL escaping would invalidate or change the configured destination.", Hint: "Trim surrounding whitespace and replace backslashes/ambiguous host spelling; use deliberate percent encoding for literal path/query characters."})
				break
			}
		}

		key := result.URL.Href(false)
		if first, ok := seen[key]; ok {
			issues = append(issues, diagnostic.Diagnostic{Rule: "links.registry", Severity: diagnostic.Warning, Location: d.Location, Path: d.Path, Related: []diagnostic.Location{first.Location}, Message: fmt.Sprintf("URL duplicates the destination already named %s.", first.Path), Hint: "Reuse one named URL at each placement; keep aliases only if they intentionally need independent future changes."})
		} else {
			seen[key] = d
		}
	}
	return issues
}
func naturalName(s string) bool {
	if s == "" {
		return false
	}
	for i, ch := range s {
		if !(unicode.IsLetter(ch) || ch == '_' || i > 0 && unicode.IsDigit(ch)) {
			return false
		}
	}
	return true
}

var urlForms = htmltemplate.Must(htmltemplate.New("forms").Parse(`<a href="{{.}}"></a><img srcset="{{.}}">`))

// The final HTML contains Go's context-specific URL normalization, which can
// differ from WHATWG's serialization (Unicode percent-hex casing, apostrophes,
// commas in srcset). Compare those exact library-produced forms as well. This
// does not broaden matching to arbitrary hosts, query variants or URL prefixes.
func renderedForms(raw string) []string {
	forms := []string{raw}
	if Embedded(raw) || !Trustable(raw) {
		return forms
	}
	var output bytes.Buffer
	if err := urlForms.Execute(&output, htmltemplate.URL(raw)); err != nil {
		return forms
	}
	for _, ref := range References(output.Bytes()) {
		forms = append(forms, ref.Value)
	}
	return forms
}
