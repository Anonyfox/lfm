package check

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/color"
	"github.com/Anonyfox/lfm/internal/stock"
)

// ConceptFacts are the decisions of concept.md the final page must honor.
type ConceptFacts struct {
	Colors   []color.Color
	Families []string // as written: a family or a stack
	Imagery  int      // planned images
}

// genericFamilies are keywords, not typefaces anyone declares.
var genericFamilies = map[string]bool{
	"serif": true, "sans-serif": true, "monospace": true, "cursive": true, "fantasy": true, "math": true, "emoji": true, "fangsong": true,
	"system-ui": true, "ui-serif": true, "ui-sans-serif": true, "ui-monospace": true, "ui-rounded": true, "-apple-system": true,
	"blinkmacsystemfont": true, "inherit": true, "initial": true, "unset": true, "revert": true, "revert-layer": true,
	"apple color emoji": true, "segoe ui emoji": true, "noto color emoji": true,
}

var fontSize = regexp.MustCompile(`^[0-9.]+(px|rem|em|%|pt|vw|vh|vmin|vmax|ch|ex|lh|rlh)(/.*)?$|^(xx-small|x-small|small|medium|large|x-large|xx-large|smaller|larger)(/.*)?$`)

// families splits a font-family value, or the family part of a font
// shorthand, into normalized names; var() references are skipped.
func families(prop, value string) []string {
	if prop == "font" {
		fields := strings.Fields(value)
		start := -1
		for i, f := range fields {
			if fontSize.MatchString(strings.ToLower(f)) {
				start = i + 1
				break
			}
		}
		if start < 0 || start >= len(fields) {
			return nil
		}
		value = strings.Join(fields[start:], " ")
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		name := stock.Family(part)
		if name == "" || strings.HasPrefix(name, "var(") || genericFamilies[name] {
			continue
		}
		out = append(out, name)
	}
	return out
}

// conceptFamilies returns the normalized typeface names concept.md declares.
func (d *Document) conceptFamilies() map[string]bool {
	declared := map[string]bool{}
	if d.Options.Concept != nil {
		for _, stack := range d.Options.Concept.Families {
			for _, name := range families("font-family", stack) {
				declared[name] = true
			}
		}
	}
	return declared
}

// conceptFidelityRule compares the page with the concept's declared type,
// palette and imagery. Translucent black and white count as shading.
func conceptFidelityRule(d *Document) []Diagnostic {
	facts := d.Options.Concept
	if facts == nil {
		return nil
	}
	var issues []Diagnostic
	m := d.styles()
	type use struct {
		count    int
		selector string
	}
	usedFamilies := map[string]*use{}
	undeclared := map[string]*use{}
	declared := d.conceptFamilies()
	// custom properties, so a family set through a token (font-family:
	// var(--body)) counts as used where the token is used
	custom := map[string][]string{}
	for _, r := range m.rules {
		for _, dcl := range r.decls {
			if strings.HasPrefix(dcl.prop, "--") {
				custom[dcl.prop] = append(custom[dcl.prop], dcl.value)
			}
		}
	}
	for _, decls := range m.inline {
		for _, dcl := range decls {
			if strings.HasPrefix(dcl.prop, "--") {
				custom[dcl.prop] = append(custom[dcl.prop], dcl.value)
			}
		}
	}
	visit := func(selector string, decls []cssDecl) {
		for _, dcl := range decls {
			if dcl.prop == "font-family" || dcl.prop == "font" || strings.HasPrefix(dcl.prop, "--") && strings.Contains(dcl.prop, "font") {
				prop := dcl.prop
				if strings.HasPrefix(prop, "--") {
					prop = "font-family"
				}
				for _, name := range families(prop, expandVars(dcl.value, custom, 0)) {
					u := usedFamilies[name]
					if u == nil {
						u = &use{selector: selector}
						usedFamilies[name] = u
					}
					u.count++
				}
			}
			if len(facts.Colors) == 0 || !colorProps.MatchString(dcl.prop) {
				continue
			}
			for _, c := range color.Parse(dcl.value) {
				if c.Alpha < 0.05 || declaredColor(c, facts.Colors) {
					continue
				}
				key := c.Hex()
				u := undeclared[key]
				if u == nil {
					u = &use{selector: selector}
					undeclared[key] = u
				}
				u.count++
			}
		}
	}
	for _, r := range m.rules {
		visit(r.selector, r.decls)
	}
	for n, decls := range m.inline {
		visit("<"+elementLabel(n)+" style>", decls)
	}
	if len(declared) > 0 {
		for _, name := range sortedNames(usedFamilies) {
			if !declared[name] {
				issues = append(issues, d.Whole(Warning, fmt.Sprintf("Typeface %q is used (%s) but not declared in concept.md.", name, usedFamilies[name].selector), "Use the typefaces the concept declares, or add this one to its Type table with the reason it serves the concept."))
			}
		}
		for _, name := range sortedKeys(declared) {
			if usedFamilies[name] == nil {
				issues = append(issues, d.Whole(Warning, fmt.Sprintf("Typeface %q is declared in concept.md but never used.", name), "Set it where the concept assigns its role, or remove it from the Type table."))
			}
		}
	}
	if len(undeclared) > 0 {
		keys := sortedNames(undeclared)
		sort.SliceStable(keys, func(i, j int) bool { return undeclared[keys[i]].count > undeclared[keys[j]].count })
		var list []string
		for i, k := range keys {
			if i == 6 {
				list = append(list, fmt.Sprintf("%d more", len(keys)-6))
				break
			}
			list = append(list, fmt.Sprintf("%s (%s)", k, undeclared[k].selector))
		}
		issues = append(issues, d.Whole(Warning, fmt.Sprintf("%d colors are not in the concept palette: %s.", len(keys), strings.Join(list, ", ")), "Use the declared palette roles, or add a color to the Palette table with its role and source; tints of a declared color and translucent black or white are fine."))
	}
	if shown := imageryCount(d); shown < facts.Imagery {
		what := "none"
		if shown > 0 {
			what = fmt.Sprintf("%d", shown)
		}
		issues = append(issues, d.Whole(Warning, fmt.Sprintf("concept.md plans %d images, but the page shows %s.", facts.Imagery, what), "Produce the missing images and place them where the concept gives them a job. An image that waits for the owner gets an \"Open:\" line in its Imagery row; a changed plan is changed in concept.md first."))
	}
	return issues
}

