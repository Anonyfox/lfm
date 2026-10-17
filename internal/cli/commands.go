package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/scaffold"
	"github.com/Anonyfox/lfm/internal/serve"
	"github.com/Anonyfox/lfm/internal/term"
)

var initCommand = command{
	name:    "init",
	args:    "[directory]",
	summary: "Create a new project that compiles cleanly",
	details: `
Writes the starter files into the directory (default: the current one),
creating it when needed. Unrelated files may exist; if any starter file is
already there, nothing is written at all.`,
	run: func(_ context.Context, env Env, p parsed) error {
		dir, err := p.directory("init")
		if err != nil {
			return err
		}
		created, err := scaffold.Create(dir)
		if err != nil {
			return err
		}
		s := term.For(env.Stdout)
		fmt.Fprintf(env.Stdout, "%s created %s\n", s.Bold(s.Green("✓")), s.Bold(dir))
		for _, name := range created {
			fmt.Fprintf(env.Stdout, "  %s\n", s.Dim(name))
		}
		next := "lfm serve --open"
		if dir != "." {
			next = "cd " + quote(dir) + " && " + next
		}
		fmt.Fprintf(env.Stdout, "\nNext: %s\n", s.Bold(next))
		return nil
	},
}

var compileCommand = command{
	name:    "compile",
	args:    "[directory] [--release]",
	summary: "Check the project and build output/",
	details: `
Runs every check and replaces output/ only when no errors remain; warnings
explain improvements without blocking. --release adds the expensive stages:
critical CSS measured in headless Chrome, a runtime probe and Lighthouse when
installed. Missing tools are reported and skipped, never installed.

Chrome is found on PATH and in standard locations, or via LFM_CHROME.
Lighthouse is found on PATH, in node_modules/.bin, or via LFM_LIGHTHOUSE.`,
	flags: []flagSpec{{name: "release", usage: "also run browser-backed optimization and audits"}},
	run: func(ctx context.Context, env Env, p parsed) error {
		dir, err := p.directory("compile")
		if err != nil {
			return err
		}
		s := term.For(env.Stdout)
		result, err := compile.Compile(ctx, dir, compile.Options{Publish: true, Release: p.has("release"), Waiting: waiting(env.Stdout, s)})
		if result.Root == "" || errors.Is(err, context.Canceled) {
			return err // nothing was compiled: a missing directory or Ctrl-C
		}
		renderResult(env.Stdout, s, result, err)
		if err != nil {
			return errReported
		}
		return nil
	},
}

var serveCommand = command{
	name:    "serve",
	args:    "[directory] [--port 8080] [--open]",
	summary: "Preview the page and recompile on every save",
	details: `
Serves the compiled page on 127.0.0.1 and reloads the browser after each
change. A failed compile shows its findings in the browser and keeps the last
good page. A busy default port falls back to a free one; --port 0 picks one.`,
	flags: []flagSpec{
		{name: "port", value: true, usage: "port to listen on (default 8080)"},
		{name: "open", usage: "open the preview in the default browser"},
	},
	run: func(ctx context.Context, env Env, p parsed) error {
		dir, err := p.directory("serve")
		if err != nil {
			return err
		}
		port := 8080
		if value, ok := p.flags["port"]; ok {
			if port, err = strconv.Atoi(value); err != nil || port < 0 || port > 65535 {
				return usageError("--port needs a number between 0 and 65535, got %q", value)
			}
		}
		s := term.For(env.Stdout)
		return serve.Serve(ctx, dir, serve.Options{
			Port: port, ExplicitPort: p.has("port"), Open: p.has("open"), Log: env.Stdout,
			Waiting: waiting(env.Stdout, s),
			OnBuild: func(r compile.Result, err error) { renderResult(env.Stdout, s, r, err); fmt.Fprintln(env.Stdout) },
		})
	},
}

