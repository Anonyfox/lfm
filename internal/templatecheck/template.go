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
		{
			ID:       "config.invalid",
			Summary:  "lfm.json and data/*.json load as valid JSON matching the config schema",
			Help:     "Triggered by malformed JSON, unknown or misspelled reserved fields, a missing title, description or language, a url that is not an absolute HTTPS directory URL ending in /, invalid build settings, or a data key defined in both lfm.json and a data file. The compiler cannot render a trustworthy page from ambiguous input, so nothing is built. Fix the field at the reported path; product-specific values belong under data.",
			Required: true,
		},
		{
			ID:       "config.duplicate-key",
			Summary:  "Every JSON key in lfm.json and data files is defined only once",
			Help:     "The same key appears twice in one JSON object. encoding/json would silently keep the last value, so the page could show content you did not mean to ship. Keep one definition; both locations are reported.",
			Required: true,
		},
		{
			ID:       "template.syntax",
			Summary:  "index.html.tmpl and every partial parse as Go html/template source",
			Help:     "Triggered by a missing entry template, unbalanced actions, unknown functions, undefined variables or other parse errors, reported once per file. A template that does not parse cannot render at all. Fix the reported line; Go's parser stops at the first error in each file, so recompile after each fix.",
			Required: true,
		},
		{
			ID:       "template.bindings",
			Summary:  "Active template paths exist in the config and have the shape each action expects",
			Help:     "Raised when a reachable action reads a missing field, ranges over a non-collection, calls an undefined template, applies an invalid operation, recurses without progress or exceeds the analysis limits. Rendering this would print blanks or fail for visitors. Fix the spelling, add the value to lfm.json or data, or guard an optional value with if/with.",
			Required: true,
		},
		{
			ID:       "template.browser-data",
			Summary:  "Every build.browserData entry selects a path that exists in the config",
			Help:     "build.browserData lists config paths that are serialized for page scripts, and one of them names a path that does not exist. Scripts reading it would get undefined at runtime. Correct the path or add the value under data; all bad paths are reported together.",
			Required: true,
		},
		{
			ID:       "template.render",
			Summary:  "The final html/template render completes without escaping or execution errors",
			Help:     "Rendering failed because a value does not fit its HTML context, an expression errored at execution time, or html/template replaced an unsafe URL or value with ZgotmplZ. Shipping that output would break links or styles, so partial HTML is discarded. Fix the binding at the reported location to use the expected type and a permitted URL scheme.",
			Required: true,
		},
		{
			ID:      "template.inactive",
			Summary: "Binding problems inside branches the current config does not render",
			Help:    "A branch that is skipped with today's data, or a short-circuited argument, reads a missing value or misuses one. It renders fine now and breaks the day the condition flips. Fix the binding the same way as an active one, or delete the dead branch.",
		},
		{
			ID:      "template.empty",
			Summary: "Rendered empty values, missing index keys and empty range or with sections",
			Help:    "An action outputs an empty string or null, index selects a missing map key, or a range or with section renders nothing. Visitors see a gap, and bindings inside an empty section cannot be checked. Supply the content, or guard intentionally optional values with if/with; false and zero count as content. Turn this off only if empty sections are product policy.",
		},
		{
			ID:      "template.value",
			Summary: "Objects or arrays printed directly, printf failure markers and template overrides",
			Help:    "Triggered when an object or array is interpolated straight into HTML, when output contains a Go formatting marker such as %!d, or when a later define replaces an earlier nonempty template. Visitors would see raw map[...] dumps or %!s(...) text. Render fields with range/with, fix the printf verb, pass structured data to scripts through build.browserData, and keep an override only if it is intentional.",
		},
		{
			ID:      "template.unused",
			Summary: "Authored config and data values or template definitions that nothing uses",
			Help:    "A config or data leaf is never read by the template, JSON-LD or browserData, or a nonempty define or partial is never called. Stale copy and prices tend to hide here and drift out of date. Remove the value or definition, reference it in the template, or list runtime-only values in build.browserData.",
		},
		{
			ID:       "jsonld.bindings",
			Summary:  "Placeholders in the lfm.json jsonld block resolve to existing values of a JSON type",
			Help:     "JSON-LD string values may contain plain field selections such as {{.title}}; this fails for missing or misspelled paths, missing index keys, references into jsonld itself, control flow, functions, pipelines, or values that cannot be serialized. Broken structured data means search engines and assistants get wrong or no facts about the page. Select an existing config or data path, declare intentionally absent keys as null, and keep logic in the HTML template.",
			Required: true,
		},
		{
			ID:      "jsonld.empty",
			Summary: "JSON-LD placeholders that select null or an empty string",
			Help:    "A jsonld placeholder resolves to null or an empty string. Search engines and assistants then read a blank fact about the page instead of no claim at all. Supply the value, or keep the explicit empty value only when it is intended.",
		},
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
