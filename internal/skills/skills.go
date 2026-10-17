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
	}
}

// Skill is one rendered skill.
type Skill struct {
	Topic       string // the argument of `lfm skills <topic>`
	Name        string // the frontmatter name, also the folder to save it in
	Description string
	Text        string // the complete SKILL.md
}

// All renders every skill, sorted by topic.
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
	sort.Slice(all, func(i, j int) bool { return all[i].Topic < all[j].Topic })
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
