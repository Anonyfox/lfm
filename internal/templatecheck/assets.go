package templatecheck

import (
	"fmt"
	htmltemplate "html/template"
	"reflect"
	"strconv"
	"text/template/parse"
)

func assetFunctions(custom htmltemplate.FuncMap) htmltemplate.FuncMap {
	funcs := htmltemplate.FuncMap{
		"dict":          dict,
		"__lfmOptional": optional,
		"script":        func(string) string { return "" },
		"style":         func(string) string { return "" },
		"__lfmScript":   func(string, string) string { return "" },
		"__lfmStyle":    func(string, string) string { return "" },
	}
	for name, fn := range custom {
		funcs[name] = fn
	}
	return funcs
}

// Go functions have no calling-template context. Bind the original source file
// into the AST once, so nested definitions/partials and dry probes agree on paths.
func bindAssetOrigins(node parse.Node, file string) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *parse.ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				bindAssetOrigins(child, file)
			}
		}
	case *parse.ActionNode:
		bindAssetOrigins(n.Pipe, file)
	case *parse.IfNode:
		bindOptional(n.Pipe)
		bindAssetOrigins(n.Pipe, file)
		bindAssetOrigins(n.List, file)
		bindAssetOrigins(n.ElseList, file)
	case *parse.WithNode:
		bindOptional(n.Pipe)
		bindAssetOrigins(n.Pipe, file)
		bindAssetOrigins(n.List, file)
		bindAssetOrigins(n.ElseList, file)
	case *parse.RangeNode:
		bindAssetOrigins(n.Pipe, file)
		bindAssetOrigins(n.List, file)
		bindAssetOrigins(n.ElseList, file)
	case *parse.TemplateNode:
		bindAssetOrigins(n.Pipe, file)
	case *parse.PipeNode:
		if n != nil {
			for _, command := range n.Cmds {
				bindAssetOrigins(command, file)
			}
		}
	case *parse.ChainNode:
		bindAssetOrigins(n.Node, file)
	case *parse.CommandNode:
		for _, arg := range n.Args {
			bindAssetOrigins(arg, file)
		}
		if len(n.Args) == 0 {
			return
		}
		id, ok := n.Args[0].(*parse.IdentifierNode)
		if !ok || (id.Ident != "script" && id.Ident != "style") {
			return
		}
		name := "__lfmScript"
		if id.Ident == "style" {
			name = "__lfmStyle"
		}
		origin := &parse.StringNode{NodeType: parse.NodeString, Pos: id.Pos, Quoted: strconv.Quote(file), Text: file}
		n.Args = append([]parse.Node{parse.NewIdentifier(name).SetPos(id.Pos), origin}, n.Args[1:]...)
	}
}

// dict preserves Go template values, including their HTML escaping types.
func dict(args ...any) (map[string]any, error) {
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("dict requires pairs of string key and value; got %d arguments", len(args))
	}
	out := make(map[string]any, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key at argument %d must be a string, got %T", i+1, args[i])
		}
		out[key] = args[i+1]
	}
	return out, nil
}

func optional(base any, fields ...string) (any, error) {
	for _, field := range fields {
		object, ok := base.(map[string]any)
		if base == nil {
			return nil, nil
		}
		if !ok {
			raw, found, err := fieldMethod(base, field)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, fmt.Errorf("optional path needs an object or a no-argument method, got %T", base)
			}
			base = raw
			continue
		}
		base = object[field]
	}
	return base, nil
}

// Only a direct field selection in an if/with condition declares an optional path.
func bindOptional(pipe *parse.PipeNode) {
	if pipe == nil || len(pipe.Cmds) != 1 || len(pipe.Cmds[0].Args) != 1 {
		return
	}
	node := pipe.Cmds[0].Args[0]
	var base parse.Node
	var fields []string
	switch n := node.(type) {
	case *parse.FieldNode:
		base = &parse.DotNode{NodeType: parse.NodeDot, Pos: n.Pos}
		fields = n.Ident
	case *parse.VariableNode:
		if len(n.Ident) < 2 {
			return
		}
		copy := n.Copy().(*parse.VariableNode)
		copy.Ident = copy.Ident[:1]
		base = copy
		fields = n.Ident[1:]
	default:
		return
	}
	args := []parse.Node{parse.NewIdentifier("__lfmOptional").SetPos(node.Position()), base}
	for _, field := range fields {
		args = append(args, &parse.StringNode{NodeType: parse.NodeString, Pos: node.Position(), Quoted: strconv.Quote(field), Text: field})
	}
	pipe.Cmds[0].Args = args
}

// fieldMethod follows the same no-argument, one-value (plus optional error)
// method convention as Go templates. JSON numbers expose these methods too.
func fieldMethod(raw any, field string) (any, bool, error) {
	rv := reflect.ValueOf(raw)
	if !rv.IsValid() {
		return nil, false, nil
	}
	method := rv.MethodByName(field)
	if !method.IsValid() || method.Type().NumIn() != 0 {
		return nil, false, nil
	}
	typ := method.Type()
	if typ.NumOut() != 1 && typ.NumOut() != 2 || typ.NumOut() == 2 && typ.Out(1) != reflect.TypeFor[error]() {
		return nil, true, fmt.Errorf("method %s must return one value, optionally followed by error", field)
	}
	results := method.Call(nil)
	if len(results) == 2 && !results[1].IsNil() {
		return nil, true, results[1].Interface().(error)
	}
	return results[0].Interface(), true, nil
}
