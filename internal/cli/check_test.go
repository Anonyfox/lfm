package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/golden"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/scaffold"
)

const checkHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Woodland field notes</title><meta name="description" content="Notes on trees and paths."><link rel="canonical" href="https://example.org/"></head><body><main><h1>Woodland field notes</h1><p>Keep track of the trees and paths you find on your walks.</p></main></body></html>`

func TestCheckCLI(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run(t, "compile", dir); code != 0 {
		t.Fatalf("compile: %s %s", out, errs)
	}
	for _, arg := range []string{filepath.Join(dir, "output"), filepath.Join(dir, "output", "index.html")} {
		if code, out, errs := run(t, "check", arg, "--json"); code != 0 {
			t.Fatalf("clean check %d: %s %s", code, out, errs)
		}
	}
	for _, args := range [][]string{{"check"}, {"check", "a", "b"}, {"check", "ftp://x"}, {"check", "file:///x.html"}, {"check", dir}, {"check", "--unknown"}} {
		code, _, errs := run(t, args...)
		if code != 2 {
			t.Fatalf("%v exit %d: %s", args, code, errs)
		}
	}
	if code, _, errs := run(t, "check", dir); code != 2 || !strings.Contains(errs, "lfm compile") {
		t.Fatal(code, errs)
	}
	missing := filepath.Join(t.TempDir(), "missing.html")
	if code, out, _ := run(t, "check", missing, "--json"); code != 1 || !strings.Contains(out, `"error"`) || !strings.Contains(out, `"next"`) {
		t.Fatal(code, out)
	}
	bad := filepath.Join(t.TempDir(), "bad.html")
	if err := os.WriteFile(bad, []byte(`<p>Broken <div id="x" id="x">page</div>`), 0644); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(t, "check", bad, "--json"); code != 1 || !strings.Contains(out, "html.syntax") {
		t.Fatal(code, out)
	}
	if code, out, _ := run(t, "help", "check"); code != 0 || !strings.Contains(out, "framework") || !strings.Contains(out, "--release") {
		t.Fatal(code, out)
	}
	if code, out, _ := run(t, "rules", "links.declared"); code != 0 || !strings.Contains(out, "Needs an lfm project (needs lfm.json urls); lfm check skips it unless --project supplies the context.") {
		t.Fatal(code, out)
	}
}
func TestCheckUnreadableHTTP(t *testing.T) {
	for _, status := range []int{404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unreadable", status) }))
			defer s.Close()
			code, out, _ := run(t, "check", s.URL, "--json")
			var r map[string]any
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				t.Fatal(err)
			}
			if code != 1 || r["validation"] == nil || r["status"] != float64(status) || r["errors"] != float64(1) || r["next"] == nil {
				t.Fatal(code, out)
			}
		})
	}
}
func TestCheckSkipped(t *testing.T) {
	for _, browser := range []bool{false, true} {
		rules, skipped := pageRules(browser)
		want := 5
		if !browser {
			want = 7
		}
		if len(skipped) != want {
			t.Fatal(skipped)
		}
		for _, r := range check.DefaultRules() {
			if r.Project == "" {
				continue
			}
			found := false
			for _, skip := range skipped {
				if skip.Rule == r.ID && skip.Reason == r.Project {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing project requirement for %s", r.ID)
			}
		}
		for _, r := range rules {
			if r.Project != "" {
				t.Fatal(r.ID)
			}
		}
	}
	raw := []byte(`<html><head><script type="application/ld+json">{}</script><script type="application/ld+json">{}</script></head><body></body></html>`)
	d, err := check.Parse(raw, check.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rules, _ := pageRules(false)
	for _, issue := range check.Run(d, rules, nil).Diagnostics {
		if issue.Rule == "jsonld.block" {
			t.Fatal(issue)
		}
	}
	found := false
	for _, issue := range check.Run(d, check.DefaultRules(), nil).Diagnostics {
		found = found || issue.Rule == "jsonld.block"
	}
	if !found {
		t.Fatal("compile's default rules must retain jsonld.block")
	}
}
func TestCheckGolden(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, checkHTML)
	}))
	defer s.Close()
	for _, format := range []string{"json", "txt"} {
		args := []string{"check", s.URL}
		if format == "json" {
			args = append(args, "--json")
		}
		code, out, errs := run(t, args...)
		if code != 0 || errs != "" {
			t.Fatalf("%d %s %s", code, out, errs)
		}
		out = strings.ReplaceAll(out, s.URL, "http://127.0.0.1:PORT")
		out = regexp.MustCompile(`"durationMs": \d+`).ReplaceAllString(out, `"durationMs": 0`)
		out = regexp.MustCompile(`; (?:<1 ms|\d+ ms|\d+\.\d s)`).ReplaceAllString(out, "; 0 ms")
		golden.Text(t, "check."+format, out)
	}
}
func TestCheckReleaseBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser test")
	}
	if _, err := release.BrowserPath(); err != nil {
		t.Skip(err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, checkHTML)
	}))
	defer s.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(checkHTML), 0644); err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{s.URL, dir} {
		t.Run(arg, func(t *testing.T) {
			code, out, errs := run(t, "check", arg, "--release", "--json")
			var r checkResult
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				t.Fatal(err, out)
			}
			if r.Release == nil || r.Release.Audit == nil {
				t.Fatal(out)
			}
			rt := r.Release.Audit.Runtime
			if rt != nil && (strings.Contains(strings.ToLower(rt.Reason), "operation not permitted") || strings.Contains(strings.ToLower(rt.Reason), "failed to start")) {
				t.Skip("sandbox cannot start Chrome: " + rt.Reason)
			}
			if code != 0 || errs != "" || rt == nil || rt.Status != "ok" || rt.Layout == nil {
				t.Fatalf("%d: %s %s", code, out, errs)
			}
		})
	}
}

func TestCheckInterruptExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cancel(); <-r.Context().Done() }))
	defer server.Close()
	var out, errs bytes.Buffer
	code := Run(ctx, Env{Stdout: &out, Stderr: &errs}, []string{"check", server.URL})
	if code != 130 || !strings.Contains(errs.String(), "interrupted") {
		t.Fatalf("%d %s", code, &errs)
	}
}

// Each banned design must stop both compile and check of the built page, from
// a linked stylesheet, with an error from the rule that owns it.
// testdata/prohibited/receipt.css is the torn-edge receipt lfm.cash used until 0.1.12.
func TestProhibitedDesignsFailCompileAndCheck(t *testing.T) {
	receipt, err := os.ReadFile(filepath.Join("testdata", "prohibited", "receipt.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, css, body, rule, message string }{
		{"warm ground in a custom property", `:root{--paper:#faf7f0}body{background:var(--paper)}`, "", "design.ground", "#faf7f0"},
		{"warm card in a custom property", `:root{--sand:#f1ebdd}.card{background:var(--sand)}`, `<div class="card"><p>Card</p></div>`, "design.ground", "#f1ebdd"},
		{"torn receipt", string(receipt), `<article class="tape"><p class="row total"><span>Sales today</span><span>$0.00</span></p><p class="tear"></p></article>`, "design.ground", "edge"},
		{"violet gradient", `.hero{background:linear-gradient(135deg,#7c3aed,#4f46e5)}`, `<section class="hero"><p>Hero</p></section>`, "design.palette", "violet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hasError := func(stage, out string) {
				t.Helper()
				var result checkResult
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatalf("%s: %v: %s", stage, err, out)
				}
				for _, d := range result.Validation.Diagnostics {
					if d.Rule == tc.rule && d.Severity == check.Error && strings.Contains(strings.ToLower(d.Message), tc.message) {
						return
					}
				}
				t.Fatalf("%s: no %s error mentioning %q: %s", stage, tc.rule, tc.message, out)
			}

			project := t.TempDir()
			if _, err := scaffold.Create(project); err != nil {
				t.Fatal(err)
			}
			template := filepath.Join(project, "index.html.tmpl")
			raw, err := os.ReadFile(template)
			if err != nil {
				t.Fatal(err)
			}
			for name, contents := range map[string]string{
				template: strings.Replace(string(raw), "</main>", tc.body+"</main>", 1),
				filepath.Join(project, "assets/styles/main.css"): tc.css,
			} {
				if err := os.WriteFile(name, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			code, out, errs := run(t, "compile", project, "--json")
			if code == 0 {
				t.Fatalf("compile passed: %s %s", out, errs)
			}
			hasError("compile", out)

			built := t.TempDir()
			page := strings.Replace(checkHTML, "</head>", `<link rel="stylesheet" href="styles.css"></head>`, 1)
			page = strings.Replace(page, "</main>", tc.body+"</main>", 1)
			for name, contents := range map[string]string{"index.html": page, "styles.css": tc.css} {
				if err := os.WriteFile(filepath.Join(built, name), []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			code, out, errs = run(t, "check", built, "--json")
			if code != 1 {
				t.Fatalf("check exit %d: %s %s", code, out, errs)
			}
			hasError("check", out)
		})
	}
}

