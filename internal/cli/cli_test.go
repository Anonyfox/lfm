package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/skills"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	return runWith(t, nil, args...)
}

// runWith runs with the given environment variables only, so the agent that
// runs the tests does not change what they see.
func runWith(t *testing.T, vars map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	env := Env{Stdout: &out, Stderr: &errs, Build: Build{Version: "1.2.3", Commit: "abc", Date: "today"},
		Getenv: func(k string) string { return vars[k] }, Home: t.TempDir()}
	code = Run(context.Background(), env, args)
	return code, out.String(), errs.String()
}

func TestHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		code, out, _ := run(t, args...)
		if code != 0 || !strings.Contains(out, "Commands:") || !strings.Contains(out, "compile") {
			t.Fatalf("%v: %d %q", args, code, out)
		}
	}
	code, out, _ := run(t, "help", "serve")
	if code != 0 || !strings.Contains(out, "--port") || !strings.Contains(out, "--open") {
		t.Fatalf("command help: %d %q", code, out)
	}
	code, out, _ = run(t, "compile", "--help")
	if code != 0 || !strings.Contains(out, "--release") {
		t.Fatalf("flag help: %d %q", code, out)
	}
	for _, args := range [][]string{{"version"}, {"--version"}} {
		if code, out, _ := run(t, args...); code != 0 || out != "lfm 1.2.3 (abc, today)\n" {
			t.Fatalf("%v: %d %q", args, code, out)
		}
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"bogus"}, `unknown command "bogus"`},
		{[]string{"help", "bogus"}, `unknown command "bogus"`},
		{[]string{"compile", "--bogus"}, "unknown flag --bogus"},
		{[]string{"compile", "-x"}, "unknown flag -x"},
		{[]string{"compile", "a", "b"}, "takes one directory"},
		{[]string{"compile", "--release=yes"}, "--release takes no value"},
		{[]string{"serve", "--port"}, "--port needs a value"},
		{[]string{"serve", "--port", "http"}, "--port needs a number"},
		{[]string{"serve", "--open", "--open"}, "--open given twice"},
		{[]string{"version", "x"}, "takes no arguments"},
		{[]string{"help", "compile", "extra"}, "at most one command"},
		{[]string{"skills", "research", "--force"}, "--force belongs to 'lfm skills install'"},
		{[]string{"skills", "install", "a", "b"}, "takes one directory"},
		{[]string{"skills", "install", "x", "--global"}, "--global installs into the home directory"},
		{[]string{"skills", "a", "b"}, "takes one topic"},
	} {
		code, out, errs := run(t, tc.args...)
		if code != 2 || out != "" || !strings.Contains(errs, tc.want) {
			t.Errorf("%v: code %d, stdout %q, stderr %q", tc.args, code, out, errs)
		}
	}
}

func TestInitThenCompile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tea-shop")
	code, out, errs := run(t, "init", dir)
	if code != 0 || !strings.Contains(out, "created") || !strings.Contains(out, "lfm.json") || !strings.Contains(out, "lfm serve --open") || !strings.Contains(out, "lfm skills install") {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	if code, _, errs := run(t, "init", dir); code != 1 || !strings.Contains(errs, "already an lfm project") {
		t.Fatalf("second init: %d %q", code, errs)
	}
	code, out, errs = run(t, "compile", dir)
	if code != 0 || !strings.Contains(out, "✓ tea-shop compiled → output/") || strings.Contains(out, "warning") || errs != "" ||
		!strings.Contains(out, "Next (research):") || !strings.Contains(out, "'lfm skills research'") {
		t.Fatalf("compile: %d %q %q", code, out, errs)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("colored output written to a buffer")
	}
}

