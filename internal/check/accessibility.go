package check

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type accessibilityFacts struct {
	ids     map[string]*html.Node
	labels  map[*html.Node][]*html.Node
	managed map[string][]*html.Node
}

func (d *Document) accessFacts() *accessibilityFacts {
	if d.accessibility != nil {
		return d.accessibility
	}
	f := &accessibilityFacts{ids: map[string]*html.Node{}, labels: map[*html.Node][]*html.Node{}, managed: map[string][]*html.Node{}}
	for _, n := range d.Elements {
		if inTemplate(n) {
			continue
		}
		if id := Attr(n, "id"); id != "" && f.ids[id] == nil {
			f.ids[id] = n
		}
		if id := strings.TrimSpace(Attr(n, "aria-activedescendant")); id != "" {
			f.managed[id] = append(f.managed[id], n)
		}
	}
	for _, n := range d.activeNodes("label") {
		var target *html.Node
		if HasAttr(n, "for") {
			target = f.ids[Attr(n, "for")]
		} else {
			target = firstLabelable(n)
		}
		if target != nil && labelable(target) {
			f.labels[target] = append(f.labels[target], n)
		}
	}
	d.accessibility = f
	return f
}

func labelable(n *html.Node) bool {
	if n == nil || n.Namespace != "" {
		return false
	}
	return n.Data == "input" && !strings.EqualFold(Attr(n, "type"), "hidden") || containsWord("button meter output progress select textarea", n.Data)
}

func nativeReferenceApplies(n *html.Node, key string) bool {
	switch key {
	case "form":
		return n.Namespace == "" && containsWord("button fieldset input object output select textarea", n.Data)
	case "list":
		return n.Namespace == "" && n.Data == "input"
	case "for":
		return n.Namespace == "" && (n.Data == "label" || n.Data == "output")
	case "headers":
		return n.Namespace == "" && (n.Data == "td" || n.Data == "th")
	case "commandfor":
		return n.Namespace == "" && n.Data == "button"
	case "popovertarget":
		return n.Namespace == "" && (n.Data == "button" || n.Data == "input" && strings.EqualFold(Attr(n, "type"), "button"))
	}
	return true // ARIA references can also apply to foreign elements.
}

func firstLabelable(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data == "template" {
			continue
		}
		if labelable(c) {
			return c
		}
		if found := firstLabelable(c); found != nil {
			return found
		}
	}
	return nil
}

func (d *Document) controlName(n *html.Node) string {
	f := d.accessFacts()
	if name := strings.TrimSpace(accessibleName(n, f.ids, map[*html.Node]bool{})); name != "" {
		return name
	}
	var names []string
	for _, label := range f.labels[n] {
		name := accessibleName(label, f.ids, map[*html.Node]bool{})
		if strings.TrimSpace(name) == "" {
			name = visibleText(label)
		}
		names = append(names, name)
	}
	return strings.TrimSpace(strings.Join(names, " "))
}

// ARIA 1.2 concrete roles plus published newer document roles. Unknown tokens
// are not rejected; this list only resolves fallback roles for focused checks.
var knownRoles = words("alert alertdialog application article banner blockquote button caption cell checkbox code columnheader combobox comment complementary contentinfo definition deletion dialog directory document emphasis feed figure form generic grid gridcell group heading img insertion link list listbox listitem log main marquee math menu menubar menuitem menuitemcheckbox menuitemradio meter navigation none note option paragraph presentation progressbar radio radiogroup region row rowgroup rowheader scrollbar search searchbox separator slider spinbutton status strong subscript suggestion superscript switch tab table tablist tabpanel term textbox time timer toolbar tooltip tree treegrid treeitem")
var contentNameRoles = words("button checkbox link menuitem menuitemcheckbox menuitemradio option radio row rowheader columnheader switch tab treeitem")
var namedRoles = words("alertdialog button checkbox combobox dialog grid img link listbox menuitem menuitemcheckbox menuitemradio meter option progressbar radio radiogroup scrollbar searchbox slider spinbutton switch tab textbox tree treegrid treeitem")
var standaloneRoles = words("button checkbox combobox link radio searchbox slider spinbutton switch textbox")

func explicitRole(n *html.Node) string {
	for _, role := range strings.Fields(Attr(n, "role")) {
		if knownRoles[role] {
			return role
		}
	}
	return ""
}

func inactiveHTML(n *html.Node) bool {
	if inTemplate(n) {
		return true
	}
	for p := n; p != nil; p = p.Parent {
		if p.Namespace == "" && (HasAttr(p, "hidden") || HasAttr(p, "inert")) {
			return true
		}
	}
	// discoveryHidden also checks aria-hidden, which is NOT a focus blocker.
	for child, p := n, n.Parent; p != nil; child, p = p, p.Parent {
		if p.Namespace != "" {
			continue
		}
		if p.Data == "dialog" && !HasAttr(p, "open") {
			return true
		}
		if p.Data == "details" && !HasAttr(p, "open") && child != firstSummary(p) {
			return true
		}
	}
	return false
}

func firstSummary(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Namespace == "" && c.Data == "summary" {
			return c
		}
	}
	return nil
}

func within(n, ancestor *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p == ancestor {
			return true
		}
	}
	return false
}

func disabledHTML(n *html.Node) bool {
	if n.Namespace != "" {
		return false
	}
	disablable := containsWord("button input select textarea fieldset optgroup option", n.Data)
	if disablable && HasAttr(n, "disabled") {
		return true
	}
	if !containsWord("button input select textarea", n.Data) {
		return false
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Namespace != "" || p.Data != "fieldset" || !HasAttr(p, "disabled") {
			continue
		}
		var legend *html.Node
		for c := p.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "legend" {
				legend = c
				break
			}
		}
		if legend == nil || !within(n, legend) {
			return true
		}
	}
	return false
}

func tabindex(n *html.Node) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(Attr(n, "tabindex")))
	return value, HasAttr(n, "tabindex") && err == nil
}

func nativeKeyboard(n *html.Node) bool {
	if n.Namespace != "" {
		return n.Namespace == "svg" && n.Data == "a" && HasAttr(n, "href")
	}
	switch n.Data {
	case "button", "select", "textarea":
		return true
	case "input":
		return !strings.EqualFold(Attr(n, "type"), "hidden")
	case "a", "area":
		return HasAttr(n, "href")
	case "summary":
		return n.Parent != nil && n.Parent.Data == "details" && firstSummary(n.Parent) == n
	}
	return false
}

func sequentialCandidate(n *html.Node) bool {
	if inactiveHTML(n) || disabledHTML(n) {
		return false
	}
	if value, valid := tabindex(n); valid {
		return value >= 0
	}
	if nativeKeyboard(n) || n.Namespace == "" && n.Data == "iframe" {
		return true
	}
	if HasAttr(n, "contenteditable") && (Attr(n, "contenteditable") == "" || strings.EqualFold(Attr(n, "contenteditable"), "true") || strings.EqualFold(Attr(n, "contenteditable"), "plaintext-only")) {
		for p := n.Parent; p != nil; p = p.Parent {
			if HasAttr(p, "contenteditable") {
				return false
			}
		}
		return true
	}
	return false
}

func (d *Document) formOwner(n *html.Node) *html.Node {
	if HasAttr(n, "form") {
		target := d.accessFacts().ids[Attr(n, "form")]
		if target != nil && target.Data == "form" && target.Namespace == "" {
			return target
		}
		return nil
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Namespace == "" && p.Data == "form" {
			return p
		}
	}
	return nil
}
