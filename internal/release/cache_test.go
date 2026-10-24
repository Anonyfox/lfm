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

func TestArtifactAuditCache(t *testing.T) {
	cacheTools(t)
	calls := 0
	run := func(context.Context, Target, string) *AuditResult {
		calls++
		return &AuditResult{Runtime: &RuntimeResult{Status: "ok"}, Lighthouse: &LighthouseResult{Status: "ok"}, Diagnostics: []diagnostic.Diagnostic{{Rule: "release.runtime", Severity: diagnostic.Warning, Message: "problem", Hint: "fix"}}}
	}
	target := Target{Page: []byte("page"), Assets: map[string][]byte{"style.css": []byte("css")}}
	ctx := context.Background()
	first := cachedAuditRun(ctx, target, ".", run)
	second := cachedAuditRun(ctx, target, ".", run)
	if calls != 1 || first.Reused != "" || second.Reused == "" || len(second.Diagnostics) != 1 {
		t.Fatalf("%d %+v", calls, second)
	}
	target.Assets["style.css"] = []byte("changed")
	cachedAuditRun(ctx, target, ".", run)
	target.Page = []byte("changed")
	cachedAuditRun(ctx, target, ".", run)
	if calls != 3 {
		t.Fatal("byte change reused audit", calls)
	}
	previous := buildid.Writer
	buildid.Writer = func() string { return previous() + "-upgraded" }
	t.Cleanup(func() { buildid.Writer = previous })
	cachedAuditRun(ctx, target, ".", run)
	if calls != 4 {
		t.Fatal("lfm version change reused audit", calls)
	}
	key := CacheKey(ctx, target, ".")
	if err := os.WriteFile(filepath.Join(cacheDir(), key+".json"), []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	cachedAuditRun(ctx, target, ".", run)
	if calls != 5 {
		t.Fatal("corrupt entry reused audit", calls)
	}
	WriteCache(key, cachedAudit{Result: &AuditResult{}})
	cachedAuditRun(ctx, target, ".", run)
	if calls != 6 {
		t.Fatal("incomplete entry reused audit", calls)
	}
	target.URL = "https://example.com/"
	cachedAuditRun(ctx, target, ".", run)
	cachedAuditRun(ctx, target, ".", run)
	if calls != 8 {
		t.Fatal("live URL reused audit", calls)
	}
}

func TestAuditCacheKeyVersionsAndCorruption(t *testing.T) {
	cacheTools(t)
	target := Target{Page: []byte("page")}
	base := artifactKey(target, "lfm1", "chrome1", "lh1")
	for _, key := range []string{artifactKey(target, "lfm2", "chrome1", "lh1"), artifactKey(target, "lfm1", "chrome2", "lh1"), artifactKey(target, "lfm1", "chrome1", "lh2")} {
		if key == base {
			t.Fatal("version did not change key")
		}
	}
	WriteCache(base, map[string]string{"ok": "yes"})
	if err := os.WriteFile(filepath.Join(cacheDir(), base+".json"), []byte(`{"broken":`), 0o644); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if !ReadCache(base, &got).IsZero() {
		t.Fatal("corrupt cache accepted")
	}
}

func TestUnwritableAuditCacheIsOptional(t *testing.T) {
	cacheTools(t)
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	userCacheDir = func() (string, error) { return blocked, nil }
	calls := 0
	run := func(context.Context, Target, string) *AuditResult {
		calls++
		return &AuditResult{Runtime: &RuntimeResult{Status: "ok"}, Lighthouse: &LighthouseResult{Status: "ok"}}
	}
	for range 2 {
		cachedAuditRun(context.Background(), Target{Page: []byte("page")}, ".", run)
	}
	if calls != 2 {
		t.Fatal("unwritable cache changed audit behavior", calls)
	}
}

func TestAuditCacheIsBounded(t *testing.T) {
	cacheTools(t)
	for i := range 130 {
		WriteCache(fmt.Sprintf("%03d", i), map[string]int{"entry": i})
	}
	entries, err := os.ReadDir(cacheDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 128 {
		t.Fatal("cache count", len(entries))
	}
}
