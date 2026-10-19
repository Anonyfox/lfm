// Package markup is the Markdown every editable deliverable is written
// in: front matter, blocks, the ::: directives, ==fields== the buyer
// replaces, and the language's typography, so authors type plain text and
// documents and decks read the same source the same way.
package markup

import (
	"bytes"
	"fmt"
	"html"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Block kinds.
const (
	Para      = "p"
	Heading   = "h" // Level 1-3
	Title     = "title"
	Subtitle  = "subtitle"
	List      = "list"
	TableKind = "table"
	ImageKind = "image"
	PageBreak = "break"
	Ornament  = "ornament"
	Box       = "box"
	Columns   = "columns"
	Form      = "form"
	Signature = "signature"
	Notes     = "notes" // a slide's speaker notes
)

// Block is one block of a document.
type Block struct {
	Kind     string
	Level    int // heading level
	Line     int // source line
	Center   bool
	Inlines  []Inline
	Items    []Item // list
	Ordered  bool
	Table    *Table
	Image    *Figure
	Children []Block   // box
	Cols     [][]Block // columns
	Widths   []float64 // column shares
	Fields   []Field   // form, signature
}

// Inline is a run of text with one look.
type Inline struct {
	Text         string
	Bold, Italic bool
	Fill         bool   // a field the buyer replaces: ==…==, highlighted
	Link         string // a hyperlink target
	Break        bool   // a line break before the text
}

// Item is one list item.
type Item struct {
	Level   int // 0-2
	Check   bool
	Checked bool
	Inlines []Inline
}

// Table is a table with a header row.
type Table struct {
	Align []string // left, center, right or "" (numbers right, text left) per column
	Head  [][]Inline
	Rows  [][][]Inline
}

// Figure is an inline picture.
type Figure struct {
	Src, Alt string
	Width    float64 // mm; 0 fits the text width
	Share    float64 // of the text width when given in %
}

// Field is a form row or a signature line.
type Field struct {
	Label []Inline
	Value []Inline
}

var (
	directive  = regexp.MustCompile(`^:::\s*([a-z]+)\s*(.*)$`)
	rule       = regexp.MustCompile(`^ {0,3}(?:-(?:[ \t]*-){2,}|\*(?:[ \t]*\*){2,}|_(?:[ \t]*_){2,})[ \t]*$`) // a thematic break line
	FieldMark  = regexp.MustCompile(`==([^=\n]+)==`)                                                          // a field in plain text
	widthTitle = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(mm|%)$`)
)

// Problem is a defect in a source file, by line.
type Problem struct {
	Line          int
	Message, Hint string
}

// quotes are the typographic substitutions per language.
func quotes(lang string) map[extension.TypographicPunctuation]string {
	base, _, _ := strings.Cut(lang, "-")
	q := map[extension.TypographicPunctuation]string{
		extension.LeftDoubleQuote: "“", extension.RightDoubleQuote: "”", extension.LeftSingleQuote: "‘", extension.RightSingleQuote: "’",
		extension.Apostrophe: "’", extension.EnDash: "–", extension.EmDash: "—", extension.Ellipsis: "…",
		extension.LeftAngleQuote: "«", extension.RightAngleQuote: "»",
	}
	switch base {
	case "de", "cs", "sk", "da":
		q[extension.LeftDoubleQuote], q[extension.RightDoubleQuote] = "„", "“"
		q[extension.LeftSingleQuote], q[extension.RightSingleQuote] = "‚", "‘"
	case "fr":
		q[extension.LeftDoubleQuote], q[extension.RightDoubleQuote] = "« ", " »"
	case "pl", "nl", "hu", "ro":
		q[extension.LeftDoubleQuote], q[extension.RightDoubleQuote] = "„", "”"
	}
	return q
}

// Setting is one front matter line.
type Setting struct {
	Value string
	Line  int
}

// Keys are the settings' keys in the order of their lines.
func Keys(settings map[string]Setting) []string {
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int { return settings[a].Line - settings[b].Line })
	return keys
}

// Lines splits a source file into lines.
// A UTF-8 byte order mark, which some editors write, is dropped, so front
// matter and a first heading still start the file.
func Lines(src []byte) []string {
	src = bytes.TrimPrefix(src, []byte("\ufeff"))
	return strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
}

// FrontMatter reads the settings between the leading --- lines by their
// lowercase keys, and where the body starts.
func FrontMatter(lines []string) (map[string]Setting, int, []Problem) {
	out := map[string]Setting{}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out, 0, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return out, len(lines), []Problem{{1, "The front matter has no closing ---.", "End the settings with a line of three hyphens."}}
	}
	var problems []Problem
	for i := 1; i < end; i++ {
		k, v, ok := strings.Cut(lines[i], ":")
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.Trim(strings.TrimSpace(v), `"`)
		if !ok || k == "" {
			if strings.TrimSpace(lines[i]) != "" {
				problems = append(problems, Problem{i + 1, fmt.Sprintf("Front matter line %q is not key: value.", lines[i]), "Write each setting as key: value."})
			}
			continue
		}
		out[k] = Setting{v, i + 1}
	}
	return out, end + 1, problems
}

