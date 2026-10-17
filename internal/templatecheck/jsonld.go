package templatecheck

import (
	"fmt"
	"sort"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// ResolveJSONLD substitutes only authored string leaves, exactly once. Selected
// objects/arrays are values, never new template source. Go evaluates expressions;
// encoding/json (at the call site) handles serialization and script escaping.
func ResolveJSONLD(options Options) (any, []string, diagnostic.Summary) {
	input := options.Values["jsonld"]
	if input == nil {
		return nil, []string{"jsonld"}, diagnostic.Summarize(nil)
	}
	c := newChecker(options)
	problem := func(path, message, hint string) {
		at, _ := options.Sources.Nearest(path)
		c.add(diagnostic.Diagnostic{Rule: "jsonld.bindings", Severity: diagnostic.Error, Location: at, Path: path, Message: message, Hint: hint})
	}
	switch input.(type) {
	case map[string]any, []any:
	default:
		problem("jsonld", "jsonld must be a JSON object or array of node objects.", "Use a structured object, optionally with @graph; omit jsonld or use null to disable it.")
		return nil, nil, diagnostic.Summarize(c.issues)
	}
	// Removing the source subtree makes self references impossible, including
	// dynamic index lookups. Selecting the entire config is rejected below too.
	rootValues := make(map[string]any, len(options.Values))
	for key, raw := range options.Values {
		if key != "jsonld" {
			rootValues[key] = raw
		}
	}
	root := value{raw: rootValues, origin: true}
	env := &environment{root: root, dot: root, variables: map[string]*value{"$": &root}}
	var walk func(any, string, int) any
	nodes := 0
	walk = func(raw any, path string, depth int) any {
		nodes++
		if depth > 128 || nodes > 100000 {
			if !c.exhausted {
				c.exhausted = true
				problem(path, "JSON-LD template exceeds depth 128 or 100,000 values.", "Reduce nesting/data volume before rendering.")
			}
			return nil
		}
		switch v := raw.(type) {
		case map[string]any:
			result := make(map[string]any, len(v))
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child := Child(path, key)
				if strings.Contains(key, "{{") {
					problem(child, "JSON-LD object keys cannot contain placeholders.", "Keep keys literal; placeholders are allowed only in string values.")
				}
				result[key] = walk(v[key], child, depth+1)
			}
			return result
		case []any:
			result := make([]any, len(v))
			for i, item := range v {
				result[i] = walk(item, fmt.Sprintf("%s[%d]", path, i), depth+1)
			}
			return result
		case string:
			if !strings.Contains(v, "{{") {
				return v
			}
			// A nonempty prefix also makes a definition named "jsonld" fail,
			// rather than allowing it to replace the root and hide its definition.
			t, err := template.New("jsonld").Option("missingkey=error").Parse(`{{""}}` + v)
			if err != nil {
				problem(path, "Invalid JSON-LD placeholder syntax.", "Use a config reference such as {{.title}} or {{index .data.items 0}}; keep programs in the HTML template.")
				return nil
			}
			if len(t.Templates()) != 1 {
				problem(path, "Template definitions are not allowed inside JSON-LD values.", "Use only references; author repeated structured data as arrays/objects.")
				return nil
			}
			t.Tree.Root.Nodes = t.Tree.Root.Nodes[1:] // discard our capture-free prefix
			c.bindingPath = path
			defer func() { c.bindingPath = "" }()
			var out strings.Builder
			for _, node := range t.Tree.Root.Nodes {
				switch n := node.(type) {
				case *parse.TextNode:
					out.Write(n.Text)
				case *parse.ActionNode:
					if !jsonReference(n.Pipe) {
						problem(path, "JSON-LD placeholders allow references and index lookups only.", "Use {{.data.name}}, {{$.title}} or {{index .data \"key-name\"}}; no functions, pipelines, assignments, conditionals or loops.")
						continue
					}
					before := len(c.issues)
					selected := c.pipeline(n.Pipe, env, true)
					if selected.unknown || diagnostic.Summarize(c.issues[before:]).Errors > 0 {
						continue
					}
					if selected.origin && selected.path == "" {
						problem(path, "JSON-LD cannot select the entire config.", "Select a specific field/subtree; jsonld cannot reference itself.")
						continue
					}
					c.consume(selected, true)
					if selected.raw == nil || selected.raw == "" {
						at, _ := options.Sources.Nearest(path)
						c.add(diagnostic.Diagnostic{Rule: "jsonld.empty", Severity: diagnostic.Warning, Location: at, Path: path, Message: "JSON-LD placeholder selects null or an empty string.", Hint: "Supply content or keep the explicit empty value only when intended."})
					}
					if len(t.Tree.Root.Nodes) == 1 {
						return selected.raw // no string conversion, no second walk
					}
					switch selected.raw.(type) {
					case map[string]any, []any, nil:
						problem(path, "An object, array or null cannot be interpolated into surrounding text.", "Use a whole-value placeholder to preserve its JSON type, or select a scalar field.")
					default:
						out.WriteString(fmt.Sprint(selected.raw))
					}
				default:
					problem(path, "Template control flow is not allowed inside JSON-LD values.", "Keep the JSON structure explicit; select whole arrays/objects with a reference.")
				}
			}
			return out.String()
		default:
			return raw
		}
	}
	resolved := walk(input, "jsonld", 0)
	used := []string{"jsonld"}
	for path := range c.whole {
		used = append(used, path)
	}
	// Index keys are consumed scalars too. Do not promote every traversed object
	// to whole-subtree usage: that would hide unused siblings (or the entire root).
	for path := range c.exact {
		if _, leaf := options.Sources.Leaves[path]; leaf && !c.whole[path] {
			used = append(used, path)
		}
	}
	sort.Strings(used)
	return resolved, used, diagnostic.Summarize(c.issues)
}

// Restrict the surface, rather than introducing a second template language.
// The normal Go evaluator still owns lookup/type/index semantics.
func jsonReference(pipe *parse.PipeNode) bool {
	if pipe == nil || len(pipe.Decl) != 0 || len(pipe.Cmds) != 1 {
		return false
	}
	command := pipe.Cmds[0]
	if len(command.Args) == 0 {
		return false
	}
	if id, ok := command.Args[0].(*parse.IdentifierNode); ok {
		if id.Ident != "index" || len(command.Args) < 3 {
			return false
		}
		for _, arg := range command.Args[1:] {
			if !jsonArgument(arg) {
				return false
			}
		}
		return true
	}
	return len(command.Args) == 1 && jsonArgument(command.Args[0])
}

func jsonArgument(node parse.Node) bool {
	switch n := node.(type) {
	case *parse.FieldNode, *parse.DotNode, *parse.StringNode, *parse.BoolNode, *parse.NilNode:
		return true
	case *parse.NumberNode:
		return !n.IsComplex
	case *parse.VariableNode:
		return n.Ident[0] == "$"
	case *parse.ChainNode:
		return jsonArgument(n.Node)
	case *parse.PipeNode:
		return jsonReference(n)
	}
	return false
}
