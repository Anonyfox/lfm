package document

import (
	"fmt"
	"strconv"

	"github.com/Anonyfox/lfm/internal/markup"
)

// The Markdown model documents share with every editable deliverable.
type (
	Block  = markup.Block
	Inline = markup.Inline
	Item   = markup.Item
	Table  = markup.Table
	Figure = markup.Figure
	Field  = markup.Field
)

// Block kinds.
const (
	Para      = markup.Para
	Heading   = markup.Heading
	Title     = markup.Title
	Subtitle  = markup.Subtitle
	List      = markup.List
	TableKind = markup.TableKind
	ImageKind = markup.ImageKind
	PageBreak = markup.PageBreak
	Ornament  = markup.Ornament
	Box       = markup.Box
	Columns   = markup.Columns
	Form      = markup.Form
	Signature = markup.Signature
)

// Doc is one document of one language, as its Markdown file describes it.
type Doc struct {
	Key, Lang  string
	File       string // path below the project
	Title      string
	Pages      int  // the pages it must print on; 0 when it may grow
	Ceremonial bool // centered layouts allowed: programs, invitations, certificates
	Cover      bool // the first page has no header or footer
	Header     string
	Footer     string // {page} and {pages} fill in; "none" for no footer
	Blocks     []Block
}

// directives are the ::: blocks a document may hold.
var directives = []string{"title", "columns", "form", "signature", "center"}

type problem = markup.Problem

// parseDoc reads a document's Markdown: front matter, directives and the
// Markdown itself. Quotes, dashes and ellipses become the language's
// typographic ones, so authors type plain text.
func parseDoc(key, lang, file string, src []byte) (*Doc, []problem) {
	d := &Doc{Key: key, Lang: lang, File: file}
	lines := markup.Lines(src)
	settings, start, problems := markup.FrontMatter(lines)
	add := func(line int, message, hint string) {
		problems = append(problems, problem{Line: line, Message: message, Hint: hint})
	}
	for _, k := range markup.Keys(settings) {
		s := settings[k]
		switch k {
		case "title":
			d.Title = s.Value
		case "pages":
			n, err := strconv.Atoi(s.Value)
			if err != nil || n < 1 || n > 500 {
				add(s.Line, fmt.Sprintf("pages %q is not a page count.", s.Value), "Give the exact pages the document prints on, such as 1, or leave pages out when it may grow.")
			}
			d.Pages = n
		case "kind":
			switch s.Value {
			case "ceremonial":
				d.Ceremonial = true
			case "flow", "":
			case "slides":
				add(s.Line, "Slide decks are their own deliverable.", "Move the deck to deliverables/deck; 'lfm skills deck' shows how.")
			default:
				add(s.Line, fmt.Sprintf("kind %q is unknown.", s.Value), "Use flow (the default) or ceremonial for programs, invitations and certificates, which may center their text.")
			}
		case "cover":
			d.Cover = s.Value == "true" || s.Value == "yes"
		case "header":
			d.Header = s.Value
		case "footer":
			d.Footer = s.Value
		default:
			add(s.Line, fmt.Sprintf("Front matter key %q is unknown.", k), "Use title, pages, kind, cover, header or footer.")
		}
	}
	if d.Title == "" {
		add(1, "The document has no title.", "Start the file with front matter: ---, title: Invoice, ---. The title names the file in every app.")
	}
	blocks, found := markup.Parse(lines[start:], start, markup.Options{Lang: lang, Directives: directives})
	problems = append(problems, found...)
	centered := false
	for i := range blocks {
		b := &blocks[i]
		switch {
		case b.Center && !d.Ceremonial && !centered:
			centered = true
			add(b.Line, "Centered text is for ceremonial documents only.", "Set kind: ceremonial in the front matter for programs, invitations and certificates; running documents read better left-aligned.")
		case (b.Kind == Title || b.Kind == Subtitle) && d.Ceremonial:
			b.Center = true
		}
	}
	d.Blocks = blocks
	return d, problems
}

var (
	plain  = markup.Plain
	fields = markup.Fields
)
