package check

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfm/internal/imaging"
	"golang.org/x/net/html"
)

// OG structured properties attach to the preceding media root, not every image.
// Missing hints are advisory; deliberately empty alternative text stays authored.
func sharingMediaRule(d *Document) []Diagnostic {
	type media struct {
		root   *html.Node
		fields map[string]*html.Node
	}
	var images []*media
	var current *media
	for _, n := range d.Elements {
		if n.Namespace != "" || n.Data != "meta" || inTemplate(n) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(Attr(n, "property")))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(Attr(n, "name")))
		}
		if key == "og:image" || key == "og:image:url" {
			current = &media{root: n, fields: map[string]*html.Node{}}
			images = append(images, current)
		} else if current != nil && (key == "og:image:alt" || key == "og:image:width" || key == "og:image:height") {
			current.fields[strings.TrimPrefix(key, "og:image:")] = n
		}
	}
	var issues []Diagnostic
	for _, image := range images {
		var missing []string
		for _, key := range []string{"alt", "width", "height"} {
			if image.fields[key] == nil {
				missing = append(missing, "og:image:"+key)
			}
		}
		if len(missing) > 0 {
			issues = append(issues, d.At(image.root, Warning, "Social image is missing "+strings.Join(missing, ", ")+".", "Place the actual pixel dimensions and useful alternative text after this og:image root; each image owns its descriptors. Deliberate empty alt is preserved."))
		}
		value := Attr(image.root, "content")
		if raw, _, ok := sharingAsset(d, value); ok {
			info := imaging.Inspect(raw)
			if info.Issue == "" && info.Unknown == "" && info.Width > 0 && info.Height > 0 {
				for _, axis := range []struct {
					key  string
					size int
				}{{"width", info.Width}, {"height", info.Height}} {
					if n := image.fields[axis.key]; n != nil {
						declared := imaging.PositiveAttribute(Attr(n, "content"))
						if declared > 0 && declared != axis.size {
							issues = append(issues, d.At(n, Warning, fmt.Sprintf("og:image:%s declares %d; emitted image is %d px on that axis.", axis.key, declared, axis.size), "Use the emitted image's actual intrinsic dimensions; this metadata does not resize or crop it."))
						}
					}
				}
			}
		}
	}
	if nodes := platformMeta(d, "twitter:image"); len(nodes) > 0 && len(platformMeta(d, "twitter:image:alt")) == 0 {
		issues = append(issues, d.At(nodes[0], Warning, "twitter:image has no twitter:image:alt.", "Describe the poster/result usefully for recipients who cannot see it; retain an intentionally empty value only when appropriate."))
	}
	return issues
}
