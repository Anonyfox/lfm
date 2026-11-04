package release

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const fakeReport = `{"lighthouseVersion":"13.5.0","userAgent":"Chrome","categories":{"performance":{"score":0.9,"auditRefs":[{"id":"unused-css-rules","weight":1}]},"accessibility":{"score":1},"best-practices":{"score":1},"seo":{"score":1}},"audits":{"unused-css-rules":{"title":"Reduce unused CSS","description":"Remove dead rules.","score":0.5,"displayValue":"12 KiB"}}}`

// fakeLighthouse installs a lighthouse executable running script and
// returns the project root it belongs to.
func fakeLighthouse(t *testing.T, script string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("browser integration")
	}
	if _, err := BrowserPath(); err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	tool := filepath.Join(root, "lighthouse")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LFM_LIGHTHOUSE", tool)
	t.Cleanup(func() { os.Remove(ReportPath(root)); os.RemoveAll(FramesPath(root)) })
	return root
}

func writeReport(t *testing.T) string {
	t.Helper()
	report := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(report, []byte(fakeReport), 0o644); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestLighthouseRunAttachesToOwnChromeAndKeepsTheReport(t *testing.T) {
	report := writeReport(t)
	args := filepath.Join(t.TempDir(), "args")
	root := fakeLighthouse(t, `echo "$@" > "`+args+`"; cat "`+report+`"`)
	external := filepath.Join(t.TempDir(), "untouched")
	if err := os.WriteFile(external, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, ReportPath(root)); err != nil {
		t.Fatal(err)
	}
	browser, _ := BrowserPath()
	r := runLighthouse(context.Background(), Target{Page: []byte("<!doctype html><title>x</title>"), Assets: nil}, browser, root)
	if r.Status != "ok" || r.Reason != "" || *r.Scores["performance"] != 90 || len(r.Findings) != 1 {
		t.Fatalf("%+v", r)
	}
	if r.Report != ReportPath(root) {
		t.Fatalf("report at %q", r.Report)
	}
	if saved, err := os.ReadFile(r.Report); err != nil || string(saved) != fakeReport {
		t.Fatalf("saved report: %v %q", err, saved)
	}
	if raw, err := os.ReadFile(external); err != nil || string(raw) != "original" {
		t.Fatalf("report followed a symlink: %v %q", err, raw)
	}
	raw, _ := os.ReadFile(args)
	for _, want := range []string{"--output=json", "--form-factor=mobile", "--port=", "http://127.0.0.1:"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("lighthouse ran without %q: %s", want, raw)
		}
	}
}

func TestLighthouseNodeCLICompletion(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip(err)
	}
	for _, tc := range []struct {
		name, tail string
		ok         bool
	}{
		{"complete", "", true},
		{"exit code", "process.exitCode = 3", false},
		{"exception", "throw new Error('CLI failed')", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fakeLighthouse(t, "")
			entry := filepath.Join(root, "lighthouse-cli", "lighthouse", "cli", "index.mjs")
			if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
				t.Fatal(err)
			}
			// A full pipe must drain; completed work must not wait for a stale timer.
			report := fakeReport[:len(fakeReport)-1] + `,"padding":"` + strings.Repeat("x", 1<<20) + `"}`
			script := "#!/usr/bin/env node\nsetTimeout(()=>{}, 30000); process.stdout.write(" + strconv.Quote(report) + ");" + tc.tail
			if err := os.WriteFile(entry, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			tool := filepath.Join(root, "lighthouse")
			if err := os.Remove(tool); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(entry, tool); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			browser, _ := BrowserPath()
			r := runLighthouseOnce(ctx, Target{Page: []byte("<!doctype html>")}, browser, root)
			if (r.Status == "ok") != tc.ok || strings.Contains(r.Reason, "deadline") {
				t.Fatalf("%+v", r)
			}
			if tc.ok {
				if saved, err := os.ReadFile(r.Report); err != nil || string(saved) != report {
					t.Fatalf("report did not drain: %v (%d bytes)", err, len(saved))
				}
			}
		})
	}
}

func TestLighthouseRunReportsFailures(t *testing.T) {
	for _, tc := range []struct{ name, script, reason string }{
		{"exit status", "echo 'Chrome too old' >&2; exit 3", "Chrome too old"},
		{"runtime error", `echo '{"runtimeError":{"message":"NO_FCP"}}'`, "NO_FCP"},
		{"garbage", "echo not json", "invalid Lighthouse JSON"},
		{"parent deadline", "sleep 6", "deadline exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fakeLighthouse(t, tc.script)
			browser, _ := BrowserPath()
			ctx := context.Background()
			if tc.name == "parent deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
			}
			r := runLighthouse(ctx, Target{Page: []byte("<!doctype html>"), Assets: nil}, browser, root)
			if r.Status == "ok" || r.Missing || !strings.Contains(r.Reason, tc.reason) || !strings.Contains(r.Hint, LighthouseSetup) || tc.name == "parent deadline" && strings.Contains(r.Reason, "45 s") {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestAuditTurnsFindingsIntoDiagnostics(t *testing.T) {
	report := writeReport(t)
	root := fakeLighthouse(t, `cat "`+report+`"`)
	page := `<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="></head><body><main><h1>Title</h1><p><a href="#x">Act</a></p><script>throw new Error("audit probe")</script></main></body></html>`
	r := Audit(context.Background(), Target{Page: []byte(page)}, root)
	if r.Runtime.Status != "ok" || r.Lighthouse.Status != "ok" {
		t.Fatalf("%+v %+v", r.Runtime, r.Lighthouse)
	}
	rules := map[string]string{}
	for _, d := range r.Diagnostics {
		rules[d.Rule] += d.Message + "\n"
	}
	if !strings.Contains(rules["release.runtime"], "audit probe") || !strings.Contains(rules["lighthouse.unused-css-rules"], "Reduce unused CSS: 12 KiB (scored: performance)") {
		t.Fatalf("%+v", r.Diagnostics)
	}
	if rules["release.browser"] != "" || rules["release.lighthouse"] != "" {
		t.Fatalf("a working audit warns about its setup: %+v", r.Diagnostics)
	}
}
