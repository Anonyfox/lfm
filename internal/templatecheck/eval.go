package templatecheck

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"
)

func (c *checker) pipeline(pipe *parse.PipeNode, env *environment, active bool) value {
	if pipe == nil {
		return value{}
	}
	var previous *value
	for _, command := range pipe.Cmds {
		v := c.command(command, env, previous, active)
		previous = &v
	}
	result := value{}
	if previous != nil {
		result = *previous
	}
	for _, variable := range pipe.Decl {
		name := variable.Ident[0]
		if pipe.IsAssign {
			if cell := env.variables[name]; cell != nil {
				*cell = result
			}
		} else {
			copy := result
			env.variables[name] = &copy
		}
		if name == "$" {
			env.root = result
		}
	}
	return result
}

func (c *checker) command(command *parse.CommandNode, env *environment, previous *value, active bool) value {
	if len(command.Args) == 0 {
		return value{}
	}
	before := len(c.issues)
	name := ""
	if id, ok := command.Args[0].(*parse.IdentifierNode); ok {
		name = id.Ident
	}
	args := command.Args
	if name != "" {
		args = args[1:]
	}
	traced := make([]value, 0, len(args)+1)
	live := active
	for _, arg := range args {
		v := c.argument(arg, env, live)
		traced = append(traced, v)
		if name == "and" || name == "or" {
			truth, _ := template.IsTrue(v.raw)
			if v.unknown || name == "and" && !truth || name == "or" && truth {
				live = false
			}
		}
	}
	if previous != nil {
		traced = append(traced, *previous)
	}
	if name != "" {
		for _, v := range traced {
			c.consume(v, name != "len" && name != "index")
		}
	}
	result := value{}
	if name == "" && len(traced) == 1 {
		result = traced[0]
	}
	if name == "and" || name == "or" {
		for _, v := range traced {
			result = v
			truth, _ := template.IsTrue(v.raw)
			if v.unknown || name == "and" && !truth || name == "or" && truth {
				break
			}
		}
	} else if name == "index" && len(traced) > 0 {
		result = c.index(command, traced, active)
	}
	// An empty range gives no item schema. Never guess a missing field/type from it.
	unknown := false
	for _, v := range traced {
		unknown = unknown || v.unknown
	}
	if (name == "and" || name == "or") && !result.unknown {
		// A short-circuited argument can be unknown without affecting this result.
		unknown = false
	}
	if unknown {
		result.unknown = true
		return result
	}
	raw, err := c.execute(command, env, previous)
	if err != nil {
		if len(c.issues) == before {
			c.issue("template.bindings", command, active, result.path, "Invalid template operation: "+safeExecutionError(err), "Check argument count/types, index bounds and nil values at this expression. JSON numbers are json.Number; format or convert deliberately.")
		}
		result.unknown = true
		return result
	}
	result.raw = raw
	return result
}

func safeExecutionError(err error) string {
	// Go's execution errors may quote arbitrary input values (e.g. failed numeric
	// comparisons). Retain the operation/type explanation, redact quoted payloads.
	message := err.Error()
	if i := strings.LastIndex(message, ": error calling "); i >= 0 {
		message = message[i+2:]
	} else if i := strings.Index(message, " at <"); i >= 0 {
		if end := strings.Index(message[i:], ">: "); end >= 0 {
			message = message[i+end+3:]
		}
	}
	var out strings.Builder
	for i := 0; i < len(message); {
		if message[i] == '"' || message[i] == '\'' || message[i] == '`' {
			quote := message[i]
			out.WriteString("[value]")
			i++
			for i < len(message) {
				if message[i] == '\\' {
					i += 2
					continue
				}
				if message[i] == quote {
					i++
					break
				}
				i++
			}
		} else {
			out.WriteByte(message[i])
			i++
		}
	}
	return out.String()
}

