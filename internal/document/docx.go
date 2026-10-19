package document

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"html"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/markup"
	"github.com/Anonyfox/lfm/internal/ooxml"
	"github.com/Anonyfox/lfm/internal/typeface"
	"github.com/Anonyfox/lfm/internal/zipx"
)

// picture is an image a document shows.
type picture struct {
	Data   []byte
	Ext    string // png or jpeg
	Px, Py int
}

// render is one document laid out for one paper: what both writers need.
type render struct {
	doc     *Doc
	m       metrics
	images  map[string]*picture // by path below the deliverable folder
	created string              // fixed date every file carries
	embed   bool                // embed the font files
	pack    *typeface.Pack      // the fonts compressed once, shared by every document
	inline  bool                // HTML: images as data, for the printer
	seller  string              // the author in the file's properties
	product string              // the set the document belongs to
}

// Units.
func twips(mm float64) int   { return int(math.Round(mm * 1440 / 25.4)) }
func ptTwips(pt float64) int { return int(math.Round(pt * 20)) }
func halfPt(pt float64) int  { return int(math.Round(pt * 2)) }
func eighths(pt float64) int { return max(2, int(math.Round(pt*8))) }
func emu(mm float64) int     { return int(math.Round(mm * 36000)) }
func hexColor(c string) string {
	return strings.ToUpper(strings.TrimPrefix(c, "#"))
}

// fontKey is a family's deterministic embedding key.
func fontKey(name string) string {
	h := sha256.Sum256([]byte("lfm-font:" + name))
	x := fmt.Sprintf("%X", h[:16])
	return "{" + x[0:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:32] + "}"
}

type docxWriter struct {
	r      render
	body   strings.Builder
	rels   []string // document relationships beyond the fixed ones
	media  []string // media part names, in order
	files  map[string][]byte
	nums   []string // w:num entries
	images map[string]string
	drawn  int                         // pictures placed, for their ids
	fonts  map[string]*typeface.Packed // embedded font parts, written as they are
}

const (
	nsW = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	nsD = ` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`
)

// docx writes a document as a .docx for Word, Google Docs, Pages and
// LibreOffice: built-in styles carry the theme, layout is flowing text and
// borderless tables, and the same input gives the same bytes.
func docx(r render) ([]byte, error) {
	w := &docxWriter{r: r, files: map[string][]byte{}, images: map[string]string{}, fonts: map[string]*typeface.Packed{}}
	if w.r.pack == nil {
		w.r.pack = typeface.NewPack()
	}
	// Numbering: 1 bullets, 2 checkboxes; ordered lists get their own.
	w.nums = []string{`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>`, `<w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>`, `<w:num w:numId="3"><w:abstractNumId w:val="3"/></w:num>`}
	w.blocks(r.doc.Blocks, r.m.TextWidth, false)
	w.section()
	return w.pack()
}

func (w *docxWriter) blocks(bs []Block, width float64, inCell bool) {
	prev := ""
	for _, b := range bs {
		switch b.Kind {
		case Title, Subtitle:
			w.para(styleID(b.Kind), b.Inlines, b.Center, "")
		case Heading:
			w.para(fmt.Sprintf("Heading%d", b.Level), b.Inlines, b.Center, "")
		case Para:
			style := "Normal"
			if w.r.m.Indent && prev == Para {
				style = "BodyTextFirstIndent"
			}
			if inCell {
				style = "TableText"
			}
			w.para(style, b.Inlines, b.Center, "")
		case List:
			w.list(b)
		case TableKind:
			w.table(b.Table, width)
		case ImageKind:
			w.image(b.Image, width, b.Center)
		case PageBreak:
			w.body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		case Ornament:
			// The theme's ornament when the font draws it, else a short rule.
			if r := []rune(w.r.m.Ornament)[0]; w.r.m.text.Regular().Has(r) {
				fmt.Fprintf(&w.body, `<w:p><w:pPr><w:pStyle w:val="Ornament"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p>`, html.EscapeString(string(r)))
			} else {
				side := twips((width - 24) / 2)
				fmt.Fprintf(&w.body, `<w:p><w:pPr><w:pStyle w:val="Ornament"/><w:pBdr>%s</w:pBdr><w:ind w:left="%d" w:right="%d"/><w:spacing w:after="%d"/></w:pPr></w:p><w:p><w:pPr><w:pStyle w:val="Spacer"/></w:pPr></w:p>`,
					strings.Replace(border("bottom", 0.75, w.r.m.Accent), `w:space="0"`, `w:space="1"`, 1), side, side, ptTwips(3*w.r.m.Unit))
			}
		case Box:
			w.box(b, width)
		case Columns:
			w.columns(b, width)
		case Form:
			w.form(b, width)
		case Signature:
			w.signature(b, width)
		}
		prev = b.Kind
	}
}

