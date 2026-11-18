package check

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

type schemaNode struct {
	value  map[string]any
	script *html.Node
}
type pageGraph struct {
	nodes         []schemaNode
	ids           map[string]map[string]any
	organizations []organization
}

func (d *Document) seoGraph() *pageGraph {
	d.jsonldFacts()
	if d.graph != nil || len(d.jsonldExpanded) == 0 {
		return d.graph
	}
	g := &pageGraph{ids: map[string]map[string]any{}}
	var visit func(any, *html.Node)
	visit = func(raw any, script *html.Node) {
		switch v := raw.(type) {
		case []any:
			for _, child := range v {
				visit(child, script)
			}
		case map[string]any:
			if _, literal := v["@value"]; literal {
				return
			}
			if id, _ := v["@id"].(string); id != "" && len(v) > 1 {
				g.ids[id] = v
			}
			node := schemaNode{v, script}
			g.nodes = append(g.nodes, node)
			if id, _ := v["@id"].(string); id != "" && hasSchemaType(v, organizationTypes) {
				g.organizations = append(g.organizations, describeOrganization(id, node))
			}
			for _, key := range slices.Sorted(maps.Keys(v)) {
				visit(v[key], script)
			}
		}
	}
	for _, block := range d.jsonldExpanded {
		visit(block.data, block.node)
	}
	d.graph = g
	return g
}

func (g *pageGraph) resolve(raw any) map[string]any {
	obj, _ := raw.(map[string]any)
	if id, _ := obj["@id"].(string); len(obj) == 1 && id != "" {
		return g.ids[id]
	}
	return obj
}

func faqText(n *html.Node, b *strings.Builder, styles *styleModel) {
	if n.Type == html.TextNode {
		if styles == nil || n.Parent == nil || !groundHidden(styles, n.Parent) {
			b.WriteString(n.Data)
		}
		return
	}
	if n.Type == html.ElementNode {
		if HasAttr(n, "hidden") || strings.EqualFold(Attr(n, "aria-hidden"), "true") {
			return
		}
		switch n.Data {
		case "head", "script", "style", "template", "noscript":
			return
		}
	}
	boundary := copyBlocks[n.Data] || n.Data == "br"
	if boundary {
		b.WriteByte(' ')
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		faqText(c, b, styles)
	}
	if boundary {
		b.WriteByte(' ')
	}
}

func faqRule(d *Document) []Diagnostic {
	g := d.seoGraph()
	if g == nil {
		return nil
	}
	var issues []Diagnostic
	var visible, compact string
	ready := false
	for _, n := range g.nodes {
		if !schemaType(n.value, "FAQPage") {
			continue
		}
		if !ready {
			var b strings.Builder
			for _, body := range d.activeNodes("body") {
				faqText(body, &b, d.styles())
			}
			visible = strings.Join(strings.Fields(b.String()), " ")
			compact = strings.ReplaceAll(visible, " ", "")
			ready = true
		}
		for _, item := range schemaValues(n.value, "mainEntity") {
			q := g.resolve(item)
			if !schemaType(q, "Question") {
				continue
			}
			name := strings.Join(strings.Fields(schemaText(q, "name")), " ")
			if name == "" || !strings.Contains(visible, name) {
				issues = append(issues, d.At(n.script, Warning, fmt.Sprintf("FAQ question %q is missing from the visible text.", name), "Show the same question on the page and in FAQPage mainEntity."))
			}
			for _, a := range schemaValues(q, "acceptedAnswer") {
				answer := schemaText(g.resolve(a), "text")
				if tree, err := html.Parse(strings.NewReader(answer)); err == nil {
					var b strings.Builder
					faqText(tree, &b, nil)
					answer = strings.Join(strings.Fields(b.String()), "")
				}
				if answer == "" || !strings.Contains(compact, answer) {
					issues = append(issues, d.At(n.script, Warning, fmt.Sprintf("The accepted answer for FAQ question %q is missing from the visible text.", name), "Render the same answer text on the page and in acceptedAnswer.text."))
				}
			}
		}
	}
	return issues
}

type organization struct {
	id     string
	fields [4]string
	node   schemaNode
}

func describeOrganization(id string, n schemaNode) organization {
	org := organization{id: id, node: n}
	for i, key := range []string{"name", "url", "logo", "sameAs"} {
		var values []string
		for _, v := range schemaValues(n.value, key) {
			encoded, _ := json.Marshal(v)
			values = append(values, string(encoded))
		}
		slices.Sort(values)
		org.fields[i] = strings.Join(slices.Compact(values), "\n")
	}
	return org
}

func identityRule(d *Document) []Diagnostic {
	g := d.seoGraph()
	if g == nil {
		return nil
	}
	var issues []Diagnostic
	seen := map[string]organization{}
	for _, org := range g.organizations {
		if first, ok := seen[org.id]; ok && first.fields != org.fields {
			issues = append(issues, d.At(org.node.script, Warning, fmt.Sprintf("Organization %q has conflicting identity fields on this page.", org.id), "Use the same name, url, logo and sameAs for this @id."))
		} else {
			seen[org.id] = org
		}
	}
	for _, n := range g.nodes {
		for _, key := range []string{"publisher", "seller", "provider", "author"} {
			for _, raw := range schemaValues(n.value, key) {
				obj, _ := raw.(map[string]any)
				id, _ := obj["@id"].(string)
				if id != "" && len(obj) == 1 && g.ids[id] == nil {
					issues = append(issues, d.At(n.script, Warning, fmt.Sprintf("%s references undefined @id %q.", key, id), "Define the referenced node in this page's JSON-LD."))
				}
				if key == "publisher" && hasSchemaType(obj, organizationTypes) && !hasOrganizationName(obj) {
					issues = append(issues, d.At(n.script, Warning, "Inline publisher Organization lacks a name.", "Add the publisher's public name to the Organization."))
				}
			}
		}
	}
	return issues
}

func hasOrganizationName(obj map[string]any) bool {
	for _, v := range schemaValues(obj, "name") {
		if name, ok := scalar(v).(string); ok && strings.TrimSpace(name) != "" {
			return true
		}
	}
	return false
}
