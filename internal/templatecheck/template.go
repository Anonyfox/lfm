// Package templatecheck validates JSON/template bindings before HTML rendering.
// It walks Go's parse trees and delegates expression evaluation to text/template.
package templatecheck

import (
	"bytes"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

func Catalog() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{ID: "config.invalid", Description: "Config/data cannot be loaded safely", Required: true},
		{ID: "config.duplicate-key", Description: "JSON keys must have a single definition", Required: true},
		{ID: "template.syntax", Description: "Parse every template/partial and report syntax errors", Required: true},
		{ID: "template.bindings", Description: "Missing values, wrong shapes, invalid operations and recursive calls", Required: true},
		{ID: "template.browser-data", Description: "Validate every runtime-data selection path", Required: true},
		{ID: "template.render", Description: "Report contextual HTML escaping/render failures safely", Required: true},
		{ID: "template.inactive", Description: "Potential binding failures in currently inactive branches"},
		{ID: "template.empty", Description: "Empty rendered values and empty ranges/with sections"},
		{ID: "template.value", Description: "Structured interpolation, printf issues and definition overrides"},
		{ID: "template.unused", Description: "Unused authored config/data fields and template definitions"},
		{ID: "jsonld.bindings", Description: "Resolve structured JSON-LD placeholders once, preserving JSON types", Required: true},
		{ID: "jsonld.empty", Description: "Empty values selected by JSON-LD placeholders"},
	}
}

type Options struct {
	Entry       string
	Values      map[string]any
	Files       map[string][]byte
	Sources     jsonsrc.Sources
	BrowserData []string
	Rules       map[string]diagnostic.Severity
	// Consumed holds compiler-rendered config subtrees and their dependencies.
	Consumed []string
	// Funcs supplies compiler helpers to both the dry evaluator and final render.
	Funcs htmltemplate.FuncMap
}