var rulesCommand = command{
	name:    "rules",
	args:    "[rule or family]",
	summary: "List every check, or explain one",
	details: `
Every rule runs on every compile; release.* and lighthouse.* findings
need --release. Projects can change a rule's level in
lfm.json under build.validation.rules, e.g. {"design.mono": "off"}.
Levels are off, warn and error; required rules always stay errors.`,
	run: func(_ context.Context, env Env, p parsed) error {
		if len(p.args) > 1 {
			return usageError("lfm rules takes at most one rule or family")
		}
		filter := ""
		if len(p.args) == 1 {
			filter = p.args[0]
		}
		return printRules(env.Stdout, filter)
	},
}

var skillsCommand = command{
	name:    "skills",
	summary: "Process guides for agents (coming soon)",
	details: `
Skills will hand agents the full process for research, UX, copy, imagery and
commerce, ready to adapt and place in a project. They are not published yet.`,
	run: func(_ context.Context, env Env, p parsed) error {
		if len(p.args) > 0 {
			return usageError("lfm skills takes no arguments yet")
		}
		fmt.Fprintln(env.Stdout, "Skills are not published yet. This command will provide the process guides for")
		fmt.Fprintln(env.Stdout, "research, UX, copy, imagery and commerce. Until then, AGENTS.md in a new project")
		fmt.Fprintln(env.Stdout, "and the compiler's own findings are the guidance.")
		return nil
	},
}

func printRules(w io.Writer, filter string) error {
	s := term.For(w)
	catalog := compile.Rules()
	for _, rule := range catalog {
		if rule.ID == filter {
			printRule(w, s, rule, rule.ID)
			return nil
		}
	}
	if filter != "lighthouse.*" && diagnostic.DynamicRule(filter) {
		// Concrete Lighthouse audit IDs come from the installed tool; they
		// share the family's help and accept their own exact override.
		for _, rule := range catalog {
			if rule.ID == "lighthouse.*" {
				printRule(w, s, rule, filter)
				return nil
			}
		}
	}
	families := map[string][]diagnostic.RuleInfo{}
	for _, rule := range catalog {
		family, _, _ := strings.Cut(rule.ID, ".")
		if filter == "" || filter == family {
			families[family] = append(families[family], rule)
		}
	}
	if len(families) == 0 {
		return usageError("no rule or family named %q; run 'lfm rules' for the list", filter)
	}
	names := make([]string, 0, len(families))
	for family := range families {
		names = append(names, family)
	}
	sort.Strings(names)
	for i, family := range names {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, s.Bold(family))
		for _, rule := range families[family] {
			marker := " "
			if rule.Required {
				marker = s.Red("•")
			}
			// Summaries wrap under their own column so IDs stay scannable.
			summary := term.Wrap(rule.Summary, term.Width-31, strings.Repeat(" ", 31))
			fmt.Fprintf(w, "  %s %s %s\n", marker, term.Pad(rule.ID, 26), s.Dim(summary))
		}
	}
	fmt.Fprintf(w, "\n%s required: always an error.  'lfm rules <id>' explains one rule.\n", s.Red("•"))
	return nil
}

// printRule explains one rule: what it checks, why and how to resolve it,
// then whether and how a project can change its level. id differs from
// rule.ID only for a concrete lighthouse.<audit-id>.
func printRule(w io.Writer, s term.Style, rule diagnostic.RuleInfo, id string) {
	fmt.Fprintf(w, "%s\n%s\n\n%s\n\n", s.Bold(id), term.Wrap(rule.Summary, term.Width, ""), term.Wrap(rule.Help, term.Width, ""))
	if rule.Required {
		fmt.Fprintln(w, "Required: yes. It always blocks the compile and cannot be turned off.")
		return
	}
	fmt.Fprintf(w, "Required: no. Change its level (off, warn or error) in lfm.json:\n\n  \"build\": {\"validation\": {\"rules\": {%q: \"off\"}}}\n", id)
}

// quote makes a path safe to paste into a POSIX shell.
func quote(path string) string {
	path = filepath.Clean(path)
	for _, r := range path {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._/-", r)) {
			return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
		}
	}
	return path
}

// waiting announces that another compile of the project holds the lock.
func waiting(w io.Writer, s term.Style) func() {
	return func() { fmt.Fprintln(w, s.Dim("waiting for another compile of this project to finish…")) }
}
