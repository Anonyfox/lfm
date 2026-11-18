package cli

import (
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/check"
)

// Folder targets include every HTML page for aggregate copy and SEO checks. OpenRoot
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
	var pageIssues []check.Diagnostic
	var pageRules []check.Rule
	for _, rule := range check.DefaultRules() {
		switch rule.ID {
		case "jsonld.faq", "jsonld.identity", "sharing.card":
			pageRules = append(pageRules, rule)
		case "copy.banned", "copy.required":
			if len(first.Options.CopyBans)+len(first.Options.CopyRequires) > 0 {
				pageRules = append(pageRules, rule)
			}
		}
	}
	total := len(first.Raw)
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(name), ".html") || name == "index.html" {
			return nil
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
		// Empty files have no copy and must not add unbounded parsed documents
		// or increase the site's editorial budget without contributing HTML.
		if len(raw) == 0 {
			return nil
		}
		total += len(raw)
		if len(raw) > 8<<20 || total > 32<<20 {
			return fmt.Errorf("site copy check exceeds HTML byte limit at %s", name)
		}
		options := first.Options
		options.File = filepath.ToSlash(name)
		if base, e := url.Parse(options.URL); e == nil {
			options.URL = base.ResolveReference(&url.URL{Path: name}).String()
		}
		options.Assets = nil
		options.Registry = nil // Page-relative references use this page's identity.
		d, err := check.Parse(raw, options)
		if err != nil {
			return err
		}
		var canonical string
		for _, n := range d.Elements {
			if n.Data == "link" && strings.EqualFold(check.Attr(n, "rel"), "canonical") {
				if u, e := url.Parse(check.Attr(n, "href")); e == nil && u.IsAbs() {
					canonical = u.String()
					d.Options.URL = canonical
					break
				}
			}
		}
		loadShareImage(root, d, first.Options.URL)
		pageIssues = append(pageIssues, check.Run(d, pageRules, options.Rules).Diagnostics...)
		// The file-derived URL above is a base for relative resources only.
		d.Options.URL = canonical
		pages = append(pages, d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	issues := append(pageIssues, check.SiteCopy(pages)...)
	issues = append(issues, check.SiteIdentity(pages)...)
	if info, e := root.Stat("sitemap.xml"); e == nil && info.Mode().IsRegular() {
		f, e := root.Open("sitemap.xml")
		if e != nil {
			return issues, nil
		}
		defer f.Close()
		issues = append(issues, check.SitemapDates(pages, io.LimitReader(f, 50<<20))...)
	}
	return issues, nil
}

// Load only the local primary image header for a secondary folder page.
func loadShareImage(root *os.Root, d *check.Document, rootURL string) {
	raw := d.PrimaryShareImage()
	if raw == "" {
		return
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return
	}
	base, err := url.Parse(d.Options.URL)
	if err != nil {
		return
	}
	origin, err := url.Parse(rootURL)
	if err != nil {
		return
	}
	// Without a public root identity, relative paths still identify files beside
	// the checked page. An absolute external image has no proven local mapping.
	if origin.Host == "" {
		if ref.IsAbs() || ref.Host != "" {
			return
		}
		base = &url.URL{Path: "/" + d.Options.File}
	}
	dest := base.ResolveReference(ref)
	directory := origin.ResolveReference(&url.URL{Path: "."})
	if dest.Host != origin.Host || dest.Scheme != origin.Scheme || !strings.HasPrefix(dest.Path, directory.Path) {
		return
	}
	file := strings.TrimPrefix(dest.Path, directory.Path)
	if !fs.ValidPath(file) {
		return
	}
	info, err := root.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	f, err := root.Open(file)
	if err != nil {
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 512<<10))
	if err == nil {
		d.InspectPrimaryShareImage(data)
	}
}
