package urlcheck

import (
	"bytes"
	htmltemplate "html/template"
	"sort"
	"strings"
	"text/template/parse"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

type templateSource struct {
	raw, masked []byte
	authored    []bool
	strings     []*parse.StringNode
}

// Template checks all parsed source branches/partials. Standard Go parse trees
// distinguish literal HTML from template syntax; HTML tokenizers then identify
// destinations. No regex scans for href= or naive splitting on template braces.
func Template(t *htmltemplate.Template, files map[string][]byte, registries ...*Registry) []diagnostic.Diagnostic {
	analyzer := New("")
	var registry *Registry
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
		analyzer = registry.Analyzer
	}
	sources := map[string]*templateSource{}
	for file, raw := range files {
		mask := bytes.Repeat([]byte{' '}, len(raw))
		for i, ch := range raw {
			if ch == '\n' || ch == '\r' {
				mask[i] = ch
			}
		}
		sources[file] = &templateSource{raw: raw, masked: mask, authored: make([]bool, len(raw))}
	}
	var visit func(parse.Node, *templateSource)
	visit = func(node parse.Node, source *templateSource) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, child := range n.Nodes {
					visit(child, source)
				}
			}
		case *parse.TextNode:
			start := int(n.Position())
			end := start + len(n.Text)
			if start >= 0 && end <= len(source.raw) {
				copy(source.masked[start:end], n.Text)
				for i := start; i < end; i++ {
					source.authored[i] = true
				}
			}
		case *parse.ActionNode:
			if len(n.Pipe.Decl) == 0 {
				position := int(n.Position())
				if position < len(source.masked) {
					source.masked[position] = '~'
				}
			}
			visit(n.Pipe, source)
		case *parse.IfNode:
			visit(n.Pipe, source)
			visit(n.List, source)
			visit(n.ElseList, source)
		case *parse.WithNode:
			visit(n.Pipe, source)
			visit(n.List, source)
			visit(n.ElseList, source)
		case *parse.RangeNode:
			visit(n.Pipe, source)
			visit(n.List, source)
			visit(n.ElseList, source)
		case *parse.TemplateNode:
			visit(n.Pipe, source)
		case *parse.PipeNode:
			if n != nil {
				for _, command := range n.Cmds {
					visit(command, source)
				}
			}
		case *parse.CommandNode:
			for _, argument := range n.Args {
				visit(argument, source)
			}
		case *parse.StringNode:
			source.strings = append(source.strings, n)
		case *parse.ChainNode:
			visit(n.Node, source)
		}
	}
	for _, target := range t.Templates() {
		if target.Tree == nil {
			continue
		}
		file := target.Tree.ParseName
		if file == "" {
			file = target.Tree.Name
		}
		if source := sources[file]; source != nil {
			visit(target.Tree.Root, source)
		}
	}
	var issues []diagnostic.Diagnostic
	filesInOrder := make([]string, 0, len(files))
	for file := range files {
		filesInOrder = append(filesInOrder, file)
	}
	sort.Strings(filesInOrder)
	for _, file := range filesInOrder {
		source := sources[file]
		locations := diagnostic.NewSource(file, source.raw)
		for _, offset := range VisibleURLLiterals(source.masked) {
			issues = append(issues, diagnostic.Diagnostic{Rule: "links.registry", Severity: diagnostic.Warning, Location: locations.At(offset), Message: "URL literal appears in visible copy.", Hint: "Render a named config URL to avoid a second maintenance location; use a real anchor when the destination should be crawlable. Code samples are exempt."})
		}

		seen := map[int]bool{}
		for _, ref := range AllReferences(source.masked) {
			if ref.Tag == "base" {
				continue
			} // existing build rule rejects base itself
			var literal strings.Builder
			dynamic := false
			for i := ref.Offset; i < ref.End && i < len(source.masked); i++ {
				if source.authored[i] {
					literal.WriteByte(source.masked[i])
				} else {
					dynamic = true
				}
			}
			text := strings.TrimSpace(literal.String())
			if strings.Contains(ref.Attribute, "CSS") || strings.HasPrefix(ref.Attribute, "platform JSON ") || strings.HasPrefix(ref.Attribute, "JSON ") {
				text = ref.Value
				dynamic = strings.Contains(text, "~")
				if dynamic {
					text = strings.TrimSpace(strings.ReplaceAll(text, "~", ""))
				}
			}
			if !dynamic {
				text = ref.Value
			}
			// A bare #fragment points into this document; html.references and
			// links.fragments check its target, so it needs no registry entry.
			sameDocument := strings.HasPrefix(text, "#") && !strings.HasPrefix(ref.Attribute, "JSON ")
			hardcoded := text != "" && !sameDocument && !LocalAsset(text) && !(ref.Resource && (Embedded(text) || localResource(text)))
			// Placement-specific query/fragment suffixes can remain dynamic. The rendered
			// ownership check still validates the complete URL and asks for an explicit
			// named variant when it no longer matches a declaration.
			if dynamic && (strings.HasPrefix(text, "?") || strings.HasPrefix(text, "&") || strings.HasPrefix(text, "#")) {
				hardcoded = false
			}
			for _, constant := range source.strings {
				pos := int(constant.Position())
				if pos >= ref.Offset && pos < ref.End && LooksURL(constant.Text) {
					hardcoded = true
				}
			}
			if !hardcoded || seen[ref.Offset] {
				continue
			}
			seen[ref.Offset] = true
			hint := URLHint(ref.Attribute)
			var related []diagnostic.Location
			if !dynamic && registry != nil {
				if declarations := registry.Match(ref.Value); len(declarations) > 0 {
					owner := declarations[0]
					if !strings.Contains(owner.Path, "[") {
						hint = "Render {{." + owner.Path + "}}; this destination already has a config owner."
					}
					related = []diagnostic.Location{owner.Location}
				}
			}
			issues = append(issues, diagnostic.Diagnostic{Rule: "links.literal", Severity: diagnostic.Error, Location: locations.At(ref.Offset), Element: ref.Tag, Related: related, Message: "Hardcoded URL destination in " + ref.Attribute + ".", Hint: hint})
			if !dynamic && !Embedded(ref.Value) {
				for _, finding := range analyzer.Inspect(ref.Value).Findings {
					issues = append(issues, diagnostic.Diagnostic{Rule: "links.syntax", Severity: finding.Severity, Location: locations.At(ref.Offset), Element: ref.Tag, Message: finding.Message, Hint: finding.Hint, Reference: "https://url.spec.whatwg.org/"})
				}
			}

		}
	}
	return issues
}

func localResource(raw string) bool {
	return Scheme(raw) == "" && !strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "?") && !looksHost(raw)
}
