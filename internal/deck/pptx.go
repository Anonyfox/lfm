package deck

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"math"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/markup"
	"github.com/Anonyfox/lfm/internal/ooxml"
	"github.com/Anonyfox/lfm/internal/theme"
	"github.com/Anonyfox/lfm/internal/typeface"
	"github.com/Anonyfox/lfm/internal/zipx"
)

const (
	nsP  = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	relT = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	ctP  = "application/vnd.openxmlformats-officedocument.presentationml."
	// decorative marks a shape screen readers skip, as Office writes it.
	decorative = `<a:extLst><a:ext uri="{C183D7F6-B498-43B3-948B-1728B52AA6E4}"><adec:decorative xmlns:adec="http://schemas.microsoft.com/office/drawing/2017/decorative" val="1"/></a:ext></a:extLst>`
)

// e is points in EMU.
func e(pt float64) int { return int(math.Round(pt * emuPerPt)) }

// pts is a size in hundredths of a point, as DrawingML counts it.
func pts(pt float64) int { return int(math.Round(pt * 100)) }

func (b box) xfrm() string {
	return fmt.Sprintf(`<a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, e(b.X), e(b.Y), e(b.W), e(b.H))
}

// layoutSpec is one slide layout: its name and type as PowerPoint knows
// them, and the placeholders it offers.
type layoutSpec struct {
	key, name, kind string
}

var layoutSpecs = []layoutSpec{
	{LayoutCover, "Title Slide", "title"},
	{LayoutContent, "Title and Content", "obj"},
	{LayoutSection, "Section Header", "secHead"},
	{LayoutTwo, "Two Content", "twoObj"},
	{LayoutPicture, "Picture", "picTx"},
	{LayoutStatement, "Statement", "objOnly"},
}

func layoutIndex(key string) int {
	for i, l := range layoutSpecs {
		if l.key == key {
			return i + 1
		}
	}
	return 2
}

type pptxWriter struct {
	r      render
	k      design
	slides []Slide
	files  map[string][]byte
	media  map[string]string // image source -> part name
	fonts  []embeddedFace
}

type embeddedFace struct {
	family, slot, part string
	data               *typeface.Packed
}

// mmEMU is millimetres in EMU.
func mmEMU(mm float64) int { return int(math.Round(mm * 36000)) }

// pptx writes a deck as a .pptx for PowerPoint, Keynote, Google Slides and
// LibreOffice: one master and six layouts carry the theme, every slide
// fills their placeholders, speaker notes sit on notes pages of the
// edition's paper, and the same input gives the same bytes.
func pptx(r render) ([]byte, error) {
	w := &pptxWriter{r: r, k: r.k, slides: r.deck.Slides, files: map[string][]byte{}, media: map[string]string{}}
	if r.embed {
		if err := w.embed(); err != nil {
			return nil, err
		}
	}
	for i, s := range w.slides {
		w.slide(i+1, s)
	}
	return w.pack()
}

func (w *pptxWriter) embed() error {
	seen := map[string]bool{}
	for _, f := range []*typeface.Family{w.k.text, w.k.heading} {
		if f == nil || seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		for _, slot := range []string{"Regular", "Bold", "Italic", "BoldItalic"} {
			face := f.Faces[slot]
			if face == nil || !face.Embeddable {
				continue
			}
			data, err := w.r.pack.EOT(face)
			if err != nil {
				return fmt.Errorf("embed %s: %w", face.File, err)
			}
			w.fonts = append(w.fonts, embeddedFace{family: f.Name, slot: slot, part: fmt.Sprintf("fonts/font%d.fntdata", len(w.fonts)+1), data: data})
		}
	}
	return nil
}

// para is one paragraph of a text body: its properties and runs.
type para struct {
	ppr, runs string
}

// look is how runs are set beyond their own bold and italic: a color, a
// font, a dark background under them.
type look struct {
	color, latin string
	dark         bool
	size         float64 // 0 inherits
}

var (
	headLook   = look{latin: "+mj-lt"}
	accentHead = look{color: `<a:solidFill><a:schemeClr val="accent1"/></a:solidFill>`, latin: "+mj-lt"}
)

// runs writes inlines as DrawingML runs; fields carry Office's yellow
// highlight in the ink, links their relationship.
func (w *pptxWriter) runs(ins []Inline, rels *[]string, l look) string {
	var b strings.Builder
	lang := html.EscapeString(w.r.deck.Lang)
	for _, in := range ins {
		if in.Break {
			fmt.Fprintf(&b, `<a:br><a:rPr lang="%s"/></a:br>`, lang)
		}
		if in.Text == "" {
			continue
		}
		attrs := ""
		if l.size > 0 {
			attrs += fmt.Sprintf(` sz="%d"`, pts(l.size))
		}
		if in.Bold {
			attrs += ` b="1"`
		}
		if in.Italic {
			attrs += ` i="1"`
		}
		// Children in schema order: fill, highlight, font, link.
		inner := l.color
		if in.Fill {
			if l.dark || l.color != "" {
				inner = `<a:solidFill><a:schemeClr val="tx1"/></a:solidFill>`
			}
			inner += `<a:highlight><a:srgbClr val="FFFF00"/></a:highlight>`
		}
		if l.latin != "" {
			inner += `<a:latin typeface="` + l.latin + `"/>`
		}
		if in.Link != "" && rels != nil {
			id := fmt.Sprintf("rId%d", len(*rels)+1)
			*rels = append(*rels, fmt.Sprintf(`<Relationship Id="%s" Type="%shyperlink" Target="%s" TargetMode="External"/>`, id, relT, html.EscapeString(in.Link)))
			inner += fmt.Sprintf(`<a:hlinkClick r:id="%s"/>`, id)
		}
		fmt.Fprintf(&b, `<a:r><a:rPr lang="%s"%s dirty="0">%s</a:rPr><a:t>%s</a:t></a:r>`, lang, attrs, inner, html.EscapeString(in.Text))
	}
	return b.String()
}

// spacing is the space before a paragraph in points.
func spacing(pt float64) string {
	return fmt.Sprintf(`<a:spcBef><a:spcPts val="%d"/></a:spcBef>`, pts(pt))
}

// text turns blocks into paragraphs as the fit check measures them: half
// a line between blocks, less between items, numbers and checkboxes as
// the master's bullets.
func (w *pptxWriter) text(bs []Block, size float64, quiz *Block, rels *[]string) []para {
	k := w.k
	var out []para
	gap := func(first bool, pt float64) string {
		if len(out) == 0 && first {
			return spacing(0)
		}
		return spacing(pt)
	}
	for _, b := range bs {
		switch b.Kind {
		case Para:
			out = append(out, para{fmt.Sprintf(`<a:pPr marL="0" indent="0">%s<a:buNone/></a:pPr>`, gap(true, size*0.5)), w.runs(b.Inlines, rels, look{})})
		case Heading:
			l := headLook
			if k.HeadingAccent {
				l = accentHead
			}
			out = append(out, para{fmt.Sprintf(`<a:pPr marL="0" indent="0">%s<a:buNone/></a:pPr>`, gap(true, size*0.5)), w.runs(boldAll(b.Inlines), rels, l)})
		case List:
			open, done, boxFont := typeface.Checkboxes(k.text)
			for j, it := range b.Items {
				ind := k.indent(size)
				before := size * 0.3
				if j == 0 {
					before = size * 0.5
				}
				marker := ""
				switch {
				case quiz != nil && quiz.Line == b.Line:
					marker = `<a:buFont typeface="+mj-lt"/><a:buAutoNum type="alphaUcPeriod"/>`
				case it.Check:
					mark := open
					if it.Checked {
						mark = done
					}
					marker = fmt.Sprintf(`<a:buFont typeface="%s"/><a:buChar char="%s"/>`, html.EscapeString(boxFont), mark)
				case b.Ordered:
					marker = fmt.Sprintf(`<a:buFont typeface="+mn-lt"/><a:buAutoNum type="%s"/>`, []string{"arabicPeriod", "alphaLcPeriod", "romanLcPeriod"}[it.Level])
				}
				out = append(out, para{fmt.Sprintf(`<a:pPr marL="%d" lvl="%d" indent="%d">%s%s</a:pPr>`, e(ind*float64(it.Level+1)), it.Level, -e(ind), gap(j == 0, before), marker), w.runs(it.Inlines, rels, look{})})
			}
		case Box:
			out = append(out, w.text(b.Children, size, quiz, rels)...)
		}
	}
	return out
}

func boldAll(ins []Inline) []Inline {
	out := append([]Inline{}, ins...)
	for i := range out {
		out[i].Bold = true
	}
	return out
}

func txBody(body string, ps []para) string {
	var b strings.Builder
	b.WriteString(`<p:txBody>` + body + `<a:lstStyle/>`)
	if len(ps) == 0 {
		b.WriteString(`<a:p><a:endParaRPr lang="en"/></a:p>`)
	}
	for _, p := range ps {
		b.WriteString(`<a:p>` + p.ppr + p.runs + `</a:p>`)
	}
	b.WriteString(`</p:txBody>`)
	return b.String()
}

// placeholder is a slide shape filling its layout's placeholder.
func placeholder(id int, name, ph string, ps []para) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>%s</p:nvPr></p:nvSpPr><p:spPr/>%s</p:sp>`, id, name, ph, txBody(`<a:bodyPr/>`, ps))
}

