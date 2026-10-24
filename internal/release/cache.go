package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/buildid"
	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// CacheKey includes the complete served artifact and the tools and settings that
// interpret it. Remote targets are mutable and cannot be cached.
func CacheKey(ctx context.Context, target Target, root string) string {
	if target.URL != "" {
		return ""
	}
	browser, err := BrowserPath()
	if err != nil {
		return ""
	}
	lighthouse, err := lighthousePath(target.toolRoot(root))
	if err != nil {
		return ""
	}
	version := func(tool string) (string, error) {
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(bounded, tool, "--version")
		cmd.WaitDelay = time.Second
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	chrome, err := version(browser)
	if err != nil || chrome == "" {
		return ""
	}
	light, err := version(lighthouse)
	if err != nil || light == "" {
		return ""
	}
	identity := buildid.Writer()
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "-ldflags" {
				identity += "\x00" + setting.Value
			}
		}
	}
	// Reports and saved layout frames belong to this artifact's location.
	absolute, _ := filepath.Abs(root)
	return artifactKey(target, identity+"\x00"+absolute, chrome, light)
}

func artifactKey(target Target, build, chrome, lighthouse string) string {
	h := sha256.New()
	// Bump when audit settings or the cached representation changes.
	for _, part := range []string{"release-cache-1", build, chrome, lighthouse, target.URL, string(target.Page)} {
		fmt.Fprintf(h, "%d:", len(part))
		h.Write([]byte(part))
	}
	names := make([]string, 0, len(target.Assets))
	for name := range target.Assets {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Fprintf(h, "%d:%s%d:", len(name), name, len(target.Assets[name]))
		h.Write(target.Assets[name])
	}
	return hex.EncodeToString(h.Sum(nil))
}

type cacheEnvelope struct {
	Date  time.Time
	Value json.RawMessage
}

var userCacheDir = os.UserCacheDir

func cacheDir() string {
	dir, err := userCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "lfm", "release")
}

// ReadCache treats absent, obsolete and corrupt files as misses.
func ReadCache(key string, value any) time.Time {
	dir := cacheDir()
	if key == "" || dir == "" {
		return time.Time{}
	}
	raw, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err != nil {
		return time.Time{}
	}
	var entry cacheEnvelope
	if json.Unmarshal(raw, &entry) != nil || entry.Date.IsZero() || time.Since(entry.Date) > 30*24*time.Hour || json.Unmarshal(entry.Value, value) != nil {
		return time.Time{}
	}
	return entry.Date
}

// WriteCache is best effort and atomic. Keep at most 128 recent entries.
func WriteCache(key string, value any) {
	dir := cacheDir()
	if key == "" || dir == "" {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	raw, err = json.Marshal(cacheEnvelope{time.Now().UTC(), raw})
	if err != nil {
		return
	}
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	f, err := os.CreateTemp(dir, ".audit-*")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return
	}
	if f.Close() != nil {
		return
	}
	if os.Rename(f.Name(), filepath.Join(dir, key+".json")) != nil {
		return
	}
	entries, _ := os.ReadDir(dir)
	type file struct {
		name string
		date time.Time
	}
	var files []file
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			if info, e := entry.Info(); e == nil {
				files = append(files, file{entry.Name(), info.ModTime()})
			}
		}
	}
	slices.SortFunc(files, func(a, b file) int { return b.date.Compare(a.date) })
	for i, entry := range files {
		if i >= 128 || time.Since(entry.date) > 30*24*time.Hour {
			_ = os.Remove(filepath.Join(dir, entry.name))
		}
	}
}

type cachedAudit struct {
	Result      *AuditResult
	Diagnostics []diagnostic.Diagnostic
}

func cachedAuditRun(ctx context.Context, target Target, root string, run func(context.Context, Target, string) *AuditResult) *AuditResult {
	key := CacheKey(ctx, target, root)
	var entry cachedAudit
	if date := ReadCache(key, &entry); !date.IsZero() && completeAudit(entry.Result) {
		entry.Result.Diagnostics = entry.Diagnostics
		entry.Result.Reused = date.Format(time.RFC3339)
		return entry.Result
	}
	result := run(ctx, target, root)
	if completeAudit(result) {
		WriteCache(key, cachedAudit{result, result.Diagnostics})
	}
	return result
}

func completeAudit(result *AuditResult) bool {
	return result != nil && result.Runtime != nil && result.Runtime.Status == "ok" && result.Lighthouse != nil && result.Lighthouse.Status == "ok"
}
