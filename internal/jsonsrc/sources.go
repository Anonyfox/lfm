// Package jsonsrc indexes authored JSON documents by field path so diagnostics
// can point at the exact key that caused them.
package jsonsrc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// Sources indexes authored JSON fields using encoding/json's token stream.
// Values are never quoted in diagnostics; only paths, types and locations are.
type Sources struct {
	Raw       map[string][]byte
	Locations map[string]diagnostic.Location
	Leaves    map[string]diagnostic.Location
	Issues    []diagnostic.Diagnostic
	locators  map[string]diagnostic.Source
}

func Index(files map[string][]byte) Sources {
	s := Sources{Raw: files, Locations: map[string]diagnostic.Location{}, Leaves: map[string]diagnostic.Location{}, locators: map[string]diagnostic.Source{}}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s.locators[name] = diagnostic.NewSource(name, files[name])
		prefix := ""
		if strings.HasPrefix(name, "data/") {
			prefix = Child("data", strings.TrimSuffix(path.Base(name), ".json"))
		}
		decoder := json.NewDecoder(bytes.NewReader(files[name]))
		decoder.UseNumber()
		err := s.indexValue(name, prefix, decoder, -1)
		if err == nil {
			if _, tail := decoder.Token(); tail != io.EOF {
				err = fmt.Errorf("expected a single JSON value")
			}
		}
		if err != nil {
			offset := int(decoder.InputOffset())
			if syntax, ok := err.(*json.SyntaxError); ok {
				offset = int(syntax.Offset) - 1
			}
			message := err.Error()
			if errors.Is(err, io.ErrUnexpectedEOF) || err == io.EOF || strings.HasSuffix(message, "EOF") {
				message = "the file ends before its last value closes"
			}
			s.Issues = append(s.Issues, diagnostic.Diagnostic{Rule: "config.invalid", Severity: diagnostic.Error, Location: diagnostic.At(name, files[name], offset), Message: name + " is not valid JSON: " + message + ".", Hint: "Fix the JSON at this place: a missing comma, quote or closing bracket is the usual cause. Nothing else is checked until it parses."})
		}
	}
	return s
}

func (s *Sources) indexValue(file, key string, d *json.Decoder, keyOffset int) error {
	raw := s.Raw[file]
	start := int(d.InputOffset())
	token, err := d.Token()
	if err != nil {
		return err
	}
	if keyOffset < 0 {
		keyOffset = nextValue(raw, start)
	}
	if key != "" {
		s.Locations[key] = s.locators[file].At(keyOffset)
	}
	delim, container := token.(json.Delim)
	if !container {
		if key != "" {
			s.Leaves[key] = s.Locations[key]
		}
		return nil
	}
	count := 0
	switch delim {
	case '{':
		seen := map[string]diagnostic.Location{}
		for d.More() {
			start := int(d.InputOffset())
			nameToken, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := nameToken.(string)
			if !ok {
				return fmt.Errorf("expected an object key")
			}
			child := Child(key, name)
			at := s.locators[file].At(nextValue(raw, start))
			if first, exists := seen[name]; exists {
				s.Issues = append(s.Issues, diagnostic.Diagnostic{Rule: "config.duplicate-key", Severity: diagnostic.Error, Location: at, Path: child, Related: []diagnostic.Location{first}, Message: "Duplicate JSON key at " + child + ".", Hint: "Keep one definition; encoding/json would silently overwrite the earlier value."})
			}
			seen[name] = at
			if err := s.indexValue(file, child, d, at.Offset); err != nil {
				return err
			}
			count++
		}
	case '[':
		for d.More() {
			if err := s.indexValue(file, fmt.Sprintf("%s[%d]", key, count), d, -1); err != nil {
				return err
			}
			count++
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if count == 0 && key != "" {
		s.Leaves[key] = s.Locations[key]
	}
	return nil
}

func nextValue(raw []byte, offset int) int {
	for offset < len(raw) && strings.ContainsRune(" \t\r\n,:", rune(raw[offset])) {
		offset++
	}
	return offset
}

func Child(parent, key string) string {
	identifier := key != ""
	for i, r := range key {
		if !(r == '_' || unicode.IsLetter(r) || i > 0 && unicode.IsDigit(r)) {
			identifier = false
			break
		}
	}
	if identifier {
		if parent == "" {
			return key
		}
		return parent + "." + key
	}
	return parent + "[" + strconv.Quote(key) + "]"
}

func Within(child, parent string) bool {
	return parent == "" || child == parent || strings.HasPrefix(child, parent+".") || strings.HasPrefix(child, parent+"[")
}

func (s Sources) Nearest(key string) (diagnostic.Location, bool) {
	for key != "" {
		if loc, ok := s.Locations[key]; ok {
			return loc, true
		}
		key = Parent(key)
	}
	return diagnostic.Location{}, false
}

func Parent(key string) string {
	last := -1
	quoted, escape := false, false
	for i, r := range key {
		if escape {
			escape = false
			continue
		}
		if quoted {
			if r == '\\' {
				escape = true
			} else if r == '"' {
				quoted = false
			}
			continue
		}
		if r == '"' {
			quoted = true
		} else if r == '.' || r == '[' {
			last = i
		}
	}
	if last < 0 {
		return ""
	}
	return key[:last]
}
