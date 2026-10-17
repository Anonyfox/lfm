package compile

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/fontcheck"
	"golang.org/x/net/html"
)

func fontExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".woff2", ".woff", ".ttf", ".otf", ".ttc", ".otc", ".eot":
		return true
	}
	return false
}

// Inventory surviving local assets from bytes already loaded by bundling.
// Source paths are retained for diagnostics; data URLs are never printed.
func (b *builder) inspectFonts(doc *html.Node) (map[string]fontcheck.Info, []fontcheck.Info) {
	resources := map[string]fontcheck.Info{}
	bySource := map[string]fontcheck.Info{}
	fontRefs := map[string]bool{}
	readCSS := func(raw []byte) {
		faces, _ := fontcheck.Faces(raw)
		for _, f := range faces {
			for _, s := range f.Sources {
				if !s.Local {
					fontRefs[fontAssetKey(s.URL)] = true
				}
			}
		}
	}
	walkHTML(doc, func(n *html.Node) {
		if n.Data == "style" && n.Namespace == "" && !dormant(n) && (attr(n, "type") == "" || strings.EqualFold(attr(n, "type"), "text/css")) {
			readCSS([]byte(textContent(n)))
		}
	})
	for _, f := range b.files {
		if strings.HasSuffix(strings.ToLower(f.name), ".css") {
			readCSS(f.contents)
		}
	}
	add := func(path, ref string, raw []byte) {
		if ref == "" || !fontRefs[fontAssetKey(ref)] && !fontExtension(path) && fontcheck.Kind(raw) == "" {
			return
		}
		i := fontcheck.Inspect(raw)
		if !fontRefs[fontAssetKey(ref)] && !fontExtension(path) && (i.Issue != "" || i.Unknown != "") {
			return
		}
		rel, _ := filepath.Rel(b.root, path)
		i.Source = filepath.ToSlash(rel)
		remember := func(key string) {
			if old, ok := resources[key]; !ok || i.Source < old.Source {
				resources[key] = i
			}
		}
		remember(ref)
		base, _, _ := strings.Cut(ref, "#")
		remember(base)
		base, _, _ = strings.Cut(base, "?")
		remember(base)
		bySource[i.Source] = i
	}
	for path, a := range b.packed {
		add(path, b.references[path], a.Bytes)
	}
	for _, f := range b.files {
		add(f.origin, b.references[f.origin], f.contents)
	}
	var inventory []fontcheck.Info
	for _, i := range bySource {
		inventory = append(inventory, i)
	}
	sort.Slice(inventory, func(i, j int) bool { return inventory[i].Source < inventory[j].Source })
	return resources, inventory
}

func fontAssetKey(ref string) string {
	base, _, _ := strings.Cut(ref, "#")
	if !strings.HasPrefix(base, "data:") {
		base, _, _ = strings.Cut(base, "?")
	}
	return base
}

func (b *builder) cssAssets() map[string][]byte {
	result := map[string][]byte{}
	for _, f := range b.files {
		if strings.HasSuffix(strings.ToLower(f.name), ".css") {
			result[fileURL(f.name)] = f.contents
		}
	}
	return result
}
