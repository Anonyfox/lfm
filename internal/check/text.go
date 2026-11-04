package check

import (
	"bytes"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/slop"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// CopyText analyzes text or Markdown with the same engine and report as HTML.
// YAML-style front matter recognizes lang/language; other keys are ignored.
func CopyText(raw []byte, markdown bool, options Options) Report {
	source := raw
	offset := 0
	if bytes.HasPrefix(source, []byte("---\n")) || bytes.HasPrefix(source, []byte("---\r\n")) {
		lines := bytes.SplitAfter(source, []byte("\n"))
		end := 0
		frontLanguage := ""
		for i, line := range lines {
			end += len(line)
			if i == 0 {
				continue
			}
			if strings.TrimSpace(string(line)) == "---" {
				offset = end
				break
			}
			key, value, ok := strings.Cut(strings.TrimSpace(string(line)), ":")
			if ok && options.Language == "" && (key == "lang" || key == "language") {
				frontLanguage = strings.Trim(strings.TrimSpace(value), `"'`)
			}
		}
		if offset > 0 {
			source = raw[offset:]
			if options.Language == "" {
				options.Language = frontLanguage
			}
		}
	}
	var blocks []string
	var positions []int
	if markdown {
		root := goldmark.DefaultParser().Parse(text.NewReader(source))
		_ = ast.Walk(root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
			if !enter {
				return ast.WalkContinue, nil
			}
			switch n.Kind() {
			case ast.KindFencedCodeBlock, ast.KindCodeBlock, ast.KindBlockquote, ast.KindHTMLBlock:
				return ast.WalkSkipChildren, nil
			}
			if n.Kind() != ast.KindParagraph && n.Kind() != ast.KindHeading {
				return ast.WalkContinue, nil
			}
			var b strings.Builder
			_ = ast.Walk(n, func(child ast.Node, enter bool) (ast.WalkStatus, error) {
				if enter {
					if child.Kind() == ast.KindCodeSpan {
						return ast.WalkSkipChildren, nil
					}
					if t, ok := child.(*ast.Text); ok {
						b.Write(t.Segment.Value(source))
						if t.SoftLineBreak() || t.HardLineBreak() {
							b.WriteByte(' ')
						}
					}
				}
				return ast.WalkContinue, nil
			})
			if n.Lines().Len() > 0 {
				blocks = append(blocks, b.String())
				positions = append(positions, offset+n.Lines().At(0).Start)
			}
			return ast.WalkSkipChildren, nil
		})
	} else {
		pos := offset
		for _, line := range bytes.SplitAfter(source, []byte("\n")) {
			if strings.TrimSpace(string(line)) != "" {
				blocks = append(blocks, string(line))
				positions = append(positions, pos)
			}
			pos += len(line)
		}
	}
	lang := slop.Language(options.Language, strings.Join(blocks, " "))
	report := Report{Stage: "copy", Diagnostics: []Diagnostic{}}
	hits := slop.Analyze(lang, blocks, options.CopyAllow)
	for _, r := range slop.Rules() {
		level := Severity(r.Severity)
		if v := options.Rules[r.ID]; v != "" {
			level = v
		}
		if level == Off {
			continue
		}
		for _, hit := range hits[r.ID] {
			message := hit.Text
			if r.ID != "copy.tells" {
				message = r.Reason + ": " + hit.Text
			}
			report.Diagnostics = append(report.Diagnostics, Diagnostic{Rule: r.ID, Severity: level, Location: location(options.File, lineStarts(raw), positions[hit.Block]), Message: message, Hint: r.Fix})
		}
	}
	summary := diagnostic.Summarize(report.Diagnostics)
	report.Diagnostics = summary.Diagnostics
	report.Errors = summary.Errors
	report.Warnings = summary.Warnings
	return report
}
