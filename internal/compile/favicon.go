package compile

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"strings"

	"golang.org/x/net/html"
)

// A tiny file-backed PNG avoids data-URL browser/search-icon compatibility issues
// and a root /favicon.ico route. Authored icons (including dynamic IDs/media variants)
// always win. The compiler supplies a neutral fallback, not an invented brand.
func (b *builder) ensureFavicon(doc *html.Node) error {
	var head *html.Node
	var found bool
	walkHTML(doc, func(n *html.Node) {
		if n.Namespace != "" {
			return
		}
		if n.Data == "head" {
			head = n
		}
		if n.Data == "link" && n.Parent == head && relToken(n, "icon") && strings.TrimSpace(attr(n, "href")) != "" {
			found = true
		}
	})
	if found || head == nil {
		return nil
	}
	hash := sha256.Sum256([]byte(b.config.URL))
	icon := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	foreground := color.NRGBA{32 + hash[0]%128, 32 + hash[1]%128, 32 + hash[2]%128, 255}
	background := color.NRGBA{245, 245, 245, 255}
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			c := background
			if x >= 18 && x < 78 && y >= 18 && y < 78 {
				col, row := (x-18)/12, (y-18)/12
				if col > 2 {
					col = 4 - col
				}
				if hash[3+row*3+col]&1 != 0 {
					c = foreground
				}
			}
			icon.SetNRGBA(x, y, c)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, icon); err != nil {
		return err
	}
	name := b.autoAssetName("favicon.png", "generated-favicon", data.Bytes())
	if err := b.emit(name, "generated-favicon", data.Bytes()); err != nil {
		return err
	}
	n := element("link", "")
	setAttr(n, "rel", "icon")
	setAttr(n, "type", "image/png")
	setAttr(n, "sizes", "96x96")
	setAttr(n, "href", fileURL(name))
	head.AppendChild(n)
	return nil
}

func relToken(n *html.Node, token string) bool {
	for _, value := range strings.Fields(strings.ToLower(attr(n, "rel"))) {
		if value == token {
			return true
		}
	}
	return false
}
