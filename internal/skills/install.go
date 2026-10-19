package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

// Project skill folders, relative to a project or home directory. Codex,
// Cursor, OpenCode, Gemini CLI and GitHub Copilot read .agents/skills; Claude
// Code reads only .claude/skills. Tools that read both load a skill twice when
// it sits in both.
const (
	AgentsDir = ".agents/skills"
	ClaudeDir = ".claude/skills"
)

// Every printed or installed skill carries the version that rendered it and a
// digest of its own text, so an install can tell an untouched copy, safe to
// replace, from one the project changed.
const (
	versionKey = "lfm-version"
	digestKey  = "lfm-sha256"
)

var digestLine = regexp.MustCompile(`(?m)^  ` + digestKey + `: "([0-9a-f]*)"$`)
var versionLine = regexp.MustCompile(`(?m)^  ` + versionKey + `: "([^"]*)"$`)

// stamp adds the metadata block to the frontmatter of text.
func stamp(text, version string) string {
	head, body, _ := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	unsigned := fmt.Sprintf("---\n%s\nmetadata:\n  %s: %q\n  %s: \"\"\n---\n%s", head, versionKey, version, digestKey, body)
	return strings.Replace(unsigned, digestKey+`: ""`, digestKey+`: "`+digest(unsigned)+`"`, 1)
}

func digest(unsigned string) string {
	sum := sha256.Sum256([]byte(unsigned))
	return hex.EncodeToString(sum[:])
}

// State says how an installed copy relates to this binary.
type State string

const (
	Current  State = "current"  // identical to what this binary prints
	Outdated State = "outdated" // untouched, printed by another lfm version
	Modified State = "modified" // changed after it was written
	Foreign  State = "foreign"  // not written by lfm: no stamp
)

// Copy is one lfm skill found on disk.
type Copy struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
	State   State  `json:"state"`
}

// inspect classifies the SKILL.md text found for a skill whose current
// rendering is want ("" for a skill this binary no longer has).
func inspect(text, want string) (version string, state State) {
	if m := versionLine.FindStringSubmatch(text); m != nil {
		version = m[1]
	}
	m := digestLine.FindStringSubmatch(text)
	switch {
	case m == nil:
		return version, Foreign
	case text == want:
		return version, Current
	case digest(strings.Replace(text, m[0], "  "+digestKey+`: ""`, 1)) != m[1]:
		return version, Modified
	}
	return version, Outdated
}

// Scan lists the lfm-* skills in one skills folder, by name.
func Scan(dir string, data Data) ([]Copy, error) {
	current, err := byName(data)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var copies []Copy
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "lfm-") {
			continue
		}
		file := filepath.Join(dir, entry.Name(), "SKILL.md")
		raw, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			return nil, err // an unreadable copy is not a missing one: Install would replace it
		}
		c := Copy{Name: entry.Name(), Path: file}
		c.Version, c.State = inspect(string(raw), current[entry.Name()].Text)
		copies = append(copies, c)
	}
	return copies, nil
}

// Find lists the lfm skills an agent working in dir may load: the skill
// folders of dir and of every directory above it, which includes the user
// folders in the home directory. Folders reached twice through links count once.
func Find(dir string, data Data) []Copy {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var all []Copy
	for {
		for _, rel := range []string{AgentsDir, ClaudeDir} {
			folder := filepath.Join(abs, rel)
			real, err := filepath.EvalSymlinks(folder)
			if err != nil || seen[real] {
				continue
			}
			seen[real] = true
			copies, _ := Scan(folder, data)
			all = append(all, copies...)
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return all
		}
		abs = parent
	}
}

// Change is what Install did with one skill.
type Change struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Action string `json:"action"` // created, updated, unchanged, kept, replaced, removed
	Reason string `json:"reason,omitempty"`
}

// Install writes every skill into dir (a skills folder such as
// <project>/.agents/skills), each as <name>/SKILL.md. Copies the project
// changed, and SKILL.md files lfm did not write, are kept unless force is
// set. Untouched copies of skills this binary no longer has are removed.
// Other files in a skill folder are never touched.
func Install(dir string, data Data, force bool) ([]Change, error) {
	current, err := byName(data)
	if err != nil {
		return nil, err
	}
	existing, err := Scan(dir, data)
	if err != nil {
		return nil, err
	}
	found := map[string]Copy{}
	for _, c := range existing {
		found[c.Name] = c
	}
	names := make([]string, 0, len(current))
	for name := range current {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return rank(current[names[i]].Topic) < rank(current[names[j]].Topic) })
	var changes []Change
	for _, name := range names {
		skill := current[name]
		file := filepath.Join(dir, name, "SKILL.md")
		change := Change{Name: name, Path: file}
		c, exists := found[name]
		switch {
		case !exists:
			change.Action = "created"
		case c.State == Current:
			change.Action = "unchanged"
		case c.State == Outdated:
			change.Action = "updated"
			if c.Version != data.Version {
				change.Reason = "was lfm " + c.Version
			}
		case force:
			change.Action = "replaced"
			change.Reason = describe(c.State)
		default:
			change.Action = "kept"
			change.Reason = describe(c.State) + "; --force replaces it"
		}
		if change.Action == "created" || change.Action == "updated" || change.Action == "replaced" {
			if err := write(file, skill.Text); err != nil {
				return changes, err
			}
		}
		changes = append(changes, change)
	}
	for _, c := range existing {
		if _, ok := current[c.Name]; ok || c.State != Outdated {
			continue
		}
		if err := os.Remove(c.Path); err != nil {
			return changes, err
		}
		_ = os.Remove(filepath.Dir(c.Path)) // only when nothing else lives there
		changes = append(changes, Change{Name: c.Name, Path: c.Path, Action: "removed", Reason: "no longer part of lfm"})
	}
	return changes, nil
}

func describe(s State) string {
	if s == Foreign {
		return "not written by lfm"
	}
	return "changed in this project"
}

func byName(data Data) (map[string]Skill, error) {
	all, err := All(data)
	if err != nil {
		return nil, err
	}
	m := make(map[string]Skill, len(all))
	for _, s := range all {
		m[s.Name] = s
	}
	return m, nil
}

// write replaces file atomically, creating its folders.
func write(file, text string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".SKILL.md.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}
