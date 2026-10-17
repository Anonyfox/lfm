// Package scaffold creates a new project: the smallest set of files that
// compiles cleanly and shows an agent where everything goes.
package scaffold

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

//go:embed files
var files embed.FS

// Files that cannot be embedded under their real name.
var renames = map[string]string{"gitignore": ".gitignore"}

// Create writes a new project into dir, creating it when absent. It writes
// nothing when any target file already exists, and returns the created paths
// relative to dir.
func Create(dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		return nil, fmt.Errorf("%s is a file, not a directory", dir)
	}
	name := filepath.Base(abs)
	replacer := strings.NewReplacer("__TITLE__", Title(name), "__SLUG__", Slug(name))
	planned := map[string][]byte{}
	err = fs.WalkDir(files, "files", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "files/")
		if renamed, ok := renames[rel]; ok {
			rel = renamed
		}
		if path.Ext(rel) != ".png" {
			raw = []byte(replacer.Replace(string(raw)))
		}
		planned[rel] = raw
		return nil
	})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(planned))
	var conflicts []string
	for rel := range planned {
		names = append(names, rel)
		if _, err := os.Lstat(filepath.Join(abs, rel)); !errors.Is(err, fs.ErrNotExist) {
			conflicts = append(conflicts, rel)
		}
	}
	sort.Strings(names)
	sort.Strings(conflicts)
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("%s already contains %s; choose an empty or new directory", dir, strings.Join(conflicts, ", "))
	}
	for _, rel := range names {
		target := filepath.Join(abs, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, planned[rel], 0o644); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// Title turns a directory name into a page title: "my-tea-shop" → "My tea shop".
func Title(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == '.' || unicode.IsSpace(r) })
	title := strings.Join(words, " ")
	if title == "" {
		return "New page"
	}
	runes := []rune(title)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// Slug turns a directory name into a URL path segment of lowercase ASCII
// letters, digits and single hyphens. Accents are dropped: "Café" → "cafe".
func Slug(name string) string {
	var b bytes.Buffer
	hyphen := false
	for _, r := range strings.ToLower(norm.NFD.String(name)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			hyphen = false
		} else {
			hyphen = true
		}
	}
	if b.Len() == 0 {
		return "page"
	}
	return b.String()
}
