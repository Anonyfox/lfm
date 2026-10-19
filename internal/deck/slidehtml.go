package deck

import (
	"encoding/base64"
	"fmt"
	"html"
	"math"
	"strings"

	"github.com/Anonyfox/lfm/internal/markup"
	"github.com/Anonyfox/lfm/internal/theme"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// slideCSS is the look of slides in HTML, laid out as the .pptx places
// them: the same boxes, sizes, spacing and PowerPoint's line of 1.2.
func slideCSS(k design) string {
	headColor, weight := k.Ink, "400"
	if k.HeadingAccent {
		headColor = k.Accent
	}
	if k.HeadingBold {
		weight = "700"
	}
	r := strings.NewReplacer("$text", fmt.Sprintf("%q", k.Text), "$heading", fmt.Sprintf("%q", k.Heading), "$ink", k.Ink, "$muted", k.Muted,
		"$accent", k.Accent, "$tint", k.Tint, "$rule", k.Stroke, "$headcolor", headColor, "$weight", weight, "$pad", fmt.Sprintf("%.1fpt", k.Pad),
		"$number", fmt.Sprintf("%.1fpt", math.Max(11, k.Small*0.75)))
	return r.Replace(`
.slide{position:relative;width:960pt;height:540pt;overflow:hidden;background:#fff;color:$ink;font-family:$text,serif;font-kerning:normal;font-variant-ligatures:common-ligatures;break-after:page}
.slide *{box-sizing:border-box}
.slide.cover{background:$accent;color:#fff}
.slide.section{background:$tint}
.tb{position:absolute;padding:$pad;display:flex;flex-direction:column;overflow:hidden}
.tb.b{justify-content:flex-end}.tb.ctr{justify-content:center}
.tb p{margin:0;line-height:1.2}
.ttl p{font-family:$heading,serif;font-weight:$weight;color:$headcolor;line-height:1.08}
.cover .ttl p{color:#fff}
.cover .sub p{color:rgba(255,255,255,.85)}
.section .sub p{color:$muted}
.sh{font-family:$heading,serif;font-weight:700}
.bu{display:inline-block;color:$accent}
.fill{background:#FFFF00;color:$ink}
a{color:inherit}
.bar{position:absolute;background:$accent}
.cover .bar{background:#fff}
.num{position:absolute;text-align:right;font-size:$number;color:$muted;display:flex;align-items:center;justify-content:flex-end}
.stmt p{font-style:italic;color:$accent}
.note{background:$tint}
.slide table{position:absolute;border-collapse:collapse;table-layout:fixed}
.slide td{vertical-align:top;line-height:1.2}
.slide img{position:absolute;display:block}
`)
}

// slideHTML is one slide as HTML; n is its number for the slide-number box.
func (r render) slideHTML(b *strings.Builder, k design, s Slide) {
	pt := func(v float64) string { return fmt.Sprintf("%.2fpt", v) }
	at := func(x box) string {
		return fmt.Sprintf("left:%s;top:%s;width:%s;height:%s", pt(x.X), pt(x.Y), pt(x.W), pt(x.H))
	}
	runs := func(ins []Inline) string {
		var out strings.Builder
		markup.HTML(&out, ins)
		return out.String()
	}
	textBox := func(class, anchor string, x box, size float64, body string, fit bool) {
		attr := ""
		if fit {
			attr = fmt.Sprintf(` data-fit="%d"`, s.N)
		}
		fmt.Fprintf(b, `<div class="tb %s %s" style="%s;font-size:%s"%s>%s</div>`, class, anchor, at(x), pt(size), attr, body)
	}
	fmt.Fprintf(b, `<section class="slide %s">`, s.Layout)
	title := `<p>` + runs(s.Title) + `</p>`
	switch s.Layout {
	case LayoutCover:
		fmt.Fprintf(b, `<div class="bar" style="%s"></div>`, at(box{k.CoverTitle.X + k.Pad, k.CoverTitle.Y - 22, 56, 5}))
		textBox("ttl", "b", k.CoverTitle, k.Cover, title, false)
		if s.Sub != nil {
			textBox("sub", "t", k.CoverSub, k.Sub, `<p>`+runs(s.Sub)+`</p>`, false)
		}
	case LayoutSection:
		fmt.Fprintf(b, `<div class="bar" style="%s"></div>`, at(box{k.SectionTitle.X - 30, k.SectionTitle.Y + 12, 6, k.SectionTitle.H + k.SectionSub.H - 24}))
		textBox("ttl", "b", k.SectionTitle, theme.Half(k.Cover*0.85), title, false)
		if s.Sub != nil {
			textBox("sub", "t", k.SectionSub, k.Sub, `<p>`+runs(s.Sub)+`</p>`, false)
		}
	default:
		fmt.Fprintf(b, `<div class="bar" style="%s"></div>`, at(k.Bar))
		textBox("ttl", "b", k.TitleBox, k.Head, title, false)
		switch s.Layout {
		case LayoutTwo:
			for i, col := range s.Body[0].Cols {
				textBox("body", "t", []box{k.Left, k.Right}[i], k.Body, r.slideText(k, col, k.Body, nil), true)
			}
		case LayoutPicture:
			if p := r.images[s.Body[0].Image.Src]; p != nil && p.Px > 0 {
				x := pictureBox(k, p)
				fmt.Fprintf(b, `<img src="%s" alt="%s" style="%s">`, html.EscapeString(r.imageSrc(s.Body[0].Image.Src, p)), html.EscapeString(s.Body[0].Image.Alt), at(x))
			}
		case LayoutStatement:
			textBox("stmt", "ctr", k.StatementBox, k.Quote, r.slideText(k, s.Body[0].Children, k.Quote, nil), true)
		default:
			r.slideStack(b, k, s)
		}
		fmt.Fprintf(b, `<div class="num" style="%s">%d</div>`, at(k.Number), s.N)
	}
	b.WriteString(`</section>`)
}

func (r render) imageSrc(src string, p *picture) string {
	if r.inline {
		return "data:image/" + p.Ext + ";base64," + base64.StdEncoding.EncodeToString(p.Data)
	}
	return "images/" + strings.TrimPrefix(src, "images/")
}

// slideStack lays out a content slide as the .pptx stacks it.
func (r render) slideStack(b *strings.Builder, k design, s Slide) {
	pt := func(v float64) string { return fmt.Sprintf("%.2fpt", v) }
	allText := true
	for _, bl := range s.Body {
		if bl.Kind != Para && bl.Kind != List && bl.Kind != Heading {
			allText = false
		}
	}
	if allText {
		fmt.Fprintf(b, `<div class="tb t" style="left:%s;top:%s;width:%s;height:%s;font-size:%s" data-fit="%d">%s</div>`, pt(k.BodyBox.X), pt(k.BodyBox.Y), pt(k.BodyBox.W), pt(k.BodyBox.H), pt(k.Body), s.N, r.slideText(k, s.Body, k.Body, s.Quiz))
		return
	}
	inner := k.BodyBox.W - 2*k.Pad
	y := k.BodyBox.Y
	var group []Block
	flush := func() {
		if len(group) == 0 {
			return
		}
		h := k.textHeight(group, inner, k.Body) + 2*k.Pad
		fmt.Fprintf(b, `<div class="tb t" style="left:%s;top:%s;width:%s;height:%s;font-size:%s">%s</div>`, pt(k.BodyBox.X), pt(y), pt(k.BodyBox.W), pt(h), pt(k.Body), r.slideText(k, group, k.Body, s.Quiz))
		y += h + k.Body*0.5
		group = nil
	}
	for _, bl := range s.Body {
		switch bl.Kind {
		case TableKind:
			flush()
			h := k.tableHeight(bl.Table, k.BodyBox.W)
			r.slideTable(b, k, bl.Table, box{k.BodyBox.X, y, k.BodyBox.W, h})
			y += h + k.Body*0.5
		case Box:
			flush()
			h := k.textHeight(bl.Children, inner-6, k.Body) + 2*k.Pad
			fmt.Fprintf(b, `<div class="tb t note" style="left:%s;top:%s;width:%s;height:%s;font-size:%s;padding-left:10.2pt">%s</div><div class="bar" style="left:%s;top:%s;width:4.5pt;height:%s"></div>`,
				pt(k.BodyBox.X), pt(y), pt(k.BodyBox.W), pt(h), pt(k.Body), r.slideText(k, bl.Children, k.Body, nil), pt(k.BodyBox.X), pt(y), pt(h))
			y += h + k.Body*0.5
		default:
			group = append(group, bl)
		}
	}
	flush()
	if y-k.Body*0.5 > k.BodyBox.Y+k.BodyBox.H+0.5 {
		fmt.Fprintf(b, `<div data-fit="%d" data-over="1"></div>`, s.N)
	}
}

// slideText is blocks as paragraphs with the .pptx's spacing and bullets.
func (r render) slideText(k design, bs []Block, size float64, quiz *Block) string {
	var b strings.Builder
	first := true
	gap := func(pt float64) string {
		if first {
			first = false
			return "0"
		}
		return fmt.Sprintf("%.2fpt", pt)
	}
	for _, bl := range bs {
		switch bl.Kind {
		case Para:
			fmt.Fprintf(&b, `<p style="margin-top:%s">`, gap(size*0.5))
			markup.HTML(&b, bl.Inlines)
			b.WriteString(`</p>`)
		case Heading:
			fmt.Fprintf(&b, `<p class="sh" style="margin-top:%s;color:%s">`, gap(size*0.5), map[bool]string{true: k.Accent, false: k.Ink}[k.HeadingAccent])
			markup.HTML(&b, bl.Inlines)
			b.WriteString(`</p>`)
		case List:
			open, done, boxFont := typeface.Checkboxes(k.text)
			bullets := []string{typeface.Glyph(k.text, '•', '-'), typeface.Glyph(k.text, '–', '-'), typeface.Glyph(k.text, '◦', '·', '-')}
			counters := [3]int{}
			for j, it := range bl.Items {
				counters[it.Level]++
				ind := k.indent(size)
				before := size * 0.3
				if j == 0 {
					before = size * 0.5
				}
				mark, font := bullets[it.Level], ""
				switch {
				case quiz != nil && quiz.Line == bl.Line:
					mark, font = string(rune('A'+j))+".", ";font-family:"+cssFamily(k.Heading)
				case it.Check:
					mark, font = open, ";font-family:"+cssFamily(boxFont)
					if it.Checked {
						mark = done
					}
				case bl.Ordered:
					mark = ordinal(counters[it.Level], it.Level) + "."
				}
				fmt.Fprintf(&b, `<p style="margin-top:%s;padding-left:%.2fpt"><span class="bu" style="margin-left:-%.2fpt;width:%.2fpt%s">%s</span>`, gap(before), ind*float64(it.Level+1), ind, ind, font, html.EscapeString(mark))
				markup.HTML(&b, it.Inlines)
				b.WriteString(`</p>`)
			}
		case Box:
			b.WriteString(r.slideText(k, bl.Children, size, quiz))
		}
	}
	return b.String()
}

// slideTable draws a table as the .pptx's cells and borders do.
func (r render) slideTable(b *strings.Builder, k design, t *Table, x box) {
	size := k.tableSize()
	ws := markup.Widths(t)
	al := markup.Aligns(t)
	line := func(pt float64, color string) string {
		if pt == 0 {
			return "none"
		}
		return fmt.Sprintf("%.2fpt solid %s", pt, color)
	}
	fmt.Fprintf(b, `<table style="left:%.2fpt;top:%.2fpt;width:%.2fpt;font-size:%.2fpt"><colgroup>`, x.X, x.Y, x.W, size)
	for _, w := range ws {
		fmt.Fprintf(b, `<col style="width:%.2fpt">`, w*x.W)
	}
	b.WriteString(`</colgroup>`)
	rows := append([][][]Inline{t.Head}, t.Rows...)
	for ri, row := range rows {
		head, last := ri == 0, ri == len(rows)-1
		total := !head && len(row) > 0 && len(row[0]) > 0 && row[0][0].Bold
		n := 1
		for i, c := range row {
			if i < len(ws) {
				n = max(n, k.lines(c, k.text, size, ws[i]*x.W-2*k.Pad, head))
			}
		}
		fmt.Fprintf(b, `<tr style="height:%.2fpt">`, float64(n)*size*k.LineFactor+2*k.Pad*0.6)
		for i := range ws {
			var top, bottom, side float64
			topC, bottomC := k.Ink, k.Ink
			bg := ""
			switch k.Table {
			case "booktabs":
				if head {
					top, bottom = 1, 0.75
				}
				if last {
					bottom = 1
				}
			case "hairline":
				if head {
					bottom = 0.75
				} else {
					bottom, bottomC = 0.5, k.Stroke
				}
			case "grid":
				top, bottom, side = 0.5, 0.5, 0.5
				topC, bottomC = k.Stroke, k.Stroke
				if head {
					bg = "background:" + k.Tint + ";"
				}
			}
			if total {
				top, topC = 0.75, k.Ink
			}
			style := fmt.Sprintf("%spadding:%.2fpt %.2fpt;text-align:%s;border-top:%s;border-bottom:%s;border-left:%s;border-right:%s", bg, k.Pad*0.6, k.Pad, al[min(i, len(al)-1)], line(top, topC), line(bottom, bottomC), line(side, k.Stroke), line(side, k.Stroke))
			if head {
				style += ";font-family:" + cssFamily(k.Heading) + ";font-weight:700"
			}
			fmt.Fprintf(b, `<td style="%s">`, style)
			if i < len(row) {
				markup.HTML(b, row[i])
			}
			b.WriteString(`</td>`)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</table>`)
}

// slidesPrint is a deck for Chrome to print: one 16:9 page per slide.
func slidesPrint(r render) []byte {
	r.inline = true
	k := r.k
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>%s%s@page{size:960pt 540pt;margin:0}html,body{margin:0;background:#fff}</style></head><body>`,
		html.EscapeString(r.deck.Lang), html.EscapeString(r.deck.Title), typeface.FontFaces("", r.k.text, r.k.heading), slideCSS(k))
	for _, s := range r.deck.Slides {
		r.slideHTML(&b, k, s)
	}
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

// notesPrint is a deck's speaker notes on the edition's paper: each slide
// above what the presenter says, a quiz's answer last.
func notesPrint(r render) []byte { return notesPages(r, 0) }

// notesPages prints the notes of every slide, or of slide only when it is
// not 0.
func notesPages(r render, only int) []byte {
	r.inline = true
	k := r.k
	img, text := notesBoxes(r.paper)
	scale := img.W / slideW
	w := wording(r.deck.Lang)
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>%s%s
@page{size:%.2fmm %.2fmm;margin:0}
html,body{margin:0;background:#fff}
.np{position:relative;width:%.2fmm;height:%.2fmm;overflow:hidden;break-after:page;font-family:%q,serif;color:%s}
.frame{position:absolute;left:%.2fpt;top:%.2fpt;width:%.2fpt;height:%.2fpt;box-shadow:0 0 0 .5pt %s;overflow:hidden}
.frame .slide{transform:scale(%.5f);transform-origin:0 0}
.notes{position:absolute;left:%.2fpt;top:%.2fpt;width:%.2fpt;height:%.2fpt;font-size:12pt;line-height:1.45;overflow:hidden}
.notes p{margin:0 0 4pt}
.notes .bu{display:inline-block;color:%s}
.head{position:absolute;left:%.2fpt;top:24pt;font-size:9pt;color:%s}
</style></head><body>`, html.EscapeString(r.deck.Lang), html.EscapeString(r.deck.Title), typeface.FontFaces("", r.k.text, r.k.heading), slideCSS(k),
		r.paper.Width, r.paper.Height, r.paper.Width, r.paper.Height, k.Text, k.Ink,
		img.X, img.Y, img.W, img.H, k.Stroke, scale, text.X, text.Y, text.W, text.H, k.Accent, img.X, k.Muted)
	ss := r.deck.Slides
	for _, s := range ss {
		if only != 0 && s.N != only {
			continue
		}
		b.WriteString(`<div class="np">`)
		fmt.Fprintf(&b, `<div class="head">%s · %d / %d</div><div class="frame">`, html.EscapeString(r.deck.Title), s.N, len(ss))
		r.slideHTML(&b, k, s)
		fmt.Fprintf(&b, `</div><div class="notes" data-fit="%d">`, s.N)
		b.WriteString(r.slideText(k, s.Notes, 12, nil))
		if s.Quiz != nil {
			fmt.Fprintf(&b, `<p style="margin-top:6pt"><strong>%s: %s</strong></p>`, html.EscapeString(w.Answer), answer(s.Quiz))
		}
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

// slideShot is one slide alone, 1280 × 720 CSS pixels, for a listing image.
func slideShot(r render, n int) []byte {
	r.inline = true
	k := r.k
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><style>%s%shtml,body{margin:0;background:#fff;overflow:hidden}</style></head><body>`,
		html.EscapeString(r.deck.Lang), typeface.FontFaces("", r.k.text, r.k.heading), slideCSS(k))
	ss := r.deck.Slides
	if n >= 1 && n <= len(ss) {
		r.slideHTML(&b, k, ss[n-1])
	}
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

// previewSlides writes a deck into the lfm serve preview: every slide with
// its notes below.
func previewSlides(b *strings.Builder, r render) {
	k := r.k
	for _, s := range r.deck.Slides {
		b.WriteString(`<div class="deck"><div class="frame">`)
		r.slideHTML(b, k, s)
		b.WriteString(`</div><div class="notes">`)
		b.WriteString(r.slideText(k, s.Notes, 11, nil))
		if s.Quiz != nil {
			fmt.Fprintf(b, `<p><strong>%s: %s</strong></p>`, html.EscapeString(wording(r.deck.Lang).Answer), answer(s.Quiz))
		}
		b.WriteString(`</div></div>`)
	}
}

// cssFamily quotes a font family for an inline style attribute.
func cssFamily(name string) string { return "'" + strings.ReplaceAll(name, "'", "") + "'" }

// ordinal numbers a list item as PowerPoint's levels do: 1, a, i.
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
