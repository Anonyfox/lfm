// Package skills holds the process guides lfm hands to agents. Each skill is
// an Agent Skills SKILL.md (agentskills.io), embedded in the binary and
// rendered with this binary's facts, so a printed skill always matches the
// compiler that printed it.
package skills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/term"
	"github.com/Anonyfox/lfm/internal/theme"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/bundle"
	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/formula"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
)

//go:embed files
var files embed.FS

// Data is what a skill template can refer to. Skills use [[ ]] delimiters so
// they can quote Go template syntax such as {{.title}} literally.
type Data struct {
	Version      string // version of the lfm binary
	Voices       string // research file with verbatim human writing
	Competitors  string // research file with the competitor analysis
	MinVoices    int    // research minimums for a project started from zero
	MinPlatforms int
	MinPeers     int
	Authors      string    // allowed author types, "a | b"
	Methods      string    // allowed fetch methods, "a | b"
	ConceptFile  string    // the design handover
	Design       Design    // minimums concept.md must meet
	Screens      string    // the screen sizes of the release layout audit
	Book         Book      // the book deliverable
	Printable    Printable // the printable deliverable
	Spreadsheet  Spreadsheet
	Document     Document
	Deck         Deck
}

// Book holds the layout and minimums of the book deliverable.
type Book struct {
	Dir, Concept                     string
	MinChapters, MinVoices, MinPeers int
	MinWords, MaxWords, Tolerance    int // word target bounds; Tolerance in percent
	CoverWidth, CoverHeight          int
	MinPath                          int
	MinObjections                    int
	Listing, Distribution            string
	MaxDescription, MinDescription   int
	MaxShort, MaxBackCover           int
	MaxKeywords, MaxTags, MaxTag     int
	Channels                         string
}

// Printable holds the layout and limits of the printable deliverable.
type Printable struct {
	Dir, Concept, Listing, Distribution string
	Bundle                              string // the bundle's listing
	Sizes, Roles, Channels              string
	Safe                                float64
	MinPath, EtsyFiles                  int
	MaxTitle, MaxTags, MaxTag           int
}

// Spreadsheet holds the layout and vocabulary of the spreadsheet deliverable.
type Spreadsheet struct {
	Dir, Concept, Listing, Distribution string
	Functions, Types, Styles            string
	MinPath                             int
}

// Document holds the layout and vocabulary of the document deliverable.
type Document struct {
	Dir, Concept, Fonts, Listing, Distribution, Themes string
	MinPath                                            int
}

// Deck holds the layout and vocabulary of the deck deliverable.
type Deck struct {
	Dir, Concept, Fonts, Listing, Distribution, Themes string
	MinPath                                            int
}

// Design holds the minimums the compiler checks in concept.md.
type Design struct {
	MinVoiceNotes, MinPathSteps, MinRejected, MinFacts, MinAxes int
	MinSparks, MaxSparks, MinDirections, MinSpine, MinColors    int
	MinCopy                                                     int
}

// NewData returns the facts of this binary.
func NewData(version string) Data {
	return Data{
		Version:      version,
		Voices:       project.ResearchDir + "/" + research.VoicesFile,
		Competitors:  project.ResearchDir + "/" + research.CompetitorsFile,
		MinVoices:    research.MinVoices,
		MinPlatforms: research.MinPlatforms,
		MinPeers:     research.MinPeers,
		Authors:      strings.Join(research.Authors, " | "),
		Methods:      strings.Join(research.Methods, " | "),
		ConceptFile:  concept.File,
		Screens:      screens(),
		Book: Book{
			Dir: book.Dir, Concept: book.ConceptFile, MinChapters: book.MinChapters, MinVoices: book.MinVoices, MinPeers: book.MinPeers,
			MinWords: book.MinWords, MaxWords: book.MaxWords, Tolerance: int(book.WordTolerance * 100),
			CoverWidth: book.CoverWidth, CoverHeight: book.CoverHeight, MinPath: deliver.MinPathLength,
			MinObjections: book.MinObjections,
			Listing:       book.ListingFile, Distribution: project.DistributionDir + "/book",
			MaxDescription: book.MaxDescription, MinDescription: book.MinDescription, MaxShort: book.MaxShort, MaxBackCover: book.MaxBackCover,
			MaxKeywords: book.MaxKeywords, MaxTags: book.MaxTags, MaxTag: book.MaxTagLength,
			Channels: strings.Join(book.Channels(), ", "),
		},
		Printable: Printable{
			Dir: printable.Dir, Concept: printable.ConceptFile, Listing: printable.ListingFile, Distribution: project.DistributionDir + "/printable", Bundle: bundle.ListingFile,
			Sizes: printable.SizeNames(), Roles: strings.Join(printable.Roles, ", "), Channels: strings.Join(deliver.Shops(), ", "),
			Safe: printable.Safe, MinPath: deliver.MinPathLength, EtsyFiles: deliver.EtsyFiles,
			MaxTitle: deliver.MaxTitle, MaxTags: deliver.MaxTags, MaxTag: deliver.MaxTag,
		},
		Spreadsheet: Spreadsheet{
			Dir: spreadsheet.Dir, Concept: spreadsheet.ConceptFile, Listing: spreadsheet.ListingFile, Distribution: project.DistributionDir + "/spreadsheet",
			Functions: strings.Join(formula.Functions(), ", "), Types: "text, number, money, percent, date, enum",
			Styles: "title, heading, label, input, output, note, total", MinPath: deliver.MinPathLength,
		},
		Document: Document{
			Dir: document.Dir, Concept: document.ConceptFile, Fonts: document.FontsDir, Listing: document.ListingFile,
			Distribution: project.DistributionDir + "/document", Themes: strings.Join(document.ThemeNames(), ", "), MinPath: deliver.MinPathLength,
		},
		Deck: Deck{
			Dir: deck.Dir, Concept: deck.ConceptFile, Fonts: deck.FontsDir, Listing: deck.ListingFile,
			Distribution: project.DistributionDir + "/deck", Themes: strings.Join(theme.Names(), ", "), MinPath: deliver.MinPathLength,
		},
		Design: Design{
			MinVoiceNotes: concept.MinVoiceNotes, MinPathSteps: concept.MinPathSteps, MinRejected: concept.MinRejected,
			MinFacts: concept.MinFacts, MinAxes: concept.MinAxes, MinSparks: concept.MinSparks, MaxSparks: concept.MaxSparks,
			MinDirections: concept.MinDirections, MinSpine: concept.MinSpine, MinColors: concept.MinColors, MinCopy: concept.MinCopy,
		},
	}
}