// Options are what a deliverable's sources may hold.
type Options struct {
	Lang       string
	Directives []string // the ::: directives allowed, such as title, columns, notes
	Slides     bool     // a line --- starts the next slide
}

// Parse reads the body of a source, lines starting at source line
// offset+1, into blocks.
func Parse(lines []string, offset int, o Options) ([]Block, []Problem) {
	var problems []Problem
	add := func(line int, message, hint string) { problems = append(problems, Problem{line, message, hint}) }
	md := goldmark.New(goldmark.WithExtensions(extension.Table, extension.TaskList, extension.NewTypographer(extension.WithTypographicSubstitutions(quotes(o.Lang)))))
	p := parsing{md: md, o: o, add: add}
	if o.Slides {
		return p.slides(lines, offset), problems
	}
	return p.blocks(lines, offset), problems
}

type parsing struct {
	md  goldmark.Markdown
	o   Options
	add func(int, string, string)
}

func (p parsing) allowed(name string) bool { return slices.Contains(p.o.Directives, name) }

// parseBlocks parses lines, which start at source line offset+1, into
// blocks: directives by hand, everything between them with goldmark.
func (p parsing) blocks(lines []string, offset int) []Block {
	md, add := p.md, p.add
	var out []Block
	flush := func(from, to int) {
		if from >= to {
			return
		}
		out = append(out, markdownBlocks(md, lines[from:to], offset+from, add)...)
	}
	from := 0
	for i := 0; i < len(lines); i++ {
		m := directive.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			continue
		}
		flush(from, i)
		depth, end := 1, -1
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if t == ":::" {
				depth--
				if depth == 0 {
					end = j
					break
				}
			} else if directive.MatchString(t) {
				depth++
			}
		}
		line := offset + i + 1
		if end < 0 {
			add(line, fmt.Sprintf("::: %s has no closing :::.", m[1]), "Close every directive with a line of three colons.")
			return out
		}
		inner := lines[i+1 : end]
		innerOffset := offset + i + 1
		if !p.allowed(m[1]) {
			names := make([]string, len(p.o.Directives))
			for i, d := range p.o.Directives {
				names[i] = "::: " + d
			}
			add(line, fmt.Sprintf("Directive ::: %s is unknown here.", m[1]), "Use "+strings.Join(names, ", ")+".")
			i, from = end, end+1
			continue
		}
		switch m[1] {
		case "title":
			// The first line is the title, a second the subtitle.
			n := 0
			for j, l := range inner {
				if strings.TrimSpace(l) == "" {
					continue
				}
				if n >= 2 {
					add(innerOffset+j+1, "A title holds one line of title and one optional line of subtitle.", "Put further text after the ::: title block.")
					break
				}
				out = append(out, Block{Kind: []string{Title, Subtitle}[n], Line: innerOffset + j + 1, Inlines: inlineText(md, strings.TrimSpace(l), innerOffset+j+1, add)})
				n++
			}
		case "center":
			for _, b := range p.blocks(inner, innerOffset) {
				b.Center = true
				out = append(out, b)
			}
		case "columns":
			b := Block{Kind: Columns, Line: line}
			for _, w := range strings.Fields(m[2]) {
				v, err := strconv.ParseFloat(w, 64)
				if err != nil || v <= 0 {
					add(line, fmt.Sprintf("Column share %q is not a number.", w), "Give the columns' widths as shares: ::: columns 1 2 makes the second twice as wide.")
					continue
				}
				b.Widths = append(b.Widths, v)
			}
			cell, cellStart := []string{}, innerOffset
			for j, l := range inner {
				if strings.TrimSpace(l) == "+++" {
					b.Cols = append(b.Cols, p.blocks(cell, cellStart))
					cell, cellStart = nil, innerOffset+j+1
					continue
				}
				cell = append(cell, l)
			}
			b.Cols = append(b.Cols, p.blocks(cell, cellStart))
			if len(b.Cols) < 2 || len(b.Cols) > 3 {
				add(line, fmt.Sprintf("Columns hold %d columns.", len(b.Cols)), "Separate two or three columns with a line +++.")
			}
			if len(b.Widths) == 0 {
				for range b.Cols {
					b.Widths = append(b.Widths, 1)
				}
			}
			if len(b.Widths) != len(b.Cols) {
				add(line, fmt.Sprintf("Columns name %d widths for %d columns.", len(b.Widths), len(b.Cols)), "Give one share per column, or none for equal columns.")
				b.Widths = nil
				for range b.Cols {
					b.Widths = append(b.Widths, 1)
				}
			}
			out = append(out, b)
		case "form", "signature":
			b := Block{Kind: Form, Line: line}
			if m[1] == "signature" {
				b.Kind = Signature
			}
			for j, l := range inner {
				if strings.TrimSpace(l) == "" {
					continue
				}
				label, value, _ := strings.Cut(l, ":")
				if b.Kind == Signature {
					label, value = l, ""
				}
				f := Field{Label: inlineText(md, strings.TrimSpace(label), innerOffset+j+1, add), Value: inlineText(md, strings.TrimSpace(value), innerOffset+j+1, add)}
				b.Fields = append(b.Fields, f)
			}
			if len(b.Fields) == 0 {
				add(line, "::: "+m[1]+" has no lines.", "Write one field per line: Label: sample value.")
			}
			if b.Kind == Signature && len(b.Fields) > 3 {
				add(line, "A signature row holds at most three lines.", "Start another ::: signature for more signers.")
			}
			out = append(out, b)
		case "notes":
			out = append(out, Block{Kind: Notes, Line: line, Children: p.blocks(inner, innerOffset)})
		default:
			add(line, fmt.Sprintf("Directive ::: %s is unknown.", m[1]), "Use one of the directives the deliverable's skill lists.")
		}
		i, from = end, end+1
	}
	flush(from, len(lines))
	return out
}

