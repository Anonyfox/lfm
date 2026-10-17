package compile

import (
	"bytes"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"golang.org/x/net/html"
)

type sizingDeclaration struct {
	matcher         cascadia.Matcher
	property, value string
	unconditional   bool
}
type imageSizing struct {
	unknown bool
	rules   []sizingDeclaration
}

// This is a conservative safety screen, not a CSS cascade/layout engine. Union
// every condition; unknown selectors/imports fail closed. Both axes must be
// unconditionally covered by simple valid CSS to add hints under CSS sizing.
func (b *builder) imageSizing(doc *html.Node) imageSizing {
	var policy imageSizing
	walkHTML(doc, func(n *html.Node) {
		if dormantImage(n) {
			return
		}
		if n.Data == "style" {
			policy.readCSS([]byte(textContent(n)), false, hasAttr(n, "media") || hasAttr(n, "title") || hasAttr(n, "disabled"))
		}
		if n.Data == "link" && stylesheetRel(strings.ToLower(attr(n, "rel"))) {
			u, err := url.Parse(attr(n, "href"))
			if err != nil {
				policy.unknown = true
				return
			}
			name := strings.TrimPrefix(u.Path, "./")
			f, ok := b.files[strings.ToLower(name)]
			if !ok {
				policy.unknown = true
				return
			}
			policy.readCSS(f.contents, false, hasAttr(n, "media") || hasAttr(n, "title") || hasAttr(n, "disabled") || relToken(n, "alternate"))
		}
	})
	return policy
}

func relevantSizing(property string) bool {
	switch property {
	case "width", "height", "min-width", "max-width", "min-height", "max-height",
		"inline-size", "block-size", "min-inline-size", "max-inline-size", "min-block-size", "max-block-size",
		"aspect-ratio", "all", "writing-mode", "image-orientation", "image-resolution", "contain", "content-visibility",
		"object-fit", "object-position", "display", "align-self", "align-items", "justify-items", "box-sizing":
		return true
	}
	return strings.HasPrefix(property, "animation") || strings.HasPrefix(property, "transition")
}

func (s *imageSizing) readCSS(raw []byte, inline, conditional bool) {
	input := parse.NewInputBytes(bytes.Clone(raw))
	defer input.Restore()
	p := css.NewParser(input, inline)
	var matcher cascadia.Matcher
	var selector string
	depth := 0
	for {
		kind, _, data := p.Next()
		var value strings.Builder
		for _, token := range p.Values() {
			value.Write(token.Data)
		}
		switch kind {
		case css.ErrorGrammar:
			if p.Err() != io.EOF {
				s.unknown = true
			}
			return
		case css.AtRuleGrammar:
			if strings.EqualFold(strings.TrimPrefix(string(data), "@"), "import") {
				s.unknown = true
			}
		case css.BeginAtRuleGrammar:
			depth++
		case css.EndAtRuleGrammar:
			depth--
		case css.BeginRulesetGrammar:
			selector = value.String()
			// Adding an attribute must not activate an authored [width]/[height]
			// selector, even one that only changes color or a parent's layout.
			lower := strings.ToLower(selector)
			if strings.Contains(selector, "[") && (strings.Contains(lower, "width") || strings.Contains(lower, "height") || strings.Contains(selector, "\\")) {
				s.unknown = true
			}
			matcher = nil
			if !strings.ContainsAny(selector, ":&\\") {
				matcher, _ = cascadia.Compile(selector)
			}
		case css.QualifiedRuleGrammar:
			s.unknown = true
		case css.DeclarationGrammar:
			property := strings.ToLower(string(data))
			if relevantSizing(property) {
				s.rules = append(s.rules, sizingDeclaration{matcher, property, strings.ToLower(strings.TrimSpace(value.String())), !conditional && depth == 0 && (inline || selector != "" && matcher != nil)})
			}
		}
	}
}