func styleID(kind string) string {
	if kind == Title {
		return "Title"
	}
	return "Subtitle"
}

func (w *docxWriter) para(style string, ins []Inline, center bool, extra string) {
	w.body.WriteString(`<w:p><w:pPr>`)
	fmt.Fprintf(&w.body, `<w:pStyle w:val="%s"/>`, style)
	w.body.WriteString(extra)
	if center {
		w.body.WriteString(`<w:jc w:val="center"/>`)
	}
	w.body.WriteString(`</w:pPr>`)
	w.runs(ins)
	w.body.WriteString(`</w:p>`)
}

func (w *docxWriter) runs(ins []Inline) {
	for _, in := range ins {
		if in.Break {
			w.body.WriteString(`<w:r><w:br/></w:r>`)
		}
		if in.Text == "" {
			continue
		}
		rpr := ""
		if in.Bold {
			rpr += `<w:b/>`
		}
		if in.Italic {
			rpr += `<w:i/>`
		}
		if in.Fill {
			rpr += `<w:highlight w:val="yellow"/>` // one click removes it in every app
		}
		run := fmt.Sprintf(`<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>`, wrap("w:rPr", rpr), html.EscapeString(in.Text))
		if in.Link != "" {
			id := w.rel("http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink", in.Link, true)
			run = fmt.Sprintf(`<w:hyperlink r:id="%s"><w:r><w:rPr><w:rStyle w:val="Hyperlink"/>%s</w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:hyperlink>`, id, rpr, html.EscapeString(in.Text))
		}
		w.body.WriteString(run)
	}
}

func wrap(tag, inner string) string {
	if inner == "" {
		return ""
	}
	return "<" + tag + ">" + inner + "</" + tag + ">"
}

func (w *docxWriter) rel(kind, target string, external bool) string {
	id := fmt.Sprintf("rId%d", 10+len(w.rels))
	mode := ""
	if external {
		mode = ` TargetMode="External"`
	}
	w.rels = append(w.rels, fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"%s/>`, id, kind, html.EscapeString(target), mode))
	return id
}

func (w *docxWriter) list(b Block) {
	num := 1
	if b.Ordered {
		num = len(w.nums) + 1
		w.nums = append(w.nums, fmt.Sprintf(`<w:num w:numId="%d"><w:abstractNumId w:val="2"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="1"/></w:lvlOverride></w:num>`, num))
	}
	for _, it := range b.Items {
		n := num
		switch {
		case it.Check && it.Checked:
			n = 3
		case it.Check:
			n = 2
		}
		w.para("ListParagraph", it.Inlines, false, fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, it.Level, n))
	}
}

// box is a note: one table cell with the accent rule on its left and the
// tint behind it, so lists and tables inside stay inside in every app.
func (w *docxWriter) box(b Block, width float64) {
	m := w.r.m
	inner := width - 2*3.5 - 3.5
	fmt.Fprintf(&w.body, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/><w:tblBorders>%s%s</w:tblBorders><w:tblLayout w:type="fixed"/><w:tblCellMar><w:top w:w="%d" w:type="dxa"/><w:left w:w="%d" w:type="dxa"/><w:bottom w:w="%d" w:type="dxa"/><w:right w:w="%d" w:type="dxa"/></w:tblCellMar><w:tblLook w:val="0000" w:firstRow="0" w:lastRow="0" w:firstColumn="0" w:lastColumn="0" w:noHBand="1" w:noVBand="1"/></w:tblPr><w:tblGrid><w:gridCol w:w="%d"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/><w:shd w:val="clear" w:color="auto" w:fill="%s"/></w:tcPr>`,
		twips(width), border("left", 2.25, m.Accent), none("top", "bottom", "right", "insideH", "insideV"), ptTwips(m.Unit), twips(3.5), ptTwips(m.Unit), twips(3.5), twips(width), twips(width), hexColor(m.Tint))
	for _, c := range b.Children {
		if c.Kind == Para || c.Kind == Heading {
			w.para("Quote", c.Inlines, false, "")
		} else {
			w.blocks([]Block{c}, inner, true)
		}
	}
	if n := len(b.Children); n == 0 || b.Children[n-1].Kind == TableKind || b.Children[n-1].Kind == Columns {
		w.body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Quote"/></w:pPr></w:p>`)
	}
	w.body.WriteString(`</w:tc></w:tr></w:tbl>`)
	w.spacer()
}

// border is one table edge in eighths of a point.
func border(edge string, pt float64, color string) string {
	return fmt.Sprintf(`<w:%s w:val="single" w:sz="%d" w:space="0" w:color="%s"/>`, edge, eighths(pt), hexColor(color))
}

func none(edges ...string) string {
	var b strings.Builder
	for _, e := range edges {
		fmt.Fprintf(&b, `<w:%s w:val="nil"/>`, e)
	}
	return b.String()
}

