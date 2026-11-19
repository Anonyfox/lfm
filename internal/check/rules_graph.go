package check

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Merge descriptions without modifying the expanded graph used by other rules.
func (g *pageGraph) mergedNodes() []schemaNode {
	var nodes []schemaNode
	positions := map[string]int{}
	owned := map[int]bool{}
	for _, n := range g.nodes {
		id, _ := n.value["@id"].(string)
		if id != "" && len(n.value) == 1 {
			continue
		}
		if id != "" && !strings.HasPrefix(id, "_:") {
			if i, ok := positions[id]; ok {
				nodes[i].publisher = nodes[i].publisher || n.publisher
				nodes[i].standalone = nodes[i].standalone || n.standalone
				nodes[i].linkedPage = nodes[i].linkedPage && n.linkedPage
				nodes[i].relatedOrganization = nodes[i].relatedOrganization && n.relatedOrganization
				merged := nodes[i].value
				if !owned[i] {
					merged = maps.Clone(merged)
					for k, v := range merged {
						if arr, ok := v.([]any); ok {
							merged[k] = slices.Clone(arr)
						}
					}
					nodes[i].value = merged
					owned[i] = true
				}
				for k, v := range n.value {
					if values, ok := v.([]any); ok {
						old, _ := merged[k].([]any)
						merged[k] = append(old, values...)
					}
				}
				continue
			}
			positions[id] = len(nodes)
		}
		nodes = append(nodes, n)
	}
	return nodes
}

func graphLanguage(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	x, ax, _ := strings.Cut(a, "-")
	y, by, _ := strings.Cut(b, "-")
	return a == b || x == y && (!hasLanguageRegion(ax) || !hasLanguageRegion(by))
}

func hasLanguageRegion(subtags string) bool {
	for part := range strings.SplitSeq(subtags, "-") {
		if len(part) == 1 {
			break
		} // Extension and private-use subtags follow a singleton.
		if len(part) == 2 && part[0] >= 'a' && part[0] <= 'z' && part[1] >= 'a' && part[1] <= 'z' || len(part) == 3 && strings.Trim(part, "0123456789") == "" {
			return true
		}
	}
	return false
}

func graphRule(d *Document) []Diagnostic {
	g := d.seoGraph()
	if g == nil || len(g.nodes) == 0 {
		return nil
	}
	nodes := g.mergedNodes()
	var orgs, sites, pages []schemaNode
	ids := map[string]map[string]any{}
	relatedIDs := map[string]bool{}
	for _, n := range g.nodes {
		if id, ok := n.value["@id"].(string); n.relatedOrganization && ok {
			relatedIDs[id] = true
		}
	}
	standaloneIDs := map[string]bool{}
	hasSiteID := false
	brands := map[string]bool{}
	for _, n := range nodes {
		id, _ := n.value["@id"].(string)
		if id != "" {
			ids[id] = n.value
			standaloneIDs[id] = n.standalone
		}
		if (n.standalone || n.publisher) && !n.relatedOrganization && !relatedIDs[id] && hasSchemaType(n.value, organizationTypes) {
			orgs = append(orgs, n)
		}
		if schemaType(n.value, "WebSite") {
			sites = append(sites, n)
			hasSiteID = hasSiteID || id != ""
		}
		if !n.linkedPage && schemaType(n.value, "WebPage") {
			pages = append(pages, n)
		}
		for _, b := range schemaValues(n.value, "brand") {
			if obj, ok := b.(map[string]any); ok {
				if id, ok := obj["@id"].(string); ok {
					brands[id] = true
				}
			}
		}
	}
	var issues []Diagnostic
	warn := func(n schemaNode, msg, hint string) { issues = append(issues, d.At(n.script, Warning, msg, hint)) }
	for _, group := range []struct {
		name  string
		nodes []schemaNode
	}{{"Organization", orgs}, {"WebPage", pages}, {"WebSite", sites}} {
		count := len(group.nodes)
		if count > 1 {
			warn(group.nodes[1], fmt.Sprintf("Graph contains %d %s nodes.", count, group.name), "Describe one "+group.name+" with a consistent @id.")
		}
	}
	checkOrg := func(n schemaNode, key string, obj map[string]any) {
		for _, raw := range schemaValues(obj, key) {
			ref, _ := raw.(map[string]any)
			id, _ := ref["@id"].(string)
			if hasSchemaType(ref, organizationTypes) || len(ref) > 1 && standaloneIDs[id] {
				warn(n, key+" contains an inline entity description.", "Define the entity separately and reference its @id.")
			} else if len(ref) == 1 && len(orgs) == 1 && hasSchemaType(ids[id], organizationTypes) && orgs[0].value["@id"] != id {
				warn(n, key+" references a different Organization.", "Reference the page's Organization @id.")
			}
		}
	}
	_, canonical := headCanonical(d)
	lang := Attr(d.activeNodes("html")[0], "lang")
	for _, n := range nodes {
		for _, k := range []string{"publisher", "seller", "provider"} {
			checkOrg(n, k, n.value)
		}
		id, _ := n.value["@id"].(string)
		if brands[id] && schemaType(n.value, "Brand") {
			checkOrg(n, "owner", n.value)
		}
		for _, b := range schemaValues(n.value, "brand") {
			obj, _ := b.(map[string]any)
			if obj["@id"] == nil && schemaType(obj, "Brand") {
				checkOrg(n, "owner", obj)
			}
		}
		checkGraphDates(n, warn)
	}
	for _, n := range pages {
		id, _ := n.value["@id"].(string)
		if canonical != nil && id != "" {
			ref, err := url.Parse(id)
			if err != nil || (strings.SplitN(canonical.ResolveReference(ref).String(), "#", 2)[0] != canonical.String() && !namesOwnFile(d, canonical.ResolveReference(ref))) {
				warn(n, "WebPage @id differs from the canonical URL.", "Use the canonical URL with an optional fragment as the WebPage @id.")
			}
		}
		refs := schemaValues(n.value, "isPartOf")
		valid := len(refs) > 0 || !hasSiteID
		for _, raw := range refs {
			obj, _ := raw.(map[string]any)
			ref, _ := obj["@id"].(string)
			valid = valid && len(obj) == 1 && schemaType(ids[ref], "WebSite")
		}
		if !valid {
			warn(n, "WebPage isPartOf does not reference the WebSite.", "Set isPartOf to the WebSite @id.")
		}
		languages := schemaValues(n.value, "inLanguage")
		matches := true
		for _, raw := range languages {
			v, _ := scalar(raw).(string)
			matches = matches && graphLanguage(v, lang)
		}
		if lang != "" && !matches {
			warn(n, "WebPage inLanguage differs from the HTML language.", "Set inLanguage to the page's HTML language.")
		}
	}
	return issues
}