// inlineText parses one line of Markdown as inlines.
func inlineText(md goldmark.Markdown, s string, line int, add func(int, string, string)) []Inline {
	if s == "" {
		return nil
	}
	blocks := markdownBlocks(md, []string{s}, line-1, add)
	if len(blocks) == 1 && blocks[0].Kind == Para {
		return blocks[0].Inlines
	}
	return []Inline{{Text: s}}
}

func markdownBlocks(md goldmark.Markdown, lines []string, offset int, add func(int, string, string)) []Block {
	src := []byte(strings.Join(lines, "\n"))
	root := md.Parser().Parse(text.NewReader(src), parser.WithContext(parser.NewContext()))
	lineOf := func(n ast.Node) int {
		for c := n; c != nil; c = c.FirstChild() {
			if c.Type() == ast.TypeBlock && c.Lines().Len() > 0 {
				return offset + 1 + bytes.Count(src[:c.Lines().At(0).Start], []byte("\n"))
			}
		}
		return offset + 1
	}
	// A thematic break records no position. It is the last rule-shaped line
	// before the block after it, so its siblings, read backwards, place it.
	breaks := func(parent ast.Node) map[ast.Node]int {
		var nodes []ast.Node
		for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
			nodes = append(nodes, n)
		}
		at := map[ast.Node]int{}
		upper := len(lines)
		for j := len(nodes) - 1; j >= 0; j-- {
			if _, ok := nodes[j].(*ast.ThematicBreak); !ok {
				upper = min(upper, lineOf(nodes[j])-offset-1)
				continue
			}
			for i := upper - 1; i >= 0; i-- {
				if rule.MatchString(lines[i]) {
					at[nodes[j]], upper = i, i
					break
				}
			}
		}
		return at
	}
	var convert func(parent ast.Node) []Block
	convert = func(parent ast.Node) []Block {
		var out []Block
		breakAt := breaks(parent)
		for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
			line := lineOf(n)
			switch x := n.(type) {
			case *ast.Heading:
				if x.Level > 3 {
					add(line, fmt.Sprintf("Heading level %d is too deep.", x.Level), "Use at most three heading levels: #, ## and ###.")
				}
				out = append(out, Block{Kind: Heading, Level: min(x.Level, 3), Line: line, Inlines: inlines(x, src, line, add)})
			case *ast.Paragraph, *ast.TextBlock:
				if img := soleImage(n, src); img != nil {
					out = append(out, Block{Kind: ImageKind, Line: line, Image: img})
					continue
				}
				out = append(out, Block{Kind: Para, Line: line, Inlines: inlines(n, src, line, add)})
			case *ast.ThematicBreak:
				// --- is a page break, * * * and ___ an ornament.
				kind := Ornament
				if i, ok := breakAt[n]; ok {
					line = offset + i + 1
					if strings.HasPrefix(strings.TrimSpace(lines[i]), "-") {
						kind = PageBreak
					}
				}
				out = append(out, Block{Kind: kind, Line: line})
			case *ast.List:
				b := Block{Kind: List, Line: line, Ordered: x.IsOrdered()}
				listItems(x, 0, src, line, &b, add)
				out = append(out, b)
			case *ast.Blockquote:
				out = append(out, Block{Kind: Box, Line: line, Children: convert(x)})
			case *east.Table:
				t := &Table{}
				for _, a := range x.Alignments {
					switch a {
					case east.AlignRight:
						t.Align = append(t.Align, "right")
					case east.AlignCenter:
						t.Align = append(t.Align, "center")
					case east.AlignLeft:
						t.Align = append(t.Align, "left")
					default:
						t.Align = append(t.Align, "") // decided by the column's content
					}
				}
				for r := x.FirstChild(); r != nil; r = r.NextSibling() {
					var row [][]Inline
					for c := r.FirstChild(); c != nil; c = c.NextSibling() {
						row = append(row, inlines(c, src, line, add))
					}
					if _, head := r.(*east.TableHeader); head {
						t.Head = row
					} else {
						t.Rows = append(t.Rows, row)
					}
				}
				out = append(out, Block{Kind: TableKind, Line: line, Table: t})
			case *ast.FencedCodeBlock, *ast.CodeBlock:
				add(line, "Code blocks have no place in an editable file.", "Write the text as paragraphs; a table lays out structured text.")
			case *ast.HTMLBlock:
				add(line, "Raw HTML does not reach Office, Google or Apple apps.", "Use Markdown, a table or a ::: directive.")
			default:
				add(line, fmt.Sprintf("%s is not supported.", n.Kind()), "Use paragraphs, headings, lists, tables, images, quotes (boxes) and ::: directives.")
			}
		}
		return out
	}
	return convert(root)
}

