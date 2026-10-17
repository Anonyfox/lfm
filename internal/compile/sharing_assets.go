package compile

import (
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/imaging"
	"golang.org/x/net/html"
)

// Structured metadata is inert data, not executable JS. Resolve only known media
// fields, leaving navigation/state targets and unknown fields intact. Invalid or
// ambiguous payloads stay byte-for-byte authored for the aggregate final gate.
func (b *builder) sharingAssets(n *html.Node) error {
	field := "content"
	kind := strings.ToLower(strings.TrimSpace(htmlx.Attr(n, "name")))
	if kind == "" {
		kind = strings.ToLower(strings.TrimSpace(htmlx.Attr(n, "property")))
	}
	if n.Data == "script" && htmlx.Attr(n, "id") == "discord:component-embed" {
		field, kind = "", "discord"
	}
	if n.Namespace != "" || kind != "fc:miniapp" && kind != "fc:frame" && kind != "discord" {
		return nil
	}
	raw := htmlx.Attr(n, field)
	if field == "" {
		raw = htmlx.RawText(n)
	}
	object, err := check.DecodeSharingJSON([]byte(raw))
	if err != nil {
		return nil
	}
	changed := false
	pack := func(object map[string]any, key string) error {
		value, ok := object[key].(string)
		if !ok || value == "" {
			return nil
		}
		remote, err := b.remote(value)
		if err != nil || remote {
			return nil // platform URL errors belong in the final aggregate report
		}
		ref, err := b.resource(value, b.root, true)
		if err != nil {
			return err
		}
		base, _ := url.Parse(b.config.URL)
		u, err := url.Parse(ref)
		if err != nil {
			return err
		}
		object[key] = base.ResolveReference(u).String()
		changed = true
		return nil
	}
	if kind == "discord" {
		var visit func(any) error
		visit = func(value any) error {
			switch value := value.(type) {
			case map[string]any:
				if media, ok := value["media"].(map[string]any); ok {
					if err := pack(media, "url"); err != nil {
						return err
					}
				}
				// Only actual component edges, not arbitrary unknown config objects.
				for _, key := range []string{"component", "components", "accessory", "items"} {
					if err := visit(value[key]); err != nil {
						return err
					}
				}
			case []any:
				for _, item := range value {
					if err := visit(item); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := visit(object); err != nil {
			return err
		}
	} else {
		if err := pack(object, "imageUrl"); err != nil {
			return err
		}
		button, _ := object["button"].(map[string]any)
		action, _ := button["action"].(map[string]any)
		if err := pack(action, "splashImageUrl"); err != nil {
			return err
		}
	}
	if changed {
		encoded, err := json.Marshal(object)
		if err != nil {
			return err
		}
		if field == "" {
			htmlx.SetText(n, string(encoded))
		} else {
			htmlx.SetAttr(n, field, string(encoded))
		}
	}
	return nil
}

// socialImageSizes adds og:image:width and og:image:height to every local
// Open Graph image that lacks them, from the emitted file's real dimensions.
// Platforms use them to lay out a preview before fetching the image; authored
// values are never changed (a mismatch is reported by the sharing checks).
func (b *builder) socialImageSizes(doc *html.Node) {
	type group struct {
		root   *html.Node
		fields map[string]bool
	}
	var groups []*group
	var current *group
	htmlx.Walk(doc, func(n *html.Node) {
		if n.Namespace != "" || n.Data != "meta" || htmlx.Dormant(n) {
			return
		}
		key := strings.ToLower(strings.TrimSpace(htmlx.Attr(n, "property")))
		switch key {
		case "og:image", "og:image:url":
			current = &group{root: n, fields: map[string]bool{}}
			groups = append(groups, current)
		case "og:image:width", "og:image:height":
			if current != nil {
				current.fields[key] = true
			}
		}
	})
	for _, g := range groups {
		content := htmlx.Attr(g.root, "content")
		u, err := url.Parse(content)
		if err != nil || !strings.HasPrefix(content, b.config.URL) {
			continue // only images this compile emitted sit under the page URL
		}
		file, ok := b.files[strings.ToLower(path.Base(u.Path))]
		if !ok {
			continue
		}
		info := imaging.Inspect(file.contents)
		if info.Issue != "" || info.Unknown != "" || info.Width <= 0 || info.Height <= 0 {
			continue
		}
		after := g.root
		for _, axis := range []struct {
			key  string
			size int
		}{{"og:image:width", info.Width}, {"og:image:height", info.Height}} {
			if g.fields[axis.key] {
				continue
			}
			meta := htmlx.Element("meta", "")
			htmlx.SetAttr(meta, "property", axis.key)
			htmlx.SetAttr(meta, "content", strconv.Itoa(axis.size))
			after.Parent.InsertBefore(meta, after.NextSibling)
			after = meta
		}
	}
}
