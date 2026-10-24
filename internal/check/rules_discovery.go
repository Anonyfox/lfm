package check

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Parse only known directive tokens. Date-valued unavailable_after and unknown
// directives stay opaque; this is not a speculative robots grammar validator.
func snippetsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, key := range []string{"robots", "googlebot", "googlebot-news", "bingbot"} {
		for _, n := range robotsNodes(d, key) {
			noSnippet, limit := false, -1
			parts := strings.Split(strings.ToLower(Attr(n, "content")), ",")
			for _, part := range parts {
				// Also recognize the common space-separated flag form; valued
				// directives use comma boundaries so spaces after ':' remain valid.
				if containsWord(part, "nosnippet") {
					noSnippet = true
				}
				name, value, valued := strings.Cut(strings.TrimSpace(part), ":")
				if valued && strings.TrimSpace(name) == "max-snippet" {
					parsed, err := strconv.Atoi(strings.TrimSpace(value))
					if err == nil && parsed >= 0 && (limit < 0 || parsed < limit) {
						limit = parsed
					}
				}
			}
			var restrictions []string
			if noSnippet {
				restrictions = append(restrictions, "nosnippet")
			}
			if limit >= 0 {
				restrictions = append(restrictions, "max-snippet:"+strconv.Itoa(limit))
			}
			if len(restrictions) > 0 {
				issues = append(issues, d.At(n, Warning, robotsMessage(n, "Search snippet/content reuse is restricted by "+strings.Join(restrictions, ", ")+"."), "Remove unintended limits; nosnippet/max-snippet:0 suppress Google text previews and direct AI input, and positive limits constrain excerpts. Keep intentional publishing controls."))
			}
		}
	}
	return issues
}

// A structural extraction floor, not a content-quality/intent/word-count score.
// No CSS execution or JS inference: only explicit HTML state is known here.
func contentDiscoveryRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	var mains []*html.Node
	for _, n := range d.activeNodes("main, [role]") {
		if n.Namespace == "" && (n.Data == "main" || strings.TrimSpace(Attr(n, "role")) == "main") && !discoveryHidden(n) {
			mains = append(mains, n)
		}
	}
	if len(mains) == 0 {
		issues = append(issues, d.Whole(Warning, "No visible main-content landmark.", "Wrap the primary experience in <main>; intentionally hidden alternate views may remain."))
	} else if len(mains) > 1 {
		issues = append(issues, d.At(mains[1], Warning, "Multiple visible main-content landmarks.", "Use one main landmark for this document; hide inactive alternate views or use ordinary sections."))
	}
	roots := mains
	if len(roots) == 0 {
		roots = d.activeNodes("body")
	}
	hasText := false
	for _, n := range roots {
		hasText = hasText || initialText(n, false)
	}
	if !hasText {
		issues = append(issues, d.Whole(Warning, "Primary content has no initially readable HTML text.", "Prerender an answer, tool method or starting example in the main content; scripts, images, canvas, templates and closed/hidden views alone do not provide it."))
	}
	// Warn only if every initially readable primary text node is suppressed by a
	// supported data-nosnippet container. Small excluded legal/UI passages are fine.
	if hasText {
		available := false
		for _, n := range roots {
			blocked := false
			for p := n.Parent; p != nil; p = p.Parent {
				blocked = blocked || snippetExcluded(p)
			}
			available = available || !blocked && initialText(n, true)
		}
		if !available {
			issues = append(issues, d.At(roots[0], Warning, "All initially readable primary text is inside data-nosnippet containers.", "Leave the useful answer/method available to snippets; restrict only passages with a defined publishing need."))
		}
	}
	return issues
}

func initialText(n *html.Node, snippets bool) bool {
	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data) != ""
	}
	if n.Type == html.ElementNode {
		if HasAttr(n, "hidden") || strings.EqualFold(Attr(n, "aria-hidden"), "true") {
			return false
		}
		switch n.Data {
		case "script", "style", "template", "noscript", "svg", "canvas", "iframe", "object", "textarea", "select":
			return false
		}
		if n.Data == "dialog" && !HasAttr(n, "open") {
			return false
		}
		if snippets && snippetExcluded(n) {
			return false
		}
		if n.Data == "details" && !HasAttr(n, "open") {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "summary" {
					return initialText(c, snippets)
				}
			}
			return false
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if initialText(c, snippets) {
			return true
		}
	}
	return false
}

func snippetExcluded(n *html.Node) bool {
	return n.Namespace == "" && HasAttr(n, "data-nosnippet") && (n.Data == "div" || n.Data == "span" || n.Data == "section")
}

// Native initial state only. A closed details summary remains visible; its
// other children and unopened dialog contents do not establish initial content.
func discoveryHidden(n *html.Node) bool {
	if hidden(n) {
		return true
	}
	for child, p := n, n.Parent; p != nil; child, p = p, p.Parent {
		if p.Namespace != "" {
			continue
		}
		if p.Data == "dialog" && !HasAttr(p, "open") {
			return true
		}
		if p.Data == "details" && !HasAttr(p, "open") {
			var summary *html.Node
			for c := p.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "summary" {
					summary = c
					break
				}
			}
			if child != summary {
				return true
			}
		}
	}
	return false
}

func (d *Document) fragmentTarget(id string) *html.Node {
	if d.fragmentIDs != nil {
		if n := d.fragmentIDs[id]; n != nil {
			return n
		}
		return d.fragmentNames[id]
	}
	d.fragmentIDs, d.fragmentNames = map[string]*html.Node{}, map[string]*html.Node{}
	for _, n := range d.Elements {
		if inTemplate(n) {
			continue
		}
		if id := Attr(n, "id"); id != "" && d.fragmentIDs[id] == nil {
			d.fragmentIDs[id] = n
		}
		if name := Attr(n, "name"); n.Namespace == "" && n.Data == "a" && name != "" && d.fragmentNames[name] == nil {
			d.fragmentNames[name] = n
		}
	}
	return d.fragmentTarget(id)
}

func fragmentID(fragment string) (string, error) {
	// Split before decoding: %3A~%3A is an ordinary part of an authored ID.
	fragment, _, _ = strings.Cut(fragment, ":~:")
	return url.PathUnescape(fragment)
}

func fragmentDiscoveryRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	_, analyzer := d.linkFacts()
	base, _ := analyzer.Resolve(d.Options.URL)
	seen := map[*html.Node]bool{}
	for _, n := range d.activeNodes("a[href], area[href]") {
		value := Attr(n, "href")
		resolved, err := analyzer.Resolve(value)
		if err != nil || resolved.Fragment() == "" {
			continue
		}
		if !strings.HasPrefix(value, "#") && (d.Options.URL == "" || base == nil || resolved.Href(true) != base.Href(true)) {
			continue
		}
		id, err := fragmentID(resolved.Fragment())
		if err != nil {
			continue
		}
		if id == "" {
			continue
		}
		target := d.fragmentTarget(id)
		if target == nil || seen[target] {
			continue
		} // Missing targets belong to reference rules.
		seen[target] = true
		for p := target; p != nil; p = p.Parent {
			if p.Namespace == "" && HasAttr(p, "hidden") && !strings.EqualFold(Attr(p, "hidden"), "until-found") {
				issue := d.At(n, Warning, fmt.Sprintf("Fragment %q targets explicitly hidden content.", short(id)), "Make the landing target visible or use native revealable content; verify direct entry with JS enabled/disabled. Closed details and hidden=until-found are allowed.")
				issue.Related = []Location{d.At(p, Warning, "", "").Location}
				issues = append(issues, issue)
				break
			}
		}
	}
	return issues
}