func soleImage(n ast.Node, src []byte) *Figure {
	img, ok := n.FirstChild().(*ast.Image)
	if !ok || img.NextSibling() != nil {
		return nil
	}
	out := &Figure{Src: string(img.Destination), Alt: nodeText(img, src)}
	if m := widthTitle.FindStringSubmatch(strings.TrimSpace(string(img.Title))); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		if m[2] == "%" {
			out.Share = v / 100
		} else {
			out.Width = v
		}
	}
	return out
}

func listItems(l *ast.List, level int, src []byte, line int, b *Block, add func(int, string, string)) {
	if level > 2 {
		add(line, "Lists nest at most three levels.", "Flatten the list or make a heading of the outer level.")
		return
	}
	for li := l.FirstChild(); li != nil; li = li.NextSibling() {
		item := Item{Level: level}
		var nested []*ast.List
		for c := li.FirstChild(); c != nil; c = c.NextSibling() {
			switch x := c.(type) {
			case *ast.List:
				nested = append(nested, x)
			case *ast.Paragraph, *ast.TextBlock:
				if cb, ok := x.FirstChild().(*east.TaskCheckBox); ok {
					item.Check, item.Checked = true, cb.IsChecked
				}
				if item.Inlines != nil {
					item.Inlines = append(item.Inlines, Inline{Break: true})
				}
				item.Inlines = append(item.Inlines, inlines(x, src, line, add)...)
			}
		}
		b.Items = append(b.Items, item)
		for _, n := range nested {
			listItems(n, level+1, src, line, b, add)
		}
	}
}