func TestFailedCompileExplainsAndKeepsOutput(t *testing.T) {
	dir := t.TempDir()
	if code, _, errs := run(t, "init", dir); code != 0 {
		t.Fatal(errs)
	}
	if code, _, _ := run(t, "compile", dir); code != 0 {
		t.Fatal("clean compile failed")
	}
	before, _ := os.ReadFile(filepath.Join(dir, "output", "index.html"))
	if err := os.WriteFile(filepath.Join(dir, "index.html.tmpl"), []byte(`<!doctype html><html lang="{{.language}}"><head><title>{{.missing}}</title></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(t, "compile", dir)
	if code != 1 || !strings.Contains(out, "error") || !strings.Contains(out, "failed") || !strings.Contains(out, "output/ unchanged") {
		t.Fatalf("failed compile: %d %q", code, out)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "output", "index.html"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed compile replaced the output")
	}
}

func TestRulesListsExplainsAndRejects(t *testing.T) {
	code, out, _ := run(t, "rules")
	if code != 0 || !strings.Contains(out, "design.eyebrows") || !strings.Contains(out, "build.failed") {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ = run(t, "rules", "design")
	if code != 0 || !strings.Contains(out, "design.eyebrows") || strings.Contains(out, "seo.metadata") {
		t.Fatalf("family: %d %q", code, out)
	}
	code, out, _ = run(t, "rules", "design.eyebrows")
	if code != 0 || !strings.Contains(out, `"design.eyebrows": "off"`) {
		t.Fatalf("explain: %d %q", code, out)
	}
	code, out, _ = run(t, "rules", "build.failed")
	if code != 0 || !strings.Contains(out, "Required") {
		t.Fatalf("required: %d %q", code, out)
	}
	if code, _, errs := run(t, "rules", "nope"); code != 2 || !strings.Contains(errs, `no rule or family named "nope"`) {
		t.Fatalf("unknown: %d %q", code, errs)
	}
}

func TestSkillsListAndPrint(t *testing.T) {
	code, out, _ := run(t, "skills")
	if code != 0 || !strings.Contains(out, "research") || !strings.Contains(out, "lfm skills <topic>") {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ = run(t, "skills", "research")
	if code != 0 || !strings.HasPrefix(out, "---\nname: lfm-research\n") || !strings.Contains(out, `lfm-version: "1.2.3"`) {
		t.Fatalf("print: %d %q", code, out[:min(len(out), 200)])
	}
	if code, _, errs := run(t, "skills", "nope"); code != 2 || !strings.Contains(errs, `no skill named "nope"`) {
		t.Fatalf("unknown: %d %q", code, errs)
	}
	if code, out, _ := run(t, "help", "skills"); code != 0 || !strings.Contains(out, ".claude/skills   Claude Code") || !strings.Contains(out, "--force") {
		t.Fatalf("help: %d %q", code, out)
	}
}

func TestParse(t *testing.T) {
	specs := []flagSpec{{name: "release"}, {name: "port", value: true}}
	p, err := parse("x", []string{"site", "--port", "9000", "--release", "--", "--literal"}, specs)
	if err != nil || p.flags["port"] != "9000" || !p.has("release") || strings.Join(p.args, " ") != "site --literal" {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = parse("x", []string{"--port=0", "-"}, specs)
	if err != nil || p.flags["port"] != "0" || len(p.args) != 1 || p.args[0] != "-" {
		t.Fatalf("%+v %v", p, err)
	}
}

func FuzzParseNeverPanics(f *testing.F) {
	f.Add("site --port 9000 --release")
	f.Add("-- --x = --port=")
	specs := []flagSpec{{name: "release"}, {name: "port", value: true}}
	f.Fuzz(func(t *testing.T, line string) {
		p, err := parse("x", strings.Fields(line), specs)
		if err == nil {
			for name := range p.flags {
				if _, ok := lookup(specs, name); !ok {
					t.Fatalf("accepted undeclared flag %q", name)
				}
			}
		}
	})
}

func TestCompileMissingDirectory(t *testing.T) {
	code, out, errs := run(t, "compile", filepath.Join(t.TempDir(), "absent"))
	if code != 1 || out != "" || !strings.Contains(errs, "absent: no such directory") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
}

func TestInitOnAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := run(t, "init", file); code != 1 || !strings.Contains(errs, "is a file, not a directory") {
		t.Fatalf("%d %q", code, errs)
	}
}

func TestQuoteIsShellSafe(t *testing.T) {
	for in, want := range map[string]string{
		"site":        "site",
		"a/b-c_d.e":   "a/b-c_d.e",
		"./site/":     "site",
		"my site":     "'my site'",
		"a;b":         "'a;b'",
		"x$HOME":      "'x$HOME'",
		"it's":        `'it'\''s'`,
		"$(rm -rf ~)": "'$(rm -rf ~)'",
		"café":        "'café'",
	} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkillsInstallPicksTheFolderAgentsRead(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := runWith(t, nil, "skills", "install", dir)
	if code != 0 || !strings.Contains(out, ".agents/skills") || !strings.Contains(out, "created   lfm-implementation") {
		t.Fatalf("default: %d %q %q", code, out, errs)
	}
	// Claude Code gets its own folder; the first install stays where it is.
	code, out, _ = runWith(t, map[string]string{"CLAUDECODE": "1"}, "skills", "install", dir)
	if code != 0 || !strings.Contains(out, "all 8 skills are current") || strings.Contains(out, ".claude/skills for") {
		t.Fatalf("existing: %d %q", code, out)
	}
	fresh := t.TempDir()
	code, out, _ = runWith(t, map[string]string{"AI_AGENT": "claude-code_2_agent"}, "skills", "install", fresh)
	if code != 0 || !strings.Contains(out, ".claude/skills for Claude Code, since Claude Code runs this") || !strings.Contains(out, "--agents") {
		t.Fatalf("claude: %d %q", code, out)
	}
	code, out, _ = runWith(t, nil, "skills", "install", fresh, "--agents")
	if code != 0 || !strings.Contains(out, "OpenCode and Cursor read both") {
		t.Fatalf("both: %d %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(fresh, ".agents", "skills", "lfm-design", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	onlyClaude := t.TempDir()
	os.Mkdir(filepath.Join(onlyClaude, ".claude"), 0o755)
	if _, out, _ := runWith(t, nil, "skills", "install", onlyClaude); !strings.Contains(out, "since the project has a .claude folder") {
		t.Fatalf("claude folder: %q", out)
	}
	if code, out, _ := runWith(t, nil, "skills", "install", "--global"); code != 0 || !strings.Contains(out, "~/.agents/skills") {
		t.Fatalf("global: %d %q", code, out)
	}
}

func TestCompileNamesTheNextPhase(t *testing.T) {
	dir := t.TempDir()
	run(t, "init", dir)
	run(t, "skills", "install", dir)
	code, out, _ := run(t, "compile", dir)
	if code != 0 || !strings.Contains(out, "Follow the lfm-research skill;") {
		t.Fatalf("installed: %d %q", code, out)
	}
	code, out, _ = run(t, "compile", dir, "--json")
	var got struct {
		Errors int `json:"errors"`
		Next   next
		Skills []skills.Copy
	}
	if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil || got.Next.Phase != "research" || got.Next.Skill != "lfm-research" || len(got.Skills) != 8 {
		t.Fatalf("json: %d %v %+v", code, err, got)
	}
	if code, _, errs := run(t, "compile", t.TempDir()); code != 1 || !strings.Contains(errs, "not an lfm project") || !strings.Contains(errs, "lfm init") {
		t.Fatalf("empty directory: %d %q", code, errs)
	}
}

func TestNextStepOrder(t *testing.T) {
	r := compile.Result{Research: research.Summary{Voices: 3}, Concept: concept.Summary{Present: true}, PageURL: "https://x.test/"}
	for _, tc := range []struct {
		edit func(*compile.Result)
		want string
	}{
		{func(r *compile.Result) { r.Research = research.Summary{} }, "research"},
		{func(r *compile.Result) { r.Concept.Present = false }, "design"},
		{func(r *compile.Result) { r.Starter = true }, "implementation"},
		{func(r *compile.Result) { r.Validation = &check.Report{Warnings: 1} }, "fix"},
		{func(r *compile.Result) { r.Validation = &check.Report{Errors: 1}; r.Research = research.Summary{} }, "fix"},
		{func(r *compile.Result) {}, "review"},
		{func(r *compile.Result) { r.Release = &release.Result{} }, "deploy"},
	} {
		c := r
		tc.edit(&c)
		if n := nextStep(c, nil, nil); n.Phase != tc.want {
			t.Errorf("want %s, got %+v", tc.want, n)
		}
	}
	clean := compile.Result{Research: research.Summary{Voices: 3}, Concept: concept.Summary{Present: true}, Release: &release.Result{}, PageURL: "https://x.test/"}
	clean.Book = &book.Summary{}
	if n := nextStep(clean, nil, nil); n.Phase != "book" || !strings.Contains(n.Do, "deliverables/book/concept.md") {
		t.Errorf("book concept: %+v", n)
	}
	clean.Book = &book.Summary{Concept: true, URL: "https://x.test/k/", Chapters: []book.Chapter{{N: 1, Title: "One", File: "chapters/01.md", Words: 900, Job: "decide what to bake first", Exists: true}, {N: 2, Title: "Two", File: "chapters/02.md", Words: 1200, Job: "plan the week"}}}
	if n := nextStep(clean, nil, nil); n.Phase != "book" || !strings.Contains(n.Do, `chapters/02.md, chapter 2 of 2, "Two" (about 1200 words): plan the week`) || len(n.Notes) != 1 {
		t.Errorf("next chapter: %+v", n)
	}
	clean.Book.Chapters[1].Exists = true
	if n := nextStep(clean, nil, nil); n.Phase != "deploy" || len(n.Notes) != 1 || !strings.Contains(n.Notes[0], "https://x.test/k/") {
		t.Errorf("deploy with book: %+v", n)
	}
	old := []skills.Copy{{Name: "lfm-design", Path: "/p/.agents/skills/lfm-design/SKILL.md", Version: "0.1", State: skills.Outdated}}
	if n := nextStep(compile.Result{Root: "/p"}, nil, old); len(n.Notes) != 1 || !strings.Contains(n.Notes[0], "lfm-design in .agents/skills is from lfm 0.1; 'lfm skills install /p' updates it.") {
		t.Fatalf("%+v", n)
	}
}

func TestVersionLeavesOutUnknownParts(t *testing.T) {
	var out bytes.Buffer
	Run(context.Background(), Env{Stdout: &out, Stderr: &out, Build: Build{Version: "dev", Commit: "none", Date: "unknown"}}, []string{"version"})
	if out.String() != "lfm dev\n" {
		t.Fatalf("%q", out.String())
	}
}

func TestCompileJSONOutsideAProject(t *testing.T) {
	code, out, errs := run(t, "compile", t.TempDir(), "--json")
	var got struct {
		Error string
		Next  next
	}
	if err := json.Unmarshal([]byte(out), &got); code != 1 || err != nil || got.Next.Phase != "init" || !strings.Contains(errs, "not an lfm project") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
}

func TestHelpConfigExplainsLfmJSON(t *testing.T) {
	if code, out, _ := run(t, "help", "config"); code != 0 || !strings.Contains(out, "deliverables") || !strings.Contains(out, "lfm help config <field>") {
		t.Fatalf("%d %s", code, out)
	}
	if code, out, _ := run(t, "help", "config", "deliverables.book"); code != 0 || !strings.Contains(out, "author") || !strings.Contains(out, "page | shops | bundle") {
		t.Fatalf("%d %s", code, out)
	}
	if code, _, errs := run(t, "help", "config", "nope"); code != 2 || !strings.Contains(errs, "no field") {
		t.Fatalf("%d %s", code, errs)
	}
}

func TestTemplateHelp(t *testing.T) {
	code, out, errs := run(t, "help", "template")
	if code != 0 || errs != "" {
		t.Fatalf("%d %s", code, errs)
	}
	for _, name := range []string{"dict", "script", "style", "eq", "ne", "lt", "le", "gt", "ge"} {
		if !strings.Contains(out, "  "+name+" — ") {
			t.Fatalf("missing helper %s: %s", name, out)
		}
	}
	if !strings.Contains(out, ".urls") || !strings.Contains(out, "Optional paths") {
		t.Fatal(out)
	}
}
