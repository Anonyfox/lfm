package check

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// genericAlt words describe that something is an image, not what it shows.
var genericAlt = words("image picture photo photograph graphic img icon illustration placeholder untitled bild foto grafik abbildung")

var fileAlt = regexp.MustCompile(`(?i)^\S+\.(jpe?g|png|webp|avif|gif|svg|heic)$|^(img|dsc|dscn|pxl|screenshot)[_ -]?\d`)

func imageAltRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	bySource := map[string]map[string]*html.Node{} // alt → src → first image
	var order []string
	for _, n := range d.activeNodes("img, area, input") {
		if n.Namespace != "" {
			continue
		}
		if n.Data == "input" && !strings.EqualFold(Attr(n, "type"), "image") {
			continue
		}
		if !HasAttr(n, "alt") {
			issues = append(issues, d.At(n, Warning, "Image has no alt attribute.", "Describe informative images; use alt=\"\" for decorative images. An absent alt and an intentionally empty alt are different."))
			continue
		}
		alt := strings.Join(strings.Fields(Attr(n, "alt")), " ")
		switch {
		case alt == "":
		case fileAlt.MatchString(alt), genericAlt[strings.ToLower(strings.Trim(alt, ".!"))]:
			issues = append(issues, d.At(n, Warning, fmt.Sprintf("Alt text %q says nothing about what the image shows.", alt), "Describe what the visitor would miss without the image, in a short phrase; use alt=\"\" when it is purely decorative."))
		case n.Data == "img" && Attr(n, "src") != "":
			if bySource[alt] == nil {
				bySource[alt] = map[string]*html.Node{}
				order = append(order, alt)
			}
			if bySource[alt][Attr(n, "src")] == nil {
				bySource[alt][Attr(n, "src")] = n
			}
		}
	}
	for _, alt := range order {
		if len(bySource[alt]) < 2 {
			continue
		}
		var first *html.Node
		for _, n := range bySource[alt] {
			if first == nil || d.positions[n] < d.positions[first] {
				first = n
			}
		}
		issues = append(issues, d.At(first, Warning, fmt.Sprintf("%d different images share the alt text %q.", len(bySource[alt]), alt), "Describe what each image shows; a repeated description hides what differs between them."))
	}
	return issues
}

func controlsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, n := range d.activeNodes("a[href], area[href], button, input, select, textarea, summary, dialog, iframe, progress, meter, [role]") {
		if n.Namespace != "" && explicitRole(n) == "" && !nativeKeyboard(n) {
			continue
		}
		if !nativeKeyboard(n) && !containsWord("dialog iframe progress meter", n.Data) && !namedRoles[explicitRole(n)] {
			continue
		}
		if n.Data == "input" && strings.EqualFold(Attr(n, "type"), "hidden") {
			continue
		}
		if hidden(n) {
			continue
		}
		name := d.controlName(n)
		if strings.TrimSpace(name) == "" {
			issues = append(issues, d.At(n, Warning, "Control, dialog or informative role has no detectable accessible name.", "Use visible button/link/summary text, an associated native label, aria-label or valid aria-labelledby; placeholders and current values do not name a field."))
		}
	}
	return issues
}

// A focused static name check, not a full accessible-name computation engine.
func accessibleName(n *html.Node, ids map[string]*html.Node, seen map[*html.Node]bool) string {
	if seen[n] {
		return ""
	}
	seen[n] = true
	if refs := strings.Fields(Attr(n, "aria-labelledby")); len(refs) > 0 {
		var names []string
		for _, ref := range refs {
			if target := ids[ref]; target != nil {
				name := strings.TrimSpace(Attr(target, "aria-label"))
				if name == "" {
					kind := strings.ToLower(Attr(target, "type"))
					if target.Namespace == "" && target.Data == "input" && (kind == "" || containsWord("text search email tel url number", kind)) {
						name = Attr(target, "value") // Embedded text control used as a label reference.
					} else {
						name = nameText(target, hidden(target))
					}
				}
				names = append(names, name) // Explicitly referenced hidden labels may name controls.
			}
		}
		if text := strings.TrimSpace(strings.Join(names, " ")); text != "" {
			return text
		}
	}
	if value := strings.TrimSpace(Attr(n, "aria-label")); value != "" {
		return value
	}
	if n.Data == "input" {
		switch strings.ToLower(Attr(n, "type")) {
		case "submit":
			if HasAttr(n, "value") {
				if value := Attr(n, "value"); value != "" {
					return value
				}
				break
			}
			return "Submit"
		case "reset":
			if HasAttr(n, "value") {
				if value := Attr(n, "value"); value != "" {
					return value
				}
				break
			}
			return "Reset"
		case "button":
			if value := Attr(n, "value"); value != "" {
				return value
			}
		case "image":
			if alt := Attr(n, "alt"); alt != "" {
				return alt
			}
		}
	} else if (n.Data == "button" || n.Data == "a" || n.Data == "summary" || contentNameRoles[explicitRole(n)]) && (explicitRole(n) == "" || contentNameRoles[explicitRole(n)]) {
		if text := strings.TrimSpace(visibleText(n)); text != "" {
			return text
		}
	}
	if n.Namespace == "svg" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "title" {
				return Text(c)
			}
		}
	}
	if n.Data == "area" {
		if alt := Attr(n, "alt"); alt != "" {
			return alt
		}
	}
	return Attr(n, "title")
}

func visibleText(n *html.Node) string {
	return nameText(n, false)
}

func nameText(n *html.Node, includeHidden bool) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && ((!includeHidden && (HasAttr(n, "hidden") || Attr(n, "aria-hidden") == "true")) || n.Data == "script" || n.Data == "style" || n.Data == "template") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			b.WriteString(Attr(n, "alt"))
		}
		if n.Type == html.ElementNode && n.Data == "input" {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}

func hidden(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if HasAttr(p, "hidden") || Attr(p, "aria-hidden") == "true" {
			return true
		}
	}
	return false
}
