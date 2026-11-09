package release

import (
	"context"
	"os"
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
	browser, closeBrowser, err := openBrowser(ctx, append(opts, chromedp.UserDataDir(profile)), limit)
	if err != nil {
		release()
		return nil, func() {}, err
	}
	return browser, func() { closeBrowser(); release() }, nil
}
