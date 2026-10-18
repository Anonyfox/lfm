package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/skills"
	"github.com/Anonyfox/lfm/internal/term"
)

var skillsCommand = command{
	name:    "skills",
	args:    "[topic] | install [directory] [--agents] [--claude] [--global] [--force]",
	summary: "Print or install the process guides for agents",
	details: `
A page is made in three phases, each with a skill: research, design and
implementation. Without arguments, lists them; with a topic, prints that
SKILL.md for this version of lfm.

install writes all of them into the project (default: the current directory)
where agents load skills, as <folder>/lfm-<topic>/SKILL.md:

  .agents/skills   Codex, Cursor, OpenCode, Gemini CLI, GitHub Copilot
  .claude/skills   Claude Code

It updates the folders that already hold lfm skills. Otherwise it picks
.claude/skills when run by Claude Code or when the project has only a .claude
folder, and .agents/skills in every other case. Run it again after updating
lfm: untouched copies are updated, copies the project changed are kept.`,
	flags: []flagSpec{
		{name: "agents", usage: "install into .agents/skills"},
		{name: "claude", usage: "install into .claude/skills"},
		{name: "global", usage: "install into the home directory, for every project"},
		{name: "force", usage: "also replace copies that were changed or not written by lfm"},
	},
	run: func(_ context.Context, env Env, p parsed) error {
		data := skills.NewData(env.Build.Version)
		if len(p.args) > 0 && p.args[0] == "install" {
			return installSkills(env, p, data)
		}
		for _, f := range []string{"agents", "claude", "global", "force"} {
			if p.has(f) {
				return usageError("--%s belongs to 'lfm skills install'", f)
			}
		}
		switch len(p.args) {
		case 0:
			return listSkills(env.Stdout, data)
		case 1:
			skill, err := skills.Get(p.args[0], data)
			if err != nil {
				return usageError("%v; run 'lfm skills' for the list", err)
			}
			fmt.Fprint(env.Stdout, skill.Text)
			return nil
		default:
			return usageError("lfm skills takes one topic, or install and a directory")
		}
	},
}

func listSkills(w io.Writer, data skills.Data) error {
	all, err := skills.All(data)
	if err != nil {
		return err
	}
	s := term.For(w)
	width := 0
	for _, skill := range all {
		width = max(width, len(skill.Topic))
	}
	fmt.Fprintln(w, "Skills for agents, in the order a page needs them:")
	fmt.Fprintln(w)
	for _, skill := range all {
		first, _, _ := strings.Cut(skill.Description, ". ")
		fmt.Fprintf(w, "  %s  %s\n", s.Bold(term.Pad(skill.Topic, width)), term.Wrap(first+".", term.Width-width-4, strings.Repeat(" ", width+4)))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "'lfm skills <topic>' prints one; 'lfm skills install' puts all of them where your agent loads them.")
	return nil
}

// folder is one skills folder an install writes to, and why it was chosen.
type folder struct{ rel, readers, why string }

var readers = map[string]string{
	skills.AgentsDir: "Codex, Cursor, OpenCode, Gemini CLI and GitHub Copilot",
	skills.ClaudeDir: "Claude Code",
}

