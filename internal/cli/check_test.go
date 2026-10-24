package cli

import (
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
	if code, out, _ := run(t, "rules", "links.declared"); code != 0 || !strings.Contains(out, "Needs an lfm project; lfm check skips it.") {
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
		for _, r := range rules {
			if r.Project {
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
