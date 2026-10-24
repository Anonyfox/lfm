package check

import (
	"strings"

	"golang.org/x/net/html"
)

// A module script and an empty mount are evidence of a client-rendered shell,
// only while the body has no meaningful initial content. Hydrated pages stay quiet.
func clientShellRule(d *Document) []Diagnostic {
	module, mount := false, false
	for _, n := range d.Elements {
		if d.inTemplate(n) || n.Namespace != "" {
			continue
		}
		if n.Data == "script" && strings.EqualFold(Attr(n, "type"), "module") {
			module = true
		}
		if (n.Data == "div" || n.Data == "main") && Attr(n, "id") != "" && n.Parent != nil && n.Parent.Data == "body" {
			empty := true
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode || c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
					empty = false
					break
				}
			}
			mount = mount || empty
		}
	}
	if !module || !mount {
		return nil
	}
	words, size := 0, 0
	var count func(*html.Node)
	count = func(n *html.Node) {
		if n.Type == html.TextNode {
			words += len(strings.Fields(n.Data))
			size += len(strings.TrimSpace(n.Data))
			return
		}
		if discoveryHidden(n) {
			return
		}
		switch n.Data {
		case "script", "style", "template", "noscript", "svg", "canvas", "iframe", "object", "textarea", "select":
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			count(c)
		}
	}
	for _, body := range d.activeNodes("body") {
		count(body)
	}
	if words > 3 || size > 40 {
		return nil
	}
	// A real heading establishes initial content even on a very short page.
	for _, n := range d.activeNodes("h1") {
		if !discoveryHidden(n) && strings.TrimSpace(Text(n)) != "" {
			return nil
		}
	}
	return []Diagnostic{d.Whole(Warning, "Initial HTML is a client-rendered shell; JavaScript renders the page content.", "Prerender or server-render readable content and head metadata so search, sharing and readers without JavaScript can use the page.")}
}

// Only absent content is explained by the shell. Invalid metadata, syntax,
// robots directives, resource problems and other independent findings remain.
func shellExplains(issue Diagnostic) bool {
	switch issue.Rule {
	case "seo.metadata":
		return issue.Message == "Page title is missing or empty." || issue.Message == "Meta description is missing or empty." || issue.Message == "Canonical URL is missing."
	case "seo.headings":
		return issue.Message == "No visible main h1 heading."
	case "seo.social":
		return strings.HasPrefix(issue.Message, "Social preview is missing nonempty ")
	case "seo.content":
		return issue.Message == "No visible main-content landmark." || issue.Message == "Primary content has no initially readable HTML text."
	}
	return false
}

func groupShellFindings(issues []Diagnostic) []Diagnostic {
	shell := -1
	for i := range issues {
		if issues[i].Rule == "html.client-shell" {
			shell = i
			break
		}
	}
	if shell < 0 {
		return issues
	}
	var missing []string
	for _, issue := range issues {
		if shellExplains(issue) {
			missing = append(missing, issue.Rule+": "+issue.Message)
			if issue.Severity == Error {
				issues[shell].Severity = Error
			}
		}
	}
	if len(missing) > 0 {
		issues[shell].Hint += " Missing in initial HTML: " + strings.Join(missing, " ")
	}
	out := issues[:0]
	for _, issue := range issues {
		if !shellExplains(issue) {
			out = append(out, issue)
		}
	}
	return out
}