func installSkills(env Env, p parsed, data skills.Data) error {
	if len(p.args) > 2 {
		return usageError("lfm skills install takes one directory")
	}
	base, shown := ".", "."
	if len(p.args) == 2 {
		base, shown = p.args[1], p.args[1]
	}
	if p.has("global") {
		if len(p.args) == 2 {
			return usageError("--global installs into the home directory; leave out %q", p.args[1])
		}
		home, err := env.home()
		if err != nil {
			return fmt.Errorf("--global needs a home directory: %w", err)
		}
		base, shown = home, "~"
	}
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		return fmt.Errorf("%s: no such directory", shown)
	}
	folders, err := chooseFolders(env, p, base, data)
	if err != nil {
		return err
	}
	s := term.For(env.Stdout)
	for i, f := range folders {
		if i > 0 {
			fmt.Fprintln(env.Stdout)
		}
		changes, err := skills.Install(filepath.Join(base, f.rel), data, p.has("force"))
		label := filepath.ToSlash(filepath.Join(shown, f.rel))
		if shown == "." {
			label = f.rel
		}
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		fmt.Fprintf(env.Stdout, "%s %s %s\n", s.Bold(s.Green("✓")), s.Bold(label), s.Dim("for "+readers[f.rel]+", "+f.why))
		width := 0
		for _, c := range changes {
			width = max(width, len(c.Name))
		}
		for _, c := range changes {
			line := fmt.Sprintf("  %s %s", term.Pad(c.Action, 9), c.Name)
			if c.Reason != "" {
				line = fmt.Sprintf("  %s %s  %s", term.Pad(c.Action, 9), term.Pad(c.Name, width), s.Dim(c.Reason))
			}
			fmt.Fprintln(env.Stdout, line)
		}
	}
	fmt.Fprintln(env.Stdout)
	both := bothFolders(base, data)
	if both {
		fmt.Fprintln(env.Stdout, term.Wrap("lfm skills are in both .agents/skills and .claude/skills. OpenCode and Cursor read both and list each skill twice; keep both only when Claude Code and another agent share this project.", term.Width, ""))
	}
	others := skills.ClaudeDir
	flag := "--claude"
	if folders[0].rel == skills.ClaudeDir {
		others, flag = skills.AgentsDir, "--agents"
	}
	if !both {
		fmt.Fprintf(env.Stdout, "%s\n", term.Wrap(fmt.Sprintf("For %s, add %s (%s).", readers[others], others, "lfm skills install "+flag), term.Width, ""))
	}
	fmt.Fprintln(env.Stdout, term.Wrap("Agents pick a skill by its description; a session that was already running may need a restart to see new ones. A page starts with lfm-research.", term.Width, ""))
	return nil
}

// chooseFolders applies the rules of 'lfm help skills': explicit flags, then
// the folders that already hold lfm skills, then the running agent and the
// project's own agent folders.
func chooseFolders(env Env, p parsed, base string, data skills.Data) ([]folder, error) {
	var out []folder
	if p.has("agents") {
		out = append(out, folder{skills.AgentsDir, readers[skills.AgentsDir], "as asked"})
	}
	if p.has("claude") {
		out = append(out, folder{skills.ClaudeDir, readers[skills.ClaudeDir], "as asked"})
	}
	if len(out) > 0 {
		return out, nil
	}
	for _, rel := range []string{skills.AgentsDir, skills.ClaudeDir} {
		if copies, _ := skills.Scan(filepath.Join(base, rel), data); len(copies) > 0 {
			out = append(out, folder{rel, readers[rel], "where lfm skills already were"})
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	if claudeCode(env) {
		return []folder{{skills.ClaudeDir, readers[skills.ClaudeDir], "since Claude Code runs this"}}, nil
	}
	if exists(filepath.Join(base, ".claude")) && !exists(filepath.Join(base, ".agents")) {
		return []folder{{skills.ClaudeDir, readers[skills.ClaudeDir], "since the project has a .claude folder"}}, nil
	}
	return []folder{{skills.AgentsDir, readers[skills.AgentsDir], "the shared default"}}, nil
}

// claudeCode reports whether Claude Code started this process. It sets
// CLAUDECODE=1 for its shells; AI_AGENT is the cross-tool convention.
func claudeCode(env Env) bool {
	return env.getenv("CLAUDECODE") == "1" || strings.HasPrefix(env.getenv("AI_AGENT"), "claude-code")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// bothFolders reports whether base has lfm skills in .agents/skills and in a
// separate .claude/skills.
func bothFolders(base string, data skills.Data) bool {
	a, c := filepath.Join(base, skills.AgentsDir), filepath.Join(base, skills.ClaudeDir)
	ai, errA := os.Stat(a)
	ci, errC := os.Stat(c)
	if errA != nil || errC != nil || os.SameFile(ai, ci) {
		return false
	}
	ca, _ := skills.Scan(a, data)
	cc, _ := skills.Scan(c, data)
	return len(ca) > 0 && len(cc) > 0
}
