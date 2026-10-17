package compile

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/imaging"
	"github.com/Anonyfox/lfm/internal/urlcheck"
	"golang.org/x/net/html"
)

type imageInput struct {
	node        *html.Node
	src, srcset string
	candidates  []string
}

func captureImages(doc *html.Node) []imageInput {
	var inputs []imageInput
	walkHTML(doc, func(n *html.Node) {
		if n.Data != "img" || n.Namespace != "" {
			return
		}
		input := imageInput{node: n, src: attr(n, "src"), srcset: attr(n, "srcset")}
		for _, candidate := range urlcheck.Candidates(input.srcset) {
			input.candidates = append(input.candidates, candidate.URL)
		}
		if n.Parent != nil && n.Parent.Data == "picture" {
			for sibling := n.Parent.FirstChild; sibling != nil; sibling = sibling.NextSibling {
				if sibling.Data == "source" {
					for _, candidate := range urlcheck.Candidates(attr(sibling, "srcset")) {
						input.candidates = append(input.candidates, candidate.URL)
					}
				}
			}
		}
		inputs = append(inputs, input)
	})
	return inputs
}

func (b *builder) prepareImages(doc *html.Node, inputs []imageInput) []imaging.Decision {
	if len(inputs) == 0 {
		return nil
	}
	policy := b.imageSizing(doc)
	// Reuse bytes the asset pipeline already read. Do not read/decode every
	// occurrence, allocate pixels, or inspect unused imported images.
	sources := map[string][]byte{}
	for path, a := range b.packed {
		sources[path] = a.Bytes
	}
	for _, f := range b.files {
		sources[f.origin] = f.contents
	}
	cache := map[string]imaging.Info{}
	inspect := func(ref string) imaging.Info {
		key, name := ref, "inline image"
		var raw []byte
		if strings.HasPrefix(ref, "data:") {
			hash := sha256.Sum256([]byte(ref))
			key = string(hash[:])
			if info, ok := cache[key]; ok {
				return info
			}
			base, _, _ := strings.Cut(ref, "#")
			header, payload, ok := strings.Cut(base, ",")
			if ok && len(payload) <= 2_000_000 {
				if decoded, err := url.PathUnescape(payload); err == nil {
					if strings.HasSuffix(strings.ToLower(header), ";base64") {
						if data, err := base64.StdEncoding.DecodeString(decoded); err == nil {
							raw = data
						}
					} else {
						raw = []byte(decoded)
					}
				}
			}
		} else {
			u, err := url.Parse(ref)
			if err != nil || u.IsAbs() || strings.HasPrefix(ref, "/") || ref == "" || strings.HasPrefix(ref, "#") {
				return imaging.Info{Source: "external or dynamic image", Unknown: "remote/dynamic images are not fetched by builds"}
			}
			path, u, err := b.assetPath(ref, b.root)
			if err != nil {
				return imaging.Info{Source: "unresolved image", Unknown: "source path could not be inspected"}
			}
			key = path
			rel, _ := filepath.Rel(b.root, path)
			name = filepath.ToSlash(rel)
			if info, ok := cache[key]; ok {
				if u.Fragment != "" && info.Format == "svg" {
					info.Width, info.Height = 0, 0
					info.Unknown = "SVG URL fragments may change the intrinsic view"
				}
				return info
			}
			raw = sources[path]
		}
		info := imaging.Inspect(raw)
		info.Source = name
		cache[key] = info
		if u, err := url.Parse(ref); err == nil && u.Fragment != "" && info.Format == "svg" {
			info.Width, info.Height = 0, 0
			info.Unknown = "SVG URL fragments may change the intrinsic view"
		}
		return info
	}
	var decisions []imaging.Decision
	for _, input := range inputs {
		n := input.node
		info := inspect(input.src)
		d := imaging.Decision{Info: info, AuthoredWidth: imaging.PositiveAttribute(attr(n, "width")), AuthoredHeight: imaging.PositiveAttribute(attr(n, "height"))}
		cssReason := policy.reason(n)
		responsive := hasAttr(n, "srcset") || hasAttr(n, "sizes") || n.Parent != nil && n.Parent.Data == "picture"
		d.CompareSize = !responsive && cssReason == "" && info.Unknown == "" && info.Issue == "" && policy.attributeBox(n) && !dormantImage(n)
		switch {
		case hasAttr(n, "width") && hasAttr(n, "height"):
			d.Reason = "authored width/height preserved"
		case dormantImage(n):
			d.Reason = "dormant template sizing preserved"
		case responsive:
			d.Reason = "responsive sources/sizes may use different dimensions or pixel densities"
		case cssReason != "":
			d.Reason = cssReason
		case info.Issue != "":
			d.Reason = info.Issue
		case info.Unknown != "" || info.Width <= 0 || info.Height <= 0:
			d.Reason = info.Unknown
		case hasAttr(n, "width") && d.AuthoredWidth == 0 || hasAttr(n, "height") && d.AuthoredHeight == 0:
			d.Reason = "explicit empty/zero/relative/malformed sizing preserved"
		default:
			width, height := info.Width, info.Height
			if hasAttr(n, "width") {
				width = d.AuthoredWidth
				scaled := int64(width) * int64(info.Height)
				if scaled%int64(info.Width) != 0 || scaled/int64(info.Width) > 1<<30 {
					d.Reason = "missing height requires fractional or excessive rounding"
					break
				}
				height = int(scaled / int64(info.Width))
			} else if hasAttr(n, "height") {
				height = d.AuthoredHeight
				scaled := int64(height) * int64(info.Width)
				if scaled%int64(info.Height) != 0 || scaled/int64(info.Height) > 1<<30 {
					d.Reason = "missing width requires fractional or excessive rounding"
					break
				}
				width = int(scaled / int64(info.Height))
			}
			if !hasAttr(n, "width") {
				setAttr(n, "width", strconv.Itoa(width))
				d.Added = append(d.Added, "width")
			}
			if !hasAttr(n, "height") {
				setAttr(n, "height", strconv.Itoa(height))
				d.Added = append(d.Added, "height")
			}
			d.Reason = "known intrinsic dimensions; missing attributes inferred without changing authored values"
		}
		// Candidate assets are inspected for absolute byte/pixel budgets only.
		// Never treat a 2x candidate as twice the intended CSS display size.
		for _, candidate := range input.candidates {
			d.Candidates = append(d.Candidates, inspect(candidate))
		}
		decisions = append(decisions, d)
	}
	// The per-image decision list stays aligned with final HTML img order.
	return decisions
}