const spTreeHead = `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`

// number is a slide's slide-number placeholder.
func (w *pptxWriter) number(id int) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Slide Number"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldNum" sz="quarter" idx="12"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:fld id="{B6F15528-21DE-4FAA-801E-634DDDAF4B2B}" type="slidenum"><a:rPr lang="%s"/><a:t>‹#›</a:t></a:fld><a:endParaRPr lang="%s"/></a:p></p:txBody></p:sp>`, id, html.EscapeString(w.r.deck.Lang), html.EscapeString(w.r.deck.Lang))
}

func (w *pptxWriter) slide(n int, s Slide) {
	k := w.k
	var rels []string
	rels = append(rels, fmt.Sprintf(`<Relationship Id="rId1" Type="%sslideLayout" Target="../slideLayouts/slideLayout%d.xml"/>`, relT, layoutIndex(s.Layout)))
	rels = append(rels, fmt.Sprintf(`<Relationship Id="rId2" Type="%snotesSlide" Target="../notesSlides/notesSlide%d.xml"/>`, relT, n))
	var tree strings.Builder
	title := []para{{"", w.runs(s.Title, &rels, look{})}}
	switch s.Layout {
	case LayoutCover:
		tree.WriteString(placeholder(2, "Title 1", `<p:ph type="ctrTitle"/>`, []para{{"", w.runs(s.Title, &rels, look{dark: true})}}))
		if s.Sub != nil {
			tree.WriteString(placeholder(3, "Subtitle 2", `<p:ph type="subTitle" idx="1"/>`, []para{{"", w.runs(s.Sub, &rels, look{dark: true})}}))
		}
	case LayoutSection:
		tree.WriteString(placeholder(2, "Title 1", `<p:ph type="title"/>`, title))
		if s.Sub != nil {
			tree.WriteString(placeholder(3, "Text 2", `<p:ph type="body" idx="1"/>`, []para{{"", w.runs(s.Sub, &rels, look{})}}))
		}
	default:
		tree.WriteString(placeholder(2, "Title 1", `<p:ph type="title"/>`, title))
		id := 3
		switch s.Layout {
		case LayoutTwo:
			for i, col := range s.Body[0].Cols {
				tree.WriteString(placeholder(id, fmt.Sprintf("Content %d", id), fmt.Sprintf(`<p:ph sz="half" idx="%d"/>`, i+1), w.text(col, k.Body, nil, &rels)))
				id++
			}
		case LayoutPicture:
			tree.WriteString(w.picture(id, s.Body[0].Image, &rels))
			id++
		case LayoutStatement:
			tree.WriteString(placeholder(id, "Statement 2", `<p:ph idx="1"/>`, w.text(s.Body[0].Children, k.Quote, nil, &rels)))
			id++
		default:
			id = w.stack(&tree, id, s, &rels)
		}
		tree.WriteString(w.number(id))
	}
	w.files[fmt.Sprintf("ppt/slides/slide%d.xml", n)] = []byte(fmt.Sprintf(`<p:sld %s><p:cSld><p:spTree>%s%s</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`, nsP, spTreeHead, tree.String()))
	w.files[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", n)] = []byte(relsXML(rels))
	w.notes(n, s)
}

// stack fills a content slide: text in the body placeholder when it is all
// text, else text, tables and boxes stacked from the top as the fit check
// measures them.
func (w *pptxWriter) stack(tree *strings.Builder, id int, s Slide, rels *[]string) int {
	k := w.k
	allText := true
	for _, b := range s.Body {
		if b.Kind != Para && b.Kind != List && b.Kind != Heading {
			allText = false
		}
	}
	if allText {
		tree.WriteString(placeholder(id, "Content 2", `<p:ph idx="1"/>`, w.text(s.Body, k.Body, s.Quiz, rels)))
		return id + 1
	}
	inner := k.BodyBox.W - 2*k.Pad
	y := k.BodyBox.Y
	var group []Block
	flush := func() {
		if len(group) == 0 {
			return
		}
		h := k.textHeight(group, inner, k.Body) + 2*k.Pad
		b := box{k.BodyBox.X, y, k.BodyBox.W, h}
		fmt.Fprintf(tree, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Text %d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>%s</p:sp>`,
			id, id, b.xfrm(), txBody(`<a:bodyPr wrap="square" lIns="91440" tIns="91440" rIns="91440" bIns="91440"><a:noAutofit/></a:bodyPr>`, w.text(group, k.Body, s.Quiz, rels)))
		id++
		y += h + k.Body*0.5
		group = nil
	}
	for _, b := range s.Body {
		switch b.Kind {
		case TableKind:
			flush()
			h := k.tableHeight(b.Table, k.BodyBox.W)
			tree.WriteString(w.table(id, b.Table, box{k.BodyBox.X, y, k.BodyBox.W, h}))
			id++
			y += h + k.Body*0.5
		case Box:
			flush()
			h := k.textHeight(b.Children, inner-6, k.Body) + 2*k.Pad
			bar := box{k.BodyBox.X, y, 4.5, h}
			fill := box{k.BodyBox.X, y, k.BodyBox.W, h}
			fmt.Fprintf(tree, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Note %d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:schemeClr val="bg2"/></a:solidFill></p:spPr>%s</p:sp>`,
				id, id, fill.xfrm(), txBody(`<a:bodyPr wrap="square" lIns="129540" tIns="91440" rIns="91440" bIns="91440"><a:noAutofit/></a:bodyPr>`, w.text(b.Children, k.Body, nil, rels)))
			fmt.Fprintf(tree, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Note bar %d">%s</p:cNvPr><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:schemeClr val="accent1"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr></p:sp>`, id+1, id+1, decorative, bar.xfrm())
			id += 2
			y += h + k.Body*0.5
		default:
			group = append(group, b)
		}
	}
	flush()
	return id
}

// picture places a slide's one picture in the layout's picture
// placeholder, as large as the body allows at its own proportions.
func (w *pptxWriter) picture(id int, img *Figure, rels *[]string) string {
	k := w.k
	p := w.r.images[img.Src]
	if p == nil || p.Px == 0 {
		return ""
	}
	part, ok := w.media[img.Src]
	if !ok {
		ext := "png"
		if p.Ext == "jpeg" {
			ext = "jpg"
		}
		part = fmt.Sprintf("media/image%d.%s", len(w.media)+1, ext)
		w.media[img.Src] = part
		w.files["ppt/"+part] = p.Data
	}
	rid := fmt.Sprintf("rId%d", len(*rels)+1)
	*rels = append(*rels, fmt.Sprintf(`<Relationship Id="%s" Type="%simage" Target="../%s"/>`, rid, relT, part))
	b := pictureBox(k, p)
	return fmt.Sprintf(`<p:pic><p:nvPicPr><p:cNvPr id="%d" name="Picture %d" descr="%s"/><p:cNvPicPr><a:picLocks noGrp="1" noChangeAspect="1"/></p:cNvPicPr><p:nvPr><p:ph type="pic" idx="1"/></p:nvPr></p:nvPicPr><p:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`,
		id, id, html.EscapeString(img.Alt), rid, b.xfrm())
}

// pictureBox is a picture fitted into the body, centered.
func pictureBox(k design, p *picture) box {
	area := k.BodyBox
	ratio := float64(p.Py) / float64(p.Px)
	w := area.W
	h := w * ratio
	if h > area.H {
		h = area.H
		w = h / ratio
	}
	return box{area.X + (area.W-w)/2, area.Y + (area.H-h)/2, w, h}
}

// table writes a table as the theme's table style draws it in documents.
func (w *pptxWriter) table(id int, t *Table, b box) string {
	k := w.k
	size := k.tableSize()
	ws := markup.Widths(t)
	al := markup.Aligns(t)
	ln := func(edge string, pt float64, color string) string {
		if pt == 0 {
			return fmt.Sprintf(`<a:%s w="0"><a:noFill/></a:%s>`, edge, edge)
		}
		return fmt.Sprintf(`<a:%s w="%d" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:prstDash val="solid"/></a:%s>`, edge, e(pt), theme.Hex(color), edge)
	}
	var s strings.Builder
	fmt.Fprintf(&s, `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="%d" name="Table %d"/><p:cNvGraphicFramePr><a:graphicFrameLocks noGrp="1"/></p:cNvGraphicFramePr><p:nvPr/></p:nvGraphicFramePr><p:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr firstRow="1" bandRow="0"/><a:tblGrid>`,
		id, id, e(b.X), e(b.Y), e(b.W), e(b.H))
	for _, share := range ws {
		fmt.Fprintf(&s, `<a:gridCol w="%d"/>`, e(share*b.W))
	}
	s.WriteString(`</a:tblGrid>`)
	rows := append([][][]Inline{t.Head}, t.Rows...)
	for ri, row := range rows {
		head := ri == 0
		last := ri == len(rows)-1
		total := !head && len(row) > 0 && len(row[0]) > 0 && row[0][0].Bold
		n := 1
		for i, c := range row {
			if i < len(ws) {
				n = max(n, k.lines(c, k.text, size, ws[i]*b.W-2*k.Pad, head))
			}
		}
		fmt.Fprintf(&s, `<a:tr h="%d">`, e(float64(n)*size*k.LineFactor+2*k.Pad*0.6))
		for i := range ws {
			var cell []Inline
			if i < len(row) {
				cell = row[i]
			}
			extra := fmt.Sprintf(`<a:latin typeface="%s"/>`, map[bool]string{true: "+mj-lt", false: "+mn-lt"}[head])
			if head {
				cell = boldAll(cell)
			}
			algn := map[string]string{"right": "r", "center": "ctr"}[al[min(i, len(al)-1)]]
			if algn != "" {
				algn = ` algn="` + algn + `"`
			}
			top, bottom := 0.0, 0.0
			topC, bottomC := k.Ink, k.Ink
			left, right := 0.0, 0.0
			fill := `<a:noFill/>`
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
				top, bottom, left, right = 0.5, 0.5, 0.5, 0.5
				topC, bottomC = k.Stroke, k.Stroke
				if head {
					fill = `<a:solidFill><a:schemeClr val="bg2"/></a:solidFill>`
				}
			}
			if total {
				top, topC = 0.75, k.Ink
			}
			fmt.Fprintf(&s, `<a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:pPr marL="0" indent="0"%s><a:buNone/></a:pPr>%s</a:p></a:txBody><a:tcPr marL="%d" marR="%d" marT="%d" marB="%d">%s%s%s%s%s</a:tcPr></a:tc>`,
				algn, w.cellRuns(cell, size, extra),
				e(k.Pad), e(k.Pad), e(k.Pad*0.6), e(k.Pad*0.6),
				ln("lnL", left, k.Stroke), ln("lnR", right, k.Stroke), ln("lnT", top, topC), ln("lnB", bottom, bottomC), fill)
		}
		s.WriteString(`</a:tr>`)
	}
	s.WriteString(`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`)
	return s.String()
}

