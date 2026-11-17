package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Anonyfox/lfm/internal/cache"
	"github.com/Anonyfox/lfm/internal/scratch"
	"github.com/chromedp/chromedp"
)

// browserProfile reuses one profile per purpose. A busy profile falls back to
// registered scratch space, also cleaned by the CLI on SIGINT/SIGTERM.
func browserProfile(purpose string) (string, func(), error) {
	path := cache.Dir("chrome-" + purpose)
	if err := os.MkdirAll(path, 0o700); err == nil {
		if unlock, err := cache.Lock(path); err == nil {
			if err := clearProfileSingletons(path); err != nil {
				unlock()
				return "", func() {}, err
			}
			cache.Touch(path)
			return path, unlock, nil
		}
	}
	path, err := scratch.Dir("lighthouse-chrome")
	return path, func() { _ = scratch.Remove(path) }, err
}

func profileBrowser(ctx context.Context, opts []chromedp.ExecAllocatorOption, limit time.Duration, purpose string) (context.Context, func(), error) {
	profile, release, err := browserProfile(purpose)
	if err != nil {
		return nil, func() {}, err
	}
	browserProgress(purpose, profile)
	browser, closeBrowser, err := openBrowser(ctx, append(opts, chromedp.UserDataDir(profile)), limit, true)
	if err != nil {
		release()
		return nil, func() {}, browserStartError(purpose, profile, err)
	}
	return browser, func() { closeBrowser(); release() }, nil
}

// Called only after obtaining the reusable profile's exclusive cache lock.
// Remove the entries themselves, never follow Chrome's symlinks.
func clearProfileSingletons(profile string) error {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		if err := os.Remove(filepath.Join(profile, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clean Chrome profile %q: %w; check profile permissions and retry", profile, err)
		}
	}
	return nil
}

func browserPhase(purpose string) string {
	if purpose == "critical" {
		return "critical CSS"
	}
	return purpose
}

func browserProgress(purpose, profile string) {
	fmt.Fprintf(os.Stderr, "lfm: %s: starting Chrome (profile %s)\n", browserPhase(purpose), profile)
}

func browserStartError(purpose, profile string, err error) error {
	return fmt.Errorf("%s: Chrome startup (profile %q): %w; check LFM_CHROME and profile permissions, then retry", browserPhase(purpose), profile, err)
}
