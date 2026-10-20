package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	browser, _ := BrowserPath()
	r := runLighthouse(context.Background(), []byte("<!doctype html><title>x</title>"), nil, browser, root)
	if r.Status != "ok" || r.Reason != "" || *r.Scores["performance"] != 90 || len(r.Findings) != 1 {
		t.Fatalf("%+v", r)
	}
	if r.Report != ReportPath(root) {
		t.Fatalf("report at %q", r.Report)
	}
	if saved, err := os.ReadFile(r.Report); err != nil || string(saved) != fakeReport {
		t.Fatalf("saved report: %v %q", err, saved)
	}
	raw, _ := os.ReadFile(args)
	for _, want := range []string{"--output=json", "--form-factor=mobile", "--port=", "http://127.0.0.1:"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("lighthouse ran without %q: %s", want, raw)
		}
	}
}

func TestLighthouseRunReportsFailures(t *testing.T) {
	for _, tc := range []struct{ name, script, reason string }{
		{"exit status", "echo 'Chrome too old' >&2; exit 3", "Chrome too old"},
		{"runtime error", `echo '{"runtimeError":{"message":"NO_FCP"}}'`, "NO_FCP"},
		{"garbage", "echo not json", "invalid Lighthouse JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fakeLighthouse(t, tc.script)
			browser, _ := BrowserPath()
			r := runLighthouse(context.Background(), []byte("<!doctype html>"), nil, browser, root)
			if r.Status == "ok" || r.Missing || !strings.Contains(r.Reason, tc.reason) || !strings.Contains(r.Hint, LighthouseSetup) {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestAuditTurnsFindingsIntoDiagnostics(t *testing.T) {
	report := writeReport(t)
	root := fakeLighthouse(t, `cat "`+report+`"`)
	page := `<!doctype html><html lang=en><head><meta name=viewport content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/png;base64,aA=="></head><body><main><h1>Title</h1><p><a href="#x">Act</a></p><script>throw new Error("audit probe")</script></main></body></html>`
	r := Audit(context.Background(), []byte(page), nil, root)
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