// screens lists the release audit sizes as prose: "390×844, 1280×680 and 1440×900".
func screens() string {
	var names []string
	for _, s := range release.Screens {
		names = append(names, s.String())
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// Skill is one rendered skill.
type Skill struct {
	Topic       string // the argument of `lfm skills <topic>`
	Name        string // the frontmatter name, also the folder to save it in
	Description string
	Text        string // the complete SKILL.md
}

// lifecycle orders the skills as a project uses them; others follow by name.
var lifecycle = []string{"research", "design", "implementation", "book", "printable", "spreadsheet", "document", "deck"}

func rank(topic string) int {
	for i, t := range lifecycle {
		if t == topic {
			return i
		}
	}
	return len(lifecycle)
}

// Topics are every skill's topic in lifecycle order, without rendering.
func Topics() []string {
	found, _ := fs.Glob(files, "files/*/SKILL.md")
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, path.Base(path.Dir(f)))
	}
	slices.SortStableFunc(out, func(a, b string) int {
		if d := rank(a) - rank(b); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	return out
}

// All renders every skill in lifecycle order.
func All(data Data) ([]Skill, error) {
	topics, err := fs.Glob(files, "files/*/SKILL.md")
	if err != nil {
		return nil, err
	}
	var all []Skill
	for _, file := range topics {
		skill, err := render(path.Base(path.Dir(file)), data)
		if err != nil {
			return nil, err
		}
		all = append(all, skill)
	}
	sort.Slice(all, func(i, j int) bool {
		if rank(all[i].Topic) != rank(all[j].Topic) {
			return rank(all[i].Topic) < rank(all[j].Topic)
		}
		return all[i].Topic < all[j].Topic
	})
	return all, nil
}

// Get renders the skill for topic.
func Get(topic string, data Data) (Skill, error) {
	if _, err := fs.Stat(files, "files/"+topic+"/SKILL.md"); err != nil || strings.ContainsAny(topic, "/.") {
		return Skill{}, fmt.Errorf("no skill named %q.%s", topic, term.DidYouMean(topic, Topics()))
	}
	return render(topic, data)
}

func render(topic string, data Data) (Skill, error) {
	source, err := files.ReadFile("files/" + topic + "/SKILL.md")
	if err != nil {
		return Skill{}, err
	}
	t, err := template.New(topic).Delims("[[", "]]").Option("missingkey=error").Parse(string(source))
	if err != nil {
		return Skill{}, fmt.Errorf("skill %s: %w", topic, err)
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return Skill{}, fmt.Errorf("skill %s: %w", topic, err)
	}
	skill := Skill{Topic: topic}
	skill.Name, skill.Description, err = frontmatter(out.String())
	if err != nil {
		return Skill{}, fmt.Errorf("skill %s: %w", topic, err)
	}
	skill.Text = stamp(out.String(), data.Version)
	return skill, nil
}

// frontmatter reads the two required fields of the YAML header. Skills keep
// them on single lines, so a full YAML parser is not needed.
func frontmatter(text string) (name, description string, err error) {
	rest, ok := strings.CutPrefix(text, "---\n")
	header, _, closed := strings.Cut(rest, "\n---\n")
	if !ok || !closed {
		return "", "", fmt.Errorf("missing --- frontmatter")
	}
	for _, line := range strings.Split(header, "\n") {
		if value, ok := strings.CutPrefix(line, "name: "); ok {
			name = value
		}
		if value, ok := strings.CutPrefix(line, "description: "); ok {
			description = value
		}
	}
	if name == "" || description == "" {
		return "", "", fmt.Errorf("frontmatter needs name and description")
	}
	return name, description, nil
}
