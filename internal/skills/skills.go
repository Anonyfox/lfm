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
	"sort"
	"strings"
	"text/template"

	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/research"
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
	Authors      string // allowed author types, "a | b"
	Methods      string // allowed fetch methods, "a | b"
	ConceptFile  string // the design handover
	Design       Design // minimums concept.md must meet
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
		Design: Design{
			MinVoiceNotes: concept.MinVoiceNotes, MinPathSteps: concept.MinPathSteps, MinRejected: concept.MinRejected,
			MinFacts: concept.MinFacts, MinAxes: concept.MinAxes, MinSparks: concept.MinSparks, MaxSparks: concept.MaxSparks,
			MinDirections: concept.MinDirections, MinSpine: concept.MinSpine, MinColors: concept.MinColors, MinCopy: concept.MinCopy,
		},
	}
}

// Skill is one rendered skill.
type Skill struct {
	Topic       string // the argument of `lfm skills <topic>`
	Name        string // the frontmatter name, also the folder to save it in
	Description string
	Text        string // the complete SKILL.md
}

// lifecycle orders the skills as a project uses them; others follow by name.
var lifecycle = []string{"research", "design", "implementation"}

func rank(topic string) int {
	for i, t := range lifecycle {
		if t == topic {
			return i
		}
	}
	return len(lifecycle)
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
		return Skill{}, fmt.Errorf("no skill named %q", topic)
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
	skill := Skill{Topic: topic, Text: out.String()}
	skill.Name, skill.Description, err = frontmatter(skill.Text)
	if err != nil {
		return Skill{}, fmt.Errorf("skill %s: %w", topic, err)
	}
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
