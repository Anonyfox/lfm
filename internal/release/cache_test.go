package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/buildid"
	"github.com/Anonyfox/lfm/internal/diagnostic"
)

func cacheTools(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	previous := userCacheDir
	userCacheDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userCacheDir = previous })
	tool := filepath.Join(t.TempDir(), "version")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho 'test-version-1'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LFM_CHROME", tool)
	t.Setenv("LFM_LIGHTHOUSE", tool)
}

func successfulAudit() *AuditResult {
	return &AuditResult{Runtime: &RuntimeResult{Status: "ok", Layout: &LayoutResult{}}, Lighthouse: &LighthouseResult{Status: "ok"}}
}

func TestArtifactAuditCache(t *testing.T) {
	cacheTools(t)
	target := Target{Page: []byte("page"), Assets: map[string][]byte{"style.css": []byte("css")}}
	calls := 0
	run := func(context.Context, Target, string) *AuditResult {
		calls++
		r := successfulAudit()
		r.Diagnostics = []diagnostic.Diagnostic{{Rule: "release.runtime", Severity: diagnostic.Warning, Message: "problem", Hint: "fix"}}
		return r
	}
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		change func()
		want   int
	}{
		{"cold", func() {}, 1}, {"warm", func() {}, 1},
		{"asset", func() { target.Assets["style.css"] = []byte("changed") }, 2},
		{"page", func() { target.Page = []byte("changed") }, 3},
		{"writer", func() {
			previous := buildid.Writer
			buildid.Writer = func() string { return previous() + " upgraded" }
			t.Cleanup(func() { buildid.Writer = previous })
		}, 4},
		{"corrupt", func() {
			_ = os.WriteFile(filepath.Join(cacheDir(), CacheKey(ctx, target, ".")+".json"), []byte("corrupt"), 0o644)
		}, 5},
		{"incomplete", func() { WriteCache(CacheKey(ctx, target, "."), cachedAudit{Result: &AuditResult{}}) }, 6},
		{"remote", func() { target.URL = "https://example.com/" }, 7}, {"remote again", func() {}, 8},
		{"unwritable", func() {
			target.URL = ""
			blocked := filepath.Join(t.TempDir(), "file")
			_ = os.WriteFile(blocked, []byte("file"), 0o644)
			userCacheDir = func() (string, error) { return blocked, nil }
		}, 9},
		{"unwritable again", func() {}, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			got := cachedAuditRun(ctx, target, ".", run)
			if calls != tc.want || len(got.Diagnostics) != 1 || tc.name == "warm" && got.Reused == "" {
				t.Fatalf("calls=%d result=%+v", calls, got)
			}
		})
	}
	base := artifactKey(target, "lfm", "chrome", "lh")
	for _, key := range []string{artifactKey(target, "new", "chrome", "lh"), artifactKey(target, "lfm", "new", "lh"), artifactKey(target, "lfm", "chrome", "new")} {
		if key == base {
			t.Fatal("tool identity did not change key")
		}
	}
}

// A transient version failure must recover before a successful audit is keyed.
func TestAuditVersionRecovery(t *testing.T) {
	for _, tool := range []string{"LFM_CHROME", "LFM_LIGHTHOUSE"} {
		for _, failure := range []string{"exit 1", "exit 0"} {
			t.Run(tool+failure, func(t *testing.T) {
				cacheTools(t)
				script := filepath.Join(t.TempDir(), "tool")
				counter := script + ".seen"
				if err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\nif [ ! -e %q ]; then touch %q; %s; fi\necho stable-version\n", counter, counter, failure)), 0o755); err != nil {
					t.Fatal(err)
				}
				t.Setenv(tool, script)
				calls := 0
				run := func(context.Context, Target, string) *AuditResult { calls++; return successfulAudit() }
				target := Target{Page: []byte("page")}
				cachedAuditRun(context.Background(), target, ".", run)
				second := cachedAuditRun(context.Background(), target, ".", run)
				if calls != 1 || second.Reused == "" {
					t.Fatalf("successful audit lost after version failure: calls=%d", calls)
				}
			})
		}
	}
}

func TestAuditLayoutCompletion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		layout *LayoutResult
		calls  int
	}{
		{"complete", &LayoutResult{}, 1},
		{"missing", nil, 2},
		{"stopped", &LayoutResult{Findings: []LayoutFinding{{Kind: "incomplete"}}}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cacheTools(t)
			calls := 0
			run := func(context.Context, Target, string) *AuditResult {
				calls++
				r := successfulAudit()
				r.Runtime.Layout = tc.layout
				return r
			}
			for range 2 {
				cachedAuditRun(context.Background(), Target{Page: []byte("page")}, ".", run)
			}
			if calls != tc.calls {
				t.Fatalf("incomplete audit reused: calls=%d", calls)
			}
		})
	}
}

func TestAuditCacheIsBounded(t *testing.T) {
	cacheTools(t)
	for i := range 130 {
		WriteCache(fmt.Sprintf("%03d", i), map[string]int{"entry": i})
	}
	entries, err := os.ReadDir(cacheDir())
	if err != nil || len(entries) != 128 {
		t.Fatalf("cache entries=%d: %v", len(entries), err)
	}
}
