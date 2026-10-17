package check

import (
	"math"
	"strconv"
	"strings"
)

// A deliberately focused, versioned subset. Do not reject unknown/new ARIA
// attributes or role tokens, infer handlers, or require optional/default states.
var ariaTokens = map[string]string{
	"aria-atomic": "true false", "aria-busy": "true false", "aria-disabled": "true false",
	"aria-hidden": "true false undefined", "aria-modal": "true false", "aria-multiline": "true false",
	"aria-multiselectable": "true false", "aria-readonly": "true false", "aria-required": "true false",
	"aria-checked": "true false mixed undefined", "aria-pressed": "true false mixed undefined",
	"aria-expanded": "true false undefined", "aria-selected": "true false undefined",
	"aria-live": "off polite assertive", "aria-orientation": "horizontal vertical undefined",
	"aria-haspopup": "false true menu listbox tree grid dialog", "aria-autocomplete": "none inline list both",
}

func ariaRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, n := range d.Elements {
		if inTemplate(n) {
			continue
		}
		role := explicitRole(n)
		for _, attr := range n.Attr {
			value := strings.TrimSpace(attr.Val)
			if value == "" {
				continue
			} // Optional empty attributes mean absent; requirements below are separate.
			if allowed := ariaTokens[attr.Key]; allowed != "" && !containsWord(allowed, strings.ToLower(value)) {
				issues = append(issues, d.At(n, Warning, "Invalid token for "+attr.Key+".", "Use one of: "+allowed+"; omit optional states to retain native/default semantics."))
			}
			if attr.Key == "aria-activedescendant" && len(strings.Fields(value)) != 1 {
				issues = append(issues, d.At(n, Warning, "aria-activedescendant must identify one active element.", "Reference one emitted ID and manage the composite's keyboard focus; aria-controls/owns may hold lists."))
			}
			if attr.Key == "aria-valuemin" || attr.Key == "aria-valuemax" || attr.Key == "aria-valuenow" {
				if _, ok := ariaNumber(value); !ok {
					issues = append(issues, d.At(n, Warning, attr.Key+" is not a finite decimal number.", "Use an unformatted finite numeric value; put currency, units or a human description in aria-valuetext/visible text."))
				}
			}
		}
		if containsWord("checkbox radio switch menuitemcheckbox menuitemradio", role) {
			native := n.Namespace == "" && n.Data == "input" && containsWord("checkbox radio", strings.ToLower(Attr(n, "type")))
			value := strings.ToLower(strings.TrimSpace(Attr(n, "aria-checked")))
			if !native && (value == "" || value == "undefined") {
				issues = append(issues, d.At(n, Warning, "Custom "+role+" does not expose its checked state.", "Use a native checkbox/radio or maintain aria-checked=true/false (mixed is allowed for checkboxes)."))
			}
			if value == "mixed" && containsWord("radio switch menuitemradio", role) {
				issues = append(issues, d.At(n, Warning, role+" cannot represent a mixed checked state.", "Use true/false; use a checkbox role when a meaningful mixed state is needed."))
			}
		}
		if role == "slider" && !(n.Namespace == "" && n.Data == "input" && strings.EqualFold(Attr(n, "type"), "range")) && strings.TrimSpace(Attr(n, "aria-valuenow")) == "" {
			issues = append(issues, d.At(n, Warning, "Custom slider has no current numeric value.", "Use a native range input or maintain aria-valuenow with its range and meaningful value text; provide a non-drag input path."))
		}
		if role == "combobox" && !(n.Namespace == "" && n.Data == "select") {
			value := strings.ToLower(strings.TrimSpace(Attr(n, "aria-expanded")))
			if value == "" || value == "undefined" {
				issues = append(issues, d.At(n, Warning, "Custom combobox does not expose expanded/collapsed state.", "Maintain aria-expanded=true/false. When expanded, aria-controls must identify the popup in the emitted/current DOM."))
			}
			if value == "true" && strings.TrimSpace(Attr(n, "aria-controls")) == "" {
				issues = append(issues, d.At(n, Warning, "Expanded combobox has no controlled popup reference.", "Set aria-controls to the popup's emitted ID; a collapsed popup may be created later by JS."))
			}
		}
		min, minOK := ariaNumber(Attr(n, "aria-valuemin"))
		max, maxOK := ariaNumber(Attr(n, "aria-valuemax"))
		now, nowOK := ariaNumber(Attr(n, "aria-valuenow"))
		if containsWord("slider scrollbar progressbar", role) {
			if strings.TrimSpace(Attr(n, "aria-valuemin")) == "" {
				min, minOK = 0, true
			}
			if strings.TrimSpace(Attr(n, "aria-valuemax")) == "" {
				max, maxOK = 100, true
			}
		}
		if minOK && maxOK && min > max {
			issues = append(issues, d.At(n, Warning, "ARIA minimum exceeds maximum.", "Fix the range and update the current value consistently."))
		} else if nowOK && (minOK && now < min || maxOK && now > max) {
			issues = append(issues, d.At(n, Warning, "ARIA current value is outside its declared/default range.", "Align aria-valuenow with min/max and the visible result; do not expose a stale or formatted value."))
		}
	}
	return issues
}

func ariaNumber(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "xXpP_") {
		return 0, false
	}
	n, err := strconv.ParseFloat(value, 64)
	return n, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}
