package compile

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/release"
)

// bookImages renders EPUB renditions; the book keeps its original print assets.
func bookImages(r *book.Report, st *stock, at func(diagnostic.Severity, string, string, string)) map[string][]byte {
	out := map[string][]byte{}
	warnedSVG := false
	for rel, data := range r.Sources {
		if !strings.EqualFold(filepath.Ext(rel), ".svg") {
			continue
		}
		w, h, err := book.SVGSize(data)
		if err != nil {
			continue
		}
		height := int64(math.Ceil(1600 * h / w))
		doc := []byte(fmt.Sprintf(`<html><style>html,body{margin:0;background:white}img{width:1600px;height:%dpx;display:block}</style><img src="image.svg"></html>`, height))
		key := printKey("book-svg-1600", doc, map[string][]byte{"image.svg": data})
		var png []byte
		if release.ReadCache(key, &png).IsZero() {
			printer, e := st.printerFor()
			if e == nil {
				png, e = printer.Picture(doc, map[string][]byte{"image.svg": data}, 1600, height, 1)
			}
			if e == nil {
				release.WriteCache(key, png)
			}
		}
		if len(png) > 0 {
			out[rel] = png
		} else if !warnedSVG {
			warnedSVG = true
			at(diagnostic.Warning, book.RuleImages, "The EPUB keeps SVG, which Kindle's Enhanced Typesetting does not support.", "Install Chrome or set LFM_CHROME so lfm can rasterise SVG images.")
		}
	}

	return out
}
