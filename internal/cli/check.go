package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/target"
	"github.com/Anonyfox/lfm/internal/term"
)

type skippedRule struct {
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
}
type checkRelease struct {
	Audit *release.AuditResult `json:"audit"`
}

type checkResult struct {
	Target  string `json:"target"`
	Project string `json:"project,omitempty"`
	target.Result
	Validation check.Report  `json:"validation"`
	Release    *checkRelease `json:"release,omitempty"`
	Skipped    []skippedRule `json:"skipped"`
	Errors     int           `json:"errors"`
	Warnings   int           `json:"warnings"`
	DurationMS int64         `json:"durationMs"`
	Next       next          `json:"next"`
}

func pageRules(browser bool) ([]check.Rule, []skippedRule) {
	rules := []check.Rule{}
	skipped := []skippedRule{}
	for _, rule := range check.DefaultRules() {
		if rule.Project != "" {
			skipped = append(skipped, skippedRule{rule.ID, rule.Project})
		} else {
			rules = append(rules, rule)
		}
	}
	if !browser {
		skipped = append(skipped, skippedRule{"release.*", "browser checks run with --release"}, skippedRule{"lighthouse.*", "browser checks run with --release"})
	}
	return rules, skipped
}

var checkCommand = command{
	name: "check", args: "[<url | folder | file.html | file.txt | file.md>] [--lang <tag>] [--project <dir>] [--release] [--json]",
	summary: "Check a built page, plain text or Markdown",
	details: `Checks any built page with the same rules compile runs on its own output,
from any framework or host: HTML, links, SEO, sharing, accessibility, forms,
performance, design defaults and copy. Rules that need an lfm project are
listed as skipped unless --project <dir> supplies their project context.
With --project and no target, check that project's output/ and compare it with
an in-memory compile of the current source to report stale files.

An http(s) URL fetches the page and its embedded resources, including from
localhost. A folder means index.html; a .html file uses its containing folder.
A .txt or .md file runs only the shared copy rules; Markdown code and quotes are skipped.
--lang overrides HTML or front-matter language; otherwise language is detected.
Use --project to apply that project's copy policy to text too.
Static checks execute nothing. --release adds runtime errors, the frame-by-frame
layout walk and Lighthouse. Release needs installed Chrome and Lighthouse plus
loopback sockets. For URLs the browser visits the real page and runs its JavaScript, including analytics. Files use a sealed artifact server.
--json prints the same findings as the human report.
Exit status: 0 with no errors (warnings allowed); 1 for findings with errors or
an unreadable page; 2 for a mistake in the command line.`,
	flags: []flagSpec{{name: "lang", value: true, usage: "override the copy language (BCP-47 tag)"}, {name: "project", value: true, usage: "pair the built page with an lfm project"}, {name: "release", usage: "also run runtime, layout and Lighthouse checks"}, {name: "json", usage: "print the result as JSON instead of the report"}},
	run:   runCheck,
}

