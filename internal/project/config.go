package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

// Config is lfm.json. Reserved fields use exact casing; product values live
// under data, which data/*.json files extend.
type Config struct {
	SchemaVersion int               `json:"schemaVersion"`
	URL           string            `json:"url"`
	Language      string            `json:"language"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	SocialImage   SocialImage       `json:"socialImage"`
	Links         map[string]any    `json:"links"`
	URLs          map[string]string `json:"urls"`
	JSONLD        any               `json:"jsonld,omitempty"`
	Build         Build             `json:"build"`
	Deliverables  Deliverables      `json:"deliverables"`
	Data          map[string]any    `json:"data"`
}

// Deliverables configures what buyers receive besides the page. A book in
// deliverables/book needs its book entry.
type Deliverables struct {
	Book *BookConfig `json:"book"`
}

// BookConfig is deliverables.book: the facts of the book that are not creative
// decisions, and the unlisted folder its files are served from.
type BookConfig struct {
	Author    string   `json:"author"`
	Language  string   `json:"language"`
	Path      string   `json:"path"`
	Offer     string   `json:"offer"`
	Formats   []string `json:"formats"`
	Publisher string   `json:"publisher"` // imprint name on the title and copyright pages
	Imprint   string   `json:"imprint"`   // free text for the copyright page, e.g. a publisher address
	Paper     string   `json:"paper"`     // white or cream, for print editions
	ISBN      BookISBN `json:"isbn"`
}

// BookISBN holds an ISBN per edition; each is optional.
type BookISBN struct {
	Ebook     string `json:"ebook"`
	Paperback string `json:"paperback"`
}

type SocialImage struct {
	Src string `json:"src"`
	Alt string `json:"alt"`
}

type Build struct {
	Assets      Assets     `json:"assets"`
	BrowserData []string   `json:"browserData"`
	Validation  Validation `json:"validation"`
}

type Validation struct {
	Rules map[string]diagnostic.Severity `json:"rules"`
}

// Assets controls packing. Default "auto" lets the compiler decide per
// resource; "inline" and "file" force a global policy. Overrides pin single
// source paths and always win.
type Assets struct {
	Default   string            `json:"default"`
	Overrides map[string]string `json:"overrides"`
}

// Packing modes.
const (
	Auto   = "auto"
	Inline = "inline"
	File   = "file"
)

// Mode returns the packing mode configured for a project-relative source path.
func (a Assets) Mode(rel string) string {
	if mode := a.Overrides[rel]; mode != "" {
		return mode
	}
	return a.Default
}

// Problem is a config or data defect located by file and JSON path.
type Problem struct {
	File, Path string
	Err        error
	Offset     int
}

// Loaded is the result of reading lfm.json and data/. Values mirrors Config
// as generic JSON so template paths equal JSON field names. Sources holds the
// raw bytes of every file read, for locating diagnostics.
type Loaded struct {
	Config   Config
	Values   map[string]any
	Sources  map[string][]byte
	Problems []Problem
}

// RuleCheck validates one build.validation.rules entry.
type RuleCheck func(id string, level diagnostic.Severity) error

var schema = map[string][]string{
	"":                       {"schemaVersion", "url", "language", "title", "description", "socialImage", "links", "urls", "jsonld", "build", "deliverables", "data"},
	"deliverables":           {"book"},
	"deliverables.book":      {"author", "language", "path", "offer", "formats", "publisher", "imprint", "paper", "isbn"},
	"deliverables.book.isbn": {"ebook", "paperback"},
	"socialImage":            {"src", "alt"},
	"build":                  {"assets", "browserData", "validation"},
	"build.assets":           {"default", "overrides"},
	"build.validation":       {"rules"},
}

// Load reads and validates the configuration. It never stops at the first
// problem: every defect is collected so one compile reports all of them.
func Load(p Project, rules RuleCheck) Loaded {
	l := Loaded{Sources: map[string][]byte{}}
	add := func(file, path string, err error) {
		offset := 0
		var mismatch *json.UnmarshalTypeError
		if errors.As(err, &mismatch) {
			offset = int(mismatch.Offset) - 1
			if path == "" {
				path = mismatch.Field
			}
		}
		l.Problems = append(l.Problems, Problem{file, path, err, offset})
	}
	if raw, err := p.Read(ConfigFile); err != nil {
		add(ConfigFile, "", err)
	} else {
		l.Sources[ConfigFile] = raw
		l.Problems = append(l.Problems, unknownFields(raw)...)
		if err := DecodeJSON(raw, &l.Config); err != nil {
			add(ConfigFile, "", fmt.Errorf("%s: %w (custom fields belong under data)", ConfigFile, err))
		} else {
			l.validate(p, rules, add)
		}
	}
	if l.Config.Data == nil {
		l.Config.Data = map[string]any{}
	}
	l.loadData(p, add)
	if len(l.Problems) > 0 {
		return l
	}
	raw, err := json.Marshal(l.Config)
	if err == nil {
		err = DecodeJSON(raw, &l.Values)
	}
	if err != nil {
		add(ConfigFile, "", err)
	}
	return l
}

func (l *Loaded) validate(p Project, rules RuleCheck, add func(file, path string, err error)) {
	c := &l.Config
	if c.SchemaVersion != 1 {
		add(ConfigFile, "schemaVersion", fmt.Errorf("schemaVersion must be 1"))
	}
	if !directoryURL(c.URL) {
		add(ConfigFile, "url", fmt.Errorf("url must be an absolute HTTPS directory URL ending in /, without query or fragment; it becomes the page canonical"))
	}
	for _, field := range []struct{ name, value string }{{"language", c.Language}, {"title", c.Title}, {"description", c.Description}} {
		if strings.TrimSpace(field.value) == "" {
			add(ConfigFile, field.name, fmt.Errorf("%s is required", field.name))
		}
	}
	for _, id := range sortedKeys(c.Build.Validation.Rules) {
		if rules == nil {
			break
		}
		if err := rules(id, c.Build.Validation.Rules[id]); err != nil {
			add(ConfigFile, jsonsrc.Child("build.validation.rules", id), err)
		}
	}
	if c.Build.Assets.Default == "" {
		c.Build.Assets.Default = Auto
	}
	if mode := c.Build.Assets.Default; mode != Auto && mode != Inline && mode != File {
		add(ConfigFile, "build.assets.default", fmt.Errorf("build.assets.default must be auto, inline or file"))
	}
	for _, name := range sortedKeys(c.Build.Assets.Overrides) {
		path := jsonsrc.Child("build.assets.overrides", name)
		if mode := c.Build.Assets.Overrides[name]; mode != Inline && mode != File {
			add(ConfigFile, path, fmt.Errorf("asset override %q must be inline or file", name))
		}
		if !filepath.IsLocal(name) || filepath.ToSlash(filepath.Clean(name)) != name {
			add(ConfigFile, path, fmt.Errorf("asset override %q must be a clean project-relative path", name))
			continue
		}
		if _, err := p.Path(name); err != nil {
			add(ConfigFile, path, fmt.Errorf("asset override: %w", err))
		}
	}
}

func (l *Loaded) loadData(p Project, add func(file, path string, err error)) {
	dir, err := p.OptionalDir(DataDir)
	if err != nil {
		add(DataDir, "data", err)
		return
	}
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		add(DataDir, "data", err)
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		file := DataDir + "/" + name
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			add(file, "data", fmt.Errorf("use flat JSON files named after data keys"))
			continue
		}
		key := strings.TrimSuffix(name, ".json")
		path := jsonsrc.Child("data", key)
		if key == "" || strings.Contains(key, ".") {
			add(file, path, fmt.Errorf("dataset names cannot contain dots"))
			continue
		}
		raw, err := p.Read(file)
		if err != nil {
			add(file, path, err)
			continue
		}
		l.Sources[file] = raw
		var value any
		if err := DecodeJSON(raw, &value); err != nil {
			add(file, path, err)
			continue
		}
		if _, exists := l.Config.Data[key]; exists {
			add(file, path, fmt.Errorf("%s is defined in both %s and %s", path, ConfigFile, file))
			continue
		}
		l.Config.Data[key] = value
	}
}

// encoding/json matches struct fields case-insensitively. The source contract
// is exact JSON paths, so reserved names are checked separately and every
// unknown key is reported. Free-form links/data/urls stay unrestricted.
func unknownFields(raw []byte) []Problem {
	var object map[string]any
	if DecodeJSON(raw, &object) != nil {
		return nil
	}
	var problems []Problem
	var walk func(map[string]any, string)
	walk = func(values map[string]any, path string) {
		allowed, restricted := schema[path]
		if !restricted {
			return
		}
		for _, key := range sortedKeys(values) {
			child := jsonsrc.Child(path, key)
			if !slices.Contains(allowed, key) {
				hint := "custom fields belong under data"
				for _, name := range allowed {
					if strings.EqualFold(name, key) {
						hint = "use exact key " + jsonsrc.Child(path, name)
					}
				}
				problems = append(problems, Problem{File: ConfigFile, Path: child, Err: fmt.Errorf("unknown field %q; %s", child, hint)})
				continue
			}
			if nested, ok := values[key].(map[string]any); ok {
				walk(nested, child)
			}
		}
	}
	walk(object, "")
	return problems
}

func directoryURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery &&
		!strings.Contains(raw, "#") && strings.HasSuffix(u.Path, "/")
}

// DecodeJSON decodes exactly one JSON value, keeping numbers exact.
func DecodeJSON(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected a single JSON value")
	}
	return nil
}

// BrowserData selects dot-separated object paths from values and returns them
// as JSON with their structure intact. encoding/json escapes <, > and &, so the
// result is safe inside a <script> element.
func BrowserData(values map[string]any, paths []string) ([]byte, error) {
	selected := map[string]any{}
	for _, path := range paths {
		parts := strings.Split(path, ".")
		var value any = values
		for _, part := range parts {
			object, ok := value.(map[string]any)
			if !ok || part == "" {
				return nil, fmt.Errorf("build.browserData: unknown object path %q", path)
			}
			if value, ok = object[part]; !ok {
				return nil, fmt.Errorf("build.browserData: unknown path %q", path)
			}
		}
		destination := selected
		for _, part := range parts[:len(parts)-1] {
			next, ok := destination[part].(map[string]any)
			if !ok {
				next = map[string]any{}
				destination[part] = next
			}
			destination = next
		}
		destination[parts[len(parts)-1]] = value
	}
	return json.Marshal(selected)
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
