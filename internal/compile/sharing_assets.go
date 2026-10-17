package compile

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/htmlx"
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
