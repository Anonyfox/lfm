package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/term"
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
	Seller        Seller            `json:"seller"`
	Links         map[string]any    `json:"links"`
	URLs          map[string]string `json:"urls"`
	JSONLD        any               `json:"jsonld,omitempty"`
	Build         Build             `json:"build"`
	Deliverables  Deliverables      `json:"deliverables"`
	Data          map[string]any    `json:"data"`
}

// Deliverables configures what buyers receive besides the page: a book in
// deliverables/book and a printable in deliverables/printable each need their
// entry, and a bundle sells both together.
type Deliverables struct {
	Book        *BookConfig        `json:"book"`
	Printable   *PrintableConfig   `json:"printable"`
	Bundle      *BundleConfig      `json:"bundle"`
	Spreadsheet *SpreadsheetConfig `json:"spreadsheet"`
	Document    *DocumentConfig    `json:"document"`
	Deck        *DeckConfig        `json:"deck"`
}

// SpreadsheetConfig is deliverables.spreadsheet: how the spreadsheet sells
// and the Google Sheets masters buyers copy.
type SpreadsheetConfig struct {
	Path     string            `json:"path"`
	Offer    string            `json:"offer"`
	Language string            `json:"language"`
	Sell     string            `json:"sell"`
	Version  string            `json:"version"`
	Sheets   map[string]string `json:"sheets"` // edition key, or key-example -> Google Sheets address
}

// DocumentConfig is deliverables.document: how the editable document set
// sells, its paper sizes and the Google Docs masters behind its copy links.
type DocumentConfig struct {
	Path     string            `json:"path"`
	Offer    string            `json:"offer"`
	Language string            `json:"language"`
	Sell     string            `json:"sell"`
	Version  string            `json:"version"`
	Papers   []string          `json:"papers"`
	Docs     map[string]string `json:"docs"` // "language/document" -> Google Docs address
}

// DeckConfig is deliverables.deck: how the decks sell, the papers their
// notes print on and the Google Slides masters behind their copy links.
type DeckConfig struct {
	Path     string            `json:"path"`
	Offer    string            `json:"offer"`
	Language string            `json:"language"`
	Sell     string            `json:"sell"`
	Version  string            `json:"version"`
	Papers   []string          `json:"papers"`
	Slides   map[string]string `json:"slides"` // "language/deck" -> Google Slides address
}

// BundleConfig is deliverables.bundle: two or more deliverables sold
// together, with its own name, listing and download path.
type BundleConfig struct {
	Name     string   `json:"name"`
	Units    []string `json:"units"` // the deliverables it holds; every one when empty
	Path     string   `json:"path"`
	Offer    string   `json:"offer"`
	Sell     string   `json:"sell"` // page (default) or shops
	Language string   `json:"language"`
}

// PrintableConfig is deliverables.printable: the unlisted folder its files
// are served from and the facts printed on its guide page.
type PrintableConfig struct {
	Path     string `json:"path"`
	Offer    string `json:"offer"`
	Language string `json:"language"`
	Version  string `json:"version"` // printed in the guide, e.g. 2027.1
	Sell     string `json:"sell"`    // page (default), shops or bundle
}

// BookConfig is deliverables.book: the facts of the book that are not creative
// decisions, and the unlisted folder its files are served from.
type BookConfig struct {
	Author   string   `json:"author"`
	Language string   `json:"language"`
	Path     string   `json:"path"`
	Offer    string   `json:"offer"`
	Formats  []string `json:"formats"`
	Paper    string   `json:"paper"` // white or cream, for print editions
	ISBN     BookISBN `json:"isbn"`
	Sell     string   `json:"sell"` // page (default), shops or bundle
}

// BookISBN holds an ISBN per edition; each is optional.
type BookISBN struct {
	Ebook     string `json:"ebook"`
	Paperback string `json:"paperback"`
}

// Seller is who sells: printed on the book's copyright page and the
// printable's guide and licence.
type Seller struct {
	Name    string `json:"name"`
	Imprint string `json:"imprint"` // the legal notice lines: address, register, VAT ID
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
	Hint       string // what the field is for and what it takes, from Spec
	AtValue    bool   // the value is wrong, not the key: locate the value
}