// cellRuns are a table cell's runs at the table's size.
func (w *pptxWriter) cellRuns(ins []Inline, size float64, extra string) string {
	lang := html.EscapeString(w.r.deck.Lang)
	var b strings.Builder
	for _, in := range ins {
		if in.Break {
			fmt.Fprintf(&b, `<a:br><a:rPr lang="%s" sz="%d"/></a:br>`, lang, pts(size))
		}
		if in.Text == "" {
			continue
		}
		attrs := fmt.Sprintf(` sz="%d"`, pts(size))
		if in.Bold {
			attrs += ` b="1"`
		}
		if in.Italic {
			attrs += ` i="1"`
		}
		inner := `<a:solidFill><a:schemeClr val="tx1"/></a:solidFill>`
		if in.Fill {
			inner += `<a:highlight><a:srgbClr val="FFFF00"/></a:highlight>`
		}
		fmt.Fprintf(&b, `<a:r><a:rPr lang="%s"%s dirty="0">%s%s</a:rPr><a:t>%s</a:t></a:r>`, lang, attrs, inner, extra, html.EscapeString(in.Text))
	}
	if b.Len() == 0 {
		fmt.Fprintf(&b, `<a:endParaRPr lang="%s" sz="%d"/>`, lang, pts(size))
	}
	return b.String()
}

