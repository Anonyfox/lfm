package templatecheck

import (
	htmltemplate "html/template"
	"strconv"
	"text/template/parse"
)

func assetFunctions(custom htmltemplate.FuncMap) htmltemplate.FuncMap {
	funcs := htmltemplate.FuncMap{
		"script":        func(string) string { return "" },
		"style":         func(string) string { return "" },
		"__lfmScript": func(string, string) string { return "" },
		"__lfmStyle":  func(string, string) string { return "" },
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
		bindAssetOrigins(n.Pipe, file)
		bindAssetOrigins(n.List, file)
		bindAssetOrigins(n.ElseList, file)
	case *parse.WithNode:
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
