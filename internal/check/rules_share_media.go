package check

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfm/internal/imaging"
	"golang.org/x/net/html"
)

// OG structured properties attach to the preceding media root, not every image.
// Missing hints are advisory; deliberately empty alternative text stays authored.
type socialImage struct {
	root   *html.Node
	fields map[string]*html.Node
	info   *imaging.Info
}

// dimensions caches the same emitted header for both sharing rules.
func (s *socialImage) dimensions(d *Document) *imaging.Info {
	if s.info == nil {
		if raw, _, ok := sharingAsset(d, Attr(s.root, "content")); ok {
			info := imaging.Inspect(raw)
			s.info = &info
		}
	}
	return s.info
}

func socialImages(d *Document) []*socialImage {
	if d.imagesScanned {
		return d.socialImages
	}
	d.imagesScanned = true
	var images []*socialImage
	var current *socialImage
	for _, n := range d.Elements {
		if n.Namespace != "" || n.Data != "meta" || d.inTemplate(n) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(Attr(n, "property")))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(Attr(n, "name")))
		}
		if key == "og:image" || key == "og:image:url" {
			current = &socialImage{root: n, fields: map[string]*html.Node{}}
			images = append(images, current)
		} else if current != nil && (key == "og:image:alt" || key == "og:image:width" || key == "og:image:height") {
			current.fields[strings.TrimPrefix(key, "og:image:")] = n
		}
	}
	d.socialImages = images
	return images
}

func sharingMediaRule(d *Document) []Diagnostic {
	images := socialImages(d)
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
		if info := image.dimensions(d); info != nil {
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

func sharingCardRule(d *Document) []Diagnostic {
	images := socialImages(d)
	if len(images) == 0 {
		return nil
	}
	primary := images[0]
	value := strings.TrimSpace(Attr(primary.root, "content"))
	if value == "" {
		return nil
	}
	var issues []Diagnostic
	switch strings.TrimSpace(metaValue(d, "twitter:card")) {
	case "app", "player":
		return nil
	case "summary_large_image":
	default:
		issues = append(issues, d.At(primary.root, Warning, "The Open Graph image has no matching large X card.", "Set twitter:card to summary_large_image."))
	}
	if nodes := platformMeta(d, "twitter:image"); len(nodes) > 0 {
		_, a := d.linkFacts()
		if a.Key(Attr(nodes[0], "content")) != a.Key(value) {
			issues = append(issues, d.At(nodes[0], Warning, "twitter:image differs from the primary og:image.", "Point both preview fields at the same image URL."))
		}
	}
	mismatch := false
	for key, size := range map[string]int{"width": 1200, "height": 630} {
		if n := primary.fields[key]; n != nil && imaging.PositiveAttribute(Attr(n, "content")) != size {
			mismatch = true
		}
	}
	if i := primary.dimensions(d); i != nil {
		if i.Width > 0 && i.Height > 0 && (i.Width != 1200 || i.Height != 630) {
			mismatch = true
		}
	}
	if mismatch {
		issues = append(issues, d.At(primary.root, Warning, "The primary og:image differs from the recommended size of 1200x630 pixels.", "Export a 1200x630 image and declare its actual dimensions."))
	}
	return issues
}

// PrimaryShareImage returns the authored URL for the folder reader's local asset lookup.
func (d *Document) PrimaryShareImage() string {
	images := socialImages(d)
	if len(images) == 0 {
		return ""
	}
	return strings.TrimSpace(Attr(images[0].root, "content"))
}

// InspectPrimaryShareImage supplies the local header found by a folder reader.
func (d *Document) InspectPrimaryShareImage(raw []byte) {
	if images := socialImages(d); len(images) > 0 {
		info := imaging.Inspect(raw)
		images[0].info = &info
	}
}
