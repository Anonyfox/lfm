package compile

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/evanw/esbuild/pkg/api"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type bundleInput struct {
	node                      *html.Node
	path, source, media, name string
	file, remote, isolated    bool
	compiledCSS               bool
	cached                    *compiled
}

// Browser loading semantics are boundaries, not module-format configuration.
// Ordinary local modules share one graph; classic globals/remote/async scripts
// and independently addressable styles remain separate when merging changes them.
type pageBundles struct {
	b               *builder
	styles, scripts []bundleInput
	prefix          string
	helpers         map[string]bool
}

func (p *pageBundles) helper(node *html.Node, entry helperEntry) {
	key := entry.kind + "\x00" + entry.path
	if p.helpers == nil {
		p.helpers = map[string]bool{}
	}
	if p.helpers[key] {
		node.Parent.RemoveChild(node)
		return
	}
	p.helpers[key] = true
	htmlx.DelAttr(node, "data-lfm-helper")
	input := bundleInput{node: node, path: entry.path, file: p.b.mode(entry.path) == "file"}
	input.name = strings.TrimSuffix(filepath.Base(entry.path), filepath.Ext(entry.path))
	if entry.kind == "script" {
		input.name += ".js"
		p.scripts = append(p.scripts, input)
	} else {
		input.name += ".css"
		p.styles = append(p.styles, input)
	}
}

func (p *pageBundles) filename(group []bundleInput, index, total int, ext string) string {
	return p.prefix + bundleName(group, index, total, ext)
}

// hashedCode names an emitted stylesheet or script after its content, so its
// URL changes whenever its bytes do and hosts can cache it forever.
func hashedCode(name string, code []byte) string {
	sum := sha256.Sum256(code)
	ext := filepath.Ext(name)
	return fmt.Sprintf("%s-%x%s", strings.TrimSuffix(name, ext), sum[:6], ext)
}

func stylesheetRel(rel string) bool {
	for _, token := range strings.Fields(rel) {
		if token == "stylesheet" {
			return true
		}
	}
	return false
}

