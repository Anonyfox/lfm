package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompactCountsRunes(t *testing.T) {
	t.Parallel()
	if got := compact("  größe   über\n alles ", 20); got != "größe über alles" {
		t.Fatalf("short multibyte text was cut: %q", got)
	}
	if got := compact("abcdef", 3); got != "abc…" {
		t.Fatalf("%q", got)
	}
}

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

func TestToolDiscoveryHonorsEnvironmentAndProjectBin(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "lighthouse")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("LFM_LIGHTHOUSE", "")
	if found, err := lighthousePath(root); err != nil || found != tool {
		t.Fatalf("project bin: %q %v", found, err)
	}
	t.Setenv("LFM_LIGHTHOUSE", filepath.Join(root, "missing"))
	if _, err := lighthousePath(root); err == nil {
		t.Fatal("a broken LFM_LIGHTHOUSE must not fall back silently")
	}
	t.Setenv("LFM_CHROME", tool)
	if found, err := BrowserPath(); err != nil || found != tool {
		t.Fatalf("LFM_CHROME: %q %v", found, err)
	}
	t.Setenv("LFM_CHROME", filepath.Join(root, "missing"))
	if _, err := BrowserPath(); err == nil || !strings.Contains(err.Error(), "LFM_CHROME") {
		t.Fatalf("broken LFM_CHROME: %v", err)
	}
	if ReportPath(root) == ReportPath(t.TempDir()) || !strings.HasPrefix(ReportPath(root), filepath.Join(os.Getenv("XDG_CACHE_HOME"), "lfm", "lighthouse")) {
		t.Fatal("report paths must be per project, in the user cache")
	}
	if len(browserCandidates()) < 10 {
		t.Fatal("browser candidates")
	}
}
