package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/skills"
	"github.com/Anonyfox/lfm/internal/term"
)

// next is the step a compile points to: the phase a project is in, or what
// is left before it ships. Agents read it to stay on the intended path.
type next struct {
	Phase string   `json:"phase"` // research, design, implementation, fix, review, deploy
	Do    string   `json:"do"`
	Skill string   `json:"skill,omitempty"` // the skill that covers the step
	Notes []string `json:"notes,omitempty"`
}

// nextStep orders the work: errors block everything; a missing phase comes
// before polishing warnings, because warnings on a page without research or a
// concept are the wrong work; then the release review, then deploying.
func nextStep(r compile.Result, err error, installed []skills.Copy) next {
	have := map[string]bool{}
	var notes []string
	for _, c := range installed {
		have[c.Name] = true
		if c.State == skills.Outdated {
			notes = append(notes, fmt.Sprintf("%s in %s is from lfm %s; 'lfm skills install' updates it.", c.Name, shortPath(r.Root, c.Path), c.Version))
		}
	}
	n := next{Notes: notes}
	skill := func(topic string) string {
		n.Skill = "lfm-" + topic
		if have[n.Skill] {
			return "Follow the " + n.Skill + " skill."
		}
		return "Follow 'lfm skills " + topic + "', or install all three with 'lfm skills install'."
	}
	switch {
	case err != nil || r.Errors() > 0:
		n.Phase, n.Do = "fix", "fix the errors above and compile again; output/ is unchanged until none remain."
	case r.Research.Empty():
		n.Phase, n.Do = "research", "collect real buyer voices and competitor pages into research/. "+skill("research")
	case !r.Concept.Present:
		n.Phase, n.Do = "design", "write concept.md from the research. "+skill("design")
	case r.Starter:
		n.Phase, n.Do = "implementation", "replace the starter page with the page concept.md describes. "+skill("implementation")
	case r.Warnings() > 0:
		n.Phase, n.Do = "fix", "resolve the warnings above; a page is done at 0 errors and 0 warnings."
	case r.Release == nil:
		n.Phase, n.Do = "review", "run 'lfm compile --release' and look at every frame it saves, at phone and desktop size."
	default:
		where := "the static host"
		if r.PageURL != "" {
			where = r.PageURL
		}
		n.Phase, n.Do = "deploy", "upload output/ as is to "+where+"."
	}
	return n
}

func shortPath(root, path string) string {
	rel, err := filepath.Rel(root, filepath.Dir(filepath.Dir(path)))
	if err != nil {
		return filepath.Dir(filepath.Dir(path))
	}
	return filepath.ToSlash(rel)
}

func renderNext(w io.Writer, s term.Style, n next) {
	indent := strings.Repeat(" ", len(n.Phase)+9)
	do, follow, _ := strings.Cut(n.Do, ". Follow ")
	fmt.Fprintf(w, "%s %s\n", s.Bold("Next ("+n.Phase+"):"), term.Wrap(strings.TrimSuffix(do, ".")+".", term.Width-len(indent), indent))
	if follow != "" {
		fmt.Fprintf(w, "%s%s\n", indent, term.Wrap("Follow "+follow, term.Width-len(indent), indent))
	}
	for _, note := range n.Notes {
		fmt.Fprintln(w, s.Dim(term.Wrap(note, term.Width, "")))
	}
}

// printJSON writes the compile result, the next step and the installed skills.
func printJSON(w io.Writer, r compile.Result, n next, installed []skills.Copy) error {
	out := struct {
		compile.Result
		Errors   int           `json:"errors"`
		Warnings int           `json:"warnings"`
		Next     next          `json:"next"`
		Skills   []skills.Copy `json:"skills"`
	}{r, r.Errors(), r.Warnings(), n, installed}
	if out.Skills == nil {
		out.Skills = []skills.Copy{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