// Parse collects one syntax error per invalid source file, not just the first
// failed file. The standard library remains responsible for Go template syntax.
func Parse(files map[string][]byte, entry string) (*htmltemplate.Template, diagnostic.Summary) {
	result := htmltemplate.New(entry).Funcs(assetFunctions(nil)).Option("missingkey=error")
	var issues []diagnostic.Diagnostic
	names := make([]string, 0, len(files))
	for name := range files {
		if name != entry {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append([]string{entry}, names...)
	defined := map[string]diagnostic.Location{}
	for _, file := range names {
		raw, exists := files[file]
		if !exists {
			issues = append(issues, diagnostic.Diagnostic{Rule: "template.syntax", Severity: diagnostic.Error, Location: diagnostic.At(file, nil, 0), Message: "Entry template is missing.", Hint: "Create index.html.tmpl in the workbench."})
			continue
		}
		module, err := htmltemplate.New(file).Funcs(assetFunctions(nil)).Option("missingkey=error").Parse(string(raw))
		if err != nil {
			issues = append(issues, ParseError(file, raw, err))
			continue
		}
		templates := module.Templates()
		sort.Slice(templates, func(i, j int) bool { return templates[i].Name() < templates[j].Name() })
		for _, t := range templates {
			if t.Tree == nil {
				continue
			}
			at := nodeLocation(t.Tree, t.Tree.Root, files)
			if first, exists := defined[t.Name()]; exists && strings.TrimSpace(t.Tree.Root.String()) != "" {
				issues = append(issues, diagnostic.Diagnostic{Rule: "template.value", Severity: diagnostic.Warning, Location: at, Related: []diagnostic.Location{first}, Message: fmt.Sprintf("Template %q overrides an earlier definition.", t.Name()), Hint: "Keep this override only if block/template customization is intentional."})
			}
			if strings.TrimSpace(t.Tree.Root.String()) != "" {
				defined[t.Name()] = at
			}
			bound := t.Tree.Copy()
			bindAssetOrigins(bound.Root, file)
			if _, err := result.AddParseTree(t.Name(), bound); err != nil {
				issues = append(issues, ParseError(file, raw, err))
			}
		}
	}
	return result, diagnostic.Summarize(issues)
}

var errorPosition = regexp.MustCompile(`(?:^template: )(.+?):([0-9]+)(?::([0-9]+))?: `)

func ParseError(file string, raw []byte, err error) diagnostic.Diagnostic {
	at := diagnostic.At(file, raw, 0)
	if match := errorPosition.FindStringSubmatch(err.Error()); match != nil {
		line, _ := strconv.Atoi(match[2])
		column := 1
		if match[3] != "" {
			column, _ = strconv.Atoi(match[3])
		}
		at = atLine(file, raw, line, column)
	}
	return diagnostic.Diagnostic{Rule: "template.syntax", Severity: diagnostic.Error, Location: at, Message: err.Error(), Hint: "Fix this Go template syntax or undefined variable/function; each invalid file is reported independently."}
}

func atLine(file string, raw []byte, line, column int) diagnostic.Location {
	offset, current := 0, 1
	for offset < len(raw) && current < line {
		if raw[offset] == '\n' {
			current++
		}
		offset++
	}
	return diagnostic.At(file, raw, offset+max(0, column-1))
}

func nodeLocation(tree *parse.Tree, node parse.Node, files map[string][]byte) diagnostic.Location {
	file := "index.html.tmpl"
	if tree != nil {
		file = tree.ParseName
		if file == "" {
			file = tree.Name
		}
	}
	offset := 0
	if node != nil {
		offset = int(node.Position())
	}
	return diagnostic.At(file, files[file], offset)
}

// Render still uses html/template's contextual escaping. Unexpected execution
// failures become diagnostics; partial output is discarded.
func Render(t *htmltemplate.Template, options Options) (raw []byte, issue *diagnostic.Diagnostic) {
	defer func() {
		if value := recover(); value != nil {
			raw = nil
			d := diagnostic.Diagnostic{Rule: "template.render", Severity: diagnostic.Error, Location: diagnostic.At(options.Entry, options.Files[options.Entry], 0), Message: "Template rendering stopped unexpectedly: " + fmt.Sprint(value), Hint: "Check the template expression and its input types; no partial output was published."}
			issue = &d
		}
	}()
	t.Funcs(assetFunctions(options.Funcs))
	var output bytes.Buffer
	if err := t.ExecuteTemplate(&output, options.Entry, options.Values); err != nil {
		at := diagnostic.At(options.Entry, options.Files[options.Entry], 0)
		var escaping *htmltemplate.Error
		if errors.As(err, &escaping) {
			if target := t.Lookup(escaping.Name); target != nil && target.Tree != nil {
				at = nodeLocation(target.Tree, escaping.Node, options.Files)
			}
			if escaping.Line > 0 {
				at = atLine(at.File, options.Files[at.File], escaping.Line, 1)
			}
		} else if match := errorPosition.FindStringSubmatch(err.Error()); match != nil {
			line, _ := strconv.Atoi(match[2])
			column := 1
			if match[3] != "" {
				column, _ = strconv.Atoi(match[3])
			}
			at = atLine(match[1], options.Files[match[1]], line, column)
		}
		d := diagnostic.Diagnostic{Rule: "template.render", Severity: diagnostic.Error, Location: at, Message: err.Error(), Hint: "Fix this template expression or inconsistent HTML escaping context. Rendering stopped safely; partial HTML was discarded."}
		return nil, &d
	}
	if bytes.Contains(output.Bytes(), []byte("ZgotmplZ")) {
		d := diagnostic.Diagnostic{Rule: "template.render", Severity: diagnostic.Error, Location: diagnostic.At(options.Entry, options.Files[options.Entry], 0), Message: "html/template rejected a URL/value (ZgotmplZ).", Hint: "Check href/src/style bindings for a disallowed URL scheme or value; use the expected value type and a permitted URL."}
		return nil, &d
	}
	return output.Bytes(), nil
}
