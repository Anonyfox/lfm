package compile

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/urlcheck"
	"golang.org/x/net/html"
)

func (b *builder) resourceHints(doc *html.Node) error {
	var head *html.Node
	var hints []*html.Node
	var modules []*html.Node
	htmlx.Walk(doc, func(n *html.Node) {
		if n.Namespace != "" || htmlx.Dormant(n) {
			return
		}
		if n.Data == "head" {
			head = n
		}
		if n.Data == "link" && (htmlx.Rel(n, "preload") || htmlx.Rel(n, "modulepreload") || htmlx.Rel(n, "prefetch")) {
			hints = append(hints, n)
		}
		if n.Data == "script" && htmlx.Attr(n, "type") == "module" && htmlx.Attr(n, "src") != "" && !htmlx.HasAttr(n, "async") {
			modules = append(modules, n)
		}
	})
	seen := map[string]bool{}
	for _, n := range hints {
		ref := htmlx.Attr(n, "href")
		if strings.HasPrefix(ref, "data:") {
			n.Parent.RemoveChild(n)
			continue
		}
		remote, err := b.remote(ref)
		if err != nil {
			return err
		}
		if !remote {
			path, original, err := b.assetPath(ref, b.root)
			if err != nil {
				return fmt.Errorf("resource hint %q: %w", ref, err)
			}
			resolved, used := b.references[path]
			if !used || resolved == "" || strings.HasPrefix(resolved, "data:") {
				n.Parent.RemoveChild(n)
				if !used {
					b.notice("assets.hint", "Removed unused local resource hint: "+ref, "Hints are reconciled with resources actually retained by bundling; import/reference the resource where it is consumed.")
				}
				continue
			}
			u, _ := url.Parse(resolved)
			if original.RawQuery != "" {
				u.RawQuery = original.RawQuery
			}
			htmlx.SetAttr(n, "href", u.String())
		}
		if htmlx.Rel(n, "preload") && strings.EqualFold(htmlx.Attr(n, "as"), "font") {
			changed := false
			// CSS font fetching always uses anonymous CORS, even same-origin.
			// Preserve authored credential modes; diagnose incompatibility later.
			if !htmlx.HasAttr(n, "crossorigin") {
				htmlx.SetAttr(n, "crossorigin", "")
				changed = true
			}
			if info, ok := b.fonts[htmlx.Attr(n, "href")]; ok && info.Issue == "" && info.Unknown == "" && !htmlx.HasAttr(n, "type") {
				if mime := fontcheck.MIME(info.Format); mime != "" {
					htmlx.SetAttr(n, "type", mime)
					changed = true
				}
			}
			if changed {
				b.fontHints++
			}
		}
		if htmlx.Attr(n, "imagesrcset") != "" {
			var candidates []string
			valid := true
			for _, candidate := range urlcheck.Candidates(htmlx.Attr(n, "imagesrcset")) {
				ref := candidate.URL
				if remote, err := b.remote(ref); err != nil {
					return err
				} else if !remote {
					path, _, err := b.assetPath(ref, b.root)
					if err != nil {
						return err
					}
					resolved, ok := b.references[path]
					if !ok || resolved == "" || strings.HasPrefix(resolved, "data:") {
						valid = false
						break
					}
					ref = resolved
				}
				if candidate.Descriptor != "" {
					ref += " " + candidate.Descriptor
				}
				candidates = append(candidates, ref)
			}
			if !valid {
				n.Parent.RemoveChild(n)
				continue
			}
			htmlx.SetAttr(n, "imagesrcset", strings.Join(candidates, ", "))
		}
		for _, module := range modules {
			if htmlx.Attr(module, "src") == htmlx.Attr(n, "href") && htmlx.Rel(n, "preload") && htmlx.Attr(n, "as") == "script" {
				htmlx.SetAttr(n, "rel", "modulepreload")
				htmlx.DelAttr(n, "as")
			}
		}
		key := hintKey(n)
		if seen[key] {
			n.Parent.RemoveChild(n)
		} else {
			seen[key] = true
		}
	}
	// At most one automatic hint: a generated body-end startup module is
	// otherwise discovered late. CSS already in head needs no duplicate preload.
	for _, module := range modules {
		ref := htmlx.Attr(module, "src")
		u, err := url.Parse(ref)
		if err != nil || u.IsAbs() || !strings.HasPrefix(ref, "./") || strings.EqualFold(htmlx.Attr(module, "fetchpriority"), "low") {
			continue
		}
		if _, generated := b.files[strings.ToLower(filepath.Base(u.Path))]; !generated {
			continue
		}
		n := htmlx.Element("link", "")
		htmlx.SetAttr(n, "rel", "modulepreload")
		htmlx.SetAttr(n, "href", ref)
		if htmlx.HasAttr(module, "crossorigin") {
			htmlx.SetAttr(n, "crossorigin", htmlx.Attr(module, "crossorigin"))
		}
		if !seen[hintKey(n)] && head != nil {
			head.AppendChild(n)
		}
		break
	}
	return nil
}

func hintKey(n *html.Node) string {
	var parts []string
	for _, key := range []string{"rel", "href", "as", "type", "media", "imagesrcset", "imagesizes", "crossorigin"} {
		value := htmlx.Attr(n, key)
		if key == "crossorigin" && htmlx.HasAttr(n, key) && !strings.EqualFold(value, "use-credentials") {
			value = "anonymous"
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "\x00")
}
