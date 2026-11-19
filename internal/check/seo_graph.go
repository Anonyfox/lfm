package check

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

type nodeContext struct {
	publisher           bool
	relatedOrganization bool
	standalone          bool
	linkedPage          bool
}

type schemaNode struct {
	value  map[string]any
	script *html.Node
	nodeContext
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
	var visit func(any, *html.Node, nodeContext)
	visit = func(raw any, script *html.Node, ctx nodeContext) {
		switch v := raw.(type) {
		case []any:
			for _, child := range v {
				visit(child, script, ctx)
			}
		case map[string]any:
			if _, literal := v["@value"]; literal {
				return
			}
			if id, _ := v["@id"].(string); id != "" && len(v) > 1 {
				g.ids[id] = v
			}
			node := schemaNode{v, script, ctx}
			g.nodes = append(g.nodes, node)
			if id, _ := v["@id"].(string); id != "" && hasSchemaType(v, organizationTypes) {
				g.organizations = append(g.organizations, describeOrganization(id, node))
			}
			for _, key := range slices.Sorted(maps.Keys(v)) {
				visit(v[key], script, nodeContext{
					publisher:           key == "https://schema.org/publisher" || key == "http://schema.org/publisher",
					relatedOrganization: ctx.relatedOrganization || relatedOrganizationProperties[key],
					standalone:          ctx.standalone && key == "@graph",
					linkedPage:          ctx.linkedPage || linkedPageProperties[key],
				})
			}
		}
	}
	for _, block := range d.jsonldExpanded {
		visit(block.data, block.node, nodeContext{standalone: true})
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

func faqText(n *html.Node, b *strings.Builder, styles *styleModel, trimInline bool) {
	if n.Type == html.TextNode {
		if styles == nil || n.Parent == nil || !groundHidden(styles, n.Parent) {
			text := n.Data
			if trimInline {
				text = strings.TrimSpace(text)
			}
			b.WriteString(text)
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
		faqText(c, b, styles, trimInline)
	}
	if boundary {
		b.WriteByte(' ')
	}
}

func faqAnswerMatches(visible []string, answer string) bool {
	tree, err := html.Parse(strings.NewReader(answer))
	if err != nil {
		return faqMatch(visible, faqTokens(answer), faqAnswerExtras)
	}
	var b strings.Builder
	faqText(tree, &b, nil, false)
	if faqMatch(visible, faqTokens(b.String()), faqAnswerExtras) {
		return true
	}
	// Inline markup may abut its surrounding word in the rendered copy.
	if !strings.Contains(answer, "<") {
		return false
	}
	b.Reset()
	faqText(tree, &b, nil, true)
	return faqMatch(visible, faqTokens(b.String()), 0)
}

func faqRule(d *Document) []Diagnostic {
	g := d.seoGraph()
	if g == nil {
		return nil
	}
	var issues []Diagnostic
	var visible []string
	ready := false
	for _, n := range g.nodes {
		if !schemaType(n.value, "FAQPage") {
			continue
		}
		if !ready {
			var b strings.Builder
			for _, body := range d.activeNodes("body") {
				faqText(body, &b, d.styles(), false)
			}
			visible = faqTokens(b.String())
			ready = true
		}
		for _, item := range schemaValues(n.value, "mainEntity") {
			q := g.resolve(item)
			if !schemaType(q, "Question") {
				continue
			}
			name := strings.Join(strings.Fields(schemaText(q, "name")), " ")
			if !faqMatch(visible, faqTokens(name), 0) {
				issues = append(issues, d.At(n.script, Warning, fmt.Sprintf("FAQ question %q is missing from the visible text.", name), "Show the same question on the page and in FAQPage mainEntity."))
			}
			for _, a := range schemaValues(q, "acceptedAnswer") {
				if !faqAnswerMatches(visible, schemaText(g.resolve(a), "text")) {
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

// conflicts reports a field both nodes state with different values. JSON-LD
// merges nodes that share an @id, so a field only one of them states adds to
// the entity instead of contradicting it.
func (o organization) conflicts(other organization, n int) bool {
	for i := range n {
		if o.fields[i] != "" && other.fields[i] != "" && o.fields[i] != other.fields[i] {
			return true
		}
	}
	return false
}

// merge fills the fields o leaves empty from other.
func (o organization) merge(other organization) organization {
	for i := range o.fields {
		if o.fields[i] == "" {
			o.fields[i] = other.fields[i]
		}
	}
	return o
}

func identityRule(d *Document) []Diagnostic {
	g := d.seoGraph()
	if g == nil {
		return nil
	}
	var issues []Diagnostic
	seen := map[string]organization{}
	for _, org := range g.organizations {
		first, ok := seen[org.id]
		if !ok {
			seen[org.id] = org
			continue
		}
		if first.conflicts(org, len(org.fields)) {
			issues = append(issues, d.At(org.node.script, Warning, fmt.Sprintf("Organization %q has conflicting identity fields on this page.", org.id), "Use the same name, url, logo and sameAs for this @id."))
		}
		seen[org.id] = first.merge(org)
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