func simpleSize(value string) bool {
	value = strings.TrimSpace(strings.TrimSuffix(value, "!important"))
	if value == "auto" || value == "0" {
		return true
	}
	if !strings.HasSuffix(value, "px") && !strings.HasSuffix(value, "%") {
		return false
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(value, "px"), "%"), 64)
	return err == nil && n >= 0 && n < 1<<30
}

func uncertainImageDisplay(value string) bool {
	value = strings.TrimSpace(strings.TrimSuffix(value, "!important"))
	switch value {
	case "block", "inline", "inline-block", "flow-root", "none", "contents", "list-item":
		return false
	}
	return true // flex/grid/table, var(), inheritance and newer layout models.
}

func (s imageSizing) reason(n *html.Node) string {
	if s.unknown {
		return "stylesheet/selector semantics are uncertain; authored layout was preserved"
	}
	affected, coveredWidth, coveredHeight, unsafe := false, false, false, false
	apply := func(rule sizingDeclaration) {
		if rule.matcher != nil && !rule.matcher.Match(n) {
			return
		}
		v := rule.value
		switch rule.property {
		case "box-sizing":
			if strings.Contains(v, "border-box") {
				affected = true
			}
			return
		case "display":
			if uncertainImageDisplay(v) {
				affected = true
			}
			return
		case "width", "height":
			affected = true
			if rule.unconditional && simpleSize(v) {
				if rule.property == "width" {
					coveredWidth = true
				} else {
					coveredHeight = true
				}
			}
		default:
			affected = true
		}
		if strings.Contains(v, "revert") || strings.Contains(v, "inherit") || rule.property == "all" || rule.property == "aspect-ratio" || rule.property == "writing-mode" || rule.property == "image-orientation" || rule.property == "image-resolution" || strings.HasPrefix(rule.property, "animation") || strings.HasPrefix(rule.property, "transition") {
			unsafe = true
		}
	}
	for _, rule := range s.rules {
		apply(rule)
	}
	if style := attr(n, "style"); style != "" {
		var inline imageSizing
		inline.readCSS([]byte(style), true, false)
		if inline.unknown {
			return "inline CSS sizing is uncertain"
		}
		for _, rule := range inline.rules {
			apply(rule)
		}
	}
	if unsafe {
		return "deliberate CSS ratio/orientation/dynamic sizing was preserved"
	}
	covered := coveredWidth && coveredHeight
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if parent.Type != html.ElementNode {
			continue
		}
		if !covered && parent.Namespace == "" && (parent.Data == "table" || parent.Data == "td" || parent.Data == "th") {
			return "native table layout may size the image"
		}
		var rules []sizingDeclaration
		rules = append(rules, s.rules...)
		if style := attr(parent, "style"); style != "" {
			var inline imageSizing
			inline.readCSS([]byte(style), true, false)
			if inline.unknown {
				return "ancestor CSS layout is uncertain"
			}
			rules = append(rules, inline.rules...)
		}
		for _, rule := range rules {
			if rule.matcher != nil && !rule.matcher.Match(parent) {
				continue
			}
			if rule.property == "image-orientation" || rule.property == "image-resolution" || rule.property == "all" || rule.property == "writing-mode" {
				return "inherited CSS orientation/layout was preserved"
			}
			if !covered && rule.property == "display" && uncertainImageDisplay(rule.value) {
				return "flex/grid/table or inherited layout may size the image"
			}
		}
	}
	if affected && !covered {
		return "CSS controls image sizing without a proven override for both axes"
	}
	return ""
}

// Authored dimension attributes are not a measured CSS display box when any
// sizing rule applies, even when adding intrinsic hints is safe under that rule.
func (s imageSizing) attributeBox(n *html.Node) bool {
	if s.unknown || attr(n, "style") != "" {
		return false
	}
	for _, rule := range s.rules {
		if rule.matcher != nil && !rule.matcher.Match(n) {
			continue
		}
		if rule.property != "display" {
			return false
		}
	}
	return true
}

func dormantImage(n *html.Node) bool {
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if parent.Data == "template" && parent.Namespace == "" {
			return true
		}
	}
	return false
}
