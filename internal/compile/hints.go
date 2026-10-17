package compile

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/urlcheck"
	"golang.org/x/net/html"
)

func dormant(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Data == "template" {
			return true
		}
	}
	return false
}

func (b *builder) resourceHints(doc *html.Node) error {
	var head *html.Node
	var hints []*html.Node
	var modules []*html.Node
	walkHTML(doc, func(n *html.Node) {
		if n.Namespace != "" || dormant(n) {
			return
		}
		if n.Data == "head" {
			head = n
		}
		if n.Data == "link" && (relToken(n, "preload") || relToken(n, "modulepreload") || relToken(n, "prefetch")) {
			hints = append(hints, n)
		}
		if n.Data == "script" && attr(n, "type") == "module" && attr(n, "src") != "" && !hasAttr(n, "async") {
			modules = append(modules, n)
		}
	})
	seen := map[string]bool{}
	for _, n := range hints {
		ref := attr(n, "href")
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
			setAttr(n, "href", u.String())
		}
		if relToken(n, "preload") && strings.EqualFold(attr(n, "as"), "font") {
			changed := false
			// CSS font fetching always uses anonymous CORS, even same-origin.
			// Preserve authored credential modes; diagnose incompatibility later.
			if !hasAttr(n, "crossorigin") {
				setAttr(n, "crossorigin", "")
				changed = true
			}
			if info, ok := b.fonts[attr(n, "href")]; ok && info.Issue == "" && info.Unknown == "" && !hasAttr(n, "type") {
				if mime := fontcheck.MIME(info.Format); mime != "" {
					setAttr(n, "type", mime)
					changed = true
				}
			}
			if changed {
				b.fontHints++
			}
		}
		if attr(n, "imagesrcset") != "" {
			var candidates []string
			valid := true
			for _, candidate := range urlcheck.Candidates(attr(n, "imagesrcset")) {
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
			setAttr(n, "imagesrcset", strings.Join(candidates, ", "))
		}
		for _, module := range modules {
			if attr(module, "src") == attr(n, "href") && relToken(n, "preload") && attr(n, "as") == "script" {
				setAttr(n, "rel", "modulepreload")
				delAttr(n, "as")
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
		ref := attr(module, "src")
		u, err := url.Parse(ref)
		if err != nil || u.IsAbs() || !strings.HasPrefix(ref, "./") || strings.EqualFold(attr(module, "fetchpriority"), "low") {
			continue
		}
		if _, generated := b.files[strings.ToLower(filepath.Base(u.Path))]; !generated {
			continue
		}
		n := element("link", "")
		setAttr(n, "rel", "modulepreload")
		setAttr(n, "href", ref)
		if hasAttr(module, "crossorigin") {
			setAttr(n, "crossorigin", attr(module, "crossorigin"))
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
		value := attr(n, key)
		if key == "crossorigin" && hasAttr(n, key) && !strings.EqualFold(value, "use-credentials") {
			value = "anonymous"
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "\x00")
}
