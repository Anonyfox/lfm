package check

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

func focusRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	autofocus := map[*html.Node]*html.Node{}
	for _, n := range d.Elements {
		if d.inTemplate(n) {
			continue
		}
		if HasAttr(n, "tabindex") {
			value, valid := tabindex(n)
			if !valid {
				issues = append(issues, d.At(n, Warning, "tabindex is not a valid integer.", "Use 0 for normal tab order or -1 for managed programmatic focus; remove accidental values."))
			} else if value > 0 {
				issues = append(issues, d.At(n, Warning, "Positive tabindex overrides document focus order.", "Use DOM order and tabindex=0/-1; verify keyboard order after visual rearrangement."))
			}
		}
		if HasAttr(n, "autofocus") {
			var scope *html.Node
			for p := n; p != nil; p = p.Parent {
				scope = p
				if p.Namespace == "" && (p.Data == "dialog" || HasAttr(p, "popover")) {
					break
				}
			}
			if first := autofocus[scope]; first != nil {
				issue := d.At(n, Warning, "Multiple autofocus elements share one document/dialog/popover scope.", "Keep one intentional initial focus target per scope and test entry/return with a keyboard.")
				issue.Related = []Location{d.At(first, Warning, "", "").Location}
				issues = append(issues, issue)
			} else {
				autofocus[scope] = n
			}
		}
		if d.sequentialCandidate(n) {
			for p := n; p != nil; p = p.Parent {
				if strings.EqualFold(Attr(p, "aria-hidden"), "true") {
					issue := d.At(n, Warning, "Tab-order candidate is inside aria-hidden content.", "aria-hidden does not disable focus. Remove it from the active control/ancestor or make the inactive view inert/hidden; verify computed visibility and keyboard behavior.")
					issue.Related = []Location{d.At(p, Warning, "", "").Location}
					issues = append(issues, issue)
					break
				}
			}
		}
		if d.inactiveHTML(n) || disabledHTML(n) {
			continue
		}
		role := explicitRole(n)
		if standaloneRoles[role] && !nativeKeyboard(n) && !strings.EqualFold(Attr(n, "aria-disabled"), "true") {
			if _, valid := tabindex(n); !valid && !managedDescendant(d, n) {
				issues = append(issues, d.At(n, Warning, "Custom "+role+" has no static focus entry.", "Prefer the native control, or provide tabindex and keyboard behavior. Roving tabindex=-1 and an owning aria-activedescendant composite are allowed; test the full navigation path."))
			}
		}
		if n.Namespace == "" && HasAttr(n, "onclick") && (role == "" || role == "none" || role == "presentation") && !nativeKeyboard(n) && !containsWord("a area label details option body html form", n.Data) && !hasControlDescendant(n) && !strings.Contains(n.Data, "-") {
			issues = append(issues, d.At(n, Warning, "Click handler is attached to a non-semantic control.", "Use a button for actions or an anchor with href for navigation. If this is a custom widget, expose its role/name/state and keyboard operation; delegated container handlers need review."))
		}
	}
	return issues
}

func hasControlDescendant(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data == "template" {
			continue
		}
		if nativeKeyboard(c) || namedRoles[explicitRole(c)] || hasControlDescendant(c) {
			return true
		}
	}
	return false
}

func managedDescendant(d *Document, n *html.Node) bool {
	id := Attr(n, "id")
	if id == "" {
		return false
	}
	for _, owner := range d.accessFacts().managed[id] {
		if within(n, owner) || slices.Contains(strings.Fields(Attr(owner, "aria-owns")), id) {
			return true
		}
	}
	return false
}