func (w *docxWriter) grid(width float64, shares []float64) []int {
	total := 0.0
	for _, s := range shares {
		total += s
	}
	cols := make([]int, len(shares))
	w.body.WriteString(`<w:tblGrid>`)
	for i, s := range shares {
		cols[i] = twips(width * s / total)
		fmt.Fprintf(&w.body, `<w:gridCol w:w="%d"/>`, cols[i])
	}
	w.body.WriteString(`</w:tblGrid>`)
	return cols
}

func (w *docxWriter) table(t *Table, width float64) {
	m := w.r.m
	al := markup.Aligns(t)
	var borders string
	switch m.Table {
	case "booktabs":
		borders = border("top", 1, m.Ink) + border("bottom", 1, m.Ink) + none("left", "right", "insideV", "insideH")
	case "grid":
		borders = border("top", 0.5, m.Stroke) + border("bottom", 0.5, m.Stroke) + border("left", 0.5, m.Stroke) + border("right", 0.5, m.Stroke) + border("insideH", 0.5, m.Stroke) + border("insideV", 0.5, m.Stroke)
	default: // hairline
		borders = border("bottom", 0.5, m.Stroke) + border("insideH", 0.5, m.Stroke) + none("top", "left", "right", "insideV")
	}
	fmt.Fprintf(&w.body, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblBorders>%s</w:tblBorders><w:tblCellMar><w:left w:w="%d" w:type="dxa"/><w:right w:w="%d" w:type="dxa"/></w:tblCellMar><w:tblLook w:val="04A0" w:firstRow="1" w:lastRow="0" w:firstColumn="0" w:lastColumn="0" w:noHBand="1" w:noVBand="1"/></w:tblPr>`,
		twips(width), borders, twips(1.5), twips(1.5))
	cols := w.grid(width, markup.Widths(t))
	row := func(cells [][]Inline, head, total bool) {
		w.body.WriteString(`<w:tr>`)
		if head {
			w.body.WriteString(`<w:trPr><w:cantSplit/><w:tblHeader/></w:trPr>`)
		} else {
			w.body.WriteString(`<w:trPr><w:cantSplit/></w:trPr>`)
		}
		for i := range cols {
			var in []Inline
			if i < len(cells) {
				in = cells[i]
			}
			tcpr := fmt.Sprintf(`<w:tcW w:w="%d" w:type="dxa"/>`, cols[i])
			switch {
			case head && m.Table == "grid":
				tcpr += fmt.Sprintf(`<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, hexColor(m.Tint))
			case head:
				tcpr += `<w:tcBorders>` + border("bottom", 0.75, m.Ink) + `</w:tcBorders>`
			case total:
				tcpr += `<w:tcBorders>` + border("top", 0.75, m.Ink) + `</w:tcBorders>`
			}
			fmt.Fprintf(&w.body, `<w:tc><w:tcPr>%s</w:tcPr>`, tcpr)
			style := "TableText"
			if head {
				style = "TableHeading"
			}
			jc := ""
			if a := al[min(i, len(al)-1)]; a != "left" {
				jc = fmt.Sprintf(`<w:jc w:val="%s"/>`, a)
			}
			w.para(style, in, false, jc)
			w.body.WriteString(`</w:tc>`)
		}
		w.body.WriteString(`</w:tr>`)
	}
	row(t.Head, true, false)
	for _, r := range t.Rows {
		total := len(r) > 0 && len(r[0]) > 0 && r[0][0].Bold
		row(r, false, total)
	}
	w.body.WriteString(`</w:tbl>`)
	w.spacer()
}

// spacer separates a table from what follows, as paragraphs separate:
// Word needs a paragraph after a table anyway.
func (w *docxWriter) spacer() {
	w.body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Spacer"/></w:pPr></w:p>`)
}

func (w *docxWriter) layoutTable(width float64, shares []float64) []int {
	fmt.Fprintf(&w.body, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblBorders>%s</w:tblBorders><w:tblCellMar><w:left w:w="0" w:type="dxa"/><w:right w:w="0" w:type="dxa"/></w:tblCellMar><w:tblLook w:val="0000" w:firstRow="0" w:lastRow="0" w:firstColumn="0" w:lastColumn="0" w:noHBand="1" w:noVBand="1"/></w:tblPr>`,
		twips(width), none("top", "left", "bottom", "right", "insideH", "insideV"))
	return w.grid(width, shares)
}

// gutter is the space between layout columns.
const gutter = 8.0

func (w *docxWriter) columns(b Block, width float64) {
	cols := w.layoutTable(width, b.Widths)
	w.body.WriteString(`<w:tr>`)
	for i, c := range b.Cols {
		pad := ""
		if i < len(b.Cols)-1 {
			pad = fmt.Sprintf(`<w:tcMar><w:right w:w="%d" w:type="dxa"/></w:tcMar>`, twips(gutter))
		}
		fmt.Fprintf(&w.body, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s</w:tcPr>`, cols[i], pad)
		inner := float64(cols[i])*25.4/1440 - gutter
		if len(c) == 0 {
			w.body.WriteString(`<w:p/>`)
		}
		w.blocks(c, inner, false)
		if len(c) > 0 && (c[len(c)-1].Kind == TableKind || c[len(c)-1].Kind == Columns) {
			w.body.WriteString(`<w:p/>`)
		}
		w.body.WriteString(`</w:tc>`)
	}
	w.body.WriteString(`</w:tr></w:tbl>`)
	w.spacer()
}

