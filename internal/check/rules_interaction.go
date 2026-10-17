package check

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var htmlNumberPattern = regexp.MustCompile(`^-?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func formsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, n := range d.activeNodes("button, input") {
		if n.Namespace != "" {
			continue
		}
		kind := strings.ToLower(Attr(n, "type"))
		if n.Data == "button" {
			valid := containsWord("button submit reset", kind)
			if HasAttr(n, "type") && !valid {
				issues = append(issues, d.At(n, Warning, "Button type is empty or unrecognized.", "Set type=button for local actions, submit for form submission, or reset for intentional resets; invalid values fall back to Auto."))
			}
			if !HasAttr(n, "type") && d.formOwner(n) != nil && !HasAttr(n, "command") && !HasAttr(n, "commandfor") && (n.Parent == nil || n.Parent.Data != "select") {
				issues = append(issues, d.At(n, Warning, "Button has implicit submit behavior in its form.", "Set type=button for local steps/calculations or type=submit for deliberate submission; this includes buttons associated through form=id."))
			}
			continue
		}
		if kind != "number" && kind != "range" {
			continue
		}
		values := map[string]float64{}
		valid := map[string]bool{}
		for _, key := range []string{"min", "max", "step", "value"} {
			if !HasAttr(n, key) || Attr(n, key) == "" || key == "step" && strings.EqualFold(Attr(n, key), "any") {
				continue
			}
			value := Attr(n, key)
			number, ok := ariaNumber(value)
			ok = ok && htmlNumberPattern.MatchString(value)
			if !ok || key == "step" && number <= 0 {
				expected := "HTML number"
				if key == "step" {
					expected = "positive step"
				}
				issues = append(issues, d.At(n, Warning, "Numeric input "+key+" is not a valid "+expected+".", "Use unformatted finite HTML numbers; step must be positive or any. Keep units/currency in labels and result text."))
			} else {
				values[key], valid[key] = number, true
			}
		}
		if kind == "range" {
			if !HasAttr(n, "min") || Attr(n, "min") == "" {
				values["min"], valid["min"] = 0, true
			}
			if !HasAttr(n, "max") || Attr(n, "max") == "" {
				values["max"], valid["max"] = 100, true
			}
		}
		if valid["min"] && valid["max"] && values["min"] > values["max"] {
			issues = append(issues, d.At(n, Warning, "Numeric input minimum exceeds maximum.", "Fix the bounds and starting value; invalid bounds can make the task impossible or trigger browser clamping."))
		} else if valid["value"] && (valid["min"] && values["value"] < values["min"] || valid["max"] && values["value"] > values["max"]) {
			issues = append(issues, d.At(n, Warning, "Numeric input starts outside its declared/default bounds.", "Choose a valid starting example or correct min/max; range inputs may silently clamp, while number inputs become invalid."))
		}
	}
	return issues
}

func disclosuresRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, n := range d.activeNodes("details") {
		var summaries []*html.Node
		var first *html.Node
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			if first == nil {
				first = c
			}
			if c.Namespace == "" && c.Data == "summary" {
				summaries = append(summaries, c)
			}
		}
		if len(summaries) == 0 {
			issues = append(issues, d.At(n, Warning, "Details has no authored summary control.", "Add a meaningful <summary> as its first element; the browser's generic fallback does not describe this disclosure."))
		}
		if len(summaries) > 1 {
			issues = append(issues, d.At(summaries[1], Error, "Details has multiple summary elements.", "Keep one summary as the first element and move secondary headings/content after it."))
		}
		if len(summaries) > 0 && first != summaries[0] {
			issues = append(issues, d.At(summaries[0], Error, "Summary is not the first element of details.", "Place summary before the disclosed content so it acts as the native toggle."))
		}
	}
	for _, n := range d.activeNodes("button[commandfor]") {
		target := d.accessFacts().ids[Attr(n, "commandfor")]
		if target == nil {
			continue
		}
		command := strings.ToLower(Attr(n, "command"))
		bad := containsWord("show-modal close request-close", command) && !(target.Namespace == "" && target.Data == "dialog") || containsWord("show-popover hide-popover toggle-popover", command) && !(target.Namespace == "" && HasAttr(target, "popover"))
		if bad {
			issues = append(issues, d.At(n, Error, "Built-in command targets the wrong kind of element.", "Use a dialog target for show-modal/close/request-close and a popover target for popover commands; custom --commands remain allowed."))
		}
	}
	return issues
}

func mediaInteractionRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, n := range d.activeNodes("audio[autoplay], video[autoplay]") {
		if !HasAttr(n, "muted") {
			issues = append(issues, d.At(n, Warning, "Media requests autoplay without an initial mute.", "Start muted or require a play gesture. Verify audio, browser autoplay policy and an accessible pause/stop path; duration and actual audio tracks are not known statically."))
		}
	}
	return issues
}
