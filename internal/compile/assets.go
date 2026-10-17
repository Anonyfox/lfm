package compile

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/urlcheck"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

func (b *builder) remote(ref string) (bool, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return false, err
	}
	if ref == "" || strings.HasPrefix(ref, "#") || u.Scheme == "data" || u.Scheme == "blob" {
		return true, nil
	}
	if u.Scheme != "" && u.Scheme != "https" && u.Scheme != "http" {
		return false, fmt.Errorf("unsupported asset URL %q; use a local assets/... path or HTTP(S) URL", ref)
	}
	if u.IsAbs() || strings.HasPrefix(ref, "/") {
		b.mu.Lock()
		b.external[ref] = true
		b.mu.Unlock()
		return true, nil
	}
	return false, nil
}

func (b *builder) assetPath(ref, directory string) (string, *url.URL, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return "", nil, err
	}
	path, err := sourcePath(b.root, filepath.Join(directory, filepath.FromSlash(u.Path)))
	if err != nil {
		return "", nil, err
	}
	return path, u, nil
}

func (b *builder) mode(path string) string {
	rel, _ := filepath.Rel(b.root, path)
	if override := b.config.Build.Assets.Overrides[filepath.ToSlash(rel)]; override != "" {
		return override
	}
	return b.config.Build.Assets.Default
}

func fileURL(name string) string {
	return "./" + (&url.URL{Path: name}).EscapedPath()
}

