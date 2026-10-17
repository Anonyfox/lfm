package compile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Anonyfox/lfm/internal/templatecheck"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/check"
)

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
	Build         Options           `json:"build"`
	Data          map[string]any    `json:"data"`
}

type SocialImage struct {
	Src string `json:"src"`
	Alt string `json:"alt"`
}

type Options struct {
	Assets      AssetOptions      `json:"assets"`
	BrowserData []string          `json:"browserData"`
	Validation  ValidationOptions `json:"validation"`
}

type ValidationOptions struct {
	Rules map[string]check.Severity `json:"rules"`
}

type AssetOptions struct {
	Default   string            `json:"default"`
	Overrides map[string]string `json:"overrides"`
}

func decodeJSON(raw []byte, target any, strict bool) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if strict {
		d.DisallowUnknownFields()
	}
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected a single JSON value")
	}
	return nil
}

type configProblem struct {
	File, Path string
	Err        error
	Offset     int
}

// encoding/json accepts struct field names case-insensitively. The source
// contract is exact JSON paths, so validate reserved names independently and
// collect every unknown key. Arbitrary links/data keys stay unrestricted.
func configFields(raw []byte) []configProblem {
	var object map[string]any
	if err := decodeJSON(raw, &object, false); err != nil {
		return nil
	}
	schema := map[string][]string{
		"":                 {"schemaVersion", "url", "language", "title", "description", "socialImage", "links", "urls", "jsonld", "build", "data"},
		"socialImage":      {"src", "alt"},
		"build":            {"assets", "browserData", "validation"},
		"build.assets":     {"default", "overrides"},
		"build.validation": {"rules"},
	}
	var problems []configProblem
	var walk func(map[string]any, string)
	walk = func(values map[string]any, path string) {
		allowed, restricted := schema[path]
		if !restricted {
			return
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := templatecheck.Child(path, key)
			known := false
			canonical := ""
			for _, name := range allowed {
				if name == key {
					known = true
				}
				if strings.EqualFold(name, key) {
					canonical = name
				}
			}
			if !known {
				hint := "custom fields belong under data"
				if canonical != "" {
					hint = "use exact key " + templatecheck.Child(path, canonical)
				}
				problems = append(problems, configProblem{File: "lfm.json", Path: child, Err: fmt.Errorf("unknown field %q; %s", child, hint)})
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

func loadConfig(root string) (Config, map[string]any, map[string][]byte, []configProblem) {
	sources := map[string][]byte{}
	var c Config
	var problems []configProblem
	add := func(file, path string, err error) {
		offset := 0
		var mismatch *json.UnmarshalTypeError
		if errors.As(err, &mismatch) {
			offset = int(mismatch.Offset) - 1
			if path == "" {
				path = mismatch.Field
			}
		}
		problems = append(problems, configProblem{file, path, err, offset})
	}
	raw, readErr := readSource(root, "lfm.json")
	if readErr != nil {
		add("lfm.json", "", readErr)
	} else {
		sources["lfm.json"] = raw
	}
	var decodeErr error
	if readErr == nil {
		decodeErr = decodeJSON(raw, &c, false)
		problems = append(problems, configFields(raw)...)
		if decodeErr != nil {
			add("lfm.json", "", fmt.Errorf("lfm.json: %w (custom fields belong under data)", decodeErr))
		}
	}
	if readErr == nil && decodeErr == nil {
		if c.SchemaVersion != 1 {
			add("lfm.json", "schemaVersion", fmt.Errorf("schemaVersion must be 1"))
		}
		u, err := url.Parse(c.URL)
		if err != nil || u.Host == "" || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(c.URL, "#") || !strings.HasSuffix(u.Path, "/") {
			add("lfm.json", "url", fmt.Errorf("url must be an absolute HTTPS directory URL ending in /, without query or fragment (including empty ?/#); use it unchanged for the page canonical"))
		}
		for _, field := range []struct{ name, value string }{{"language", c.Language}, {"title", c.Title}, {"description", c.Description}} {
			if strings.TrimSpace(field.value) == "" {
				add("lfm.json", field.name, fmt.Errorf("%s is required", field.name))
			}
		}
		if c.Build.Assets.Default == "" {
			c.Build.Assets.Default = "auto"
		}
		validMode := func(mode string) bool { return mode == "inline" || mode == "file" }
		ruleKeys := make([]string, 0, len(c.Build.Validation.Rules))
		for key := range c.Build.Validation.Rules {
			ruleKeys = append(ruleKeys, key)
		}
		sort.Strings(ruleKeys)
		for _, key := range ruleKeys {
			if err := check.ValidateOptions(map[string]check.Severity{key: c.Build.Validation.Rules[key]}); err != nil {
				add("lfm.json", templatecheck.Child("build.validation.rules", key), err)
			}
		}
		if c.Build.Assets.Default != "auto" && !validMode(c.Build.Assets.Default) {
			add("lfm.json", "build.assets.default", fmt.Errorf("build.assets.default must be auto, inline or file; omit it for automatic planning"))
		}
		names := make([]string, 0, len(c.Build.Assets.Overrides))
		for name := range c.Build.Assets.Overrides {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			path := templatecheck.Child("build.assets.overrides", name)
			if !validMode(c.Build.Assets.Overrides[name]) {
				add("lfm.json", path, fmt.Errorf("asset override %q must be inline or file", name))
			}
			if !filepath.IsLocal(name) || name == "." || filepath.ToSlash(filepath.Clean(name)) != name {
				add("lfm.json", path, fmt.Errorf("asset override %q must be a clean workbench-relative source path", name))
				continue
			}
			if _, err := sourcePath(root, name); err != nil {
				add("lfm.json", path, fmt.Errorf("asset override: %w", err))
			}
		}
	}
	if c.Data == nil {
		c.Data = map[string]any{}
	}
	if dir, err := optionalDirectory(root, "data"); err != nil {
		add("data", "data", err)
	} else if dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			add("data", "data", err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			file := "data/" + entry.Name()
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				add(file, "data", fmt.Errorf("use flat JSON files named after data keys"))
				continue
			}
			key := strings.TrimSuffix(entry.Name(), ".json")
			path := templatecheck.Child("data", key)
			if strings.Contains(key, ".") || key == "" {
				add(file, path, fmt.Errorf("dataset names cannot contain dots"))
				continue
			}
			raw, err := readSource(root, file)
			if err != nil {
				add(file, path, err)
				continue
			}
			sources[file] = raw
			var value any
			if err := decodeJSON(raw, &value, false); err != nil {
				add(file, path, err)
				continue
			}
			if _, exists := c.Data[key]; exists {
				add(file, path, fmt.Errorf("%s is defined in both lfm.json and %s", path, file))
				continue
			}
			c.Data[key] = value
		}
	}
	if len(problems) > 0 {
		return c, nil, sources, problems
	}
	// Keep template paths identical to JSON field names, including defaults/datasets.
	raw, err := json.Marshal(c)
	if err != nil {
		add("lfm.json", "", err)
		return c, nil, sources, problems
	}
	var values map[string]any
	if err := decodeJSON(raw, &values, false); err != nil {
		add("lfm.json", "", err)
	}
	return c, values, sources, problems
}

func browserJSON(values map[string]any, paths []string) ([]byte, error) {
	selected := map[string]any{}
	for _, path := range paths {
		parts := strings.Split(path, ".")
		var value any = values
		for _, part := range parts {
			object, ok := value.(map[string]any)
			if !ok || part == "" {
				return nil, fmt.Errorf("build.browserData: unknown object path %q", path)
			}
			value, ok = object[part]
			if !ok {
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
	// encoding/json escapes <, > and &; </script> cannot terminate this element.
	return json.Marshal(selected)
}

func sourcePath(root, name string) (string, error) {
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	contained := func(path string) bool {
		rel, err := filepath.Rel(root, path)
		if err != nil || !filepath.IsLocal(rel) {
			return false
		}
		first := strings.Split(filepath.ToSlash(rel), "/")[0]
		return first != "output" && first != ".micro"
	}
	if !contained(path) {
		return "", fmt.Errorf("%s: source must stay inside the workbench, outside output/ and .micro/", name)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	if !contained(real) {
		return "", fmt.Errorf("%s: symlink leaves the permitted source tree", name)
	}
	return real, nil
}

func readSource(root, name string) ([]byte, error) {
	path, err := sourcePath(root, name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func optionalDirectory(root, name string) (string, error) {
	if _, err := os.Lstat(filepath.Join(root, name)); os.IsNotExist(err) {
		return "", nil
	}
	return sourcePath(root, name)
}
