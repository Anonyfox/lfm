package release

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ImageIdentity names the renderer and Chrome that determine rendered pixels.
// Bump the version for changes to Printer's loading, media, fitting or capture
// behavior, or the listing composition/rasterization pipeline in compile.
func ImageIdentity(ctx context.Context) string {
	browser, err := BrowserPath()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browser, "--version")
	cmd.WaitDelay = time.Second
	raw, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		return ""
	}
	return "image-renderer-1\x00" + strings.TrimSpace(string(raw))
}

func imageKey(identity string, doc []byte, assets map[string][]byte, width, height int64, scale float64) string {
	if identity == "" {
		return ""
	}
	return artifactKey(Target{Page: doc, Assets: assets}, identity, fmt.Sprintf("%d/%d/%g", width, height, scale), fitScript)
}

func cachedPicture(key string, render func() ([]byte, error)) ([]byte, error) {
	var raw []byte
	if !readCache(filepath.Join(filepath.Dir(cacheDir()), "release-pictures"), key, &raw).IsZero() && len(raw) > 0 {
		return raw, nil
	}
	raw, err := render()
	if err == nil && len(raw) > 0 {
		writeCache(filepath.Join(filepath.Dir(cacheDir()), "release-pictures"), key, raw)
	}
	return raw, err
}

func cachedPageImages(key string, count int, render func() ([][]byte, error)) ([][]byte, error) {
	dir := filepath.Join(filepath.Dir(cacheDir()), "release-pictures")
	var images [][]byte
	if !readCache(dir, key, &images).IsZero() && len(images) == count {
		return images, nil
	}
	images, err := render()
	if err == nil && len(images) == count {
		writeCache(dir, key, images)
	}
	return images, err
}

// ReadImageCache reads a persistent render or complete listing set. Keeping
// these separately prevents image-heavy releases evicting audit reports.
func ReadImageCache(key string, value any) bool {
	return !readCache(filepath.Join(filepath.Dir(cacheDir()), "release-images"), key, value).IsZero()
}

// WriteImageCache saves rendered images even when a bundle does not publish them.
func WriteImageCache(key string, value any) {
	writeCache(filepath.Join(filepath.Dir(cacheDir()), "release-images"), key, value)
}

// ImageSetKey keys a complete listing recipe before expensive page rasters exist.
// The recipe must include every source document, frame and asset it will render.
func ImageSetKey(identity string, recipe []byte, assets map[string][]byte) string {
	return imageKey(identity, recipe, assets, 1500, 1125, 2)
}