// notes writes a slide's notes page: the slide above, the notes below,
// and a quiz's answer as their last line.
func (w *pptxWriter) notes(n int, s Slide) {
	ps := w.text(s.Notes, 12, nil, nil)
	if s.Quiz != nil {
		ps = append(ps, para{`<a:pPr marL="0" indent="0"><a:spcBef><a:spcPts val="600"/></a:spcBef><a:buNone/></a:pPr>`, w.runs([]Inline{{Text: wording(w.r.deck.Lang).Answer + ": " + answer(s.Quiz), Bold: true}}, nil, look{})})
	}
	w.files[fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", n)] = []byte(fmt.Sprintf(`<p:notes %s><p:cSld><p:spTree>%s<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image 1"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg"/></p:nvPr></p:nvSpPr><p:spPr/></p:sp>%s</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:notes>`,
		nsP, spTreeHead, placeholder(3, "Notes 2", `<p:ph type="body" idx="1"/>`, ps)))
	w.files[fmt.Sprintf("ppt/notesSlides/_rels/notesSlide%d.xml.rels", n)] = []byte(relsXML([]string{
		fmt.Sprintf(`<Relationship Id="rId1" Type="%snotesMaster" Target="../notesMasters/notesMaster1.xml"/>`, relT),
		fmt.Sprintf(`<Relationship Id="rId2" Type="%sslide" Target="../slides/slide%d.xml"/>`, relT, n),
	}))
}