func independentlyAddressable(node *html.Node, allowed ...string) bool {
	for _, item := range node.Attr {
		found := false
		for _, key := range allowed {
			if item.Key == key {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

func (p *pageBundles) stylesheet(node *html.Node) error {
	input := bundleInput{node: node, source: htmlx.RawText(node), media: htmlx.Attr(node, "media")}
	if node.Data == "link" {
		ref := htmlx.Attr(node, "href")
		remote, err := p.b.remote(ref)
		if err != nil {
			return err
		}
		input.remote = remote
		if !remote {
			input.path, _, err = p.b.assetPath(ref, p.b.root)
			if err != nil {
				return err
			}
		}
	}
	input.file = p.b.config.Build.Assets.Default == "file"
	if input.path != "" {
		if strings.HasSuffix(strings.ToLower(input.path), ".module.css") {
			return fmt.Errorf("import .module.css from JavaScript to use its generated class names; native stylesheet tags expect global CSS")
		}
		input.file = p.b.mode(input.path) == "file"
		input.name = strings.TrimSuffix(filepath.Base(input.path), filepath.Ext(input.path)) + ".css"
	}
	input.isolated = independentlyAddressable(node, "rel", "href", "type", "media", "integrity", "crossorigin") || strings.Contains(strings.ToLower(htmlx.Attr(node, "rel")), "alternate")
	if input.isolated {
		input.media = ""
	} // preserve addressable sheet.media on its node
	if node.Data == "link" && input.isolated {
		input.file = true
	}
	p.styles = append(p.styles, input)
	return nil
}

func (p *pageBundles) script(node *html.Node, module bool) error {
	legacy := htmlx.HasAttr(node, "data-lfm-classic")
	if legacy && module {
		return fmt.Errorf("data-lfm-classic cannot be combined with type=module")
	}
	htmlx.DelAttr(node, "data-lfm-classic")
	input := bundleInput{node: node, source: htmlx.RawText(node)}
	if ref := htmlx.Attr(node, "src"); ref != "" {
		remote, err := p.b.remote(ref)
		if err != nil {
			return err
		}
		input.remote = remote
		if !remote {
			input.path, _, err = p.b.assetPath(ref, p.b.root)
			if err != nil {
				return err
			}
		}
	}
	input.file = p.b.config.Build.Assets.Default == "file"
	if input.path != "" {
		input.file = p.b.mode(input.path) == "file"
		input.name = strings.TrimSuffix(filepath.Base(input.path), filepath.Ext(input.path)) + ".js"
	}
	input.isolated = independentlyAddressable(node, "src", "type", "defer", "integrity", "crossorigin")
	if !input.remote && !module {
		var result compiled
		var err error
		if legacy {
			result, err = p.b.compileClassic(input.path, input.source)
		} else {
			result, err = p.b.compile(input.path, input.source, false, false)
		}
		if err != nil {
			return err
		}
		module = result.module
		if !module {
			input.cached = &result
		}
	}
	if module {
		htmlx.SetAttr(node, "type", "module")
	} else {
		input.isolated = true
		// These attributes only retain their native classic-script behavior with
		// an external src, so inlining would silently change scheduling/events.
		if input.path != "" && (htmlx.HasAttr(node, "async") || htmlx.HasAttr(node, "defer") || htmlx.HasAttr(node, "onload") || htmlx.HasAttr(node, "onerror")) {
			input.file = true
		}
	}
	p.scripts = append(p.scripts, input)
	return nil
}

// Explicit escape hatch for a UMD browser-global script: wrapping it as CommonJS
// would select its exports branch instead of attaching the intended window API.
func (b *builder) compileClassic(path, source string) (compiled, error) {
	options := b.bundleOptions(".js")
	options.Bundle = false
	options.Format = api.FormatDefault
	options.TreeShaking = api.TreeShakingFalse
	options.MinifyIdentifiers = false
	label, directory := "index.html.tmpl", b.root
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return compiled{}, err
		}
		source = string(raw)
		rel, _ := filepath.Rel(b.root, path)
		label = filepath.ToSlash(rel)
		directory = filepath.Dir(path)
	}
	// Stdin explicitly sets script parsing, independent of package.json type.
	options.Stdin = &api.StdinOptions{Contents: source, Loader: api.LoaderJS, ResolveDir: directory, Sourcefile: label + " (classic)"}
	result := api.Build(options)
	if len(result.Errors) > 0 {
		return compiled{}, bundleError(result.Errors)
	}
	var metadata struct {
		Inputs map[string]struct {
			Format string `json:"format"`
		} `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(result.Metafile), &metadata); err != nil {
		return compiled{}, err
	}
	for _, input := range metadata.Inputs {
		if input.Format == "esm" {
			return compiled{}, fmt.Errorf("data-lfm-classic is for standalone browser globals; use a module entry for imports/exports")
		}
	}
	return b.bundleResult(result, ".js", false)
}

func groups(inputs []bundleInput) [][]bundleInput {
	var result [][]bundleInput
	for _, input := range inputs {
		if len(result) == 0 || input.remote || input.isolated {
			result = append(result, []bundleInput{input})
			continue
		}
		last := result[len(result)-1]
		if last[0].remote || last[0].isolated || last[0].file != input.file {
			result = append(result, []bundleInput{input})
			continue
		}
		result[len(result)-1] = append(last, input)
	}
	return result
}

func bundleName(group []bundleInput, index, total int, ext string) string {
	if total == 1 && len(group) == 1 && group[0].name != "" {
		return group[0].name
	}
	if total == 1 {
		return "bundle" + ext
	}
	return "bundle-" + strconv.Itoa(index+1) + ext
}

func (p *pageBundles) compileScripts() ([]*html.Node, error) {
	var nodes []*html.Node
	all := groups(p.scripts)
	for i, group := range all {
		first := group[0]
		if first.remote {
			nodes = append(nodes, first.node)
			continue
		}
		var result compiled
		var err error
		if len(group) == 1 && first.cached != nil {
			result = *first.cached
		} else {
			result, err = p.b.compileInputs(group, false)
		}
		if err != nil {
			return nil, err
		}
		if result.cssModules {
			p.b.cssModuleGraphs++
			if p.b.cssModuleGraphs > 1 {
				return nil, fmt.Errorf("CSS Modules must share one JavaScript graph: independent bundles can generate colliding class names; combine their script entries with the same asset mode/loading semantics, or use global CSS across those boundaries")
			}
		}
		node := first.node
		htmlx.DelAttr(node, "integrity")
		if first.file {
			name := hashedCode(p.filename(group, i, len(all), ".js"), result.code)
			if err := p.b.emit(name, fmt.Sprintf("%sscript-bundle:%d", p.prefix, i), result.code); err != nil {
				return nil, err
			}
			htmlx.SetAttr(node, "src", fileURL(name))
			htmlx.SetText(node, "")
		} else {
			htmlx.DelAttr(node, "src", "crossorigin", "defer")
			htmlx.SetText(node, string(result.code))
		}
		p.b.bundleReferences(group, htmlx.Attr(node, "src"))
		if p.b.automaticGroup(group) && !first.isolated && !first.file {
			p.b.autoCode = append(p.b.autoCode, codeCandidate{node: node, inputs: group, name: p.filename(group, i, len(all), ".js"), origin: fmt.Sprintf("%sscript-bundle:%d", p.prefix, i), kind: "script"})
		}
		for _, input := range group {
			if input.node.Parent != nil {
				input.node.Parent.RemoveChild(input.node)
			}
		}
		nodes = append(nodes, node)
		if len(result.css) > 0 {
			p.styles = append(p.styles, bundleInput{source: string(result.css), compiledCSS: true, file: first.file, name: strings.TrimSuffix(bundleName(group, i, len(all), ".js"), ".js") + ".css"})
		}
	}
	return nodes, nil
}

func (p *pageBundles) compileStyles(head *html.Node) error {
	all := groups(p.styles)
	for i, group := range all {
		first := group[0]
		if first.remote {
			htmlx.Move(head, first.node)
			continue
		}
		result, err := p.b.compileInputs(group, true)
		if err != nil {
			return err
		}
		node := first.node
		if node == nil {
			node = htmlx.Element("style", "")
		}
		alternate := strings.Contains(strings.ToLower(htmlx.Attr(node, "rel")), "alternate")
		for _, input := range group {
			if input.node != nil && input.node.Parent != nil {
				input.node.Parent.RemoveChild(input.node)
			}
		}
		htmlx.DelAttr(node, "rel", "href", "integrity", "crossorigin")
		if !first.isolated {
			htmlx.DelAttr(node, "media")
		}
		if first.file {
			name := hashedCode(p.filename(group, i, len(all), ".css"), result.code)
			if err := p.b.emit(name, fmt.Sprintf("%sstyle-bundle:%d", p.prefix, i), result.code); err != nil {
				return err
			}
			node.Data, node.DataAtom = "link", atom.Link
			htmlx.SetText(node, "")
			// Void elements must not retain even an empty text child.
			for node.FirstChild != nil {
				node.RemoveChild(node.FirstChild)
			}
			htmlx.SetAttr(node, "rel", "stylesheet")
			htmlx.SetAttr(node, "href", fileURL(name))
			if alternate {
				htmlx.SetAttr(node, "rel", "alternate stylesheet")
			}
		} else {
			node.Data, node.DataAtom = "style", atom.Style
			htmlx.SetText(node, string(result.code))
		}
		p.b.bundleReferences(group, htmlx.Attr(node, "href"))
		if p.b.automaticGroup(group) && !first.isolated && !first.file {
			p.b.autoCode = append(p.b.autoCode, codeCandidate{node: node, inputs: group, name: p.filename(group, i, len(all), ".css"), origin: fmt.Sprintf("%sstyle-bundle:%d", p.prefix, i), kind: "style"})
		}
		htmlx.Move(head, node)
	}
	return nil
}

func (b *builder) automaticGroup(group []bundleInput) bool {
	if b.config.Build.Assets.Default != "auto" {
		return false
	}
	for _, input := range group {
		if input.path != "" && b.mode(input.path) != "auto" {
			return false
		}
	}
	return true
}

func (b *builder) bundleReferences(group []bundleInput, ref string) {
	if b.references == nil {
		b.references = map[string]string{}
	}
	for _, input := range group {
		if input.path != "" {
			b.references[input.path] = ref
		}
	}
}

// A virtual entry lets esbuild resolve/deduplicate the whole graph, including
// package.json browser/exports/sideEffects semantics. Nothing is written to source.
func (b *builder) compileInputs(inputs []bundleInput, css bool) (compiled, error) {
	var source strings.Builder
	for i, input := range inputs {
		specifier, _ := json.Marshal("lfm-input:" + strconv.Itoa(i))
		if css {
			source.WriteString("@import " + string(specifier))
			if input.media != "" {
				source.WriteString(" " + input.media)
			}
			source.WriteString(";\n")
		} else {
			source.WriteString("import " + string(specifier) + ";\n")
		}
	}
	loader, ext := api.LoaderJS, ".js"
	if css {
		loader, ext = api.LoaderCSS, ".css"
	}
	options := b.bundleOptions(ext)
	options.Stdin = &api.StdinOptions{Contents: source.String(), ResolveDir: b.root, Sourcefile: "page-entry" + ext, Loader: loader}
	options.Plugins = []api.Plugin{entryPlugin(inputs, loader, b.root), b.assetPlugin()}
	result := api.Build(options)
	if len(result.Errors) > 0 {
		return compiled{}, bundleError(result.Errors)
	}
	return b.bundleResult(result, ext, !css)
}

func entryPlugin(inputs []bundleInput, loader api.Loader, root string) api.Plugin {
	return api.Plugin{Name: "page-entries", Setup: func(build api.PluginBuild) {
		build.OnResolve(api.OnResolveOptions{Filter: `^lfm-input:`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			index, err := strconv.Atoi(strings.TrimPrefix(args.Path, "lfm-input:"))
			if err != nil || index < 0 || index >= len(inputs) {
				return api.OnResolveResult{}, fmt.Errorf("invalid virtual page entry")
			}
			if path := inputs[index].path; path != "" {
				return api.OnResolveResult{Path: path, Namespace: "file", SideEffects: api.SideEffectsTrue}, nil
			}
			return api.OnResolveResult{Path: strconv.Itoa(index), Namespace: "page-source", SideEffects: api.SideEffectsTrue}, nil
		})
		build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "page-source"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
			index, _ := strconv.Atoi(args.Path)
			contents := inputs[index].source
			return api.OnLoadResult{Contents: &contents, Loader: loader, ResolveDir: root, PluginData: inputs[index].compiledCSS}, nil
		})
	}}
}
