package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCappedBufferKeepsTheLimitAndReportsTruncation(t *testing.T) {
	t.Parallel()
	b := cappedBuffer{Limit: 5}
	n, err := b.Write([]byte("abc"))
	if n != 3 || err != nil || b.Truncated {
		t.Fatal(n, err, b.Truncated)
	}
	n, _ = b.Write([]byte("defg"))
	if n != 4 || b.String() != "abcde" || !b.Truncated {
		t.Fatalf("%d %q %v", n, b.String(), b.Truncated)
	}
}

func TestLighthouseDiagnostics(t *testing.T) {
	t.Parallel()
	if LighthouseDiagnostics(nil) != nil {
		t.Fatal("nil result must yield nothing")
	}
	got := LighthouseDiagnostics(&LighthouseResult{
		Report:   "/tmp/report.json",
		Findings: []AuditFinding{{ID: "unused-css-rules", Title: "Reduce unused CSS", Value: "12 KiB", Categories: []string{"performance"}, Hint: "Remove dead rules.", Samples: []string{"style.css"}, Items: 3}},
		Warnings: []string{"Chrome extension interfered"},
	})
	if len(got) != 2 || got[0].Rule != "lighthouse.unused-css-rules" || got[1].Rule != "lighthouse.run" {
		t.Fatalf("%+v", got)
	}
	if got[0].Message != "Reduce unused CSS: 12 KiB (scored: performance)" || !strings.Contains(got[0].Hint, "style.css") || !strings.Contains(got[0].Hint, "3 detail rows; full audit: /tmp/report.json") {
		t.Fatalf("%+v", got[0])
	}
}

func TestToolDiscoveryHonorsPrecedence(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "node_modules", ".bin")
	global := filepath.Join(root, "global", "bin")
	path := filepath.Join(root, "path")
	for _, dir := range []string{bin, global, path} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) string {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	projectTool := write(filepath.Join(bin, "lighthouse"), "#!/bin/sh\n")
	chrome := write(filepath.Join(path, "google-chrome"), "#!/bin/sh\n")
	write(filepath.Join(path, "chromium"), "#!/bin/sh\n")
	npmTool := write(filepath.Join(global, "fixture-tool"), "#!/bin/sh\n")
	write(filepath.Join(path, "npm"), "#!/bin/sh\nprintf '%s\\n' \"$LFM_TEST_PREFIX\"\n")
	for _, tc := range []struct {
		name, env, override, want string
		find                      func() (string, error)
	}{
		{"chrome override", "LFM_CHROME", projectTool, projectTool, BrowserPath},
		{"broken chrome override", "LFM_CHROME", filepath.Join(root, "missing"), "", BrowserPath},
		{"chrome PATH order", "LFM_CHROME", "", chrome, BrowserPath},
		{"lighthouse override", "LFM_LIGHTHOUSE", chrome, chrome, func() (string, error) { return LighthousePath(root) }},
		{"broken lighthouse override", "LFM_LIGHTHOUSE", filepath.Join(root, "missing"), "", func() (string, error) { return LighthousePath(root) }},
		{"PATH before install", "LFM_TEST_TOOL", "", chrome, func() (string, error) { return resolveTool("LFM_TEST_TOOL", []string{"google-chrome", projectTool}) }},
		{"installed path", "LFM_TEST_TOOL", "", projectTool, func() (string, error) { return resolveTool("LFM_TEST_TOOL", []string{"missing-tool", projectTool}) }},
		{"npm global bin", "LFM_TEST_TOOL", "", npmTool, func() (string, error) { return resolveTool("LFM_TEST_TOOL", []string{"fixture-tool"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PATH", path)
			t.Setenv("LFM_TEST_PREFIX", filepath.Dir(global))
			t.Setenv(tc.env, tc.override)
			got, err := tc.find()
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
			if err != nil && !strings.Contains(err.Error(), tc.env) {
				t.Fatalf("missing override name: %v", err)
			}
		})
	}
	if ReportPath(root) == ReportPath(t.TempDir()) || !strings.HasPrefix(ReportPath(root), filepath.Join(userCache(t), "lfm", "lighthouse")) {
		t.Fatal("report paths must be per project, in the user cache")
	}
}
