package check

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfm/internal/jsonldcheck"
	"golang.org/x/net/html"
)

type ldBlock struct {
	node *html.Node
	data []any
}

func ldScripts(d *Document) []*html.Node {
	var nodes []*html.Node
	for _, n := range d.activeNodes("script[type]") {
		if strings.EqualFold(strings.TrimSpace(Attr(n, "type")), "application/ld+json") {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

func jsonldBlockRule(d *Document) []Diagnostic {
	nodes := ldScripts(d)
	expected := len(nodes)
	if d.Options.Registry != nil {
		expected = 0
		if d.Options.JSONLDGenerated {
			expected = 1
		}
	} else if expected > 1 {
		expected = 1
	}
	if len(nodes) == expected {
		return nil
	}
	message := fmt.Sprintf("Expected %d compiler-owned JSON-LD block(s), found %d.", expected, len(nodes))
	hint := "Declare structured data under lfm.json jsonld and remove handwritten script blocks; use @graph or a top-level array for multiple entities. One block is a maintenance convention."
	if len(nodes) > 0 {
		return []Diagnostic{d.At(nodes[0], Error, message, hint)}
	}
	return []Diagnostic{d.Whole(Error, message, hint)}
}

func (d *Document) jsonldFacts() []Diagnostic {
	if d.jsonldAnalyzed {
		return d.jsonldFindings
	}
	d.jsonldAnalyzed = true
	nodes := ldScripts(d)
	for i, node := range nodes {
		expanded, findings := jsonldcheck.Expand([]byte(Text(node)), d.Options.URL)
		d.jsonldExpanded = append(d.jsonldExpanded, ldBlock{node, expanded})
		for _, finding := range findings {
			level := Error
			if finding.Warning {
				level = Warning
			}
			issue := d.At(node, level, finding.Message, finding.Hint)
			issue.Rule = finding.Rule
			if d.Options.JSONLDGenerated && d.Options.JSONLDSource != nil && i == len(nodes)-1 {
				issue.Path = "jsonld"
				issue.Related = []Location{*d.Options.JSONLDSource}
			}
			issue.Reference = "https://www.w3.org/TR/json-ld11-api/"
			d.jsonldFindings = append(d.jsonldFindings, issue)
		}
	}
	return d.jsonldFindings
}

func jsonldRule(id string) func(*Document) []Diagnostic {
	return func(d *Document) []Diagnostic {
		var issues []Diagnostic
		for _, issue := range d.jsonldFacts() {
			if issue.Rule == id {
				issues = append(issues, issue)
			}
		}
		return issues
	}
}
