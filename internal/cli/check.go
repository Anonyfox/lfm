package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/check"
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
	Target string `json:"target"`
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
	name: "check", args: "<url | folder | file.html> [--release] [--json]",
	summary: "Check any built page from a URL or disk",
	details: `Checks any built page with the same rules compile runs on its own output,
from any framework or host: HTML, links, SEO, sharing, accessibility, forms,
performance, design defaults and copy. Rules that need an lfm project are
listed as skipped. Config, templates, research, concept and deliverables
are outside this command's scope.

An http(s) URL fetches the page and its embedded resources, including from
localhost. A folder means index.html; a .html file uses its containing folder.
Static checks execute nothing. --release adds runtime errors, the frame-by-frame
layout walk and Lighthouse when installed. For URLs the browser visits the real
page and runs its JavaScript, including analytics. Files use a sealed artifact server.
--json prints the same findings as the human report.
Exit status: 0 with no errors (warnings allowed); 1 for findings with errors or
an unreadable page; 2 for a mistake in the command line.`,
	flags: []flagSpec{{name: "release", usage: "also run runtime, layout and Lighthouse checks"}, {name: "json", usage: "print the result as JSON instead of the report"}},
	run:   runCheck,
}

func runCheck(ctx context.Context, env Env, p parsed) error {
	if len(p.args) != 1 {
		return usageError("lfm check takes exactly one URL, folder or .html file")
	}
	start := time.Now()
	input, err := target.Read(ctx, p.args[0], env.Build.Version)
	if err != nil {
		if errors.Is(err, target.ErrUsage) {
			return usageError("%s", err)
		}
		if p.has("json") {
			if err := printJSONError(env.Stdout, err); err != nil {
				return err
			}
			return errReported
		}
		return err
	}
	browser := p.has("release")
	rules, skipped := pageRules(browser)
	doc := input.Document
	r := checkResult{Target: p.args[0], Result: input, Validation: check.Run(doc, rules, nil), Skipped: skipped}
	issues := r.Validation.Diagnostics
	if browser {
		root := p.args[0]
		r.Release = &checkRelease{Audit: release.Audit(ctx, input.Browser, root)}
		for i := range r.Release.Audit.Diagnostics {
			r.Release.Audit.Diagnostics[i].Location.File = input.Options.File
		}
		issues = append(append([]diagnostic.Diagnostic{}, issues...), r.Release.Audit.Diagnostics...)
	}
	summary := diagnostic.Summarize(issues)
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
	if !browser {
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