func checkGraphDates(n schemaNode, warn func(schemaNode, string, string)) {
	var latest, earliest time.Time
	var published, modified string
	for _, k := range []string{"datePublished", "dateModified"} {
		for _, raw := range schemaValues(n.value, k) {
			v, _ := scalar(raw).(string)
			t, ok := parseCalendarDate(v)
			if !ok {
				warn(n, fmt.Sprintf("%s %q is an invalid ISO 8601 date.", k, v), "Use an ISO 8601 calendar date or datetime.")
				continue
			}
			if k == "datePublished" && (published == "" || t.After(latest)) {
				latest = t
				published = v
			}
			if k == "dateModified" && (modified == "" || t.Before(earliest)) {
				earliest = t
				modified = v
			}
		}
	}
	if published == "" || modified == "" {
		return
	}
	precedes := earliest.Before(latest)
	if len(published) == 10 || len(modified) == 10 {
		precedes = modified[:10] < published[:10]
	}
	if precedes {
		warn(n, "dateModified precedes datePublished.", "Use a modification date on or after publication.")
	}
}

// SiteGraph reports the first page that departs from each shared identity.
func SiteGraph(pages []*Document) []Diagnostic {
	first := map[string]string{}
	reported := map[string]bool{}
	var issues []Diagnostic
	for _, d := range pages {
		g := d.seoGraph()
		if g == nil {
			continue
		}
		seen := map[string]bool{}
		for _, n := range g.nodes {
			id, _ := n.value["@id"].(string)
			if id == "" || strings.HasPrefix(id, "_:") {
				continue
			}
			ref, err := url.Parse(id)
			if err != nil {
				continue
			}
			if !ref.IsAbs() {
				base := sitemapCanonical(d)
				if base == nil {
					base = &url.URL{Path: "/" + d.Options.File}
				}
				id = base.ResolveReference(ref).String()
			}
			for _, kind := range []string{"Organization", "WebSite"} {
				if seen[kind] || reported[kind] || !(kind == "Organization" && hasSchemaType(n.value, organizationTypes) || kind == "WebSite" && schemaType(n.value, kind)) {
					continue
				}
				seen[kind] = true
				if first[kind] == "" {
					first[kind] = id
					continue
				}
				if first[kind] != id {
					reported[kind] = true
					i := d.At(n.script, Warning, fmt.Sprintf("%s @id %q differs from %q on an earlier page.", kind, id, first[kind]), "Use the same absolute "+kind+" @id across pages.")
					i.Rule = "jsonld.graph"
					issues = append(issues, i)
				}
			}
		}
	}
	return issues
}

// namesOwnFile reports whether u is the public path of a nested file being
// checked. A page that canonicalizes to a parent hub may still identify its own
// WebPage; the site root is never exempt.
func namesOwnFile(d *Document, u *url.URL) bool {
	own := strings.Trim(strings.TrimSuffix(d.Options.File, "index.html"), "/")
	return own != "" && own == strings.Trim(u.Path, "/")
}
