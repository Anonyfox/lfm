package document

import (
	"encoding/base64"
	"fmt"
	"html"
	"math"
	"strings"

	"github.com/Anonyfox/lfm/internal/markup"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// previewHTML shows an edition's documents for lfm serve: each on sheets of
// its paper, in the theme's fonts and styles, with a sheet per explicit page
// break. It is a quick look while writing; a release renders the real
// files in LibreOffice.
func previewHTML(product string, rs []render) []byte {
	var b strings.Builder
	if len(rs) == 0 {
		return nil
	}
	m := rs[0].m
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title><style>%s</style></head><body><nav>`,
		html.EscapeString(rs[0].doc.Lang), html.EscapeString(product), previewCSS(m))
	for _, r := range rs {
		fmt.Fprintf(&b, `<a href="#%s">%s</a>`, html.EscapeString(r.doc.Key), html.EscapeString(r.doc.Title))
	}
	b.WriteString(`</nav>`)
	for _, r := range rs {
		fmt.Fprintf(&b, `<section id="%s"><h6 class="label">%s · %s</h6><div class="sheet">`, html.EscapeString(r.doc.Key), html.EscapeString(r.doc.Title), html.EscapeString(strings.ToUpper(r.m.Paper.Key)))
		if r.doc.Header != "" {
			b.WriteString(`<div class="running">`)
			markup.HTML(&b, fields([]Inline{{Text: r.doc.Header}}))
			b.WriteString(`</div>`)
		}
		htmlBlocks(&b, r, r.doc.Blocks, false)
		b.WriteString(`</div></section>`)
	}
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

func previewCSS(m metrics) string { return styleFor(m, previewStyle, "fonts/") }

// styleFor is a style with its $tokens filled from the metrics, after the
// font faces, loaded from prefix.
func styleFor(m metrics, style, prefix string) string {
	var b strings.Builder
	for _, f := range []*family{m.text, m.heading} {
		if f == nil {
			continue
		}
		for _, face := range f.Faces {
			fstyle := "normal"
			if face.Italic {
				fstyle = "italic"
			}
			fmt.Fprintf(&b, `@font-face{font-family:%q;src:url(%q);font-weight:%d;font-style:%s}`, f.Name, prefix+baseName(face.File), face.Weight, fstyle)
		}
	}
	headColor, weight := m.Ink, "400"
	if m.HeadingAccent {
		headColor = m.Accent
	}
	if m.HeadingBold {
		weight = "700"
	}
	title := m.Ink
	if m.HeadingAccent || m.Accent != m.Ink {
		title = m.Accent
	}
	after := 2 * m.Unit
	if m.Indent {
		after = 0
	}
	pt := func(v float64) string { return fmt.Sprintf("%.2fpt", v) }
	mm := func(v float64) string { return fmt.Sprintf("%.2fmm", v) }
	r := strings.NewReplacer(
		"$base", pt(m.Base), "$line", pt(m.PrintLine), "$headlh", fmt.Sprintf("%.3f", m.PrintHeadFactor), "$smalllh", pt(math.Max(m.Small*1.3, m.Small*m.PrintLine/m.Base)), "$text", fmt.Sprintf("%q", m.Text), "$heading", fmt.Sprintf("%q", m.Heading),
		"$ink", m.Ink, "$muted", m.Muted, "$accent", m.Accent, "$tint", m.Tint, "$rule", m.Stroke, "$headcolor", headColor, "$titlecolor", title, "$weight", weight,
		"$pagew", mm(m.Paper.Width), "$pageh", mm(m.Paper.Height), "$top", mm(m.Top), "$margin", mm(m.Margin), "$bottom", mm(m.Bottom), "$gutter", mm(gutter),
		"$small", pt(m.Small), "$label", pt(m.Small+0.5), "$after", pt(after), "$indent", pt(m.Base*1.2),
		"$title", pt(m.Title), "$h1", pt(m.H1), "$h2", pt(m.H2), "$h3", pt(m.H3),
		"$spacer", pt(spacerHeight(m)), "$u05", pt(m.Unit/2), "$u15", pt(1.5*m.Unit), "$u10", pt(10*m.Unit), "$u1", pt(m.Unit), "$u2", pt(2*m.Unit), "$u3", pt(3*m.Unit), "$u4", pt(4*m.Unit), "$u5", pt(5*m.Unit), "$u6", pt(6*m.Unit),
	)
	b.WriteString(r.Replace(style))
	return b.String()
}

// spacerHeight is the paragraph Word needs after a table, exactly as
// high in every app.
func spacerHeight(m metrics) float64 { return 6 }

const previewStyle = `
*{box-sizing:border-box}
body{margin:0;background:#E9E7E3;font:$base/$line $text,serif;color:$ink}
nav{position:sticky;top:0;z-index:1;display:flex;gap:6px;flex-wrap:wrap;padding:10px 16px;background:#F7F6F4;border-bottom:1px solid #D9D6D0;font:13px system-ui,sans-serif}
nav a{color:#333;text-decoration:none;padding:4px 10px;border-radius:6px;background:#ECEAE6}
section{padding:24px 16px}
.label{font:12px system-ui,sans-serif;color:#666;margin:0 auto 8px;max-width:$pagew}
.sheet{background:#fff;width:$pagew;max-width:100%;min-height:$pageh;margin:0 auto;padding:$top $margin $bottom;box-shadow:0 1px 3px rgba(0,0,0,.12),0 8px 24px rgba(0,0,0,.08);position:relative}
.running{position:absolute;top:11mm;left:$margin;font-size:$small;color:$muted}
.break{height:0;margin:$bottom calc(-1 * $margin);border-top:16px solid #E9E7E3}
` + docStyle

// docStyle is the typography of a document, as its .docx styles set it:
// the preview and the print share it.
const docStyle = `
*{box-sizing:border-box}
body{font-kerning:normal;font-variant-ligatures:common-ligatures}
p{margin:0;padding:0 0 $after;orphans:2;widows:2}
p.indent{text-indent:$indent}
.title{font-family:$heading;font-size:$title;line-height:$headlh;color:$titlecolor;font-weight:$weight;padding-bottom:$u3}
.subtitle{font-size:$h2;line-height:$headlh;color:$muted;padding-bottom:$u6}
h1,h2,h3{font-family:$heading;color:$headcolor;font-weight:$weight;line-height:$headlh;margin:0}
h1{font-size:$h1;padding:$u6 0 $u2}
h2{font-size:$h2;padding:$u5 0 $u15}
h3{font-size:$h3;padding:$u4 0 $u1}
.center{text-align:center}
.fill{background:#FFFF00}
a{color:$accent}
.list{padding:0 0 $u1}
.item{display:flex;break-inside:avoid}
.item .mark{flex:0 0 5mm;color:$accent}
.item .mark.plain{color:inherit}
.item .mark.check::before{content:"";display:inline-block;width:.6em;height:.6em;border:.07em solid currentColor;vertical-align:-.03em}
.item .mark.check.done::before{background:currentColor}
.box{border-left:2.25pt solid $accent;background:$tint;padding:$u1 3.5mm;margin:0 0 $spacer}
.box p{padding:0}
table{border-collapse:collapse;width:100%;table-layout:fixed;margin-bottom:$spacer}
tr{break-inside:avoid}
thead{display:table-header-group}
h1,h2,h3,.title,.subtitle{break-after:avoid}
td,th{padding:$u1 1.5mm;vertical-align:top;text-align:left;font-weight:400;font-variant-numeric:tabular-nums}
th{font-family:$heading;font-weight:700;font-size:$label}
.t-booktabs{border-top:1pt solid $ink;border-bottom:1pt solid $ink}
.t-booktabs th,.t-hairline th{border-bottom:.75pt solid $ink}
.t-hairline td{border-bottom:.5pt solid $rule}
.t-grid td,.t-grid th{border:.5pt solid $rule}
.t-grid th{background:$tint}
tr.total td{border-top:.75pt solid $ink}
.layout td{padding:0;border:0}
.columns td:not(:last-child){padding-right:$gutter}
.form td{padding:$u2 0 $u05}
.form td.label{color:$muted;font-size:$label;vertical-align:bottom}
.form td.value{border-bottom:.5pt solid $muted;vertical-align:bottom}
.sign td{padding:0;vertical-align:top}
.sign .line{height:$u10;border-bottom:.5pt solid $ink}
.sign .who{color:$muted;font-size:$label;padding-top:$u1}
.ornament{text-align:center;color:$accent;padding:$u3 0}
.ornament.rule::after{content:"";display:inline-block;width:24mm;border-bottom:.75pt solid $accent}
img{display:block;max-width:100%;margin:$u1 0 $u2}
.box{break-inside:avoid}
`

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func htmlBlocks(b *strings.Builder, r render, bs []Block, inCell bool) {
	m := r.m
	prev := ""
	for _, bl := range bs {
		center := ""
		if bl.Center {
			center = ` center`
		}
		switch bl.Kind {
		case Title, Subtitle:
			fmt.Fprintf(b, `<p class="%s%s">`, bl.Kind, center)
			markup.HTML(b, bl.Inlines)
			b.WriteString(`</p>`)
		case Heading:
			fmt.Fprintf(b, `<h%d class="%s">`, bl.Level, strings.TrimSpace(center))
			markup.HTML(b, bl.Inlines)
			fmt.Fprintf(b, `</h%d>`, bl.Level)
		case Para:
			class := strings.TrimSpace(center)
			if m.Indent && prev == Para && !inCell {
				class = strings.TrimSpace(class + " indent")
			}
			fmt.Fprintf(b, `<p class="%s">`, class)
			markup.HTML(b, bl.Inlines)
			b.WriteString(`</p>`)
		case List:
			b.WriteString(`<div class="list">`)
			counters := [3]int{}
			for _, it := range bl.Items {
				counters[it.Level]++
				for l := it.Level + 1; l < 3; l++ {
					counters[l] = 0
				}
				mark, class := []string{typeface.Glyph(m.text, '•', '-'), typeface.Glyph(m.text, '–', '-'), typeface.Glyph(m.text, '◦', '·', '-')}[it.Level], "mark"
				switch {
				case it.Check:
					// Drawn, not a glyph: the print needs no font that has one.
					mark, class = "", "mark plain check"
					if it.Checked {
						class += " done"
					}
				case bl.Ordered:
					mark, class = ordinal(counters[it.Level], it.Level)+".", "mark plain"
				}
				fmt.Fprintf(b, `<div class="item" style="padding-left:%dmm"><span class="%s">%s</span><span>`, 5*it.Level, class, html.EscapeString(mark))
				markup.HTML(b, it.Inlines)
				b.WriteString(`</span></div>`)
			}
			b.WriteString(`</div>`)
		case TableKind:
			t := bl.Table
			al := markup.Aligns(t)
			fmt.Fprintf(b, `<table class="t-%s"><colgroup>`, m.Table)
			for _, w := range markup.Widths(t) {
				fmt.Fprintf(b, `<col style="width:%.1f%%">`, w*100)
			}
			b.WriteString(`</colgroup><thead><tr>`)
			for i, c := range t.Head {
				fmt.Fprintf(b, `<th style="text-align:%s">`, al[min(i, len(al)-1)])
				markup.HTML(b, c)
				b.WriteString(`</th>`)
			}
			b.WriteString(`</tr></thead>`)
			for _, row := range t.Rows {
				class := ""
				if len(row) > 0 && len(row[0]) > 0 && row[0][0].Bold {
					class = ` class="total"`
				}
				fmt.Fprintf(b, `<tr%s>`, class)
				for i := range t.Align {
					fmt.Fprintf(b, `<td style="text-align:%s">`, al[i])
					if i < len(row) {
						markup.HTML(b, row[i])
					}
					b.WriteString(`</td>`)
				}
				b.WriteString(`</tr>`)
			}
			b.WriteString(`</table>`)
		case ImageKind:
			p := r.images[bl.Image.Src]
			if p == nil || p.Px == 0 {
				continue
			}
			w, _ := imageSize(bl.Image, p, m.TextWidth)
			style := fmt.Sprintf("width:%.1fmm", w)
			if bl.Center {
				style += ";margin-left:auto;margin-right:auto"
			}
			src := "images/" + strings.TrimPrefix(bl.Image.Src, "images/")
			if r.inline {
				src = "data:image/" + p.Ext + ";base64," + base64.StdEncoding.EncodeToString(p.Data)
			}
			fmt.Fprintf(b, `<img src="%s" alt="%s" style="%s">`, html.EscapeString(src), html.EscapeString(bl.Image.Alt), style)
		case PageBreak:
			b.WriteString(`<div class="break"></div>`)
		case Ornament:
			if r0 := []rune(m.Ornament)[0]; m.text.Regular().Has(r0) {
				fmt.Fprintf(b, `<div class="ornament">%s</div>`, html.EscapeString(string(r0)))
			} else {
				b.WriteString(`<div class="ornament rule"></div>`)
			}
		case Box:
			b.WriteString(`<div class="box">`)
			htmlBlocks(b, r, bl.Children, true)
			b.WriteString(`</div>`)
		case Columns:
			b.WriteString(`<table class="layout columns"><colgroup>`)
			total := 0.0
			for _, w := range bl.Widths {
				total += w
			}
			for _, w := range bl.Widths {
				fmt.Fprintf(b, `<col style="width:%.1f%%">`, w/total*100)
			}
			b.WriteString(`</colgroup><tr>`)
			for _, c := range bl.Cols {
				b.WriteString(`<td>`)
				htmlBlocks(b, r, c, false)
				b.WriteString(`</td>`)
			}
			b.WriteString(`</tr></table>`)
		case Form:
			b.WriteString(`<table class="layout form"><colgroup><col style="width:34%"><col></colgroup>`)
			for _, f := range bl.Fields {
				b.WriteString(`<tr><td class="label">`)
				markup.HTML(b, f.Label)
				b.WriteString(`</td><td class="value">`)
				markup.HTML(b, f.Value)
				b.WriteString(`</td></tr>`)
			}
			b.WriteString(`</table>`)
		case Signature:
			// The signers share the width as in the .docx: one part each,
			// 0.12 between them.
			n := float64(len(bl.Fields))
			part := 100 / (n + 0.12*(n-1))
			b.WriteString(`<table class="layout sign"><colgroup>`)
			for i := range bl.Fields {
				if i > 0 {
					fmt.Fprintf(b, `<col style="width:%.2f%%">`, part*0.12)
				}
				fmt.Fprintf(b, `<col style="width:%.2f%%">`, part)
			}
			b.WriteString(`</colgroup><tr>`)
			for i, f := range bl.Fields {
				if i > 0 {
					b.WriteString(`<td></td>`)
				}
				b.WriteString(`<td><div class="line"></div><div class="who">`)
				markup.HTML(b, f.Label)
				b.WriteString(`</div></td>`)
			}
			b.WriteString(`</tr></table>`)
		}
		prev = bl.Kind
	}
}

// ordinal numbers a list item as Word's levels do: 1, a, i.
func ordinal(n, level int) string {
	switch level {
	case 1:
		return string(rune('a' + (n-1)%26))
	case 2:
		romans := []string{"i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x", "xi", "xii"}
		if n <= len(romans) {
			return romans[n-1]
		}
	}
	return fmt.Sprint(n)
}