func relsXML(rels []string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(rels, "") + `</Relationships>`
}

// notesBoxes are the slide image and the notes on a notes page of the
// paper, in points.
func notesBoxes(p Paper) (img, text box) {
	w, h := p.Width*72/25.4, p.Height*72/25.4
	margin := 54.0
	iw := w - 2*margin
	ih := iw * slideH / slideW
	img = box{margin, margin, iw, ih}
	text = box{margin, margin + ih + 24, iw, h - margin - (margin + ih + 24)}
	return img, text
}

// master is the slide master: the title and body placeholders, the slide
// number, and the text styles every layout and slide inherits.
func (w *pptxWriter) master() string {
	k := w.k
	headColor := "tx1"
	if k.HeadingAccent {
		headColor = "accent1"
	}
	bold := map[bool]string{true: ` b="1"`, false: ""}[k.HeadingBold]
	bullets := []string{typeface.Glyph(k.text, '•', '-'), typeface.Glyph(k.text, '–', '-'), typeface.Glyph(k.text, '◦', '·', '-')}
	var body strings.Builder
	for i := range 3 {
		ind := k.indent(k.Body)
		fmt.Fprintf(&body, `<a:lvl%dpPr marL="%d" indent="%d" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:lnSpc><a:spcPct val="100000"/></a:lnSpc><a:spcBef><a:spcPts val="%d"/></a:spcBef><a:buClr><a:schemeClr val="accent1"/></a:buClr><a:buFont typeface="+mn-lt"/><a:buChar char="%s"/><a:defRPr sz="%d" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl%dpPr>`,
			i+1, e(ind*float64(i+1)), -e(ind), pts(k.Body*0.5), html.EscapeString(bullets[i]), pts(k.Body), i+1)
	}
	return fmt.Sprintf(`<p:sldMaster %s><p:cSld><p:bg><p:bgPr><a:solidFill><a:schemeClr val="bg1"/></a:solidFill><a:effectLst/></p:bgPr></p:bg><p:spTree>%s`+
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title Placeholder 1"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr>%s</p:spPr><p:txBody><a:bodyPr vert="horz" lIns="91440" tIns="91440" rIns="91440" bIns="91440" rtlCol="0" anchor="b"><a:noAutofit/></a:bodyPr><a:lstStyle/><a:p><a:r><a:rPr lang="en"/><a:t>Title</a:t></a:r></a:p></p:txBody></p:sp>`+
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Text Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr>%s</p:spPr><p:txBody><a:bodyPr vert="horz" lIns="91440" tIns="91440" rIns="91440" bIns="91440" rtlCol="0"><a:noAutofit/></a:bodyPr><a:lstStyle/><a:p><a:pPr lvl="0"/><a:r><a:rPr lang="en"/><a:t>Text</a:t></a:r></a:p></p:txBody></p:sp>`+
		`%s</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/><p:sldLayoutIdLst>%s</p:sldLayoutIdLst>`+
		`<p:txStyles><p:titleStyle><a:lvl1pPr algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:lnSpc><a:spcPct val="90000"/></a:lnSpc><a:spcBef><a:spcPct val="0"/></a:spcBef><a:buNone/><a:defRPr sz="%d"%s kern="1200"><a:solidFill><a:schemeClr val="%s"/></a:solidFill><a:latin typeface="+mj-lt"/><a:ea typeface="+mj-ea"/><a:cs typeface="+mj-cs"/></a:defRPr></a:lvl1pPr></p:titleStyle>`+
		`<p:bodyStyle>%s</p:bodyStyle><p:otherStyle><a:defPPr><a:defRPr lang="en"/></a:defPPr><a:lvl1pPr marL="0" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="%d" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:otherStyle></p:txStyles></p:sldMaster>`,
		nsP, spTreeHead, k.TitleBox.xfrm(), k.BodyBox.xfrm(), w.masterNumber(), w.layoutIDs(), pts(k.Head), bold, headColor, body.String(), pts(k.Small))
}

func (w *pptxWriter) masterNumber() string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="4" name="Slide Number Placeholder 3"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldNum" sz="quarter" idx="12"/></p:nvPr></p:nvSpPr><p:spPr>%s</p:spPr><p:txBody><a:bodyPr vert="horz" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0" anchor="ctr"/><a:lstStyle><a:lvl1pPr algn="r"><a:defRPr sz="%d"><a:solidFill><a:schemeClr val="tx2"/></a:solidFill><a:latin typeface="+mn-lt"/></a:defRPr></a:lvl1pPr></a:lstStyle><a:p><a:fld id="{B6F15528-21DE-4FAA-801E-634DDDAF4B2B}" type="slidenum"><a:rPr lang="en"/><a:t>‹#›</a:t></a:fld><a:endParaRPr lang="en"/></a:p></p:txBody></p:sp>`,
		w.k.Number.xfrm(), pts(math.Max(11, w.k.Small*0.75)))
}

func (w *pptxWriter) layoutIDs() string {
	var b strings.Builder
	for i := range layoutSpecs {
		fmt.Fprintf(&b, `<p:sldLayoutId id="%d" r:id="rId%d"/>`, 2147483649+i, i+1)
	}
	return b.String()
}