func (c *checker) argument(node parse.Node, env *environment, active bool) value {
	switch n := node.(type) {
	case *parse.DotNode:
		return env.dot
	case *parse.FieldNode:
		return c.fields(n, env.dot, n.Ident, active)
	case *parse.VariableNode:
		name := n.Ident[0]
		v := env.rootValue()
		if name != "$" {
			cell, ok := env.variables[name]
			if !ok {
				c.issue("template.bindings", n, active, "", "Variable "+name+" has no value in this scope.", "Declare it before use; variables do not cross template calls.")
				return value{unknown: true}
			}
			v = *cell
		}
		return c.fields(n, v, n.Ident[1:], active)
	case *parse.ChainNode:
		return c.fields(n, c.argument(n.Node, env, active), n.Field, active)
	case *parse.PipeNode:
		return c.pipeline(n, env, active)
	case *parse.StringNode:
		return value{raw: n.Text}
	case *parse.BoolNode:
		return value{raw: n.True}
	case *parse.NilNode:
		return value{}
	case *parse.NumberNode:
		if n.IsInt {
			return value{raw: int(n.Int64)}
		}
		if n.IsUint {
			return value{raw: n.Uint64}
		}
		if n.IsFloat {
			return value{raw: n.Float64}
		}
		return value{raw: n.Complex128}
	default:
		return value{unknown: true}
	}
}

func (c *checker) fields(node parse.Node, v value, fields []string, active bool) value {
	for _, field := range fields {
		c.consume(v, false)
		key := field
		if v.origin {
			key = Child(v.path, field)
		}
		if v.unknown {
			v.path = key
			continue
		}
		if object, ok := v.raw.(map[string]any); ok {
			raw, exists := object[field]
			if !exists {
				c.issue("template.bindings", node, active, key, "Missing config value: "+key+".", missingHint(field, v.path, object))
				return value{path: key, origin: v.origin, unknown: true}
			}
			v = value{raw: raw, path: key, origin: v.origin}
			continue
		}
		// JSON numbers expose standard json.Number methods. The probe evaluates them
		// using Go's own method/type rules; they are not config child fields.
		if v.raw != nil && c.bindingPath == "" {
			rv := reflect.ValueOf(v.raw)
			if method := rv.MethodByName(field); method.IsValid() && method.Type().NumIn() == 0 {
				c.consume(v, true)
				results := method.Call(nil)
				if len(results) > 0 {
					v = value{raw: results[0].Interface()}
					continue
				}
			}
		}
		c.issue("template.bindings", node, active, key, "Cannot read "+key+" from "+typeName(v.raw)+".", "Check the enclosing with/range/template scope and the JSON shape; this binding needs an object.")
		return value{path: key, origin: v.origin, unknown: true}
	}
	return v
}

func (c *checker) index(node parse.Node, args []value, active bool) value {
	v := args[0]
	for _, index := range args[1:] {
		if v.unknown || index.unknown {
			v.unknown = true
			return v
		}
		switch container := v.raw.(type) {
		case map[string]any:
			key, ok := index.raw.(string)
			if !ok {
				return value{}
			}
			raw, exists := container[key]
			path := Child(v.path, key)
			if !exists && v.origin {
				c.issue("template.empty", node, active, path, "index selects a missing map key: "+path+".", "Go's index returns null for absent map keys; check spelling or explicitly guard this optional lookup.")
			}
			v = value{raw: raw, path: path, origin: v.origin}
		case []any:
			rv := reflect.ValueOf(index.raw)
			if !rv.IsValid() || !rv.CanInt() {
				return value{}
			}
			i := rv.Int()
			if i < 0 || i >= int64(len(container)) {
				return value{}
			}
			v = value{raw: container[i], path: fmt.Sprintf("%s[%d]", v.path, i), origin: v.origin}
		default:
			return value{}
		}
	}
	return v
}

func missingHint(field, parent string, object map[string]any) string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	best := ""
	distance := 4
	for _, key := range keys {
		d := editDistance(strings.ToLower(field), strings.ToLower(key))
		if d < distance {
			best = key
			distance = d
		}
	}
	hint := "Add this key or fix the template path/scope."
	if best != "" && distance <= max(1, min(3, (len(field)+2)/3)) {
		hint += " Did you mean " + Child(parent, best) + "?"
	}
	if len(keys) > 0 {
		if len(keys) > 8 {
			keys = keys[:8]
		}
		for i, key := range keys {
			keys[i] = strconv.Quote(key)
		}
		hint += " Available keys: " + strings.Join(keys, ", ") + "."
	}
	return hint
}
func editDistance(a, b string) int {
	aa, bb := []rune(a), []rune(b)
	row := make([]int, len(bb)+1)
	for j := range row {
		row[j] = j
	}
	for i, x := range aa {
		previous := row[0]
		row[0] = i + 1
		for j, y := range bb {
			old := row[j+1]
			cost := 0
			if x != y {
				cost = 1
			}
			row[j+1] = min(row[j+1]+1, row[j]+1, previous+cost)
			previous = old
		}
	}
	return row[len(bb)]
}
