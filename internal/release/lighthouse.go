package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Anonyfox/lfm/internal/cache"
)

func runLighthouse(ctx context.Context, target Target, browser, root string) *LighthouseResult {
	tool, err := LighthousePath(target.toolRoot(root))
	if err != nil {
		return &LighthouseResult{Status: "skipped", Missing: true, Reason: err.Error(), Hint: LighthouseSetup, Scope: target.auditScope()}
	}
	unlock, err := lighthouseLock(ctx, os.Stderr)
	if err != nil {
		return &LighthouseResult{Status: "skipped", Reason: err.Error(), Hint: "Retry when the running audit finishes; all lfm processes sharing the user cache serialize Lighthouse.", Scope: target.auditScope()}
	}
	defer unlock()
	return measureLighthouse(ctx, func(ctx context.Context) *LighthouseResult {
		return runLighthouseOnce(ctx, target, browser, root, tool)
	})
}

// The lock lives outside lfm's pruned artifact tree. Never unlink it: waiters
// must all flock the same inode, including after the holder crashes.
func lighthouseLock(ctx context.Context, progress io.Writer) (func(), error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(base, "lfm-lighthouse.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	owner := "unknown"
	announced := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("waiting for Lighthouse lock (held by pid %s; wait capped at 10m): %w", owner, err)
		}
		unlock, err := cache.Lock(path)
		if err == nil {
			err = file.Truncate(0)
			if err == nil {
				_, err = file.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
			}
			if err != nil {
				unlock()
				return nil, err
			}
			return unlock, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("lighthouse lock: %w", err)
		}
		if raw, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(string(raw)); err == nil && pid > 0 {
				owner = strconv.Itoa(pid)
			}
		}
		if owner != announced {
			_, _ = fmt.Fprintf(progress, "waiting for Lighthouse lock (held by pid %s)\n", owner)
			announced = owner
		}
		select {
		case <-ctx.Done():
		case <-tick.C:
		}
	}
}

// Linux's one-minute host load describes contention from the whole fleet.
// Elsewhere an unavailable load reading leaves the ordinary 45s budget.
func lighthouseContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	var load float64
	if raw, err := os.ReadFile("/proc/loadavg"); err == nil {
		_, _ = fmt.Fscan(strings.NewReader(string(raw)), &load)
	}
	cpus := runtime.NumCPU()
	limit, err := lighthouseBudget(os.Getenv("LFM_LIGHTHOUSE_TIMEOUT"), load, cpus)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeoutCause(ctx, limit, fmt.Errorf("audit exceeded %s (host load at start %.2f, %d CPUs); wait for load to fall or set LFM_LIGHTHOUSE_TIMEOUT (for example 3m): %w", limit, load, cpus, context.DeadlineExceeded))
	return ctx, cancel, nil
}

func lighthouseBudget(override string, load float64, cpus int) (time.Duration, error) {
	limit := time.Duration(math.Min(300, 45*math.Max(1, load/float64(max(1, cpus))))) * time.Second
	if override != "" {
		var err error
		limit, err = time.ParseDuration(override)
		if err != nil || limit <= 0 {
			return 0, fmt.Errorf("LFM_LIGHTHOUSE_TIMEOUT must be a positive Go duration (for example 3m or 90s)")
		}
	}
	return limit, nil
}