// layout writes one slide layout with its placeholders at their places.
func (w *pptxWriter) layout(l layoutSpec) string {
	k := w.k
	ph := func(id int, name, typ string, b box, body, lst string) string {
		return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>%s</p:nvPr></p:nvSpPr><p:spPr>%s</p:spPr><p:txBody>%s<a:lstStyle>%s</a:lstStyle><a:p><a:r><a:rPr lang="en"/><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`,
			id, name, typ, b.xfrm(), body, lst, html.EscapeString(name))
	}
	bodyPr := func(anchor string) string {
		return fmt.Sprintf(`<a:bodyPr lIns="91440" tIns="91440" rIns="91440" bIns="91440" anchor="%s"><a:noAutofit/></a:bodyPr>`, anchor)
	}
	accent := func(id int, b box) string {
		return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Accent">%s</p:cNvPr><p:cNvSpPr/><p:nvPr userDrawn="1"/></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:schemeClr val="accent1"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr></p:sp>`, id, decorative, b.xfrm())
	}
	white := `<a:solidFill><a:schemeClr val="bg1"/></a:solidFill>`
	bar := k.Bar
	number := `<p:sp><p:nvSpPr><p:cNvPr id="9" name="Slide Number Placeholder 8"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldNum" sz="quarter" idx="12"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:fld id="{B6F15528-21DE-4FAA-801E-634DDDAF4B2B}" type="slidenum"><a:rPr lang="en"/><a:t>‹#›</a:t></a:fld><a:endParaRPr lang="en"/></a:p></p:txBody></p:sp>`
	bold := map[bool]string{true: ` b="1"`, false: ""}[k.HeadingBold]
	bg := ""
	var tree strings.Builder
	switch l.key {
	case LayoutCover:
		bg = `<p:bg><p:bgPr><a:solidFill><a:schemeClr val="accent1"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>`
		tree.WriteString(ph(2, "Title 1", `<p:ph type="ctrTitle"/>`, k.CoverTitle, bodyPr("b"), fmt.Sprintf(`<a:lvl1pPr algn="l"><a:defRPr sz="%d"%s>%s</a:defRPr></a:lvl1pPr>`, pts(k.Cover), bold, white)))
		tree.WriteString(ph(3, "Subtitle 2", `<p:ph type="subTitle" idx="1"/>`, k.CoverSub, bodyPr("t"), fmt.Sprintf(`<a:lvl1pPr marL="0" indent="0" algn="l"><a:buNone/><a:defRPr sz="%d"><a:solidFill><a:schemeClr val="bg1"><a:alpha val="85000"/></a:schemeClr></a:solidFill></a:defRPr></a:lvl1pPr>`, pts(k.Sub))))
		tree.WriteString(fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="4" name="Accent">%s</p:cNvPr><p:cNvSpPr/><p:nvPr userDrawn="1"/></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:schemeClr val="bg1"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr></p:sp>`, decorative, box{k.CoverTitle.X + k.Pad, k.CoverTitle.Y - 22, 56, 5}.xfrm()))
	case LayoutSection:
		bg = `<p:bg><p:bgPr><a:solidFill><a:schemeClr val="bg2"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>`
		tree.WriteString(ph(2, "Title 1", `<p:ph type="title"/>`, k.SectionTitle, bodyPr("b"), fmt.Sprintf(`<a:lvl1pPr><a:defRPr sz="%d"/></a:lvl1pPr>`, pts(theme.Half(k.Cover*0.85)))))
		tree.WriteString(ph(3, "Text 2", `<p:ph type="body" idx="1"/>`, k.SectionSub, bodyPr("t"), fmt.Sprintf(`<a:lvl1pPr marL="0" indent="0"><a:buNone/><a:defRPr sz="%d"><a:solidFill><a:schemeClr val="tx2"/></a:solidFill></a:defRPr></a:lvl1pPr>`, pts(k.Sub))))
		tree.WriteString(accent(4, box{k.SectionTitle.X - 30, k.SectionTitle.Y + 12, 6, k.SectionTitle.H + k.SectionSub.H - 24}))
		tree.WriteString(number)
	case LayoutContent:
		tree.WriteString(ph(2, "Title 1", `<p:ph type="title"/>`, k.TitleBox, bodyPr("b"), ""))
		tree.WriteString(ph(3, "Content 2", `<p:ph idx="1"/>`, k.BodyBox, bodyPr("t"), ""))
		tree.WriteString(accent(4, bar))
		tree.WriteString(number)
	case LayoutTwo:
		tree.WriteString(ph(2, "Title 1", `<p:ph type="title"/>`, k.TitleBox, bodyPr("b"), ""))
		tree.WriteString(ph(3, "Content 2", `<p:ph sz="half" idx="1"/>`, k.Left, bodyPr("t"), ""))
		tree.WriteString(ph(4, "Content 3", `<p:ph sz="half" idx="2"/>`, k.Right, bodyPr("t"), ""))
		tree.WriteString(accent(5, bar))
		tree.WriteString(number)
	case LayoutPicture:
		tree.WriteString(ph(2, "Title 1", `<p:ph type="title"/>`, k.TitleBox, bodyPr("b"), ""))
		tree.WriteString(fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Picture 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="pic" idx="1"/></p:nvPr></p:nvSpPr><p:spPr>%s</p:spPr><p:txBody><a:bodyPr/><a:lstStyle><a:lvl1pPr marL="0" indent="0"><a:buNone/></a:lvl1pPr></a:lstStyle><a:p><a:endParaRPr lang="en"/></a:p></p:txBody></p:sp>`, k.BodyBox.xfrm()))
		tree.WriteString(accent(4, bar))
		tree.WriteString(number)
	case LayoutStatement:
		tree.WriteString(ph(2, "Title 1", `<p:ph type="title"/>`, k.TitleBox, bodyPr("b"), ""))
		tree.WriteString(ph(3, "Statement 2", `<p:ph idx="1"/>`, k.StatementBox, bodyPr("ctr"), fmt.Sprintf(`<a:lvl1pPr marL="0" indent="0"><a:spcBef><a:spcPts val="0"/></a:spcBef><a:buNone/><a:defRPr sz="%d" i="1"><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:defRPr></a:lvl1pPr>`, pts(k.Quote))))
		tree.WriteString(accent(4, bar))
		tree.WriteString(number)
	}
	return fmt.Sprintf(`<p:sldLayout %s type="%s" preserve="1"><p:cSld name="%s">%s<p:spTree>%s%s</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`,
		nsP, l.kind, l.name, bg, spTreeHead, tree.String())
}

