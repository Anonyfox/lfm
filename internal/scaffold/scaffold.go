// Package scaffold creates a new project: the smallest set of files that
// compiles cleanly and shows an agent where everything goes.
package scaffold

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
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
		listed := strings.Join(conflicts, ", ")
		if len(conflicts) > 3 {
			listed = fmt.Sprintf("%s and %d more starter files", strings.Join(conflicts[:3], ", "), len(conflicts)-3)
		}
		if slices.Contains(conflicts, "lfm.json") {
			return nil, fmt.Errorf("%s is already an lfm project (%s); run 'lfm compile' there, or init into an empty or new directory", dir, listed)
		}
		return nil, fmt.Errorf("%s already contains %s; choose an empty or new directory", dir, listed)
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

// IsStarterPage reports whether template is still the page lfm init wrote,
// ignoring line endings and surrounding whitespace.
func IsStarterPage(template []byte) bool {
	starter, err := files.ReadFile("files/index.html.tmpl")
	if err != nil {
		return false
	}
	normalize := func(b []byte) []byte { return bytes.TrimSpace(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))) }
	return bytes.Equal(normalize(template), normalize(starter))
}

// Placeholders are the starter texts and image of lfm init that a finished
// page must have replaced: the description, the social image alt text and the
// social image itself.
func Placeholders() (texts []string, images [][32]byte) {
	raw, err := files.ReadFile("files/lfm.json")
	if err == nil {
		var c struct {
			Description string
			SocialImage struct{ Alt string }
		}
		if json.Unmarshal(raw, &c) == nil {
			texts = append(texts, c.Description, c.SocialImage.Alt)
		}
	}
	if png, err := files.ReadFile("files/assets/images/share.png"); err == nil {
		images = append(images, sha256.Sum256(png))
	}
	return texts, images
}
