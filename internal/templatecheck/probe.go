package templatecheck

import (
	"io"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"
)

type probeKey struct {
	command   *parse.CommandNode
	variables string
	previous  bool
}
type probe struct {
	template         *template.Template
	env              *environment
	previous, result any
}

// A cached standard-library template evaluates one original command and captures
// its value without stringifying it. This preserves Go's builtin/type semantics,
// including pipeline argument order and lazy and/or evaluation.
func (c *checker) execute(command *parse.CommandNode, env *environment, previous *value) (any, error) {
	// Bind only the variables the command reads: every binding costs a
	// reflective call on each evaluation, and ranges evaluate bodies per item.
	used := c.variablesIn(command)
	variables := make([]string, 0, len(used))
	for name := range env.variables {
		if used[name] {
			variables = append(variables, name)
		}
	}
	sort.Strings(variables)
	key := probeKey{command, strings.Join(variables, "\x00"), previous != nil}
	p := c.probes[key]
	if p == nil {
		p = &probe{}
		funcs := template.FuncMap{
			"__lfmRoot":     func() any { return p.env.rootValue().raw },
			"__lfmVar":      func(name string) any { return p.env.variables[name].raw },
			"__lfmPrevious": func() any { return p.previous },
			"__lfmCapture":  func(v any) string { p.result = v; return "" },
		}
		for name, fn := range assetFunctions(c.options.Funcs) {
			funcs[name] = fn
		}
		var prologue strings.Builder
		if used["$"] {
			prologue.WriteString("{{$ = __lfmRoot}}")
		}
		for _, name := range variables {
			if name == "$" {
				continue
			}
			prologue.WriteString("{{" + name + " := __lfmVar " + strconv.Quote(name) + "}}")
		}
		prologue.WriteString("{{__lfmCapture .}}")
		t, err := template.New("__lfmProbe").Funcs(funcs).Option("missingkey=error").Parse(prologue.String())
		if err != nil {
			return nil, err
		}
		action := t.Tree.Root.Nodes[len(t.Tree.Root.Nodes)-1].(*parse.ActionNode)
		copy := command.Copy().(*parse.CommandNode)
		if previous != nil {
			copy.Args = append(copy.Args, parse.NewIdentifier("__lfmPrevious"))
		}
		capture := action.Pipe.Cmds[0]
		capture.Args = capture.Args[:1]
		action.Pipe.Cmds = []*parse.CommandNode{copy, capture}
		p.template = t
		c.probes[key] = p
	}
	p.env = env
	p.previous = nil
	p.result = nil
	if previous != nil {
		p.previous = previous.raw
	}
	err := p.template.Execute(io.Discard, env.dot.raw)
	return p.result, err
}

// variablesIn lists the variables a command reads, including inside nested
// parenthesized pipelines. Results are cached per command node.
func (c *checker) variablesIn(command *parse.CommandNode) map[string]bool {
	if used, ok := c.commandVariables[command]; ok {
		return used
	}
	used := map[string]bool{}
	var visit func(parse.Node)
	visit = func(n parse.Node) {
		switch n := n.(type) {
		case *parse.VariableNode:
			used[n.Ident[0]] = true
		case *parse.ChainNode:
			visit(n.Node)
		case *parse.PipeNode:
			for _, cmd := range n.Cmds {
				visit(cmd)
			}
		case *parse.CommandNode:
			for _, arg := range n.Args {
				visit(arg)
			}
		}
	}
	visit(command)
	c.commandVariables[command] = used
	return used
}