func TestCheckProjectTargetsAndStaleOutput(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run(t, "compile", dir); code != 0 {
		t.Fatal(code, out, errs)
	}
	page, err := os.ReadFile(filepath.Join(dir, "output", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the live fixture's resource requests local; production share images
	// are checked separately by the sharing rules' fixtures.
	page = regexp.MustCompile(`<meta\b[^>]*\bproperty=["']?og:image["']?(?:\s[^>]*)?>`).ReplaceAll(page, nil)
	files := http.FileServer(http.Dir(filepath.Join(dir, "output")))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(page)
	}))
	defer server.Close()
	for _, target := range []string{"", filepath.Join(dir, "output"), filepath.Join(dir, "output", "index.html"), server.URL} {
		args := []string{"check", "--project", dir, "--json"}
		if target != "" {
			args = append(args, target)
		}
		code, out, errs := run(t, args...)
		var result checkResult
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err, out, errs)
		}
		if code != 0 || result.Project != dir || len(result.Skipped) != 2 || strings.Contains(out, "build.stale") {
			t.Fatal(code, out, errs)
		}
	}
	output := filepath.Join(dir, "output", "index.html")
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, append(raw, []byte("<!--old-->")...), 0644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := run(t, "check", "--project", dir, "--json")
	if code != 0 || !strings.Contains(out, "output/ is stale against the current source: index.html") {
		t.Fatal(code, out, errs)
	}
}