// notesMaster is the notes page: the slide above, the notes below, on the
// edition's paper.
func (w *pptxWriter) notesMaster() string {
	img, text := notesBoxes(w.r.paper)
	return fmt.Sprintf(`<p:notesMaster %s><p:cSld><p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg><p:spTree>%s`+
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image Placeholder 1"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg" idx="2"/></p:nvPr></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/><a:ln w="6350"><a:solidFill><a:schemeClr val="tx2"/></a:solidFill></a:ln></p:spPr></p:sp>`+
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" sz="quarter" idx="3"/></p:nvPr></p:nvSpPr><p:spPr>%s</p:spPr><p:txBody><a:bodyPr vert="horz" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0"/><a:lstStyle/><a:p><a:pPr lvl="0"/><a:r><a:rPr lang="en"/><a:t>Notes</a:t></a:r></a:p></p:txBody></p:sp>`+
		`</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`+
		`<p:notesStyle><a:lvl1pPr marL="0" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:spcBef><a:spcPts val="400"/></a:spcBef><a:defRPr sz="1200" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:notesStyle></p:notesMaster>`,
		nsP, spTreeHead, img.xfrm(), text.xfrm())
}

func (w *pptxWriter) pack() ([]byte, error) {
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	var types, presRels, sldIDs strings.Builder
	types.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="jpg" ContentType="image/jpeg"/><Default Extension="fntdata" ContentType="application/x-fontdata"/>`)
	override := func(part, ct string) {
		fmt.Fprintf(&types, `<Override PartName="/%s" ContentType="%s"/>`, part, ct)
	}
	override("ppt/presentation.xml", ctP+"presentation.main+xml")
	override("ppt/slideMasters/slideMaster1.xml", ctP+"slideMaster+xml")
	override("ppt/notesMasters/notesMaster1.xml", ctP+"notesMaster+xml")
	override("ppt/theme/theme1.xml", "application/vnd.openxmlformats-officedocument.theme+xml")
	override("ppt/theme/theme2.xml", "application/vnd.openxmlformats-officedocument.theme+xml")
	override("ppt/presProps.xml", ctP+"presProps+xml")
	override("ppt/viewProps.xml", ctP+"viewProps+xml")
	override("ppt/tableStyles.xml", ctP+"tableStyles+xml")
	override("docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml")
	override("docProps/app.xml", "application/vnd.openxmlformats-officedocument.extended-properties+xml")
	rid := 0
	next := func(kind, target string) string {
		rid++
		fmt.Fprintf(&presRels, `<Relationship Id="rId%d" Type="%s%s" Target="%s"/>`, rid, relT, kind, target)
		return fmt.Sprintf("rId%d", rid)
	}
	masterRel := next("slideMaster", "slideMasters/slideMaster1.xml")
	notesRel := next("notesMaster", "notesMasters/notesMaster1.xml")
	for i := range w.slides {
		id := next("slide", fmt.Sprintf("slides/slide%d.xml", i+1))
		fmt.Fprintf(&sldIDs, `<p:sldId id="%d" r:id="%s"/>`, 256+i, id)
		override(fmt.Sprintf("ppt/slides/slide%d.xml", i+1), ctP+"slide+xml")
		override(fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i+1), ctP+"notesSlide+xml")
	}
	for i := range layoutSpecs {
		override(fmt.Sprintf("ppt/slideLayouts/slideLayout%d.xml", i+1), ctP+"slideLayout+xml")
	}
	next("presProps", "presProps.xml")
	next("viewProps", "viewProps.xml")
	next("theme", "theme/theme1.xml")
	next("tableStyles", "tableStyles.xml")
	var fontList strings.Builder
	if len(w.fonts) > 0 {
		fontList.WriteString(`<p:embeddedFontLst>`)
		for i := 0; i < len(w.fonts); {
			f := w.fonts[i]
			fmt.Fprintf(&fontList, `<p:embeddedFont><p:font typeface="%s"/>`, html.EscapeString(f.family))
			for ; i < len(w.fonts) && w.fonts[i].family == f.family; i++ {
				id := next("font", w.fonts[i].part)
				tag := map[string]string{"Regular": "regular", "Bold": "bold", "Italic": "italic", "BoldItalic": "boldItalic"}[w.fonts[i].slot]
				fmt.Fprintf(&fontList, `<p:%s r:id="%s"/>`, tag, id)
			}
			fontList.WriteString(`</p:embeddedFont>`)
		}
		fontList.WriteString(`</p:embeddedFontLst>`)
	}
	types.WriteString(`</Types>`)
	embedAttr := ""
	if len(w.fonts) > 0 {
		embedAttr = ` embedTrueTypeFonts="1"`
	}
	notesW, notesH := mmEMU(w.r.paper.Width), mmEMU(w.r.paper.Height)
	lang := html.EscapeString(w.r.deck.Lang)
	presentation := fmt.Sprintf(`<p:presentation %s saveSubsetFonts="0"%s><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="%s"/></p:sldMasterIdLst><p:notesMasterIdLst><p:notesMasterId r:id="%s"/></p:notesMasterIdLst><p:sldIdLst>%s</p:sldIdLst><p:sldSz cx="%d" cy="%d"/><p:notesSz cx="%d" cy="%d"/>%s<p:defaultTextStyle><a:defPPr><a:defRPr lang="%s"/></a:defPPr><a:lvl1pPr marL="0" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="%d" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:defaultTextStyle></p:presentation>`,
		nsP, embedAttr, masterRel, notesRel, sldIDs.String(), e(slideW), e(slideH), notesW, notesH, fontList.String(), lang, pts(w.k.Body))
	modified := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if t, err := time.Parse("2006-01-02", w.r.created); err == nil {
		modified = t
	}
	drawing := w.k.DrawingML()
	var layoutRels []string
	for i := range layoutSpecs {
		layoutRels = append(layoutRels, fmt.Sprintf(`<Relationship Id="rId%d" Type="%sslideLayout" Target="../slideLayouts/slideLayout%d.xml"/>`, i+1, relT, i+1))
	}
	layoutRels = append(layoutRels, fmt.Sprintf(`<Relationship Id="rId%d" Type="%stheme" Target="../theme/theme1.xml"/>`, len(layoutSpecs)+1, relT))
	parts := map[string]string{
		"[Content_Types].xml": xml + types.String(),
		"_rels/.rels": relsXML([]string{
			`<Relationship Id="rId1" Type="` + relT + `officeDocument" Target="ppt/presentation.xml"/>`,
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>`,
			`<Relationship Id="rId3" Type="` + relT + `extended-properties" Target="docProps/app.xml"/>`,
		}),
		"docProps/core.xml": xml + fmt.Sprintf(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>%s</dc:title><dc:subject>%s</dc:subject><dc:creator>%s</dc:creator><cp:lastModifiedBy>%s</cp:lastModifiedBy><dc:language>%s</dc:language><dcterms:created xsi:type="dcterms:W3CDTF">%sT00:00:00Z</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">%sT00:00:00Z</dcterms:modified></cp:coreProperties>`,
			html.EscapeString(w.r.deck.Title), html.EscapeString(w.r.product), html.EscapeString(w.r.seller), html.EscapeString(w.r.seller), lang, modified.Format("2006-01-02"), modified.Format("2006-01-02")),
		"docProps/app.xml":                             xml + fmt.Sprintf(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>lfm</Application><PresentationFormat>Widescreen</PresentationFormat><Slides>%d</Slides><Notes>%d</Notes><Company>%s</Company></Properties>`, len(w.slides), len(w.slides), html.EscapeString(w.r.seller)),
		"ppt/presentation.xml":                         xml + presentation,
		"ppt/_rels/presentation.xml.rels":              relsXML([]string{presRels.String()}),
		"ppt/presProps.xml":                            xml + `<p:presentationPr ` + nsP + `/>`,
		"ppt/viewProps.xml":                            xml + `<p:viewPr ` + nsP + `><p:normalViewPr><p:restoredLeft sz="15620"/><p:restoredTop sz="94660"/></p:normalViewPr><p:gridSpacing cx="76200" cy="76200"/></p:viewPr>`,
		"ppt/tableStyles.xml":                          xml + `<a:tblStyleLst xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"/>`,
		"ppt/theme/theme1.xml":                         xml + drawing,
		"ppt/theme/theme2.xml":                         xml + drawing,
		"ppt/slideMasters/slideMaster1.xml":            xml + w.master(),
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": relsXML(layoutRels),
		"ppt/notesMasters/notesMaster1.xml":            xml + w.notesMaster(),
		"ppt/notesMasters/_rels/notesMaster1.xml.rels": relsXML([]string{`<Relationship Id="rId1" Type="` + relT + `theme" Target="../theme/theme2.xml"/>`}),
	}
	for i, l := range layoutSpecs {
		parts[fmt.Sprintf("ppt/slideLayouts/slideLayout%d.xml", i+1)] = xml + w.layout(l)
		parts[fmt.Sprintf("ppt/slideLayouts/_rels/slideLayout%d.xml.rels", i+1)] = relsXML([]string{`<Relationship Id="rId1" Type="` + relT + `slideMaster" Target="../slideMasters/slideMaster1.xml"/>`})
	}
	for name, data := range w.files {
		if strings.HasSuffix(name, ".xml") {
			parts[name] = xml + string(data)
		} else if strings.HasSuffix(name, ".rels") {
			parts[name] = string(data)
		}
	}
	var buf bytes.Buffer
	z := zipx.New(&buf, modified)
	put := func(name string, data []byte, method uint16) error {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			data = ooxml.Order(data)
		}
		if method == zip.Store {
			return z.Store(name, data)
		}
		return z.Deflate(name, data)
	}
	// [Content_Types].xml first, as Office writes it.
	if err := put("[Content_Types].xml", []byte(parts["[Content_Types].xml"]), zip.Deflate); err != nil {
		return nil, err
	}
	for _, name := range sortedKeys(parts) {
		if name == "[Content_Types].xml" {
			continue
		}
		if err := put(name, []byte(parts[name]), zip.Deflate); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(w.files) {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			if err := put(name, w.files[name], zip.Store); err != nil {
				return nil, err
			}
		}
	}
	for _, f := range w.fonts {
		// Deflated once per compile, shared by every deck.
		if err := z.Raw("ppt/"+f.part, *f.data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
