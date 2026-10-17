package templatecheck

import (
	"fmt"
	htmltemplate "html/template"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

func (c *checker) browserData() {
	for i, path := range c.options.BrowserData {
		v := value{raw: c.options.Values, origin: true}
		valid := true
		for _, part := range strings.Split(path, ".") {
			key := jsonsrc.Child(v.path, part)
			object, ok := v.raw.(map[string]any)
			if !ok || part == "" {
				c.browserIssue(i, path, key, "Selection must follow nonempty object keys separated by dots.")
				valid = false
				break
			}
			raw, exists := object[part]
			if !exists {
				c.browserIssue(i, path, key, missingHint(part, v.path, object))
				valid = false
				break
			}
			v = value{raw: raw, path: key, origin: true}
		}
		if valid {
			c.consume(v, true)
		}
	}
}
func (c *checker) browserIssue(i int, path, key, hint string) {
	at, ok := c.options.Sources.Locations[fmt.Sprintf("build.browserData[%d]", i)]
	if !ok {
		at = diagnostic.At("lfm.json", c.options.Sources.Raw["lfm.json"], 0)
	}
	c.add(diagnostic.Diagnostic{Rule: "template.browser-data", Severity: diagnostic.Error, Location: at, Path: key, Message: fmt.Sprintf("build.browserData[%d] selects unknown path %q.", i, path), Hint: hint})
}

// Normalize actual array subscripts, preserving arbitrary quoted object keys.
func normalizeIndices(path string) string {
	var out strings.Builder
	quoted, escape := false, false
	for i := 0; i < len(path); i++ {
		ch := path[i]
		if escape {
			out.WriteByte(ch)
			escape = false
			continue
		}
		if quoted {
			out.WriteByte(ch)
			if ch == '\\' {
				escape = true
			} else if ch == '"' {
				quoted = false
			}
			continue
		}
		if ch == '"' {
			quoted = true
		}
		if ch == '[' {
			end := i + 1
			for end < len(path) && path[end] >= '0' && path[end] <= '9' {
				end++
			}
			if end > i+1 && end < len(path) && path[end] == ']' {
				out.WriteString("[*]")
				i = end
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func (c *checker) unused(t *htmltemplate.Template) {
	type group struct {
		path  string
		at    diagnostic.Location
		count int
	}
	groups := map[string]*group{}
	paths := make([]string, 0, len(c.options.Sources.Leaves))
	for path := range c.options.Sources.Leaves {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		used := c.exact[path]
		for prefix := path; !used; prefix = jsonsrc.Parent(prefix) {
			if c.whole[prefix] {
				used = true
				break
			}
			if prefix == "" {
				break
			}
		}
		if used {
			continue
		}
		normalized := normalizeIndices(path)
		if g := groups[normalized]; g != nil {
			g.count++
		} else {
			groups[normalized] = &group{normalized, c.options.Sources.Leaves[path], 1}
		}
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		g := groups[name]
		message := "Unused config/data field: " + g.path + "."
		if g.count > 1 {
			message = fmt.Sprintf("Unused config/data field: %s (%d values).", g.path, g.count)
		}
		c.add(diagnostic.Diagnostic{Rule: "template.unused", Severity: diagnostic.Warning, Location: g.at, Path: g.path, Message: message, Hint: "Remove stale data or reference it in the template; runtime-only data belongs in build.browserData. Both conditional branches count as usage."})
	}
	for _, target := range t.Templates() {
		if c.reached[target.Name()] || target.Tree == nil || strings.TrimSpace(target.Tree.Root.String()) == "" {
			continue
		}
		c.add(diagnostic.Diagnostic{Rule: "template.unused", Severity: diagnostic.Warning, Location: nodeLocation(target.Tree, target.Tree.Root, c.options.Files), Message: fmt.Sprintf("Template %q is never called from %q.", target.Name(), c.options.Entry), Hint: "Call it with template/block, or remove an obsolete partial/definition."})
	}
}