func TestStaleFilesIncludesAddedChangedAndRemoved(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"same": "same", "changed": "old", "removed": "old"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := staleFiles(dir, map[string][]byte{"same": []byte("same"), "changed": []byte("new"), "added": []byte("new")})
	if err != nil || strings.Join(got, ",") != "added,changed,removed" {
		t.Fatal(got, err)
	}
}

func TestCheckProjectRunsDeclaredLinksOnLivePage(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run(t, "compile", dir); code != 0 {
		t.Fatal(code, out, errs)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "output", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	raw = regexp.MustCompile(`<meta\b[^>]*\bproperty=["']?og:image["']?(?:\s[^>]*)?>`).ReplaceAll(raw, nil)
	raw = bytes.Replace(raw, []byte("</body>"), []byte(`<a href="https://undeclared.invalid/unlisted">Unlisted</a></body>`), 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	_, out, errs := run(t, "check", server.URL, "--project", dir, "--json")
	if !strings.Contains(out, `"rule": "links.declared"`) {
		t.Fatal(out, errs)
	}
	_, out, errs = run(t, "check", server.URL, "--json")
	var result checkResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err, out, errs)
	}
	for _, finding := range result.Validation.Diagnostics {
		if finding.Rule == "links.declared" {
			t.Fatal(finding)
		}
	}
}

