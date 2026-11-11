package check

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/slop"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

func slopRules() []Rule {
	out := []Rule{{RuleInfo: diagnostic.RuleInfo{ID: "copy.banned", Summary: "Project-banned copy", Help: "Reports an error or warns when copy matches build.validation.copyBans. Project policy keeps domain clichés out of sales copy. Replace the matched wording using the entry's hint, or update copyBans in lfm.json."}, Check: slopRule("copy.banned")}}
	for _, r := range slop.Rules() {
		if r.ID == "copy.tells" {
			continue
		}
		level := "Warns"
		if r.Severity == "error" {
			level = "Reports an error"
		}
		out = append(out, Rule{RuleInfo: diagnostic.RuleInfo{ID: r.ID, Summary: r.Reason, Help: fmt.Sprintf("%s when the shared copy engine finds this pattern (%s language). %s. %s", level, r.Language, r.Reason, r.Fix)}, Check: slopRule(r.ID)})
	}
	return out
}

func slopRule(id string) func(*Document) []Diagnostic {
	return func(d *Document) []Diagnostic {
		if d.copyFindings == nil {
			d.copyFindings = analyzeSlop(d)
		}
		return d.copyFindings[id]
	}
}

func analyzeSlop(d *Document) map[string][]Diagnostic {
	segments := append([]copySegment(nil), copySegments(d)...)
	prose := len(segments)
	seen := map[string]bool{}
	for _, n := range d.Elements {
		if n.Data == "title" || n.Data == "meta" && (Attr(n, "name") == "description" || Attr(n, "property") == "og:description" || Attr(n, "property") == "og:title") {
			if HasAttr(n, "data-slop-skip") {
				continue
			}
			text := Attr(n, "content")
			if n.Data == "title" {
				text = nodeText(n)
			}
			if text != "" && !seen[text] {
				segments = append(segments, copySegment{n, text})
				seen[text] = true
			}
		}
	}
	texts := make([]string, len(segments))
	for i, s := range segments {
		texts[i] = s.text
	}
	lang := d.Options.Language
	if lang == "" {
		for _, n := range d.Elements {
			if n.Data == "html" {
				lang = Attr(n, "lang")
				break
			}
		}
	}
	lang = slop.Language(lang, strings.Join(texts, " "))
	hits := slop.Analyze(lang, texts, d.Options.CopyAllow)
	sentences := append([]string(nil), texts[:prose]...)
	for i, segment := range segments[:prose] {
		// Dated records can repeat an event summary for distinct releases.
		if hasCopyTime(segment.node) {
			sentences[i] = ""
		}
	}
	for _, id := range []string{"copy.duplicate-sentence", "copy.rhythm"} {
		hits[id] = slop.Match(id, lang, sentences, d.Options.CopyAllow)
	}
	out := map[string][]Diagnostic{}
	for _, r := range slop.Rules() {
		for _, hit := range hits[r.ID] {
			message := hit.Text
			if r.ID != "copy.tells" {
				message = fmt.Sprintf("%s: %q.", r.Reason, hit.Text)
			}
			out[r.ID] = append(out[r.ID], d.At(segments[hit.Block].node, Severity(r.Severity), message, r.Fix))
		}
	}
	for _, ban := range d.Options.CopyBans {
		blocks, targets := texts, segments
		if ban.Selector != "" {
			sel, err := cascadia.ParseGroup(ban.Selector)
			if err != nil {
				continue
			} // project.Load reports invalid selectors.
			for _, n := range d.Elements {
				if n.Data == "body" {
					targets = scopedSegments(n, false, false, false, sel)
					break
				}
			}
			blocks = make([]string, len(targets))
			for i, s := range targets {
				blocks[i] = s.text
			}
			ban.Selector = ""
		}
		for _, hit := range slop.MatchBans(blocks, []slop.Ban{ban}) {
			out["copy.banned"] = append(out["copy.banned"], d.At(targets[hit.Block].node, Severity(hit.Severity), fmt.Sprintf("Project-banned copy: %q.", hit.Text), hit.Hint))
		}
	}
	return out
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func hasCopyTime(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && !skipCopy(c) {
			if c.Data == "time" && Attr(c, "datetime") != "" || !copyBlocks[c.Data] && hasCopyTime(c) {
				return true
			}
		}
	}
	return false
}
