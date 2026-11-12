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
	"github.com/Anonyfox/lfm/internal/cache"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/scratch"
)

// CacheKey includes the complete served artifact and the tools and settings that
// interpret it. Remote targets are mutable and cannot be cached.
func CacheKey(ctx context.Context, target Target, root string) string {
	if target.URL != "" {
		return ""
	}
	tools := target.Tools
	if tools == nil {
		tools = ResolveTools(ctx, root)
	}
	if tools.err != nil {
		return ""
	}
	return artifactKey(target, tools.build, tools.chrome, tools.lighthouse)
}

// ToolIdentity is the immutable tool version snapshot for one invocation.
type ToolIdentity struct {
	build, chrome, lighthouse string
	err                       error
}

// ResolveTools retries a failed or empty version once before giving up. No audit
// is cached under an unknown tool version.
func ResolveTools(ctx context.Context, root string) *ToolIdentity {
	t := &ToolIdentity{}
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
	t.build = identity + "\x00" + absolute
	version := func(tool string, findErr error) (string, error) {
		if findErr != nil {
			return "", findErr
		}
		real, _ := filepath.EvalSymlinks(tool)
		info, statErr := os.Stat(real)
		probeKey := ""
		if statErr == nil {
			sum := sha256.Sum256(fmt.Appendf(nil, "%s:%d:%d", real, info.Size(), info.ModTime().UnixNano()))
			probeKey = "tool-" + hex.EncodeToString(sum[:])
			var cached string
			if !ReadCache(probeKey, &cached).IsZero() && cached != "" {
				return cached, nil
			}
		}
		var err error
		for range 2 {
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			cmd := exec.CommandContext(bounded, tool, "--version")
			cmd.WaitDelay = time.Second
			out, runErr := cmd.Output()
			cancel()
			if v := strings.TrimSpace(string(out)); runErr == nil && v != "" {
				_ = WriteCache(probeKey, v)
				return v, nil
			}
			err = fmt.Errorf("%s --version failed or returned no version: %v", tool, runErr)
			if ctx.Err() != nil {
				break
			}
		}
		return "", err
	}
	t.chrome, t.err = version(BrowserPath())
	if t.err == nil {
		t.lighthouse, t.err = version(LighthousePath((Target{}).toolRoot(root)))
	}
	return t
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

type cacheEnvelope[T any] struct {
	Date  time.Time
	Value T
}

var userCacheDir = os.UserCacheDir

func cacheDir() string {
	dir, err := userCacheDir()
	if err != nil {
		dir = os.TempDir()
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
	var entry cacheEnvelope[json.RawMessage]
	if json.Unmarshal(raw, &entry) != nil || entry.Date.IsZero() || time.Since(entry.Date) > 30*24*time.Hour || json.Unmarshal(entry.Value, value) != nil {
		return time.Time{}
	}
	cache.Touch(filepath.Join(dir, key+".json"))
	return entry.Date
}

// WriteCache atomically saves an optional entry.
// Callers that need to explain lost reuse can report the returned write error.
func WriteCache(key string, value any) error {
	if key == "" {
		return nil
	}
	raw, err := json.Marshal(cacheEnvelope[any]{time.Now().UTC(), value})
	if err != nil {
		return err
	}
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := scratch.WriteFile(filepath.Join(dir, key+".json"), raw); err != nil {
		return err
	}
	return nil
}

type cachedAudit struct {
	Result      *AuditResult
	Diagnostics []diagnostic.Diagnostic
}

func cachedAuditRun(ctx context.Context, target Target, root string, run func(context.Context, Target, string) *AuditResult) *AuditResult {
	if target.Tools == nil && target.URL == "" {
		target.Tools = ResolveTools(ctx, root)
	}
	key := CacheKey(ctx, target, root)
	if result := readCachedAudit(key, root); result != nil {
		return result
	}
	result := run(ctx, target, root)
	if completeAudit(result) {
		var err error
		if target.Tools != nil {
			err = target.Tools.err
		}
		if err == nil {
			err = WriteCache(key, cachedAudit{result, result.Diagnostics})
		}
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, diagnostic.Diagnostic{Rule: "release.runtime", Severity: diagnostic.Warning, Location: diagnostic.Location{File: "index.html", Line: 1, Column: 1}, Message: "The audit could not be cached: " + err.Error(), Hint: "Check the user cache directory is writable and Chrome and Lighthouse --version succeed; then run the release again."})
		}
	}

	return result
}

func completeAudit(result *AuditResult) bool {
	if result == nil || result.Runtime == nil || result.Runtime.Status != "ok" || result.Runtime.Layout == nil || result.Lighthouse == nil || result.Lighthouse.Status != "ok" {
		return false
	}
	return !slices.ContainsFunc(result.Runtime.Layout.Findings, func(f LayoutFinding) bool { return f.Kind == "incomplete" })
}

// CachedAudit returns only a complete audit for the exact served artifact.
func CachedAudit(ctx context.Context, target Target, root string) *AuditResult {
	return readCachedAudit(CacheKey(ctx, target, root), root)
}

func readCachedAudit(key, root string) *AuditResult {
	var entry cachedAudit
	date := ReadCache(key, &entry)
	if date.IsZero() || !completeAudit(entry.Result) {
		return nil
	}
	FramesPath(root)
	ReportPath(root)
	entry.Result.Diagnostics = entry.Diagnostics
	entry.Result.Reused = date.Format(time.RFC3339)
	return entry.Result
}