func (w *docxWriter) form(b Block, width float64) {
	cols := w.layoutTable(width, []float64{0.34, 0.66})
	for _, f := range b.Fields {
		w.body.WriteString(`<w:tr><w:trPr><w:cantSplit/></w:trPr>`)
		fmt.Fprintf(&w.body, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/><w:vAlign w:val="bottom"/></w:tcPr>`, cols[0])
		w.para("FormLabel", f.Label, false, "")
		fmt.Fprintf(&w.body, `</w:tc><w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/><w:tcBorders>%s</w:tcBorders><w:vAlign w:val="bottom"/></w:tcPr>`, cols[1], border("bottom", 0.5, w.r.m.Muted))
		w.para("FormValue", f.Value, false, "")
		w.body.WriteString(`</w:tc></w:tr>`)
	}
	w.body.WriteString(`</w:tbl>`)
	w.spacer()
}

func (w *docxWriter) signature(b Block, width float64) {
	n := len(b.Fields)
	shares := make([]float64, 0, 2*n-1)
	for i := range n {
		if i > 0 {
			shares = append(shares, 0.12)
		}
		shares = append(shares, 1)
	}
	cols := w.layoutTable(width, shares)
	w.body.WriteString(`<w:tr><w:trPr><w:cantSplit/></w:trPr>`)
	for i := range shares {
		if i%2 == 1 {
			fmt.Fprintf(&w.body, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr><w:p><w:pPr><w:pStyle w:val="Signature"/></w:pPr></w:p></w:tc>`, cols[i])
			continue
		}
		// Room to sign, the line, then who signs.
		fmt.Fprintf(&w.body, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr><w:p><w:pPr><w:pStyle w:val="SignatureLine"/></w:pPr></w:p>`, cols[i])
		w.para("Signature", b.Fields[i/2].Label, false, "")
		w.body.WriteString(`</w:tc>`)
	}
	w.body.WriteString(`</w:tr></w:tbl>`)
	w.spacer()
}

// imageSize is an image's printed width and height in mm: as written,
// or at 150 dpi, never wider than the text.
func imageSize(img *Figure, p *picture, width float64) (float64, float64) {
	wmm := float64(p.Px) / 150 * 25.4
	switch {
	case img.Width > 0:
		wmm = img.Width
	case img.Share > 0:
		wmm = width * img.Share
	}
	wmm = math.Min(wmm, width)
	return wmm, wmm * float64(p.Py) / float64(p.Px)
}

func (w *docxWriter) image(img *Figure, width float64, center bool) {
	p := w.r.images[img.Src]
	if p == nil || p.Px == 0 {
		return
	}
	id, ok := w.images[img.Src]
	if !ok {
		ext := "png"
		if p.Ext == "jpeg" {
			ext = "jpg"
		}
		name := fmt.Sprintf("media/image%d.%s", len(w.media)+1, ext)
		w.media = append(w.media, name)
		w.files["word/"+name] = p.Data
		id = w.rel("http://schemas.openxmlformats.org/officeDocument/2006/relationships/image", name, false)
		w.images[img.Src] = id
	}
	wmm, hmm := imageSize(img, p, width)
	w.drawn++
	n := w.drawn
	jc := ""
	if center {
		jc = `<w:jc w:val="center"/>`
	}
	fmt.Fprintf(&w.body, `<w:p><w:pPr><w:pStyle w:val="Picture"/>%s</w:pPr><w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="Picture %d" descr="%s"/><wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="%d" name="Picture %d" descr="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`,
		jc, emu(wmm), emu(hmm), n, n, html.EscapeString(img.Alt), n, n, html.EscapeString(img.Alt), id, emu(wmm), emu(hmm))
}