// inlines flattens a node's inline children into runs.
func inlines(n ast.Node, src []byte, line int, add func(int, string, string)) []Inline {
	var out []Inline
	var walk func(n ast.Node, style Inline)
	walk = func(n ast.Node, style Inline) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch x := c.(type) {
			case *ast.Text:
				in := style
				in.Text = string(x.Segment.Value(src))
				out = append(out, in)
				// A new line in the source is a new line in the document, as
				// addresses and schedules need; paragraphs are separated by
				// a blank line.
				if x.HardLineBreak() || x.SoftLineBreak() {
					out = append(out, Inline{Break: true})
				}
			case *ast.String:
				in := style
				in.Text = string(x.Value)
				out = append(out, in)
			case *ast.Emphasis:
				s := style
				if x.Level >= 2 {
					s.Bold = true
				} else {
					s.Italic = true
				}
				walk(x, s)
			case *ast.Link:
				s := style
				s.Link = string(x.Destination)
				walk(x, s)
			case *ast.AutoLink:
				s := style
				s.Link = string(x.URL(src))
				s.Text = string(x.Label(src))
				out = append(out, s)
			case *ast.CodeSpan:
				in := style
				in.Text = nodeText(x, src)
				out = append(out, in)
			case *east.TaskCheckBox:
			case *ast.Image:
				add(line, "Images go on a line of their own.", "Put the image in its own paragraph; text flows above and below it.")
			case *ast.RawHTML:
				add(line, "Raw HTML does not reach Office, Google or Apple apps.", "Use Markdown: **bold**, *italic*, [link](https://…).")
			default:
				walk(c, style)
			}
		}
	}
	walk(n, Inline{})
	out = Fields(merge(out))
	for i := range out {
		out[i].Text = Tie(out[i].Text)
	}
	return out
}

// ties keep together what must not break across lines: a number and its
// unit or currency, abbreviations such as e. V. or z. B., § 14, No. 5 and
// dates such as 1. Dezember.
var ties = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`(\d) (€|%|‰|\$|£|¥|kg|g|km|cm|mm|m|ml|l|Uhr|pm|am|Std\.|min|h|EUR|USD|GBP|CHF)([^\p{L}\d]|$)`), "$1\u00a0$2$3"},
	{regexp.MustCompile(`(^|[\s(])(\p{L}\.) (\p{L}\.)`), "$1$2\u00a0$3"},
	{regexp.MustCompile(`(§|Nr\.|No\.|S\.|p\.|Abs\.|Art\.) (\d)`), "$1\u00a0$2"},
	{regexp.MustCompile(`(^|\s)(\d{1,2}\.) (\p{Lu})`), "$1$2\u00a0$3"},
}

// Tie keeps together what must not break across lines.
func Tie(s string) string {
	if !strings.Contains(s, " ") {
		return s
	}
	for _, t := range ties {
		s = t.re.ReplaceAllString(s, t.with)
	}
	return s
}

// merge joins neighbouring runs of the same look.
func merge(in []Inline) []Inline {
	var out []Inline
	for _, x := range in {
		if n := len(out); n > 0 && !x.Break && !out[n-1].Break && x.Bold == out[n-1].Bold && x.Italic == out[n-1].Italic && x.Link == out[n-1].Link && x.Fill == out[n-1].Fill {
			out[n-1].Text += x.Text
			continue
		}
		out = append(out, x)
	}
	return out
}

