package templatecheck

import (
	"fmt"
	htmltemplate "html/template"
	"reflect"
	"sort"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

type value struct {
	raw             any
	path            string
	origin, unknown bool
}
type environment struct {
	root, dot value
	variables map[string]*value
}

func (e *environment) scope(isolated bool) *environment {
	n := &environment{root: e.root, dot: e.dot, variables: map[string]*value{}}
	for k, v := range e.variables {
		if isolated {
			copy := *v
			n.variables[k] = &copy
		} else {
			n.variables[k] = v
		}
	}
	return n
}

func (e *environment) rootValue() value {
	if v := e.variables["$"]; v != nil {
		return *v
	}
	return e.root
}

type checker struct {
	locations    map[string]diagnostic.Source
	options      Options
	tree         *parse.Tree
	issues       []diagnostic.Diagnostic
	seen         map[string]bool
	exact, whole map[string]bool
	reached      map[string]bool
	stack        map[string]bool
	probes       map[probeKey]*probe
	steps        int
	exhausted    bool
	bindingPath  string // nonempty while evaluating a lfm.json JSON-LD leaf
}

// Check walks the standard parse trees against the actual JSON shapes. Dormant
// branches are inspected as warnings; only active expressions block rendering.
func Check(t *htmltemplate.Template, options Options) diagnostic.Summary {
	c := newChecker(options)
	for _, key := range options.Consumed {
		c.whole[key] = true
	}
	for file, raw := range options.Files {
		c.locations[file] = diagnostic.NewSource(file, raw)
	}
	for _, issue := range options.Sources.Issues {
		c.add(issue)
	}
	for _, key := range []string{"schemaVersion", "url", "build"} {
		c.whole[key] = true
	}
	c.browserData()
	root := value{raw: options.Values, origin: true}
	c.call(t, options.Entry, root, true, nil)
	c.unused(t)
	return diagnostic.Summarize(c.issues)
}

func newChecker(options Options) *checker {
	return &checker{locations: map[string]diagnostic.Source{}, options: options, seen: map[string]bool{}, exact: map[string]bool{}, whole: map[string]bool{}, reached: map[string]bool{}, stack: map[string]bool{}, probes: map[probeKey]*probe{}}
}

func (c *checker) add(d diagnostic.Diagnostic) {
	if d.Severity == diagnostic.Warning {
		if level := c.options.Rules[d.Rule]; level != "" {
			d.Severity = level
		}
		if d.Severity == diagnostic.Off {
			return
		}
	}
	key := fmt.Sprintf("%s/%s/%s/%d/%s/%s", d.Rule, d.Severity, d.Location.File, d.Location.Offset, d.Path, d.Message)
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	c.issues = append(c.issues, d)
}
func (c *checker) issue(rule string, node parse.Node, active bool, key, message, hint string) {
	severity := diagnostic.Warning
	if !active && rule == "template.bindings" {
		rule = "template.inactive"
	}
	if active && rule == "template.bindings" {
		severity = diagnostic.Error
	}
	at := nodeLocation(c.tree, node, nil)
	if source, ok := c.locations[at.File]; ok {
		offset := 0
		if node != nil {
			offset = int(node.Position())
		}
		at = source.At(offset)
	}
	d := diagnostic.Diagnostic{Rule: rule, Severity: severity, Location: at, Path: key, Message: message, Hint: hint}
	if c.bindingPath != "" {
		d.Rule, d.Severity = "jsonld.bindings", diagnostic.Error
		if rule == "template.empty" {
			// A missing index is an error here: optional data must be explicit null.
			d.Hint = "Declare the selected key explicitly (null is allowed), or correct its spelling."
		}
		if Within(key, "jsonld") {
			d.Message = "JSON-LD cannot reference its own jsonld subtree."
			d.Hint = "Select an ordinary config/data field; JSON-LD references are resolved once without recursive dependencies."
		}
		d.Path = c.bindingPath
		d.Location, _ = c.options.Sources.Nearest(c.bindingPath)
	}
	if rule == "template.inactive" {
		d.Hint = "Inactive branch: " + d.Hint
	}
	if at, ok := c.options.Sources.Nearest(key); ok && key != "" {
		d.Related = []diagnostic.Location{at}
	}
	c.add(d)
}
func (c *checker) consume(v value, whole bool) {
	if !v.origin {
		return
	}
	c.exact[v.path] = true
	if whole {
		c.whole[v.path] = true
	}
}
func (c *checker) tick(node parse.Node) bool {
	c.steps++
	if c.steps <= 100000 {
		return true
	}
	if !c.exhausted {
		c.exhausted = true
		c.issue("template.bindings", node, true, "", "Template analysis exceeded 100,000 nodes/iterations.", "Reduce generated data/iteration size or recursive template expansion; analysis cannot safely prove this render.")
	}
	return false
}
func (c *checker) call(t *htmltemplate.Template, name string, arg value, active bool, node parse.Node) {
	target := t.Lookup(name)
	if target == nil || target.Tree == nil {
		// html/template inspects calls in both branches during contextual escaping.
		c.issue("template.bindings", node, true, "", fmt.Sprintf("Template %q is not defined.", name), "Define this partial or fix its template name; HTML escaping also checks inactive calls.")
		return
	}
	previous := c.tree
	c.tree = target.Tree
	defer func() { c.tree = previous }()
	c.reached[name] = true
	key := name + "/" + arg.path
	if c.stack[key] || len(c.stack) >= 128 {
		c.issue("template.bindings", node, active, arg.path, "Recursive template call has no verifiable data progress or exceeds depth 128.", "Pass a smaller child dataset or terminate recursion with a condition.")
		return
	}
	c.stack[key] = true
	defer delete(c.stack, key)
	root := arg
	c.walk(t, target.Tree.Root, &environment{root: arg, dot: arg, variables: map[string]*value{"$": &root}}, active)
}

type flow uint8

const (
	normal flow = iota
	breaking
	continuing
)

func (c *checker) walk(t *htmltemplate.Template, list *parse.ListNode, env *environment, active bool) flow {
	if list == nil {
		return normal
	}
	result := normal
	for _, node := range list.Nodes {
		if !c.tick(node) {
			return result
		}
		live := active && result == normal
		switch n := node.(type) {
		case *parse.ActionNode:
			v := c.pipeline(n.Pipe, env, live)
			if len(n.Pipe.Decl) == 0 {
				c.consume(v, true)
				c.output(n, v, live)
			}
		case *parse.IfNode:
			scope := env.scope(!live)
			v := c.pipeline(n.Pipe, scope, live)
			c.consume(v, true)
			yes, _ := template.IsTrue(v.raw)
			yes = yes && !v.unknown
			f := c.walk(t, n.List, scope.scope(!live || !yes), live && yes)
			other := c.walk(t, n.ElseList, scope.scope(!live || yes), live && !yes)
			if live {
				if yes {
					result = f
				} else {
					result = other
				}
			}
		case *parse.WithNode:
			scope := env.scope(!live)
			v := c.pipeline(n.Pipe, scope, live)
			c.consume(v, false)
			yes, _ := template.IsTrue(v.raw)
			yes = yes && !v.unknown
			body := scope.scope(!live || !yes)
			body.dot = v
			if !yes {
				body.dot.unknown = true
			}
			if live && !yes && !v.unknown {
				c.issue("template.empty", n, true, v.path, "Empty with value skips its section.", "Confirm this section is optional; its child bindings cannot be verified without a nonempty sample.")
			}
			f := c.walk(t, n.List, body, live && yes)
			other := c.walk(t, n.ElseList, scope.scope(!live || yes), live && !yes)
			if live {
				if yes {
					result = f
				} else {
					result = other
				}
			}
		case *parse.RangeNode:
			c.rangeNode(t, n, env, live)
		case *parse.TemplateNode:
			arg := value{}
			if n.Pipe != nil {
				arg = c.pipeline(n.Pipe, env, live)
			}
			c.consume(arg, false)
			c.call(t, n.Name, arg, live, n)
		case *parse.BreakNode:
			if live {
				result = breaking
			}
		case *parse.ContinueNode:
			if live {
				result = continuing
			}
		}
	}
	return result
}

type item struct{ key, value value }

func (c *checker) rangeNode(t *htmltemplate.Template, n *parse.RangeNode, env *environment, active bool) {
	scope := env.scope(!active)
	// Range declarations have different binding rules than ordinary pipelines.
	pipe := n.Pipe.CopyPipe()
	pipe.Decl = nil
	pipe.IsAssign = false
	collection := c.pipeline(pipe, scope, active)
	c.consume(collection, false)
	var items []item
	valid := true
	switch v := collection.raw.(type) {
	case nil:
	case []any:
		for i, x := range v {
			items = append(items, item{value{raw: i}, value{raw: x, path: fmt.Sprintf("%s[%d]", collection.path, i), origin: collection.origin}})
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			items = append(items, item{value{raw: key}, value{raw: v[key], path: Child(collection.path, key), origin: collection.origin}})
		}
	default:
		rv := reflect.ValueOf(v)
		if rv.IsValid() && rv.CanInt() {
			count := rv.Int()
			if count > 100000 {
				c.issue("template.bindings", n, active, collection.path, "Integer range exceeds 100,000 iterations.", "Reduce the range before rendering.")
				valid = false
			} else {
				for i := int64(0); i < count; i++ {
					items = append(items, item{value{raw: i}, value{raw: i}})
				}
			}
		} else {
			valid = false
			if !collection.unknown {
				c.issue("template.bindings", n, active, collection.path, "Cannot range over "+typeName(collection.raw)+".", "Use a JSON array/object; JSON numbers use json.Number and are not Go integer ranges.")
			}
		}
	}
	bind := func(e *environment, k, v value) {
		for i, variable := range n.Pipe.Decl {
			x := v
			if len(n.Pipe.Decl) == 2 && i == 0 {
				x = k
			}
			name := variable.Ident[0]
			if n.Pipe.IsAssign {
				if cell := e.variables[name]; cell != nil {
					*cell = x
				}
			} else {
				copy := x
				e.variables[name] = &copy
			}
		}
	}
	// Go initializes range variables to the collection for an empty range's else.
	bind(scope, collection, collection)
	if len(items) == 0 {
		if active && valid && !collection.unknown {
			c.issue("template.empty", n, true, collection.path, "Empty range renders no items.", "Confirm the empty case is intended; provide a representative item to validate bindings inside its body.")
		}
		body := scope.scope(true)
		unknown := value{unknown: true, path: collection.path + "[*]", origin: collection.origin}
		body.dot = unknown
		bind(body, unknown, unknown)
		c.walk(t, n.List, body, false)
		c.walk(t, n.ElseList, scope, active && valid)
		return
	}
	stopped := false
	for _, entry := range items {
		body := scope.scope(!active || stopped)
		body.dot = entry.value
		bind(body, entry.key, entry.value)
		if c.walk(t, n.List, body, active && !stopped) == breaking {
			stopped = true
		}
		if c.exhausted {
			break
		}
	}
	c.walk(t, n.ElseList, scope.scope(true), false)
}

func (c *checker) output(node parse.Node, v value, active bool) {
	if !active || v.unknown {
		return
	}
	if v.raw == nil || reflect.ValueOf(v.raw).Kind() == reflect.String && strings.TrimSpace(fmt.Sprint(v.raw)) == "" {
		c.issue("template.empty", node, true, v.path, "Binding renders an empty value.", "Supply content or guard an intentionally optional value with if/with; false and numeric zero are accepted.")
	}
	switch v.raw.(type) {
	case map[string]any, []any:
		c.issue("template.value", node, true, v.path, "Object/array is interpolated directly into HTML.", "Render its fields with range/with, or select it through build.browserData for JavaScript.")
	}
	if s, ok := v.raw.(string); ok && strings.Contains(s, "%!") {
		c.issue("template.value", node, true, v.path, "Rendered string contains a Go formatting failure marker (%!).", "Check printf verb/argument types; JSON numbers are json.Number values.")
	}
}
func typeName(v any) string {
	if v == nil {
		return "null"
	}
	return reflect.TypeOf(v).String()
}