// fieldRuns writes header or footer text with {page} and {pages} as fields.
func fieldRuns(s string, pages int) string {
	var b strings.Builder
	field := func(instr, cached string) {
		fmt.Fprintf(&b, `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> %s </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>%s</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`, instr, cached)
	}
	// ==…== in a header or footer is a field, as in the text.
	if m := markup.FieldMark.FindStringSubmatchIndex(s); m != nil {
		return fieldRuns(s[:m[0]], pages) + fmt.Sprintf(`<w:r><w:rPr><w:highlight w:val="yellow"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r>`, html.EscapeString(s[m[2]:m[3]])) + fieldRuns(s[m[1]:], pages)
	}
	for s != "" {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			fmt.Fprintf(&b, `<w:r><w:t xml:space="preserve">%s</w:t></w:r>`, html.EscapeString(s))
			break
		}
		if i > 0 {
			fmt.Fprintf(&b, `<w:r><w:t xml:space="preserve">%s</w:t></w:r>`, html.EscapeString(s[:i]))
		}
		switch {
		case strings.HasPrefix(s[i:], "{page}"):
			field("PAGE", "1")
			s = s[i+6:]
		case strings.HasPrefix(s[i:], "{pages}"):
			field("NUMPAGES", fmt.Sprint(max(pages, 1)))
			s = s[i+7:]
		default:
			fmt.Fprintf(&b, `<w:r><w:t>{</w:t></w:r>`)
			s = s[i+1:]
		}
	}
	return b.String()
}

// footerText is the footer a document shows: its own, or page numbers
// when it may run longer than a page.
func (d *Doc) footerText() string {
	switch {
	case d.Footer == "none":
		return ""
	case d.Footer != "":
		return d.Footer
	case d.Pages == 1:
		return ""
	}
	return "{page} / {pages}"
}

func (w *docxWriter) section() {
	d, m := w.r.doc, w.r.m
	refs := ""
	add := func(kind, part, root, style, text string) {
		if text == "" {
			return
		}
		name := fmt.Sprintf("word/%s.xml", part)
		w.files[name] = []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+`<w:%s %s><w:p><w:pPr><w:pStyle w:val="%s"/></w:pPr>%s</w:p></w:%s>`, root, nsW, style, fieldRuns(text, d.Pages), root))
		id := w.rel("http://schemas.openxmlformats.org/officeDocument/2006/relationships/"+map[string]string{"hdr": "header", "ftr": "footer"}[root], part+".xml", false)
		refs += fmt.Sprintf(`<w:%sReference w:type="%s" r:id="%s"/>`, map[string]string{"hdr": "header", "ftr": "footer"}[root], kind, id)
	}
	add("default", "header1", "hdr", "Header", d.Header)
	add("default", "footer1", "ftr", "Footer", d.footerText())
	title := ""
	if d.Cover && (d.Header != "" || d.footerText() != "") {
		add("first", "header2", "hdr", "Header", " ")
		add("first", "footer2", "ftr", "Footer", " ")
		title = `<w:titlePg/>`
	}
	fmt.Fprintf(&w.body, `<w:sectPr>%s<w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="%d" w:footer="%d" w:gutter="0"/>%s</w:sectPr>`,
		refs, twips(m.Paper.Width), twips(m.Paper.Height), twips(m.Top), twips(m.Margin), twips(m.Bottom), twips(m.Margin), twips(11), twips(11), title)
}

