package document

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/markup"
)

// printHTML is a document laid out for print as its .docx is: the same
// paper, margins, running header and footer, sizes, spacing and breaks,
// so Chrome prints the PDF buyers get and counts its pages without any
// office suite.
func printHTML(r render) []byte {
	r.inline = true
	m, d := r.m, r.doc
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>%s%s</style></head><body>`,
		html.EscapeString(d.Lang), html.EscapeString(d.Title), styleFor(m, docStyle, ""), pageStyle(r))
	htmlBlocks(&b, r, d.Blocks, false)
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

// pageStyle is the @page rule of a document: its paper and margins, the
// header top left and the footer bottom right at Word's 11 mm from the
// edge, none on a cover page.
func pageStyle(r render) string {
	m, d := r.m, r.doc
	margin := func(edge float64) string { return fmt.Sprintf("%.2fmm", edge) }
	box := fmt.Sprintf("font:%.1fpt %q,serif;color:%s;", m.Small, m.Text, m.Muted)
	var page strings.Builder
	// Half a millimetre less measure than Word: where a line ends within a
	// point of the margin, Word wraps and Chrome would not.
	fmt.Fprintf(&page, `@page{size:%.2fmm %.2fmm;margin:%s %s %s %s;`, m.Paper.Width, m.Paper.Height, margin(m.Top), margin(m.Margin+0.5), margin(m.Bottom), margin(m.Margin))
	if d.Header != "" {
		fmt.Fprintf(&page, `@top-left{content:%s;%svertical-align:top;padding-top:%s}`, cssContent(plain(fields([]Inline{{Text: d.Header}}))), box, margin(11))
	}
	if f := d.footerText(); f != "" {
		fmt.Fprintf(&page, `@bottom-right{content:%s;%svertical-align:bottom;padding-bottom:%s}`, cssContent(f), box, margin(11))
	}
	page.WriteString(`}`)
	if d.Cover {
		page.WriteString(`@page:first{@top-left{content:none}@bottom-right{content:none}}`)
	}
	fmt.Fprintf(&page, `html,body{margin:0;background:#fff;font:%.2fpt/%.2fpt %q,serif;color:%s}.break{break-after:page}`, m.Base, m.PrintLine, m.Text, m.Ink)
	return page.String()
}

// cssContent turns header and footer text with {page} and {pages} into a
// CSS content value.
func cssContent(s string) string {
	var parts []string
	for s != "" {
		i := strings.IndexByte(s, '{')
		switch {
		case i < 0:
			parts = append(parts, strconv.Quote(s))
			s = ""
		case strings.HasPrefix(s[i:], "{page}"):
			parts = append(parts, strconv.Quote(s[:i]), "counter(page)")
			s = s[i+6:]
		case strings.HasPrefix(s[i:], "{pages}"):
			parts = append(parts, strconv.Quote(s[:i]), "counter(pages)")
			s = s[i+7:]
		default:
			parts = append(parts, strconv.Quote(s[:i+1]))
			s = s[i+1:]
		}
	}
	return strings.Join(parts, " ")
}

// pagePixels is the paper in CSS pixels.
func pagePixels(p Paper) (int64, int64) {
	return int64(p.Width / 25.4 * 96), int64(p.Height / 25.4 * 96)
}

// shotHTML is a document's first page on screen, the size of its paper,
// for the listing images: the print layout with the margins as padding,
// the running header and footer drawn, and everything below the page cut.
func shotHTML(r render) []byte {
	r.inline = true
	m, d := r.m, r.doc
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><style>%s
html,body{margin:0;background:#fff}
body{width:%.2fmm;height:%.2fmm;overflow:hidden;padding:%.2fmm %.2fmm %.2fmm %.2fmm;position:relative;font:%.2fpt/%.2fpt %q,serif;color:%s}
.running{position:absolute;top:11mm;left:%.2fmm;font-size:%.1fpt;color:%s}
.foot{position:absolute;bottom:11mm;right:%.2fmm;font-size:%.1fpt;color:%s}
.break~*{display:none}
</style></head><body>`, html.EscapeString(d.Lang), styleFor(m, docStyle, ""),
		m.Paper.Width, m.Paper.Height, m.Top, m.Margin+0.5, m.Bottom, m.Margin, m.Base, m.PrintLine, m.Text, m.Ink, m.Margin, m.Small, m.Muted, m.Margin, m.Small, m.Muted)
	if d.Header != "" && !d.Cover {
		b.WriteString(`<div class="running">`)
		markup.HTML(&b, fields([]Inline{{Text: d.Header}}))
		b.WriteString(`</div>`)
	}
	if f := d.footerText(); f != "" && !d.Cover {
		pages := strconv.Itoa(max(d.Pages, 1))
		fmt.Fprintf(&b, `<div class="foot">%s</div>`, html.EscapeString(strings.NewReplacer("{page}", "1", "{pages}", pages).Replace(f)))
	}
	htmlBlocks(&b, r, d.Blocks, false)
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}
