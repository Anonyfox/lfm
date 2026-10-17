package check

import (
	"net/url"
	"strings"

	"github.com/Anonyfox/lfm/internal/urlcheck"
)

func (d *Document) linkFacts() ([]urlcheck.Reference, *urlcheck.Analyzer) {
	if d.linkAnalyzer != nil {
		return d.linkRefs, d.linkAnalyzer
	}
	d.linkAnalyzer = urlcheck.New(d.Options.URL)
	if d.Options.Registry != nil {
		d.linkAnalyzer = d.Options.Registry.Analyzer
	}
	d.linkRefs = urlcheck.AllReferences(d.Raw)
	return d.linkRefs, d.linkAnalyzer
}
func (d *Document) linkAt(ref urlcheck.Reference, severity Severity, message, hint string) Diagnostic {
	finding := Diagnostic{Severity: severity, Location: location("index.html", d.lines, ref.Offset), Element: ref.Tag, Message: message, Hint: hint}
	if d.Options.Registry != nil {
		if names := d.Options.Registry.Match(ref.Value); len(names) > 0 {
			finding.Path = names[0].Path
			finding.Related = []Location{names[0].Location}
		}
	}
	return finding
}
func linksSyntaxRule(d *Document) []Diagnostic {
	refs, analyzer := d.linkFacts()
	var issues []Diagnostic
	for _, ref := range refs {
		if urlcheck.Embedded(ref.Value) {
			continue
		}
		// Named values are already checked at the original config location. Validate
		// the final value too, but avoid repeating identical config findings verbatim.
		named := d.Options.Registry != nil && len(d.Options.Registry.Match(ref.Value)) > 0
		for _, finding := range analyzer.Inspect(ref.Value).Findings {
			if named && d.Options.Registry.Checked() && finding.Severity != Error {
				continue
			}
			issue := d.linkAt(ref, finding.Severity, ref.Attribute+": "+finding.Message, finding.Hint)
			issue.Reference = "https://url.spec.whatwg.org/"
			issues = append(issues, issue)
		}
	}
	return issues
}
func linksDeclaredRule(d *Document) []Diagnostic {
	if d.Options.Registry == nil {
		return nil
	} // standalone HTML rule tests need no workbench
	refs, analyzer := d.linkFacts()
	var issues []Diagnostic
	for _, ref := range refs {
		if urlcheck.Embedded(ref.Value) || ref.Resource && strings.HasPrefix(ref.Value, "#") || d.generatedURL(ref.Value, analyzer) {
			continue
		}
		if len(d.Options.Registry.Match(ref.Value)) > 0 {
			continue
		}
		issues = append(issues, d.linkAt(ref, Error, "Rendered "+ref.Attribute+" destination has no named config URL.", urlcheck.URLHint(ref.Attribute)+" Declare the complete URL, including deliberate query/fragment variants."))
	}
	return issues
}
func (d *Document) generatedURL(raw string, analyzer *urlcheck.Analyzer) bool {
	if len(d.Options.GeneratedAssets) == 0 {
		return false
	}
	u, err := analyzer.Resolve(raw)
	if err != nil {
		return false
	}
	base, err := analyzer.Resolve(".")
	if err != nil {
		return false
	}
	if u.Scheme() != base.Scheme() || u.Host() != base.Host() {
		return false
	}
	path, err := url.PathUnescape(u.Pathname())
	if err != nil {
		return false
	}
	directory, err := url.PathUnescape(base.Pathname())
	if err != nil {
		return false
	}
	if !strings.HasPrefix(path, directory) {
		return false
	}
	return d.Options.GeneratedAssets[strings.TrimPrefix(path, directory)]
}
func linksBehaviorRule(d *Document) []Diagnostic {
	refs, analyzer := d.linkFacts()
	var issues []Diagnostic
	for _, ref := range refs {
		scheme := urlcheck.Scheme(ref.Value)
		if ref.Resource && scheme == "http" && strings.HasPrefix(strings.ToLower(d.Options.URL), "https:") {
			issues = append(issues, d.linkAt(ref, Warning, "HTTP resource is referenced by an HTTPS page.", "Use an HTTPS resource endpoint; browser mixed-content blocking/upgrades can break this dependency."))
		}
		if (ref.Tag == "a" || ref.Tag == "area") && (scheme == "javascript" || scheme == "vbscript" || strings.TrimSpace(ref.Value) == "#") {
			issues = append(issues, d.linkAt(ref, Warning, "Link is a script/placeholder rather than a crawlable destination.", "Use a real named href for navigation or a button for an interaction."))
		}
	}
	for _, node := range d.activeNodes("a, area, [role=link]") {
		// Short fragments belong to html.references. Full/path/query-relative
		// links need the configured document identity; inert template links do not.
		value := Attr(node, "href")
		if (node.Data == "a" || node.Data == "area") && d.Options.URL != "" && HasAttr(node, "href") && !strings.HasPrefix(value, "#") {
			u, err := analyzer.Resolve(value)
			base, baseErr := analyzer.Resolve(d.Options.URL)
			if err == nil && baseErr == nil && u.Href(true) == base.Href(true) && u.Fragment() != "" {
				id, err := fragmentID(u.Fragment())
				if err == nil && id != "" && !strings.EqualFold(id, "top") && d.fragmentTarget(id) == nil {
					issues = append(issues, d.At(node, Warning, "Same-page URL fragment has no matching target.", "Add/fix the destination id or legacy named anchor; keep externally generated targets only when intentional."))
				}
			}
		}
		if !HasAttr(node, "href") && (HasAttr(node, "onclick") || Attr(node, "role") == "link") {
			issue := d.At(node, Warning, "Navigation depends on a handler/role without a crawlable href.", "Provide an <a href> for navigation. Use a button for actions.")
			issue.Reference = "https://developers.google.com/search/docs/crawling-indexing/links-crawlable"
			issues = append(issues, issue)
		}
		if d.Options.URL != "" && HasAttr(node, "download") && HasAttr(node, "href") {
			destination, err := analyzer.Resolve(Attr(node, "href"))
			base, baseErr := analyzer.Resolve(d.Options.URL)
			if err == nil && baseErr == nil && (destination.Scheme() == "http" || destination.Scheme() == "https") && (destination.Scheme() != base.Scheme() || destination.Host() != base.Host()) {
				issues = append(issues, d.At(node, Warning, "download points to another origin; the browser can ignore the attribute.", "Use a same-origin emitted file or make the remote server return an appropriate download response."))
			}
		}
	}

	for _, node := range d.activeNodes("[href]") {
		if node.Namespace == "" && node.Data != "a" && node.Data != "area" && node.Data != "link" && node.Data != "base" {
			issues = append(issues, d.At(node, Warning, "href is on an element that does not create a standard hyperlink.", "Use an <a href> for crawlable navigation; custom component behavior is not a crawler-readable link by itself."))
		}
	}
	return issues
}