func (w *docxWriter) pack() ([]byte, error) {
	m := w.r.m
	var buf bytes.Buffer
	modified := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if t, err := time.Parse("2006-01-02", w.r.created); err == nil {
		modified = t
	}
	z := zipx.New(&buf, modified)
	put := func(name string, data []byte) error {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			data = ooxml.Order(data) // Word refuses elements out of schema order
		}
		return z.Deflate(name, data)
	}
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	overrides := `<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/><Override PartName="/word/fontTable.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.fontTable+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/><Override PartName="/word/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`
	for _, part := range []string{"header1", "header2", "footer1", "footer2"} {
		if _, ok := w.files["word/"+part+".xml"]; ok {
			kind := "header"
			if strings.HasPrefix(part, "footer") {
				kind = "footer"
			}
			overrides += fmt.Sprintf(`<Override PartName="/word/%s.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.%s+xml"/>`, part, kind)
		}
	}
	defaults := `<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="jpg" ContentType="image/jpeg"/>`
	fonts, fontRels := w.fontTable()
	if fontRels != "" {
		defaults += `<Default Extension="odttf" ContentType="application/vnd.openxmlformats-officedocument.obfuscatedFont"/>`
	}
	parts := []struct {
		name string
		data string
	}{
		{"[Content_Types].xml", xml + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` + defaults + overrides + `</Types>`},
		{"_rels/.rels", xml + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`},
		{"docProps/core.xml", xml + fmt.Sprintf(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>%s</dc:title><dc:subject>%s</dc:subject><dc:creator>%s</dc:creator><cp:lastModifiedBy>%s</cp:lastModifiedBy><dc:language>%s</dc:language><dcterms:created xsi:type="dcterms:W3CDTF">%sT00:00:00Z</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">%sT00:00:00Z</dcterms:modified></cp:coreProperties>`, html.EscapeString(w.r.doc.Title), html.EscapeString(w.r.product), html.EscapeString(w.r.seller), html.EscapeString(w.r.seller), html.EscapeString(w.r.doc.Lang), modified.Format("2006-01-02"), modified.Format("2006-01-02"))},
		{"word/document.xml", xml + `<w:document ` + nsW + nsD + `><w:body>` + themed(w.body.String(), m) + `</w:body></w:document>`},
		{"word/_rels/document.xml.rels", xml + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/fontTable" Target="fontTable.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/><Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>` + strings.Join(w.rels, "") + `</Relationships>`},
		{"docProps/app.xml", xml + fmt.Sprintf(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Template>Normal.dotm</Template><Application>lfm</Application><DocSecurity>0</DocSecurity><Company>%s</Company></Properties>`, html.EscapeString(w.r.seller))},
		{"word/styles.xml", xml + themed(styles(m, w.r.doc.Lang), m)},
		{"word/numbering.xml", xml + themed(numbering(m, w.nums), m)},
		{"word/settings.xml", xml + settings(fontRels != "", w.r.doc.Lang)},
		{"word/theme/theme1.xml", xml + themeXML(m)},
		{"word/fontTable.xml", xml + fonts},
	}
	if fontRels != "" {
		parts = append(parts, struct {
			name string
			data string
		}{"word/_rels/fontTable.xml.rels", xml + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + fontRels + `</Relationships>`})
	}
	for _, p := range parts {
		if err := put(p.name, []byte(p.data)); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(w.files) {
		if err := put(name, w.files[name]); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(w.fonts) {
		if err := z.Raw(name, *w.fonts[name]); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fontTable lists the families with the faces to embed.
func (w *docxWriter) fontTable() (string, string) {
	var table, rels strings.Builder
	table.WriteString(`<w:fonts ` + nsW + `>`)
	seen := map[string]bool{}
	n := 0
	for _, f := range []*family{w.r.m.text, w.r.m.heading} {
		if f == nil || seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		kind := "swiss"
		if f.Regular() != nil && f.Regular().Serif {
			kind = "roman"
		}
		fmt.Fprintf(&table, `<w:font w:name="%s"><w:charset w:val="00"/><w:family w:val="%s"/><w:pitch w:val="variable"/>`, html.EscapeString(f.Name), kind)
		if w.r.embed {
			key := fontKey(f.Name)
			for _, slot := range []string{"Regular", "Bold", "Italic", "BoldItalic"} {
				face := f.Faces[slot]
				if face == nil || !face.Embeddable {
					continue
				}
				n++
				part := fmt.Sprintf("fonts/font%d.odttf", n)
				w.fonts["word/"+part] = w.r.pack.Obfuscated(face, key)
				fmt.Fprintf(&rels, `<Relationship Id="rIdF%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/font" Target="%s"/>`, n, part)
				fmt.Fprintf(&table, `<w:embed%s r:id="rIdF%d" w:fontKey="%s"/>`, slot, n, key)
			}
		}
		table.WriteString(`</w:font>`)
	}
	table.WriteString(`</w:fonts>`)
	return table.String(), rels.String()
}

func settings(embed bool, lang string) string {
	e := ""
	if embed {
		e = `<w:embedTrueTypeFonts/>`
	}
	decimal, list := separators(lang)
	e += fmt.Sprintf(`<w:themeFontLang w:val="%s"/><w:decimalSymbol w:val="%s"/><w:listSeparator w:val="%s"/>`, html.EscapeString(lang), decimal, list)
	return `<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` + e + `<w:zoom w:percent="100"/><w:defaultTabStop w:val="720"/><w:characterSpacingControl w:val="doNotCompress"/><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/><w:compatSetting w:name="overrideTableStyleFontSizeAndJustification" w:uri="http://schemas.microsoft.com/office/word" w:val="1"/><w:compatSetting w:name="enableOpenTypeFeatures" w:uri="http://schemas.microsoft.com/office/word" w:val="1"/><w:compatSetting w:name="doNotFlipMirrorIndents" w:uri="http://schemas.microsoft.com/office/word" w:val="1"/></w:compat></w:settings>`
}

func numbering(m metrics, nums []string) string {
	bullet := []string{typeface.Glyph(m.text, '•', '-'), typeface.Glyph(m.text, '–', '-'), typeface.Glyph(m.text, '◦', '·', '-')}
	open, done, boxFont := typeface.Checkboxes(m.text)
	var b strings.Builder
	b.WriteString(`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`)
	level := func(i int, format, text, font string, colored bool) {
		ind := 5 + 5*i
		// Word draws markers in its own default font unless the level names one.
		rpr := fmt.Sprintf(`<w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:cs="%[1]s"/>`, html.EscapeString(font))
		if colored {
			rpr += fmt.Sprintf(`<w:color w:val="%s"/>`, hexColor(m.Accent))
		}
		fmt.Fprintf(&b, `<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="%s"/><w:lvlText w:val="%s"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="%d" w:hanging="%d"/></w:pPr><w:rPr>%s</w:rPr></w:lvl>`, i, format, html.EscapeString(text), twips(float64(ind)), twips(5), rpr)
	}
	abstract := func(id int, each func(i int)) {
		fmt.Fprintf(&b, `<w:abstractNum w:abstractNumId="%d"><w:multiLevelType w:val="hybridMultilevel"/>`, id)
		for i := range 3 {
			each(i)
		}
		b.WriteString(`</w:abstractNum>`)
	}
	abstract(0, func(i int) { level(i, "bullet", bullet[i], m.Text, true) })
	abstract(1, func(i int) { level(i, "bullet", open, boxFont, false) })
	abstract(2, func(i int) {
		level(i, []string{"decimal", "lowerLetter", "lowerRoman"}[i], []string{"%1.", "%2.", "%3."}[i], m.Text, false)
	})
	abstract(3, func(i int) { level(i, "bullet", done, boxFont, false) })
	b.WriteString(strings.Join(nums, ""))
	b.WriteString(`</w:numbering>`)
	return b.String()
}

// styles turns the theme's metrics into Word's built-in styles, so a buyer
// changes the look of a whole set in one place and Google Docs keeps the
// headings.
func styles(m metrics, lang string) string {
	var b strings.Builder
	// Lines are at least the design leading: Word for Windows, Word for
	// Mac and LibreOffice measure a font's "single" line differently, so a
	// multiple of it would set the same file looser on Windows.
	sp := func(before, after, line float64) string {
		return fmt.Sprintf(`<w:spacing w:before="%d" w:after="%d" w:line="%d" w:lineRule="atLeast"/>`, ptTwips(before), ptTwips(after), ptTwips(line))
	}
	fonts := func(f string) string {
		return fmt.Sprintf(`<w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:eastAsia="%[1]s" w:cs="%[1]s"/>`, html.EscapeString(f))
	}
	body, spacer := m.Line, spacerHeight(m)
	head := func(size float64) float64 { return m.HeadLine * size }
	after := 2 * m.Unit
	if m.Indent {
		after = 0
	}
	headColor := m.Ink
	if m.HeadingAccent {
		headColor = m.Accent
	}
	bold := ""
	if m.HeadingBold {
		bold = `<w:b/><w:bCs/>`
	}
	// Word kerns and joins ligatures only when told to; browsers and
	// LibreOffice do by default. Word 2010's w14 settings are ignorable,
	// so older apps read the file as before.
	fmt.Fprintf(&b, `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml" mc:Ignorable="w14"><w:docDefaults><w:rPrDefault><w:rPr>%s<w:color w:val="%s"/><w:kern w:val="16"/><w:sz w:val="%d"/><w:szCs w:val="%d"/><w:lang w:val="%s"/><w14:ligatures w14:val="standard"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr>%s</w:pPr></w:pPrDefault></w:docDefaults>`,
		fonts(m.Text), hexColor(m.Ink), halfPt(m.Base), halfPt(m.Base), html.EscapeString(lang), sp(0, after, body))
	style := func(kind, id, name, based, next, ppr, rpr string, extra string) {
		b2 := ""
		if based != "" {
			b2 = fmt.Sprintf(`<w:basedOn w:val="%s"/>`, based)
		}
		if next != "" {
			b2 += fmt.Sprintf(`<w:next w:val="%s"/>`, next)
		}
		def := ""
		if id == "Normal" {
			def = ` w:default="1"`
		}
		fmt.Fprintf(&b, `<w:style w:type="%s"%s w:styleId="%s"><w:name w:val="%s"/>%s%s<w:qFormat/>%s%s</w:style>`, kind, def, id, name, b2, extra, wrap("w:pPr", ppr), wrap("w:rPr", rpr))
	}
	style("paragraph", "Normal", "Normal", "", "", `<w:widowControl/>`, "", "")
	style("paragraph", "BodyTextFirstIndent", "Body Text First Indent", "Normal", "BodyTextFirstIndent", fmt.Sprintf(`<w:ind w:firstLine="%d"/>`, ptTwips(m.Base*1.2)), "", "")
	title := m.Ink
	if m.HeadingAccent || m.Accent != m.Ink {
		title = m.Accent
	}
	style("paragraph", "Title", "Title", "Normal", "Normal", `<w:keepNext/><w:keepLines/>`+sp(0, 3*m.Unit, head(m.Title)), fonts(m.Heading)+bold+fmt.Sprintf(`<w:color w:val="%s"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, hexColor(title), halfPt(m.Title), halfPt(m.Title)), "")
	style("paragraph", "Subtitle", "Subtitle", "Normal", "Normal", `<w:keepNext/>`+sp(0, 6*m.Unit, head(m.H2)), fmt.Sprintf(`<w:color w:val="%s"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, hexColor(m.Muted), halfPt(m.H2), halfPt(m.H2)), "")
	for i, h := range []struct {
		size, before, after float64
	}{{m.H1, 6 * m.Unit, 2 * m.Unit}, {m.H2, 5 * m.Unit, 1.5 * m.Unit}, {m.H3, 4 * m.Unit, 1 * m.Unit}} {
		italic := ""
		if i == 2 && !m.HeadingBold {
			italic = `<w:i/><w:iCs/>`
		}
		style("paragraph", fmt.Sprintf("Heading%d", i+1), fmt.Sprintf("heading %d", i+1), "Normal", "Normal",
			fmt.Sprintf(`<w:keepNext/><w:keepLines/>%s<w:outlineLvl w:val="%d"/>`, sp(h.before, h.after, head(h.size)), i),
			fonts(m.Heading)+bold+italic+fmt.Sprintf(`<w:color w:val="%s"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, hexColor(headColor), halfPt(h.size), halfPt(h.size)), "")
	}
	style("paragraph", "ListParagraph", "List Paragraph", "Normal", "", `<w:contextualSpacing/>`+sp(0, m.Unit, body), "", "")
	style("paragraph", "Quote", "Quote", "Normal", "", sp(0, m.Unit, body), "", "")
	// Figures in tables share one width, so columns of numbers line up.
	style("paragraph", "TableText", "Table Text", "Normal", "", sp(m.Unit, m.Unit, body), `<w14:numSpacing w14:val="tabular"/>`, "")
	style("paragraph", "TableHeading", "Table Heading", "TableText", "", `<w:keepNext/>`, fonts(m.Heading)+fmt.Sprintf(`<w:b/><w:bCs/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, halfPt(m.Small+0.5), halfPt(m.Small+0.5)), "")
	style("paragraph", "FormLabel", "Form Label", "Normal", "", sp(2*m.Unit, m.Unit/2, body), fmt.Sprintf(`<w:color w:val="%s"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, hexColor(m.Muted), halfPt(m.Small+0.5), halfPt(m.Small+0.5)), "")
	style("paragraph", "FormValue", "Form Value", "Normal", "", sp(2*m.Unit, m.Unit/2, body), "", "")
	style("paragraph", "Signature", "Signature", "Normal", "", sp(m.Unit, 0, body), fmt.Sprintf(`<w:color w:val="%s"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, hexColor(m.Muted), halfPt(m.Small+0.5), halfPt(m.Small+0.5)), "")
	style("paragraph", "SignatureLine", "Signature Line", "Normal", "", fmt.Sprintf(`<w:keepNext/><w:pBdr>%s</w:pBdr>`, border("bottom", 0.5, m.Ink))+sp(10*m.Unit, 0, body), "", "")
	style("paragraph", "Spacer", "Spacer", "Normal", "", fmt.Sprintf(`<w:spacing w:before="0" w:after="0" w:line="%d" w:lineRule="exact"/>`, ptTwips(spacer)), `<w:sz w:val="12"/><w:szCs w:val="12"/>`, "")
	style("paragraph", "Picture", "Picture", "Normal", "", sp(m.Unit, 2*m.Unit, 1), "", "")
	style("paragraph", "Ornament", "Ornament", "Normal", "", `<w:jc w:val="center"/>`+sp(3*m.Unit, 3*m.Unit, body), fmt.Sprintf(`<w:color w:val="%s"/>`, hexColor(m.Accent)), "")
	style("paragraph", "Header", "header", "Normal", "", sp(0, 0, m.Small*1.3), fmt.Sprintf(`<w:color w:val="%s"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, hexColor(m.Muted), halfPt(m.Small), halfPt(m.Small)), "")
	style("paragraph", "Footer", "footer", "Normal", "", `<w:jc w:val="right"/>`+sp(0, 0, m.Small*1.3), fmt.Sprintf(`<w:color w:val="%s"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, hexColor(m.Muted), halfPt(m.Small), halfPt(m.Small)), "")
	style("character", "Hyperlink", "Hyperlink", "", "", "", fmt.Sprintf(`<w:color w:val="%s"/><w:u w:val="single"/>`, hexColor(m.Accent)), "")
	b.WriteString(`<w:style w:type="table" w:default="1" w:styleId="TableNormal"><w:name w:val="Normal Table"/><w:tblPr><w:tblInd w:w="0" w:type="dxa"/><w:tblCellMar><w:top w:w="0" w:type="dxa"/><w:left w:w="108" w:type="dxa"/><w:bottom w:w="0" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>`)
	b.WriteString(`</w:styles>`)
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