// declaredColor accepts a declared color in any opacity, a tint close to one
// (OKLab distance under 0.04), and translucent black or white as shading.
var varRef = regexp.MustCompile(`var\(\s*(--[A-Za-z0-9_-]+)\s*(?:,([^()]*))?\)`)

// expandVars replaces var(--x, fallback) with every value declared for --x,
// joined as one stack, or with the fallback when --x is never declared.
func expandVars(value string, custom map[string][]string, depth int) string {
	if depth > 4 || !strings.Contains(value, "var(") {
		return value
	}
	return varRef.ReplaceAllStringFunc(value, func(ref string) string {
		m := varRef.FindStringSubmatch(ref)
		values := custom[m[1]]
		if len(values) == 0 {
			return m[2]
		}
		return expandVars(strings.Join(values, ", "), custom, depth+1)
	})
}

func declaredColor(c color.Color, palette []color.Color) bool {
	if c.Alpha < 1 && c.Chroma() < 0.02 && (c.L < 0.05 || c.L > 0.97) {
		return true
	}
	for _, p := range palette {
		if c.Distance(p) < 0.04 {
			return true
		}
	}
	return false
}

// imageryCount counts the distinct images a visitor sees: img and video
// sources, canvases, standalone SVG drawings of 64 px or more, background
// image URLs and the social preview image. An image reused counts once.
func imageryCount(d *Document) int {
	seen := map[string]bool{}
	canvases := 0
	for _, n := range d.Elements {
		// a planned share preview lives in the social image metadata
		if n.Data == "meta" {
			if key := strings.ToLower(Attr(n, "property") + Attr(n, "name")); key == "og:image" || key == "twitter:image" {
				seen[Attr(n, "content")] = true
			}
			continue
		}
		if inTemplate(n) || !designVisible(n) {
			continue
		}
		switch n.Data {
		case "img", "video":
			key := Attr(n, "src")
			if key == "" {
				key = Attr(n, "poster")
			}
			if key == "" {
				key = fmt.Sprintf("%p", n)
			}
			seen[key] = true
		case "canvas":
			canvases++
		case "svg":
			if n.Namespace == "svg" && n.Parent != nil && n.Parent.Namespace != "svg" && (Attr(n, "viewBox") != "" || Attr(n, "width") != "") {
				if w := sizeAttr(Attr(n, "width")); w == 0 || w >= 64 {
					seen[fmt.Sprintf("%p", n)] = true
				}
			}
		}
	}
	m := d.styles()
	backgrounds := func(decls []cssDecl) {
		for _, dcl := range decls {
			if strings.HasPrefix(dcl.prop, "background") {
				for _, u := range cssURL.FindAllStringSubmatch(dcl.value, -1) {
					seen["url:"+u[1]] = true
				}
			}
		}
	}
	for _, r := range m.rules {
		backgrounds(r.decls)
	}
	for _, decls := range m.inline {
		backgrounds(decls)
	}
	return len(seen) + canvases
}

var cssURL = regexp.MustCompile(`url\(\s*['"]?([^'")]+)`)

func sizeAttr(v string) int {
	n := 0
	for _, c := range strings.TrimSuffix(strings.TrimSpace(v), "px") {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func sortedNames[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys(m map[string]bool) []string { return sortedNames(m) }