// Fields splits ==…== out of the runs as fields the buyer replaces.
func Fields(in []Inline) []Inline {
	var out []Inline
	for _, x := range in {
		if x.Break || !strings.Contains(x.Text, "==") {
			out = append(out, x)
			continue
		}
		rest := x.Text
		for _, m := range FieldMark.FindAllStringSubmatchIndex(x.Text, -1) {
			before := x.Text[len(x.Text)-len(rest) : m[0]]
			if before != "" {
				y := x
				y.Text = before
				out = append(out, y)
			}
			y := x
			y.Text, y.Fill = x.Text[m[2]:m[3]], true
			out = append(out, y)
			rest = x.Text[m[1]:]
		}
		if rest != "" {
			y := x
			y.Text = rest
			out = append(out, y)
		}
	}
	return out
}

// nodeText is the text an inline node holds, such as an image's alt text.
func nodeText(n ast.Node, src []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch x := c.(type) {
		case *ast.Text:
			b.Write(x.Segment.Value(src))
		case *ast.String:
			b.Write(x.Value)
		default:
			b.WriteString(nodeText(c, src))
		}
	}
	return b.String()
}

// Plain is the text of runs.
func Plain(in []Inline) string {
	var b strings.Builder
	for _, x := range in {
		if x.Break {
			b.WriteByte('\n')
		}
		b.WriteString(x.Text)
	}
	return b.String()
}

// slideBlocks parses a deck: a line --- outside ::: blocks starts the next
// slide, even right after text, where Markdown would read a heading.
func (p parsing) slides(lines []string, offset int) []Block {
	var out []Block
	from, depth := 0, 0
	for i := 0; i <= len(lines); i++ {
		if i < len(lines) {
			t := strings.TrimSpace(lines[i])
			switch {
			case directive.MatchString(t):
				depth++
				continue
			case t == ":::" && depth > 0:
				depth--
				continue
			case t != "---" || depth > 0:
				continue
			}
		}
		if from > 0 {
			out = append(out, Block{Kind: PageBreak, Line: offset + from})
		}
		out = append(out, p.blocks(lines[from:i], offset+from)...)
		from = i + 1
	}
	return out
}

var numberCell = regexp.MustCompile(`^[\s(+\-−]*[$€£¥]?\s*[\d.,'  ]*\d[\d.,'  ]*\s*(%|[$€£¥]|[A-Za-z]{1,3})?[)]?$`)

// Numeric reports whether a column holds figures in every filled body cell.
func Numeric(t *Table, col int) bool {
	seen := false
	for _, row := range t.Rows {
		if col >= len(row) {
			continue
		}
		s := strings.TrimSpace(Plain(row[col]))
		if s == "" {
			continue
		}
		if !numberCell.MatchString(s) {
			return false
		}
		seen = true
	}
	return seen
}

// Aligns resolves a table's column alignments: as written, or numbers right.
func Aligns(t *Table) []string {
	out := make([]string, len(t.Align))
	for i, a := range t.Align {
		out[i] = a
		if a == "" {
			out[i] = "left"
			if Numeric(t, i) {
				out[i] = "right"
			}
		}
	}
	return out
}

// Widths shares a table's width by its columns' longest words and lines:
// short figures get narrow columns, prose wide ones.
func Widths(t *Table) []float64 {
	n := len(t.Align)
	w := make([]float64, n)
	measure := func(row [][]Inline) {
		for i := 0; i < n && i < len(row); i++ {
			l := float64(utf8.RuneCountInString(Plain(row[i])))
			w[i] = math.Max(w[i], math.Min(l, 36))
		}
	}
	measure(t.Head)
	for _, r := range t.Rows {
		measure(r)
	}
	total := 0.0
	for i := range w {
		w[i] = math.Max(w[i], 6) + 3
		total += w[i]
	}
	for i := range w {
		w[i] /= total
	}
	return w
}

// HTML writes runs as HTML: bold, italic, fields and links.
func HTML(b *strings.Builder, ins []Inline) {
	for _, in := range ins {
		if in.Break {
			b.WriteString("<br>")
		}
		t := html.EscapeString(in.Text)
		if in.Bold {
			t = "<strong>" + t + "</strong>"
		}
		if in.Italic {
			t = "<em>" + t + "</em>"
		}
		if in.Fill {
			t = `<span class="fill">` + t + `</span>`
		}
		if in.Link != "" {
			t = fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(in.Link), t)
		}
		b.WriteString(t)
	}
}
