package printable

import (
	"fmt"
	"html"
	"strings"

	"github.com/Anonyfox/lfm/internal/deliver"
)

// Listing images are 3000×2250 pixels (4:3), drawn at 1500×1125 CSS pixels
// and twice the density. The product sits in the central 900 CSS pixels, so
// Etsy's square and portrait thumbnail crops keep it whole.
const (
	ImageWidth  = 1500
	ImageHeight = 1125
	ImageScale  = 2
	// PageScale is the density page rasters are rendered at for the images.
	PageScale = 2
)

// Shot asks for one page raster of the first edition.
type Shot struct {
	Page int    // 1-based
	Name string // the asset name the image documents use, page-3.png
}

// ImagePlan is one listing image: its document and the page rasters it shows.
type ImagePlan struct {
	Name string
	Doc  []byte
}

// Shots lists the pages the listing images show: the cover, then the first
// and second page of every part.
func (r Report) Shots() []Shot {
	if len(r.Editions) == 0 {
		return nil
	}
	var out []Shot
	seen := map[int]bool{}
	add := func(n int) {
		if n >= 1 && n <= len(r.Editions[0].Pages) && !seen[n] {
			seen[n] = true
			out = append(out, Shot{Page: n, Name: fmt.Sprintf("page-%d.png", n)})
		}
	}
	add(1)
	for _, part := range r.partPages() {
		add(part.first)
		if part.count > 1 {
			add(part.first + 1)
		}
	}
	return out
}

type partSpan struct {
	Part
	first, count int
}

// partPages finds where each planned part starts in the first edition.
func (r Report) partPages() []partSpan {
	var out []partSpan
	for _, part := range r.Plan.Parts {
		span := partSpan{Part: part}
		for _, pg := range r.Editions[0].Pages {
			if strings.EqualFold(pg.Part, part.Name) {
				if span.first == 0 {
					span.first = pg.N
				}
				span.count++
			}
		}
		if span.first > 0 {
			out = append(out, span)
		}
	}
	return out
}

// Images plans the listing images in the order stores show them: the cover
// alone, everything included, each part, the sizes.
func (r Report) Images() []ImagePlan {
	if len(r.Editions) == 0 {
		return nil
	}
	w := wordsFor(r.Config.Language)
	accent := r.Plan.Accent
	if accent == "" {
		accent = "#444444"
	}
	ratio := r.Editions[0].Edition.Size.Width / r.Editions[0].Edition.Size.Height
	page := func(n int, height float64, extra string) string {
		return fmt.Sprintf(`<img class="pg" src="page-%d.png" style="height:%gpx;width:%gpx%s" alt="">`, n, height, height*ratio, extra)
	}
	doc := func(body string) []byte {
		return []byte(fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:%dpx;height:%dpx;overflow:hidden}
body{display:flex;align-items:center;justify-content:center;gap:36px;font:600 30px/1.25 system-ui,sans-serif;color:#1d1d1d;
background:radial-gradient(circle at 50%% 40%%,#ffffff 0,color-mix(in srgb,%s 10%%,#f4f2ee) 75%%)}
.pg{display:block;background:#fff;box-shadow:0 2px 6px rgba(0,0,0,.18),0 18px 40px rgba(0,0,0,.16)}
.col{display:flex;flex-direction:column;align-items:center;gap:22px}
.grid{display:grid;gap:30px 34px;align-items:end}
.cap{text-align:center;max-width:260px;font-size:22px}
.cap small{display:block;font-weight:400;color:#555;font-size:18px}
h2{margin:0;font-size:40px}
.sizes{display:flex;align-items:flex-end;gap:42px}
.paper{border:3px solid %s;background:#fff;box-shadow:0 18px 40px rgba(0,0,0,.14)}
.paper img{width:100%%;height:100%%;object-fit:contain;display:block}
</style></head><body>%s</body></html>`, ImageWidth, ImageHeight, accent, accent, body))
	}
	var out []ImagePlan
	out = append(out, ImagePlan{Name: "01-cover.jpg", Doc: doc(page(1, 1000, ""))})

	parts := r.partPages()
	// Every part in two rows, the thumbnails as large as the columns allow.
	shown := parts[:min(len(parts), 12)]
	cols := max(1, (len(shown)+1)/2)
	thumb := min(330, (float64(ImageWidth)-160-float64(cols-1)*34)/float64(cols)/ratio)
	var grid strings.Builder
	for _, part := range shown {
		fmt.Fprintf(&grid, `<div class="col">%s<div class="cap">%s<small>%s</small></div></div>`, page(part.first, thumb, ""), html.EscapeString(part.Name), w.count(part.count))
	}
	out = append(out, ImagePlan{Name: "02-included.jpg", Doc: doc(fmt.Sprintf(`<div class="col"><h2>%s</h2><div class="grid" style="grid-template-columns:repeat(%d,auto)">%s</div></div>`, html.EscapeString(fmt.Sprintf(w.Included, r.Summary.Pages)), cols, grid.String()))})

	n := 3
	for _, part := range parts {
		if n > EtsyImages-1 {
			break
		}
		if part.first == 1 {
			continue // the cover has its own image
		}
		// Pages above a caption: one page at full height, two side by side
		// as large as the width allows.
		height := 900.0
		pages := page(part.first, height, "")
		if part.count > 1 {
			height = min(860, (ImageWidth-160-36)/(2*ratio))
			pages = page(part.first, height, "") + page(part.first+1, height, "")
		}
		caption := fmt.Sprintf(`<div class="cap" style="max-width:1100px">%s<small>%s · %s</small></div>`, html.EscapeString(part.Name), w.count(part.count), html.EscapeString(part.Job))
		out = append(out, ImagePlan{Name: fmt.Sprintf("%02d-%s.jpg", n, deliver.Slug(part.Name, "part")), Doc: doc(`<div class="col"><div style="display:flex;gap:36px">` + pages + `</div>` + caption + `</div>`)})
		n++
	}

	var sizes strings.Builder
	seen := map[string]bool{}
	for _, ed := range r.Editions {
		s := ed.Edition.Size
		if seen[s.Key] {
			continue
		}
		seen[s.Key] = true
		const px = 2.6 // CSS pixels per millimetre: true scale between the sizes
		fmt.Fprintf(&sizes, `<div class="col"><div class="paper" style="width:%gpx;height:%gpx"><img src="cover-%s.png" alt=""></div><div class="cap">%s<small>%g × %g mm</small></div></div>`, s.Width*px, s.Height*px, s.Key, html.EscapeString(s.Label), s.Width, s.Height)
	}
	facts := fmt.Sprintf(w.SizesLine, r.Summary.Pages)
	if r.Summary.Links > 0 {
		facts += " · " + fmt.Sprintf(w.LinksLine, r.Summary.Links)
	}
	out = append(out, ImagePlan{Name: fmt.Sprintf("%02d-sizes.jpg", n), Doc: doc(`<div class="col"><div class="sizes">` + sizes.String() + `</div><div class="cap" style="max-width:none">` + html.EscapeString(facts) + `</div></div>`)})
	return out
}

// SizeCovers returns the first edition of every paper size, whose cover the
// sizes image shows as cover-<size>.png.
func (r Report) SizeCovers() []Rendered {
	var out []Rendered
	seen := map[string]bool{}
	for _, ed := range r.Editions {
		if !seen[ed.Edition.Size.Key] {
			seen[ed.Edition.Size.Key] = true
			out = append(out, ed)
		}
	}
	return out
}
