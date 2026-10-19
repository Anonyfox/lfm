package deck

import (
	"fmt"
	"strconv"

	"github.com/Anonyfox/lfm/internal/markup"
)

// The Markdown model decks share with every editable deliverable.
type (
	Block  = markup.Block
	Inline = markup.Inline
	Table  = markup.Table
	Figure = markup.Figure
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
	Notes     = markup.Notes
)

// Deck is one deck of one language, as its Markdown file describes it.
type Deck struct {
	Key, Lang string
	File      string // path below the project
	Title     string
	Planned   int // the slides it must have; 0 when not planned
	Minutes   int // the speaking time; 0 when not planned
	Blocks    []Block
	Slides    []Slide
}

// directives are the ::: blocks a deck may hold.
var directives = []string{"title", "columns", "notes"}

// parse reads a deck's Markdown: front matter, then slides separated by
// lines of ---.
func parse(key, lang, file string, src []byte) (*Deck, []markup.Problem) {
	d := &Deck{Key: key, Lang: lang, File: file}
	lines := markup.Lines(src)
	settings, start, problems := markup.FrontMatter(lines)
	add := func(line int, message, hint string) {
		problems = append(problems, markup.Problem{Line: line, Message: message, Hint: hint})
	}
	number := func(s markup.Setting, max int, what string) int {
		n, err := strconv.Atoi(s.Value)
		if err != nil || n < 1 || n > max {
			add(s.Line, fmt.Sprintf("%q is not %s.", s.Value, what), "Give a whole number, or leave the setting out.")
			return 0
		}
		return n
	}
	for _, k := range markup.Keys(settings) {
		s := settings[k]
		switch k {
		case "title":
			d.Title = s.Value
		case "minutes":
			d.Minutes = number(s, 600, "a speaking time in minutes")
		case "slides":
			d.Planned = number(s, 500, "a slide count")
		default:
			add(s.Line, fmt.Sprintf("Front matter key %q is unknown.", k), "Use title, minutes and slides.")
		}
	}
	if d.Title == "" {
		add(1, "The deck has no title.", "Start the file with front matter: ---, title: Fire Safety Briefing, ---. The title names the file in every app.")
	}
	blocks, found := markup.Parse(lines[start:], start, markup.Options{Lang: lang, Directives: directives, Slides: true})
	d.Blocks = blocks
	d.Slides = split(blocks)
	return d, append(problems, found...)
}

var plain = markup.Plain