func runCheck(ctx context.Context, env Env, p parsed) (resultErr error) {
	defer func() {
		if resultErr != nil && !errors.Is(resultErr, errReported) && !errors.Is(resultErr, errUsage) && p.has("json") {
			if err := printJSONError(env.Stdout, resultErr); err != nil {
				resultErr = err
			} else {
				resultErr = errReported
			}
		}
	}()
	if len(p.args) > 1 || (len(p.args) == 0 && !p.has("project")) {
		return usageError("lfm check takes one URL, folder or .html file, or --project <dir> without a target")
	}
	start := time.Now()
	projectDir := ""
	noTarget := len(p.args) == 0
	if p.has("project") {
		var err error
		projectDir, err = filepath.Abs(p.flags["project"])
		if err != nil {
			return err
		}
		if noTarget {
			p.args = []string{filepath.Join(projectDir, "output")}
		}
	}
	textTarget := !strings.Contains(p.args[0], "://") && (strings.EqualFold(filepath.Ext(p.args[0]), ".txt") || strings.EqualFold(filepath.Ext(p.args[0]), ".md"))
	if textTarget && p.has("release") {
		return usageError("--release requires an HTML page")
	}
	var input target.Result
	var err error
	if textTarget {
		f, openErr := os.Open(p.args[0])
		err = openErr
		if err == nil {
			input.Page, err = io.ReadAll(io.LimitReader(f, (8<<20)+1))
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
			if len(input.Page) > 8<<20 {
				err = fmt.Errorf("text target exceeds 8 MiB")
			}
		}
		input.Options = check.Options{File: p.args[0], Language: p.flags["lang"]}
	} else {
		input, err = target.Read(ctx, p.args[0], env.Build.Version)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if errors.Is(err, target.ErrUsage) {
			return usageError("%s", err)
		}
	}
	readErr := err
	browser := p.has("release")
	rules, skipped := pageRules(browser)
	doc := input.Document
	input.Options.Language = p.flags["lang"]
	if doc != nil {
		doc.Options.Language = p.flags["lang"]
	}
	if textTarget {
		rules = nil
		skipped = []skippedRule{{"page.*", "text targets run copy rules only"}}
	}
	var overrides map[string]diagnostic.Severity
	if projectDir != "" && readErr == nil {
		var options check.Options
		var err error
		if textTarget {
			options, err = compile.ProjectCopyOptions(projectDir, input.Options)
		} else {
			options, err = compile.ProjectCheckOptions(projectDir, input.Options)
		}
		if err != nil {
			return err
		}
		input.Options = options
		if !textTarget {
			doc, err = check.Parse(input.Page, options)
		}
		if err != nil {
			return err
		}
		overrides = options.Rules
		rules, skipped = check.DefaultRules(), nil
		if !browser && !textTarget {
			skipped = []skippedRule{{"release.*", "browser checks run with --release"}, {"lighthouse.*", "browser checks run with --release"}}
		}
	}
	if textTarget {
		skipped = []skippedRule{{"page.*", "text targets run copy rules only"}}
	}
	r := checkResult{Target: p.args[0], Project: projectDir, Result: input, Validation: check.Report{Stage: "html", Diagnostics: []check.Diagnostic{}}, Skipped: skipped}
	if readErr == nil {
		if textTarget {
			r.Validation = check.CopyText(input.Page, strings.EqualFold(filepath.Ext(p.args[0]), ".md"), input.Options)
		} else {
			r.Validation = check.Run(doc, rules, overrides)
		}
	} else {
		r.Validation.Diagnostics = append(r.Validation.Diagnostics, diagnostic.Diagnostic{Rule: "build.failed", Severity: diagnostic.Error, Location: diagnostic.Location{File: p.args[0], Line: 1, Column: 1}, Message: readErr.Error(), Hint: "Make the page readable at this target and check again."})
	}
	issues := r.Validation.Diagnostics
	if noTarget && readErr == nil {
		built, err := compile.Compile(ctx, projectDir, compile.Options{})
		if err != nil {
			return fmt.Errorf("compare current source: %w", err)
		}
		different, err := staleFiles(filepath.Join(projectDir, "output"), built.Artifact)
		if err != nil {
			return err
		}
		if len(different) > 0 {
			issues = append(issues, diagnostic.Diagnostic{Rule: "build.stale", Severity: diagnostic.Warning, Location: diagnostic.Location{File: "output/", Line: 1, Column: 1}, Message: "output/ is stale against the current source: " + strings.Join(different, ", "), Hint: "Run lfm compile to rebuild output/ from the current source."})
		}
	}
	if browser && readErr == nil {
		root := p.args[0]
		if projectDir != "" {
			root = projectDir
		}
		r.Release = &checkRelease{Audit: release.Audit(ctx, input.Browser, root)}
		for i := range r.Release.Audit.Diagnostics {
			r.Release.Audit.Diagnostics[i].Location.File = input.Options.File
		}
		issues = append(append([]diagnostic.Diagnostic{}, issues...), r.Release.Audit.Diagnostics...)
	}
	summary := diagnostic.Summarize(diagnostic.Apply(issues, overrides))
	r.Validation.Diagnostics = summary.Diagnostics
	r.Validation.Errors = summary.Errors
	r.Validation.Warnings = summary.Warnings
	r.Errors = summary.Errors
	r.Warnings = summary.Warnings
	r.DurationMS = time.Since(start).Milliseconds()
	r.Next = next{Phase: "done", Do: "no findings remain.", Notes: []string{"'lfm rules <id>' explains a rule"}}
	if r.Errors > 0 {
		r.Next.Phase = "fix"
		r.Next.Do = "fix the errors above and check again."
	} else if r.Warnings > 0 {
		r.Next.Phase = "review"
		r.Next.Do = "review the warnings above and check again."
	}
	if doc != nil && doc.LoopbackPreview() {
		r.Next.Notes = append(r.Next.Notes, "Loopback URL requirements are judged on the live URL: the declared public canonical origin is used where available; otherwise public-host and mixed-content findings for loopback URLs are deferred.")
	}
	if !browser && !textTarget {
		r.Next.Notes = append(r.Next.Notes, "run with --release for runtime, layout and Lighthouse")
	}
	if p.has("json") {
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			return err
		}
	} else {
		s := term.For(env.Stdout)
		fmt.Fprintf(env.Stdout, "Check %s\n", r.Target)
		if r.Status != 0 {
			fmt.Fprintf(env.Stdout, "  HTTP %d  %s\n", r.Status, r.URL)
		}
		if r.Identity != "" {
			fmt.Fprintf(env.Stdout, "  identity %s\n", r.Identity)
		}
		fetched := 0
		for _, file := range r.Files {
			if file.Error == "" {
				fetched++
			}
		}
		fmt.Fprintf(env.Stdout, "  %s inspected; %s, %s; %s\n", term.Plural(fetched, "file"), term.Plural(r.Errors, "error"), term.Plural(r.Warnings, "warning"), term.Duration(time.Since(start)))
		names := make([]string, len(r.Skipped))
		for i, rule := range r.Skipped {
			names[i] = rule.Rule
		}
		fmt.Fprintln(env.Stdout, "  skipped: "+term.Wrap(strings.Join(names, ", "), term.Width-11, "           "))
		renderDiagnostics(env.Stdout, s, summary.Diagnostics)
		if r.Release != nil {
			renderRelease(env.Stdout, s, &release.Result{Audit: r.Release.Audit})
		}
		renderNext(env.Stdout, s, r.Next)
	}
	if r.Errors > 0 {
		return errReported
	}
	return nil
}

// staleFiles compares the full output tree, including removed source assets.
func staleFiles(dir string, artifact map[string][]byte) ([]string, error) {
	different := map[string]bool{}
	for name, want := range artifact {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err != nil || !bytes.Equal(got, want) {
			different[name] = true
		}
	}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		if _, ok := artifact[name]; !ok {
			different[name] = true
		}
		return nil
	})
	names := make([]string, 0, len(different))
	for name := range different {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, err
}