// ValueOffset returns where the value of the key at keyOffset starts in raw:
// past the quoted key, the colon and spaces.
func ValueOffset(raw []byte, keyOffset int) int {
	i := keyOffset
	if i < len(raw) && raw[i] == '"' {
		for i++; i < len(raw) && raw[i] != '"'; i++ {
			if raw[i] == '\\' {
				i++
			}
		}
		i++
	}
	for i < len(raw) && strings.ContainsRune(" \t\r\n:", rune(raw[i])) {
		i++
	}
	return min(i, len(raw))
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
		l.Problems = append(l.Problems, Problem{File: file, Path: path, Err: err, Offset: offset})
	}
	if raw, err := p.Read(ConfigFile); err != nil {
		add(ConfigFile, "", err)
	} else {
		l.Sources[ConfigFile] = raw
		shape := checkShape(raw)
		l.Problems = append(l.Problems, shape...)
		if err := DecodeJSON(raw, &l.Config); err != nil {
			if len(shape) == 0 { // a wrong type the shape check reported already
				add(ConfigFile, "", fmt.Errorf("%s: %w (custom fields belong under data)", ConfigFile, err))
			}
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
	if c.URL != "" && !directoryURL(c.URL) {
		add(ConfigFile, "url", fmt.Errorf("url must be an absolute HTTPS directory URL ending in /, without query or fragment; it becomes the page canonical"))
	}
	for _, id := range slices.Sorted(maps.Keys(c.Build.Validation.Rules)) {
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
	for _, name := range slices.Sorted(maps.Keys(c.Build.Assets.Overrides)) {
		path := jsonsrc.Child("build.assets.overrides", name)
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

// checkShape holds lfm.json to Spec: every key known (encoding/json would
// match keys case-insensitively, but the source contract is exact paths),
// every value of its kind and allowed values, every required field present.
// Free fields (data, links, jsonld) are not looked into.
func checkShape(raw []byte) []Problem {
	var root map[string]any
	if DecodeJSON(raw, &root) != nil {
		return nil
	}
	var problems []Problem
	report := func(path string, f Field, format string, args ...any) {
		required := strings.HasSuffix(format, "is required")
		problems = append(problems, Problem{File: ConfigFile, Path: path, Err: fmt.Errorf(format, args...), Hint: f.Hint(), AtValue: !required})
	}
	var walk func(values map[string]any, path, pattern string, mapped bool)
	walk = func(values map[string]any, path, pattern string, mapped bool) {
		for _, key := range slices.Sorted(maps.Keys(values)) {
			child := jsonsrc.Child(path, key)
			childPattern := key
			if mapped {
				childPattern = "*"
			}
			if pattern != "" {
				childPattern = pattern + "." + childPattern
			}
			f, known := specAt(childPattern)
			if !known {
				hint := "Custom values belong under data."
				if to, moved := Moved[child]; moved {
					hint = "It moved to " + to + "."
				}
				var names []string
				for _, sib := range Children(pattern) {
					names = append(names, sib.Name())
				}
				if near := term.Suggest(key, names); near != "" && !strings.HasPrefix(hint, "It moved") {
					hint = "Did you mean " + jsonsrc.Child(path, near) + "?"
				}
				for _, sib := range Children(pattern) {
					if strings.EqualFold(sib.Name(), key) {
						hint = "Use the exact key " + jsonsrc.Child(path, sib.Name()) + "."
					}
				}
				if pattern != "" {
					hint += " 'lfm help config " + strings.ReplaceAll(pattern, ".*", "") + "' lists the options."
				} else {
					hint += " 'lfm help config' lists the options."
				}
				problems = append(problems, Problem{File: ConfigFile, Path: child, Err: fmt.Errorf("unknown field %q", child), Hint: hint})
				continue
			}
			checkValue(values[key], child, childPattern, f, report)
			if nested, ok := values[key].(map[string]any); ok && (f.Kind == Object || f.Kind == Map) {
				walk(nested, child, childPattern, f.Kind == Map)
			}
		}
		for _, f := range Children(pattern) {
			if _, present := values[f.Name()]; f.Required && !present && !mapped {
				report(jsonsrc.Child(path, f.Name()), f, "%s is required", jsonsrc.Child(path, f.Name()))
			}
		}
	}
	walk(root, "", "", false)
	return problems
}

// specAt is Lookup by pattern; a Free field is the end of the known shape.
func specAt(pattern string) (Field, bool) {
	for _, f := range Spec {
		if f.Path == pattern {
			return f, true
		}
	}
	return Field{}, false
}

func checkValue(v any, path, pattern string, f Field, report func(string, Field, string, ...any)) {
	switch f.Kind {
	case String:
		s, ok := v.(string)
		switch {
		case !ok:
			report(path, f, "%s must be a string", path)
		case f.Required && strings.TrimSpace(s) == "":
			report(path, f, "%s is required", path)
		case !f.allowed(s):
			report(path, f, "%s %q is not %s", path, s, orList(f.Values))
		}
	case Int:
		n, ok := v.(json.Number)
		_, err := n.Int64()
		switch {
		case !ok || err != nil:
			report(path, f, "%s must be a whole number", path)
		case !f.allowed(n.String()):
			report(path, f, "%s must be %s", path, orList(f.Values))
		}
	case List:
		items, ok := v.([]any)
		if !ok {
			report(path, f, "%s must be a list", path)
			return
		}
		for i, item := range items {
			s, ok := item.(string)
			switch {
			case !ok:
				report(fmt.Sprintf("%s[%d]", path, i), f, "%s must hold strings", path)
			case !f.allowed(s):
				report(fmt.Sprintf("%s[%d]", path, i), f, "%s: %q is not %s", path, s, orList(f.Values))
			}
		}
	case Object, Map:
		if _, ok := v.(map[string]any); !ok {
			report(path, f, "%s must be an object", path)
		}
	}
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
