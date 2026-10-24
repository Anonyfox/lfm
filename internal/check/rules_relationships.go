package check

import (
	"strings"

	"golang.org/x/net/html"
)

func relationshipsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	f := d.accessFacts()
	for _, n := range d.Elements {
		if n.Namespace != "" || d.inTemplate(n) {
			continue
		}
		if n.Data == "label" {
			var target *html.Node
			if HasAttr(n, "for") {
				target = f.ids[Attr(n, "for")]
			} else {
				target = firstLabelable(n)
			}
			if target != nil && !labelable(target) && !strings.Contains(target.Data, "-") {
				issues = append(issues, d.At(n, Error, "Label for points to an element that cannot receive a native label.", "Point for at the input/select/textarea or another labelable control; hidden inputs and ordinary containers are not labelable. Use aria-labelledby for custom roles."))
			}
			var visit func(*html.Node)
			visit = func(p *html.Node) {
				for c := p.FirstChild; c != nil; c = c.NextSibling {
					if c.Type != html.ElementNode || c.Data == "template" {
						continue
					}
					if labelable(c) && c != target {
						issues = append(issues, d.At(c, Error, "Label contains a control other than its associated control.", "Give each control its own label; match for/id or wrap only the associated control."))
					}
					visit(c)
				}
			}
			visit(n)
		}
		for _, key := range []string{"form", "list", "popovertarget", "commandfor", "headers"} {
			if !HasAttr(n, key) {
				continue
			}
			if !nativeReferenceApplies(n, key) {
				continue
			}
			refs := []string{Attr(n, key)}
			if key == "headers" {
				refs = strings.Fields(Attr(n, key))
			}
			for _, id := range refs {
				target := f.ids[id]
				if target == nil {
					continue
				} // html.references owns missing targets.
				bad := key == "form" && (target.Data != "form" || target.Namespace != "") || key == "list" && (target.Data != "datalist" || target.Namespace != "") || key == "popovertarget" && !(target.Namespace == "" && HasAttr(target, "popover")) || key == "headers" && (target.Data != "th" || nearestTable(target) != nearestTable(n))
				if bad {
					hint := "Point headers at th IDs in this same table."
					switch key {
					case "form":
						hint = "Point form at the ID of a form element in this document."
					case "list":
						hint = "Point list at the ID of a datalist element in this document."
					case "popovertarget":
						hint = "Point popovertarget at an HTML element with the popover attribute."
					}
					issue := d.At(n, Error, key+" references the wrong kind of target.", hint)
					issue.Related = []Location{d.At(target, Error, "", "").Location}
					issues = append(issues, issue)
				}
			}
		}
	}
	return issues
}

func nearestTable(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Namespace == "" && p.Data == "table" {
			return p
		}
	}
	return nil
}
