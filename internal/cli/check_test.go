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
	"regexp"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
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
			if code != 1 || r["error"] == nil || r["next"] == nil {
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

// A linked stylesheet must reach the shared ground rule when checking a
// prebuilt folder, without compile or project configuration supplying facts.
func TestCheckFolderRejectsCreamWithBrickAccent(t *testing.T) {
	dir := t.TempDir()
	raw := strings.Replace(checkHTML, "</head>", `<link rel="stylesheet" href="styles.css"></head>`, 1)
	for name, contents := range map[string]string{
		"index.html": raw,
		"styles.css": `:root{--ground:#F4EFE6;--accent:#8F3A26}body{background:var(--ground);color:var(--accent)}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errs := run(t, "check", dir, "--json")
	var result checkResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("%v: %s %s", err, out, errs)
	}
	if code != 1 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	for _, d := range result.Validation.Diagnostics {
		if d.Rule == "design.ground" && d.Severity == check.Error && strings.Contains(strings.ToLower(d.Message), "#f4efe6") {
			return
		}
	}
	t.Fatalf("missing design.ground error for cream ground: %s", out)
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