func TestProjectStaleFindingObeysValidationPolicy(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run(t, "compile", dir); code != 0 {
		t.Fatal(code, out, errs)
	}
	page := filepath.Join(dir, "output", "index.html")
	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, append(raw, []byte("<!-- stale -->")...), 0644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "lfm.json")
	raw, err = os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	for _, level := range []string{"off", "error"} {
		build, ok := values["build"].(map[string]any)
		if !ok {
			build = map[string]any{}
			values["build"] = build
		}
		build["validation"] = map[string]any{"rules": map[string]any{"build.stale": level}}
		raw, err = json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(config, raw, 0644); err != nil {
			t.Fatal(err)
		}
		code, out, errs := run(t, "check", "--project", dir, "--json")
		var report checkResult
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatal(err, out, errs)
		}
		if level == "off" && (code != 0 || strings.Contains(out, `"rule": "build.stale"`)) {
			t.Fatal(code, out, errs)
		}
		if level == "error" && (code != 1 || report.Errors != 1 || !strings.Contains(out, `"rule": "build.stale"`)) {
			t.Fatal(code, out, errs)
		}
	}
}

func TestCheckProjectErrorsRemainJSONAndNameTheSourceKey(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "index.html"), []byte(checkHTML), 0644); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if _, err := scaffold.Create(project); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(project, "lfm.json")
	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	values["url"] = "not a URL"
	raw, err = json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, raw, 0644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{project, filepath.Join(project, "missing")} {
		code, out, errs := run(t, "check", target, "--project", dir, "--json")
		var result struct {
			Error string `json:"error"`
			Next  any    `json:"next"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err, out, errs)
		}
		if code != 1 || result.Error == "" || result.Next == nil {
			t.Fatal(code, out, errs)
		}
		if dir == project && (!strings.Contains(result.Error, "lfm.json") || !strings.Contains(result.Error, "url")) {
			t.Fatal(out)
		}
	}
}

func TestCheckCompiledDesignContext(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	template := filepath.Join(dir, "index.html.tmpl")
	raw, err := os.ReadFile(template)
	if err != nil {
		t.Fatal(err)
	}
	labels := `<style>.labels{font-family:"Courier New",monospace}</style>` + strings.Repeat(`<p class=labels>Reference label</p>`, 8)
	raw = []byte(strings.Replace(string(raw), "</main>", labels+"</main>", 1))
	if err := os.WriteFile(template, raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "concept.md"), []byte("# Concept\n\n## Type\n| Role | Family | Source |\n| --- | --- | --- |\n| Labels | Courier New | System |\n"), 0644); err != nil {
		t.Fatal(err)
	}
	compiled, err := compile.Compile(context.Background(), dir, compile.Options{Publish: true})
	if err != nil {
		t.Fatal(err, compiled.Validation)
	}
	design := func(issues []check.Diagnostic) []string {
		var out []string
		for _, d := range issues {
			if strings.HasPrefix(d.Rule, "design.") {
				out = append(out, d.Rule+": "+string(d.Severity)+": "+d.Message)
			}
		}
		return out
	}
	want := design(compiled.Validation.Diagnostics)
	for _, paired := range []bool{false, true} {
		args := []string{"check", filepath.Join(dir, "output"), "--json"}
		if paired {
			args = append(args, "--project", dir)
		}
		code, out, errs := run(t, args...)
		var result checkResult
		if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 {
			t.Fatal(code, out, errs, err)
		}
		if paired {
			if !reflect.DeepEqual(design(result.Validation.Diagnostics), want) {
				t.Fatalf("compile %v; check %v", want, design(result.Validation.Diagnostics))
			}
		} else {
			found := false
			for _, d := range result.Validation.Diagnostics {
				if d.Rule == "design.mono" {
					found = true
					if !strings.Contains(d.Hint, "--project") || !strings.Contains(d.Hint, "declared faces") {
						t.Fatal(d)
					}
				}
			}
			if !found {
				t.Fatal("standalone check did not detect undeclared mono", out)
			}
		}
	}
}

func TestCheckNetworkFailureReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	target := server.URL
	server.Close()
	code, out, errs := run(t, "check", target, "--json")
	var report checkResult
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err, out)
	}
	if code != 1 || report.Errors != 1 || report.Validation.Diagnostics[0].Rule != "build.failed" || report.Next.Phase != "fix" {
		t.Fatal(code, out, errs)
	}
}

func TestCheckTimeoutReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var out, errs bytes.Buffer
	code := Run(ctx, Env{Stdout: &out, Stderr: &errs}, []string{"check", server.URL, "--json"})
	var report checkResult
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err, out.String())
	}
	if code != 1 || report.Errors != 1 || len(report.Validation.Diagnostics) != 1 || !strings.Contains(report.Validation.Diagnostics[0].Message, "timed out") {
		t.Fatal(code, out.String(), errs.String())
	}
}

func TestCheckReleaseProjectBudgets(t *testing.T) {
	// Browser availability does not change which budgets the static checks use.
	t.Setenv("LFM_CHROME", filepath.Join(t.TempDir(), "missing-chrome"))
	t.Setenv("LFM_LIGHTHOUSE", filepath.Join(t.TempDir(), "missing-lighthouse"))
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "lfm.json")
	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	build, ok := c["build"].(map[string]any)
	if !ok {
		build = map[string]any{}
		c["build"] = build
	}
	build["validation"] = map[string]any{"budgets": map[string]any{"cssKiB": 40}}
	raw, err = json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, strings.Replace(checkHTML, "</head>", "<style>"+strings.Repeat(" ", 21*1024)+"</style></head>", 1))
	}))
	defer server.Close()
	for _, browser := range []bool{false, true} {
		for _, project := range []bool{false, true} {
			args := []string{"check", server.URL, "--json"}
			if browser {
				args = append(args, "--release")
			}
			if project {
				args = append(args, "--project", dir)
			}
			_, out, errs := run(t, args...)
			var result checkResult
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err, out, errs)
			}
			found := false
			for _, d := range result.Validation.Diagnostics {
				if d.Rule == "perf.budgets" && strings.Contains(d.Message, "CSS") {
					found = true
					if !strings.Contains(d.Hint, "--project <dir>") {
						t.Fatal(d)
					}
				}
			}
			if found == project {
				t.Fatalf("release=%v project=%v: %s", browser, project, out)
			}
			if browser && result.Release == nil {
				t.Fatal("release path not exercised")
			}
		}
	}
}

func TestCheckStandaloneCopy(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		args         []string
		code         int
		rule         string
	}{
		{"warn.txt", "Kulinarische Vielfalt.", []string{"--lang", "de"}, 0, "copy.food-filler"},
		{"block.md", "---\nlang: en\n---\nAs an AI language model, I cannot answer.", nil, 1, "copy.internal-leak"},
		{"quote.md", "> As an AI language model, I cannot answer.", nil, 0, ""},
		{"frontmatter.md", "---\nlang: de\n---\nKulinarische Vielfalt.", nil, 0, "copy.food-filler"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := filepath.Join(t.TempDir(), tc.name)
			if err := os.WriteFile(name, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"check", name, "--json"}, tc.args...)
			code, out, errs := run(t, args...)
			var r checkResult
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				t.Fatal(err, out, errs)
			}
			if code != tc.code || r.Next.Do == "" {
				t.Fatal(code, out, errs)
			}
			if tc.rule != "" {
				if len(r.Validation.Diagnostics) != 1 || r.Validation.Diagnostics[0].Rule != tc.rule {
					t.Fatal(out)
				}
			} else if len(r.Validation.Diagnostics) != 0 {
				t.Fatal(out)
			}
		})
	}
}