func (b *builder) resource(ref, directory string, forceFile bool) (string, error) {
	remote, err := b.remote(ref)
	if err != nil || remote {
		return ref, err
	}
	path, u, err := b.assetPath(ref, directory)
	if err != nil {
		return "", err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	b.recordInput(path)
	// Query-bearing references retain their native file URL identity. A data URL
	// cannot represent the resource's query semantics.
	forceFile = forceFile || u.RawQuery != ""
	reason := b.resourcePolicy(path, int64(len(contents)))
	var result string
	if forceFile || reason != "" {
		if forceFile {
			reason = "required native/social file URL"
		}
		b.rememberFile(path, reason)
		name := b.resourceName(path)
		if err := b.emit(name, path, contents); err != nil {
			return "", err
		}
		result = fileURL(name)
		if u.RawQuery != "" {
			result += "?" + u.RawQuery
		}
	} else {
		kind := httpMIME(contents, path)
		b.mu.Lock()
		cached, exists := b.packed[path]
		b.mu.Unlock()
		encoding := cached.Encoding
		if exists && bytes.Equal(cached.Bytes, contents) {
			result = cached.URI
		} else {
			result, encoding = inlineData(contents, kind)
		}
		b.recordInline(result, path)
		b.mu.Lock()
		if b.packed == nil {
			b.packed = map[string]packedAsset{}
		}
		b.packed[path] = packedAsset{URI: result, Bytes: contents, Encoding: encoding}
		b.mu.Unlock()
	}
	if u.Fragment != "" {
		result += "#" + u.EscapedFragment()
	}
	b.mu.Lock()
	if b.references == nil {
		b.references = map[string]string{}
	}
	b.references[path] = result
	b.mu.Unlock()
	return result, nil
}

func httpMIME(contents []byte, path string) string {
	if kind := fontcheck.Kind(contents); kind != "" {
		if i := fontcheck.Inspect(contents); i.Issue == "" && i.Unknown == "" {
			return fontcheck.MIME(kind)
		}
	}
	kind := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if kind == "" {
		kind = http.DetectContentType(contents)
	}
	return strings.Split(kind, ";")[0]
}

type compiled struct {
	code, css  []byte
	module     bool
	cssModules bool
}

func (b *builder) recordInline(ref, path string) {
	rel, _ := filepath.Rel(b.root, path)
	b.mu.Lock()
	key := sha256.Sum256([]byte(ref))
	pathName := filepath.ToSlash(rel)
	if !slices.Contains(b.inlineSources[key], pathName) {
		b.inlineSources[key] = append(b.inlineSources[key], pathName)
		slices.Sort(b.inlineSources[key])
	}
	b.mu.Unlock()
}

// Plain classic scripts retain globals. ESM/CommonJS entry syntax is detected by
// esbuild's metadata and automatically uses a browser module bundle instead.
func (b *builder) compile(path, source string, css, module bool) (compiled, error) {
	ext, loader := ".js", api.LoaderJS
	if css {
		ext, loader = ".css", api.LoaderCSS
	}
	options := b.bundleOptions(ext)
	if !css && !module {
		options.MinifyIdentifiers = false
		options.TreeShaking = api.TreeShakingFalse
	}
	if path != "" {
		options.EntryPoints = []string{path}
	} else {
		options.Stdin = &api.StdinOptions{Contents: source, ResolveDir: b.root, Sourcefile: "index.html.tmpl" + ext, Loader: loader}
	}
	result := api.Build(options)
	if len(result.Errors) != 0 {
		return compiled{}, bundleError(result.Errors)
	}
	if !css && !module {
		type moduleInfo struct {
			Format string `json:"format"`
		}
		var metadata struct {
			Inputs map[string]moduleInfo `json:"inputs"`
		}
		if err := json.Unmarshal([]byte(result.Metafile), &metadata); err != nil {
			return compiled{}, err
		}
		entry := "index.html.tmpl.js"
		if path != "" {
			rel, _ := filepath.Rel(b.root, path)
			entry = filepath.ToSlash(rel)
		}
		if format := metadata.Inputs[entry].Format; format == "esm" || format == "cjs" {
			// The shared page graph owns final output and sidecars.
			return compiled{module: true}, nil
		}
	}
	return b.bundleResult(result, ext, module)
}

func (b *builder) bundleOptions(ext string) api.BuildOptions {
	return api.BuildOptions{
		AbsWorkingDir: b.root, Outfile: filepath.Join(b.stage, "bundle"+ext), Write: false, Bundle: true,
		Platform: api.PlatformBrowser, Target: api.ES2020, Charset: api.CharsetUTF8, Format: api.FormatESModule,
		MinifyWhitespace: true, MinifySyntax: true, MinifyIdentifiers: true, TreeShaking: api.TreeShakingTrue,
		LegalComments: api.LegalCommentsInline, LogLevel: api.LogLevelSilent, Metafile: true, TsconfigRaw: "{}",
		Plugins: []api.Plugin{b.assetPlugin()}, AssetNames: "[name]", PublicPath: "./",
	}
}

func bundleError(messages []api.Message) error {
	message := strings.Join(api.FormatMessages(messages, api.FormatMessagesOptions{Kind: api.ErrorMessage}), "\n")
	for _, item := range messages {
		if strings.Contains(item.Text, "Could not resolve") {
			message += "\nPackage imports need dependencies installed in this workbench's node_modules/ (npm ci with a lockfile, or npm install during setup). Builds never install packages; Node-only packages need a browser-compatible alternative."
			break
		}
	}
	return fmt.Errorf("%s", message)
}

func (b *builder) bundleResult(result api.BuildResult, ext string, module bool) (compiled, error) {
	b.mu.Lock()
	b.warnings = append(b.warnings, api.FormatMessages(result.Warnings, api.FormatMessagesOptions{Kind: api.WarningMessage})...)
	b.mu.Unlock()
	var metadata struct {
		Outputs map[string]struct {
			Inputs map[string]any `json:"inputs"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(result.Metafile), &metadata); err != nil {
		return compiled{}, err
	}
	output := compiled{module: module}
	for _, file := range result.OutputFiles {
		if file.Path == filepath.Join(b.stage, "bundle"+ext) {
			output.code = file.Contents
			continue
		}
		if ext == ".js" && file.Path == filepath.Join(b.stage, "bundle.css") {
			output.css = file.Contents
			rel, _ := filepath.Rel(b.root, file.Path)
			for input := range metadata.Outputs[filepath.ToSlash(rel)].Inputs {
				if loader, code := sourceLoader(input); code && loader == api.LoaderLocalCSS {
					output.cssModules = true
				}
			}
			continue
		}
		rel, _ := filepath.Rel(b.root, file.Path)
		info := metadata.Outputs[filepath.ToSlash(rel)]
		if len(info.Inputs) == 0 {
			return compiled{}, fmt.Errorf("unexpected esbuild output %s", file.Path)
		}
		var path string
		for key := range info.Inputs {
			b.mu.Lock()
			origin := b.assetOrigins[key]
			b.mu.Unlock()
			if origin == "" {
				origin = key
			}
			if !filepath.IsAbs(origin) {
				origin = filepath.Join(b.root, filepath.FromSlash(origin))
			}
			real, err := sourcePath(b.root, origin)
			if err != nil {
				return compiled{}, err
			}
			if path != "" && path != real {
				return compiled{}, fmt.Errorf("output filename collision %q between imported asset sources; rename one source asset", filepath.Base(real))
			}
			path = real
		}
		// The virtual asset filename is URL-escaped for esbuild's references.
		// The real sidecar retains its original filesystem basename.
		if err := b.emit(b.resourceName(path), path, file.Contents); err != nil {
			return compiled{}, err
		}
		if b.references == nil {
			b.references = map[string]string{}
		}
		b.references[path] = fileURL(b.resourceName(path))
	}
	return output, nil
}

func (b *builder) assetPlugin() api.Plugin {
	return api.Plugin{Name: "workbench-assets", Setup: func(build api.PluginBuild) {
		build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			if marker, _ := args.PluginData.(string); marker == "lfm-default-resolve" {
				return api.OnResolveResult{}, nil
			}
			// esbuild entry points are absolute filesystem paths, not site-root URLs.
			if args.Kind == api.ResolveEntryPoint {
				return api.OnResolveResult{}, nil
			}
			// Browser maps can explicitly disable node: builtins. Delegate these
			// to esbuild; unsupported Node-only imports still fail for browser builds.
			if strings.HasPrefix(args.Path, "node:") {
				return api.OnResolveResult{}, nil
			}
			remote, err := b.remote(args.Path)
			if err != nil {
				return api.OnResolveResult{}, err
			}
			if remote {
				return api.OnResolveResult{Path: args.Path, External: true}, nil
			}
			if args.Kind == api.ResolveCSSURLToken {
				if compiled, _ := args.PluginData.(bool); compiled {
					return api.OnResolveResult{Path: args.Path, External: true}, nil
				}
				path, u, err := b.assetPath(args.Path, args.ResolveDir)
				if err != nil {
					return api.OnResolveResult{}, err
				}
				if b.resourceFile(path) {
					suffix := ""
					if u.RawQuery != "" {
						suffix = "?" + u.RawQuery
					}
					if u.Fragment != "" {
						suffix += "#" + u.EscapedFragment()
					}
					return b.nativeAsset(path, suffix)
				}
				ref, err := b.resource(args.Path, args.ResolveDir, false)
				return api.OnResolveResult{Path: ref, External: true}, err
			}
			if _, code := sourceLoader(args.Path); !code {
				// Delegate resolution (including browser/exports/sideEffects) to
				// esbuild before choosing an asset loader. This also handles bare
				// package names without guessing which file their metadata selects.
				resolved := build.Resolve(args.Path, api.ResolveOptions{Kind: args.Kind, Importer: args.Importer, Namespace: args.Namespace, ResolveDir: args.ResolveDir, PluginData: "lfm-default-resolve"})
				if len(resolved.Errors) > 0 {
					return api.OnResolveResult{Errors: resolved.Errors, Warnings: resolved.Warnings}, nil
				}
				if _, code := sourceLoader(resolved.Path); !resolved.External && resolved.Namespace == "file" && !code && b.resourceFile(resolved.Path) {
					return b.nativeAsset(resolved.Path, resolved.Suffix)
				}
				// Let the normal resolver retain its complete package metadata and
				// disabled-module semantics for every non-asset result.
				return api.OnResolveResult{}, nil
			}
			return api.OnResolveResult{}, nil
		})
		build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "resource-file"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
			actual, _ := args.PluginData.(string)
			path, err := sourcePath(b.root, actual)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			b.recordInput(path)
			raw, err := os.ReadFile(path)
			contents := string(raw)
			return api.OnLoadResult{Contents: &contents, Loader: api.LoaderFile}, err
		})
		build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "file"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
			path, err := sourcePath(b.root, args.Path)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			b.recordInput(path)
			loader, code := sourceLoader(path)
			if !code {
				if b.resourceFile(path) {
					raw, err := os.ReadFile(path)
					contents := string(raw)
					return api.OnLoadResult{Contents: &contents, Loader: api.LoaderFile}, err
				}
				ref, err := b.resource(fileURL(filepath.Base(path))+args.Suffix, filepath.Dir(path), false)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				encoded, _ := json.Marshal(ref)
				contents := "export default " + string(encoded)
				return api.OnLoadResult{Contents: &contents, Loader: api.LoaderJS}, nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			contents := string(raw)
			return api.OnLoadResult{Contents: &contents, Loader: loader, ResolveDir: filepath.Dir(path)}, nil
		})
	}}
}

func sourceLoader(path string) (api.Loader, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".mjs", ".cjs":
		return api.LoaderJS, true
	case ".ts", ".mts", ".cts":
		return api.LoaderTS, true
	case ".jsx":
		return api.LoaderJSX, true
	case ".tsx":
		return api.LoaderTSX, true
	case ".css":
		if strings.HasSuffix(strings.ToLower(path), ".module.css") {
			return api.LoaderLocalCSS, true
		}
		return api.LoaderCSS, true
	case ".json":
		return api.LoaderJSON, true
	}
	return api.LoaderNone, false
}

// Keep filesystem identity separate from URL spelling and optional URL suffixes.
// esbuild's file loader still decides whether the asset survives tree shaking.
func (b *builder) nativeAsset(path, suffix string) (api.OnResolveResult, error) {
	real, err := sourcePath(b.root, path)
	if err != nil {
		return api.OnResolveResult{}, err
	}
	virtual := filepath.Join(filepath.Dir(real), strings.TrimPrefix(fileURL(b.resourceName(real)), "./"))
	key := "resource-file:" + filepath.ToSlash(virtual) + suffix
	b.mu.Lock()
	if b.assetOrigins == nil {
		b.assetOrigins = map[string]string{}
	}
	b.assetOrigins[key] = real
	b.mu.Unlock()
	return api.OnResolveResult{Path: virtual, Namespace: "resource-file", Suffix: suffix, PluginData: real, SideEffects: api.SideEffectsFalse}, nil
}

// URL tokens may contain commas (notably data URLs); don't split srcset on commas.
func (b *builder) srcset(value string) (string, error) {
	var candidates []string
	for _, candidate := range urlcheck.Candidates(value) {
		resolved, err := b.resource(candidate.URL, b.root, false)
		if err != nil {
			return "", err
		}
		if candidate.Descriptor != "" {
			resolved += " " + candidate.Descriptor
		}
		candidates = append(candidates, resolved)
	}
	return strings.Join(candidates, ", "), nil
}
