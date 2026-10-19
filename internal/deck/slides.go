package deck

import (
	"fmt"
	"math"
	"strings"

	"github.com/Anonyfox/lfm/internal/markup"
	"github.com/Anonyfox/lfm/internal/theme"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// Slide layouts: what a slide's content asks for, named as PowerPoint
// lists them, so a buyer finds the same layout for a new slide.
const (
	LayoutCover     = "cover"     // the first slide: title and subtitle on the accent
	LayoutSection   = "section"   // a # heading: a chapter divider
	LayoutContent   = "content"   // title and one body
	LayoutTwo       = "two"       // title and two columns
	LayoutPicture   = "picture"   // title and one picture
	LayoutStatement = "statement" // title and one quote or key message
)

// Slide is one slide of a deck.
type Slide struct {
	N      int // 1-based
	Line   int
	Layout string
	Title  []Inline
	Sub    []Inline // the cover's subtitle, a section's line
	Body   []Block
	Notes  []Block
	Quiz   *Block // the list of answers when the slide asks a question
}

// split cuts a deck's blocks at its slide breaks and gives each slide the
// layout its content asks for.
func split(blocks []Block) []Slide {
	var out []Slide
	var chunk []Block
	line := 1
	flush := func() {
		s := Slide{N: len(out) + 1, Line: line}
		for _, b := range chunk {
			if b.Kind == Notes {
				s.Notes = append(s.Notes, b.Children...)
				continue
			}
			s.Body = append(s.Body, b)
		}
		s.shape(len(out) == 0)
		out = append(out, s)
		chunk = nil
	}
	for _, b := range blocks {
		if b.Kind == PageBreak {
			flush()
			line = b.Line
			continue
		}
		if len(chunk) == 0 {
			line = b.Line
		}
		chunk = append(chunk, b)
	}
	flush()
	return out
}

// shape takes the slide's title from its first block and picks the layout.
func (s *Slide) shape(first bool) {
	body := s.Body
	level := 0
	if len(body) > 0 {
		switch b := body[0]; b.Kind {
		case Title:
			s.Title, level, body = b.Inlines, 1, body[1:]
			if len(body) > 0 && body[0].Kind == Subtitle {
				s.Sub, body = body[0].Inlines, body[1:]
			}
		case Heading:
			s.Title, level, body = b.Inlines, b.Level, body[1:]
		}
	}
	if level == 1 && len(body) <= 1 && (len(body) == 0 || body[0].Kind == Para) {
		if len(body) == 1 && s.Sub == nil {
			s.Sub = body[0].Inlines
		}
		s.Body, s.Layout = nil, LayoutSection
		if first {
			s.Layout = LayoutCover
		}
		return
	}
	s.Body = body
	s.Layout = LayoutContent
	switch {
	case len(body) == 1 && body[0].Kind == Columns && len(body[0].Cols) == 2:
		s.Layout = LayoutTwo
	case len(body) == 1 && body[0].Kind == ImageKind:
		s.Layout = LayoutPicture
	case len(body) == 1 && body[0].Kind == Box:
		s.Layout = LayoutStatement
	}
	for i, b := range body {
		if b.Kind == List && isQuiz(b) {
			s.Quiz = &s.Body[i]
		}
	}
}

// isQuiz reports whether a checklist asks a question: some answers are
// ticked as right, some not.
func isQuiz(b Block) bool {
	right, wrong := 0, 0
	for _, it := range b.Items {
		switch {
		case !it.Check:
			return false
		case it.Checked:
			right++
		default:
			wrong++
		}
	}
	return right > 0 && wrong > 0
}

// Answer is the letters of a quiz's right answers: "B" or "A, C".
func answer(b *Block) string {
	var out []string
	for i, it := range b.Items {
		if it.Checked {
			out = append(out, string(rune('A'+i)))
		}
	}
	return strings.Join(out, ", ")
}

// The slide is 16:9, 13.333 × 7.5 in: 960 × 540 pt.
const (
	slideW, slideH = 960.0, 540.0
	emuPerPt       = 12700
)

// box is a rectangle on a slide in points.
type box struct{ X, Y, W, H float64 }

// design is a theme laid out on a slide: sizes in points, the boxes of every
// layout. The .pptx, its print and the fit check all read it.
type design struct {
	theme.Theme
	text, heading                                                                                             *typeface.Family
	Body, Small, Head, Cover, Sub, Quote                                                                      float64 // sizes
	LineFactor, HeadFactor                                                                                    float64 // line height as a multiple of the size
	Pad                                                                                                       float64 // the inset of every text box
	TitleBox, BodyBox, CoverTitle, CoverSub, SectionTitle, SectionSub, Left, Right, StatementBox, Number, Bar box
}

// clamp keeps v within lo and hi.
func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func newDesign(t theme.Theme, text, heading *typeface.Family) design {
	m := t
	k := design{Theme: t, text: text, heading: heading}
	k.Body = theme.Half(clamp(m.Base*2.2, 20, 26))
	k.Small = theme.Half(math.Max(14, k.Body*0.75))
	k.Head = theme.Half(clamp(k.Body*m.Ratio*m.Ratio, k.Body+8, 40))
	k.Cover = theme.Half(clamp(k.Head*1.45, k.Head+8, 60))
	k.Sub = theme.Half(k.Body * 1.1)
	k.Quote = theme.Half(k.Body * 1.3)
	typo := func(f *typeface.Family) float64 {
		if f != nil && f.Regular() != nil && f.Regular().Typo > 0 {
			return f.Regular().Typo
		}
		return 1.2
	}
	// PowerPoint sets a single line at 1.2 times the size; Keynote and
	// LibreOffice take the font's typographic line. The fit check counts
	// the taller, so a slide that fits here fits in every app; titles are
	// set at 90 %.
	k.LineFactor = math.Max(1.2, typo(text))
	k.HeadFactor = math.Max(1.2, typo(heading)) * 0.9
	k.Pad = 7.2 // PowerPoint's 0.1 in inset
	x, w := 60.0, slideW-120
	// The title stands on its last line, the accent bar right below it,
	// so one-line and two-line titles meet the body alike.
	k.TitleBox = box{x, 30, w, 2*k.Head*k.HeadFactor + 2*k.Pad}
	k.Bar = box{x + k.Pad, k.TitleBox.Y + k.TitleBox.H + 4, 44, 4}
	top := k.TitleBox.Y + k.TitleBox.H + 22
	k.BodyBox = box{x, top, w, slideH - 52 - top}
	k.Left = box{x, top, (w - 36) / 2, k.BodyBox.H}
	k.Right = box{x + (w+36)/2, top, (w - 36) / 2, k.BodyBox.H}
	k.CoverTitle = box{72, 120, slideW - 144, 3*k.Cover*k.HeadFactor + 2*k.Pad}
	k.CoverSub = box{72, k.CoverTitle.Y + k.CoverTitle.H + 12, slideW - 144, 2*k.Sub*k.LineFactor + 2*k.Pad}
	k.SectionTitle = box{96, 170, slideW - 192, 2*k.Cover*0.85*k.HeadFactor + 2*k.Pad}
	k.SectionSub = box{96, k.SectionTitle.Y + k.SectionTitle.H + 8, slideW - 192, 2*k.Sub*k.LineFactor + 2*k.Pad}
	k.StatementBox = box{120, top + 10, slideW - 240, k.BodyBox.H - 20}
	k.Number = box{slideW - 60 - 80, slideH - 40, 80, 24}
	return k
}

// lines counts the lines runs wrap to in width points at size, breaking
// at spaces as apps do and never at a no-break space.
func (k design) lines(ins []Inline, f *typeface.Family, size, width float64, bold bool) int {
	n, x := 1, 0.0
	space := f.Face(false, false).Width(" ", size)
	for _, in := range ins {
		if in.Break {
			n++
			x = 0
		}
		face := f.Face(bold || in.Bold, in.Italic)
		for i, word := range strings.Split(in.Text, " ") {
			w := face.Width(word, size)
			need := w
			if i > 0 && x > 0 {
				need += space
			}
			if x > 0 && x+need > width+0.01 {
				n++
				x, need = 0, w
			}
			x += need
			for x > width+0.01 {
				// A word longer than the line breaks within it.
				n++
				x -= width
			}
		}
	}
	return n
}

// textHeight is how tall blocks of text set in width points stand: the
// lines of every paragraph and item, the space between them.
func (k design) textHeight(bs []Block, width, size float64) float64 {
	line := size * k.LineFactor
	gap := size * 0.5
	h := 0.0
	for i, b := range bs {
		if i > 0 {
			h += gap
		}
		switch b.Kind {
		case Para:
			h += float64(k.lines(b.Inlines, k.text, size, width, false)) * line
		case Heading:
			h += float64(k.lines(b.Inlines, k.heading, size, width, true)) * line
		case List:
			for j, it := range b.Items {
				if j > 0 {
					h += gap * 0.6
				}
				indent := k.indent(size) * float64(it.Level+1)
				h += float64(k.lines(it.Inlines, k.text, size, width-indent, false)) * line
			}
		case Box:
			h += k.textHeight(b.Children, width-2*k.Pad-6, size) + 2*k.Pad
		case TableKind:
			h += k.tableHeight(b.Table, width)
		case ImageKind:
			h += 2 * line // an image takes what is left, at least two lines
		}
	}
	return h
}

// indent is a list level's indent at size.
func (k design) indent(size float64) float64 { return math.Round(size * 1.15) }

// tableSize is the text size of a slide's tables.
func (k design) tableSize() float64 { return theme.Half(k.Body * 0.8) }

// tableHeight is a table's height in width points.
func (k design) tableHeight(t *Table, width float64) float64 {
	size := k.tableSize()
	ws := markup.Widths(t)
	row := func(cells [][]Inline, bold bool) float64 {
		n := 1
		for i, c := range cells {
			if i < len(ws) {
				n = max(n, k.lines(c, k.text, size, ws[i]*width-2*k.Pad, bold))
			}
		}
		return float64(n)*size*k.LineFactor + 2*k.Pad*0.6
	}
	h := row(t.Head, true)
	for _, r := range t.Rows {
		h += row(r, false)
	}
	return h
}

// fit is the room a slide's content needs and has, in points; content
// that needs more is cut off or spills in every app.
func (k design) fit(s Slide) (need, have float64, where string) {
	titleW := k.TitleBox.W - 2*k.Pad
	switch s.Layout {
	case LayoutCover:
		if n := k.lines(s.Title, k.heading, k.Cover, k.CoverTitle.W-2*k.Pad, k.HeadingBold); n > 3 {
			return float64(n), 3, "the title"
		}
		return 0, 0, ""
	case LayoutSection:
		if n := k.lines(s.Title, k.heading, k.Cover*0.85, k.SectionTitle.W-2*k.Pad, k.HeadingBold); n > 2 {
			return float64(n), 2, "the title"
		}
		return 0, 0, ""
	}
	if n := k.lines(s.Title, k.heading, k.Head, titleW, k.HeadingBold); n > 2 {
		return float64(n), 2, "the title"
	}
	switch s.Layout {
	case LayoutTwo:
		for i, col := range s.Body[0].Cols {
			b := []box{k.Left, k.Right}[i]
			if h := k.textHeight(col, b.W-2*k.Pad, k.Body); h > b.H-2*k.Pad {
				return h, b.H - 2*k.Pad, []string{"the left column", "the right column"}[i]
			}
		}
	case LayoutStatement:
		if h := k.textHeight(s.Body[0].Children, k.StatementBox.W-2*k.Pad, k.Quote); h > k.StatementBox.H-2*k.Pad {
			return h, k.StatementBox.H - 2*k.Pad, "the statement"
		}
	case LayoutContent:
		if h := k.textHeight(s.Body, k.BodyBox.W-2*k.Pad, k.Body); h > k.BodyBox.H-2*k.Pad {
			return h, k.BodyBox.H - 2*k.Pad, "the body"
		}
	}
	return 0, 0, ""
}

// wordCount counts the words a slide shows, notes aside.
func wordCount(bs []Block) int {
	n := 0
	for _, b := range bs {
		n += len(strings.Fields(plain(b.Inlines)))
		for _, it := range b.Items {
			n += len(strings.Fields(plain(it.Inlines)))
		}
		n += wordCount(b.Children)
		for _, c := range b.Cols {
			n += wordCount(c)
		}
		if t := b.Table; t != nil {
			for _, c := range t.Head {
				n += len(strings.Fields(plain(c)))
			}
			for _, r := range t.Rows {
				for _, c := range r {
					n += len(strings.Fields(plain(c)))
				}
			}
		}
	}
	return n
}

// slideProblems are what the deck's slides cannot be: errors for content
// that has no place on a slide, warnings for slides that hold too much,
// lack a title or notes, and quizzes without a clear answer.
func (r *Report) slideProblems(d *Deck, errorAt, warn func(string, int, string, string, string)) {
	k := r.design()
	ss := d.Slides
	if d.Planned > 0 && len(ss) != d.Planned {
		warn(d.File, 1, RuleDesign, fmt.Sprintf("The deck has %d slides; its front matter plans %d.", len(ss), d.Planned), "Change slides to the count you mean, or leave it out.")
	}
	notes := 0
	walk := func(s Slide, bs []Block) {
		for _, b := range bs {
			switch b.Kind {
			case Form, Signature, Ornament, Title, Subtitle:
				errorAt(d.File, b.Line, RuleSource, fmt.Sprintf("Slide %d holds a %s, which has no place on a slide.", s.N, b.Kind), "Use headings, text, lists, tables, images, quotes and ::: columns on slides; ::: title only opens the first slide.")
			case Columns:
				if len(b.Cols) != 2 {
					errorAt(d.File, b.Line, RuleSource, fmt.Sprintf("Slide %d has %d columns.", s.N, len(b.Cols)), "A slide holds two columns: ::: columns with one +++.")
				}
				if len(bs) > 1 {
					errorAt(d.File, b.Line, RuleSource, fmt.Sprintf("Slide %d has columns and more content.", s.N), "Give the columns a slide of their own, with its title above them.")
				}
				for _, c := range b.Cols {
					for _, cb := range c {
						if cb.Kind != Para && cb.Kind != List && cb.Kind != Heading {
							errorAt(d.File, cb.Line, RuleSource, fmt.Sprintf("A column on slide %d holds a %s.", s.N, cb.Kind), "Columns on slides hold text, headings and lists.")
						}
					}
				}
			case ImageKind:
				if len(bs) > 1 && s.Layout != LayoutPicture {
					errorAt(d.File, b.Line, RuleSource, fmt.Sprintf("Slide %d has a picture and more content.", s.N), "Give a picture a slide of its own, with its title; say more in the notes.")
				}
			case TableKind:
				if len(b.Table.Align) > 6 || len(b.Table.Rows) > 8 {
					warn(d.File, b.Line, RuleDesign, fmt.Sprintf("Slide %d has a table of %d columns and %d rows.", s.N, len(b.Table.Align), len(b.Table.Rows)), "Keep tables on slides to six columns and eight rows; the rest belongs in a handout.")
				}
			case Heading:
				if b.Level < 3 {
					errorAt(d.File, b.Line, RuleSource, fmt.Sprintf("Slide %d has a second # or ## heading.", s.N), "Start a new slide with --- for a new title; ### sets a subheading inside a slide.")
				}
			}
		}
	}
	for _, s := range ss {
		file, line := d.File, s.Line
		walk(s, s.Body)
		switch {
		case len(s.Title) == 0 && len(s.Body) == 0:
			errorAt(file, line, RuleSource, fmt.Sprintf("Slide %d is empty.", s.N), "Remove the extra --- or write the slide.")
			continue
		case len(s.Title) == 0:
			warn(file, line, RuleDesign, fmt.Sprintf("Slide %d has no title.", s.N), "Start every slide with a ## heading: screen readers, the outline and the buyer's slide list name slides by it.")
		}
		if s.N > 1 && s.Layout == LayoutCover {
			continue
		}
		if need, have, where := k.fit(s); need > have {
			if where == "the title" {
				warn(file, line, RuleDesign, fmt.Sprintf("Slide %d: the title wraps to %.0f lines; the slide has room for %.0f.", s.N, need, have), "Shorten the title; say the rest on the slide or in the notes.")
			} else {
				warn(file, line, RuleDesign, fmt.Sprintf("Slide %d: %s needs %.0f pt of a %.0f pt box and would run off the slide.", s.N, where, need, have), "Cut the text to what the audience must read, move detail into ::: notes, or split the slide.")
			}
		}
		if n := wordCount(s.Body); n > 60 {
			warn(file, line, RuleDesign, fmt.Sprintf("Slide %d shows %d words.", s.N, n), "A slide the audience reads is a document; keep it under 60 words and say the rest from the notes.")
		}
		for _, b := range s.Body {
			if b.Kind == List && len(b.Items) > 7 {
				warn(file, b.Line, RuleDesign, fmt.Sprintf("Slide %d lists %d points.", s.N, len(b.Items)), "Keep a slide to seven points or fewer; split it.")
			}
		}
		if q := s.Quiz; q != nil && len(q.Items) > 6 {
			warn(file, q.Line, RuleContent, fmt.Sprintf("The question on slide %d has %d answers.", s.N, len(q.Items)), "Offer two to six answers.")
		}
		if len(s.Notes) > 0 {
			notes += wordCount(s.Notes)
		} else if d.Minutes > 0 && s.Layout != LayoutCover && s.Layout != LayoutSection {
			warn(file, line, RuleContent, fmt.Sprintf("Slide %d has no speaker notes.", s.N), "Add ::: notes with what the presenter says; a deck sold to present is bought for its notes.")
		}
	}
	// People speak about 130 words a minute. Questions, a quiz and talk
	// fill much of a briefing, but notes for less than a fifth of its time
	// leave the presenter without words.
	if d.Minutes > 0 {
		if spoken := float64(notes) / 130; spoken < float64(d.Minutes)*0.2 {
			warn(d.File, 1, RuleContent, fmt.Sprintf("The notes carry about %.0f minutes of speech for a %d minute deck.", spoken, d.Minutes), "Write what the presenter says and asks, about 130 words a minute, or plan fewer minutes.")
		}
		if per := float64(d.Minutes) / float64(len(ss)); per < 0.5 {
			warn(d.File, 1, RuleDesign, fmt.Sprintf("%d slides in %d minutes leave %.0f seconds a slide.", len(ss), d.Minutes, per*60), "Plan at least half a minute a slide; merge slides or plan more time.")
		}
	}
}
