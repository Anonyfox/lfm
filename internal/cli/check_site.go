package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/check"
)

// Folder targets include every HTML page for aggregate copy checks. OpenRoot
// keeps reads confined just like the target reader; symlink directories are not
// traversed. URL and explicit file targets remain single-page checks.
func folderCopy(target string, first *check.Document) ([]check.Diagnostic, error) {
	if strings.Contains(target, "://") {
		return check.SiteCopy([]*check.Document{first}), nil
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return check.SiteCopy([]*check.Document{first}), err
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	pages := []*check.Document{first}
	total := len(first.Raw)
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(name), ".html") || name == "index.html" {
			return nil
		}
		if len(pages) >= 512 {
			return fmt.Errorf("site copy check exceeds 512 HTML pages")
		}
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		total += len(raw)
		if len(raw) > 8<<20 || total > 32<<20 {
			return fmt.Errorf("site copy check exceeds HTML byte limit at %s", name)
		}
		options := first.Options
		options.File = filepath.ToSlash(name)
		d, err := check.Parse(raw, options)
		if err != nil {
			return err
		}
		pages = append(pages, d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return check.SiteCopy(pages), nil
}
