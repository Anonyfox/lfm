package release

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func BrowserPath() (string, error) {
	return resolveTool("LFM_CHROME", browserCandidates())
}

func browserCandidates() []string {
	paths := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
		"chromium-headless-shell", "chrome-headless-shell", "headless_shell", "microsoft-edge", "brave-browser",
		"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/usr/bin/chromium", "/usr/bin/chromium-browser", "/opt/google/chrome/chrome", "/snap/bin/chromium"}
	home, _ := os.UserHomeDir()
	for _, app := range []string{"Google Chrome", "Chromium", "Microsoft Edge", "Brave Browser"} {
		path := filepath.Join("Applications", app+".app", "Contents", "MacOS", app)
		paths = append(paths, "/"+path, filepath.Join(home, path))
	}
	return paths
}

// LighthousePath finds the same executable used by release audits.
func LighthousePath(root string) (string, error) {
	return resolveTool("LFM_LIGHTHOUSE", []string{"lighthouse", "/usr/local/bin/lighthouse", "/usr/bin/lighthouse", "/opt/homebrew/bin/lighthouse",
		filepath.Join(root, "node_modules", ".bin", "lighthouse"), filepath.Join(root, "node_modules", ".bin", "lighthouse.cmd")})
}

// resolveTool never silently replaces an explicit override. npm's prefix is
// queried only after installed candidates fail; no tools are downloaded.
func resolveTool(env string, candidates []string) (string, error) {
	find := func(path string) (string, error) {
		found, err := exec.LookPath(path)
		if err != nil {
			return "", err
		}
		return filepath.Abs(found)
	}
	if path := os.Getenv(env); path != "" {
		found, err := find(path)
		if err != nil {
			return "", fmt.Errorf("%s: %w", env, err)
		}
		return found, nil
	}
	for _, path := range candidates {
		if found, err := find(path); err == nil {
			return found, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npm", "prefix", "-g")
	cmd.WaitDelay = time.Second
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		prefix := strings.TrimSpace(string(out))
		for _, name := range candidates {
			if filepath.Base(name) != name {
				continue
			}
			if found, err := find(filepath.Join(prefix, "bin", name)); err == nil {
				return found, nil
			}
		}
	}
	return "", fmt.Errorf("installed executable not found; install it or set %s to its executable", env)
}
