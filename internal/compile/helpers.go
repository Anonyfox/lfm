package compile

import (
	"crypto/rand"
	"fmt"
	htmltemplate "html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/project"
	"github.com/evanw/esbuild/pkg/api"
)

type helperEntry struct{ kind, path, origin string }
type helperRegistry struct {
	project      project.Project
	root, prefix string
	resolved     map[string]helperEntry
	entries      map[string]helperEntry
}

func newHelpers(p project.Project) *helperRegistry {
	return &helperRegistry{project: p, root: p.Root, prefix: fmt.Sprintf("%x", randomToken()), resolved: map[string]helperEntry{}, entries: map[string]helperEntry{}}
}

func randomToken() [16]byte { var token [16]byte; _, _ = rand.Read(token[:]); return token }

func (h *helperRegistry) functions(render bool) htmltemplate.FuncMap {
	makeHelper := func(kind string) any {
		return func(origin string, value any) (htmltemplate.HTML, error) {
			ref, ok := value.(string)
			if trusted, isURL := value.(htmltemplate.URL); isURL {
				ref, ok = string(trusted), true
			}
			if !ok {
				return "", fmt.Errorf("%s helper needs a file/package string, got %T", kind, value)
			}
			entry, err := h.resolve(kind, origin, ref)
			if err != nil {
				return "", err
			}
			if !render {
				return htmltemplate.HTML("compiler asset declaration"), nil
			}
			id := fmt.Sprintf("%s-%d", h.prefix, len(h.entries))
			h.entries[id] = entry
			tag, extra := "style", ""
			if kind == "script" {
				tag, extra = "script", ` type="module"`
			}
			return htmltemplate.HTML(fmt.Sprintf(`<%s%s data-lfm-helper="%s"></%s>`, tag, extra, id, tag)), nil
		}
	}
	funcs := htmltemplate.FuncMap{"__lfmScript": makeHelper("script"), "__lfmStyle": makeHelper("style")}
	for name, f := range comparisons {
		funcs[name] = f
	}
	return funcs
}

// Existing local paths win over package names. Explicit dot paths are relative
// to the source template; bare local paths also have a project-root fallback.
// esbuild alone interprets extensions, directories, browser maps and exports.
func (h *helperRegistry) resolve(kind, origin, ref string) (helperEntry, error) {
	key := kind + "\x00" + origin + "\x00" + ref
	if entry, ok := h.resolved[key]; ok {
		return entry, nil
	}
	if ref == "" || strings.TrimSpace(ref) != ref || strings.ContainsAny(ref, "\x00\r\n") {
		return helperEntry{}, fmt.Errorf("%s helper needs a nonempty file path or installed package specifier", kind)
	}
	if strings.Contains(ref, "://") || strings.HasPrefix(ref, "//") || strings.HasPrefix(ref, "node:") || strings.HasPrefix(ref, "data:") {
		return helperEntry{}, fmt.Errorf("%s helper bundles local/browser sources; use a native HTML tag for a remote resource", kind)
	}
	directory := filepath.Dir(filepath.Join(h.root, filepath.FromSlash(origin)))
	specifier := ref
	explicit := filepath.IsAbs(ref) || strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../")
	candidates := []string{filepath.Join(directory, filepath.FromSlash(ref))}
	if filepath.IsAbs(ref) {
		candidates = []string{ref}
	} else if !explicit && directory != h.root {
		candidates = append(candidates, filepath.Join(h.root, filepath.FromSlash(ref)))
	}
	local := ""
	for _, candidate := range candidates {
		if _, err := os.Lstat(candidate); err == nil {
			actual, err := h.project.Path(candidate)
			if err != nil {
				return helperEntry{}, err
			}
			specifier = actual
			inPackage := strings.Contains(filepath.ToSlash(actual), "/node_modules/")
			if info, err := os.Stat(actual); err == nil && info.Mode().IsRegular() && !inPackage {
				local = actual
			}
			break
		}
	}
	// An existing project file needs no resolver. Package names, directories and
	// files inside packages (whose package.json may remap them) go through
	// esbuild, whose resolver rescans every parent directory per build.
	if local != "" {
		return h.register(key, kind, origin, local)
	}
	resolveKind := api.ResolveJSImportStatement
	if kind == "style" {
		resolveKind = api.ResolveCSSImportRule
	}
	var resolved api.ResolveResult
	result := api.Build(api.BuildOptions{
		AbsWorkingDir: h.root, Write: false, Bundle: true, Platform: api.PlatformBrowser, LogLevel: api.LogLevelSilent, TsconfigRaw: "{}",
		Stdin: &api.StdinOptions{Contents: "", Loader: api.LoaderJS, ResolveDir: h.root},
		Plugins: []api.Plugin{{Name: "resolve-helper", Setup: func(build api.PluginBuild) {
			build.OnStart(func() (api.OnStartResult, error) {
				resolved = build.Resolve(specifier, api.ResolveOptions{Kind: resolveKind, ResolveDir: directory})
				return api.OnStartResult{Errors: resolved.Errors, Warnings: resolved.Warnings}, nil
			})
		}}},
	})
	if len(result.Errors) > 0 {
		return helperEntry{}, fmt.Errorf("%s helper in %s: %w", kind, origin, bundleError(result.Errors))
	}
	if resolved.External || resolved.Namespace != "file" || resolved.Path == "" || resolved.Suffix != "" {
		return helperEntry{}, fmt.Errorf("%s helper must resolve to one local source file without URL query/fragment suffixes", kind)
	}
	path, err := h.project.Path(resolved.Path)
	if err != nil {
		return helperEntry{}, err
	}
	return h.register(key, kind, origin, path)
}

// register validates the resolved source type and remembers the helper.
func (h *helperRegistry) register(key, kind, origin, path string) (helperEntry, error) {
	loader, code := sourceLoader(path)
	if !code || (kind == "style" && loader != api.LoaderCSS && loader != api.LoaderLocalCSS) || (kind == "script" && (loader == api.LoaderCSS || loader == api.LoaderLocalCSS || loader == api.LoaderJSON)) {
		return helperEntry{}, fmt.Errorf("%s helper resolved to an incompatible source type: %s", kind, filepath.Ext(path))
	}
	if kind == "style" && loader == api.LoaderLocalCSS {
		return helperEntry{}, fmt.Errorf("import .module.css from JavaScript to use its generated class names; style helper expects global CSS")
	}
	entry := helperEntry{kind: kind, path: path, origin: origin}
	h.resolved[key] = entry
	return entry, nil
}
