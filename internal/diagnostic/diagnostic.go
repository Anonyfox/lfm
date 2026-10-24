// Package diagnostic defines the shared compiler diagnostic format.
package diagnostic

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warn"
	Off     Severity = "off"
)

type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Offset int    `json:"offset"`
}

type Diagnostic struct {
	Rule      string     `json:"rule"`
	Severity  Severity   `json:"severity"`
	Location  Location   `json:"location"`
	Element   string     `json:"element,omitempty"`
	Path      string     `json:"path,omitempty"`
	Message   string     `json:"message"`
	Hint      string     `json:"hint"`
	Related   []Location `json:"related,omitempty"`
	Reference string     `json:"reference,omitempty"`
}

func (d Diagnostic) String() string {
	message := d.Message
	if d.Path != "" && !strings.Contains(message, d.Path) {
		message = d.Path + ": " + message
	}
	text := fmt.Sprintf("%s:%d:%d: %s [%s] %s\n  %s", d.Location.File, d.Location.Line, d.Location.Column, d.Severity, d.Rule, message, d.Hint)
	for _, at := range d.Related {
		text += fmt.Sprintf("\n  Related: %s:%d:%d", at.File, at.Line, at.Column)
	}
	return text
}

// RuleInfo is the single definition of a rule's identity and help. Summary is
// one line naming what is checked; Help explains what triggers it, why it
// matters and how to resolve it. Required rules can never leave error level.
type RuleInfo struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Help     string `json:"help"`
	Required bool   `json:"required,omitempty"`
	Project  bool   `json:"project,omitempty"` // Needs the lfm project's sources.
}

type Summary struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Errors      int          `json:"errors"`
	Warnings    int          `json:"warnings"`
}

func Summarize(issues []Diagnostic) Summary {
	result := Summary{Diagnostics: append([]Diagnostic{}, issues...)}
	sort.SliceStable(result.Diagnostics, func(i, j int) bool {
		a, b := result.Diagnostics[i], result.Diagnostics[j]
		if a.Severity != b.Severity {
			return a.Severity == Error
		}
		if a.Location.File != b.Location.File {
			return a.Location.File < b.Location.File
		}
		if a.Location.Offset != b.Location.Offset {
			return a.Location.Offset < b.Location.Offset
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Path < b.Path
	})
	result.Diagnostics = collapse(result.Diagnostics)
	for _, issue := range result.Diagnostics {
		if issue.Severity == Error {
			result.Errors++
		} else {
			result.Warnings++
		}
	}
	return result
}

var index = regexp.MustCompile(`\[[0-9]+\]`)

// collapse merges findings that one mistake repeats at one place, such as a
// binding inside a range reported for every item: same rule, severity,
// location and hint, and the same message apart from list indexes. Input is
// sorted, so repeats are adjacent.
func collapse(issues []Diagnostic) []Diagnostic {
	if len(issues) < 2 {
		return issues
	}
	key := func(d Diagnostic) string {
		return d.Rule + "\x00" + string(d.Severity) + "\x00" + d.Location.File + fmt.Sprintf("\x00%d:%d:%d\x00", d.Location.Offset, d.Location.Line, d.Location.Column) + d.Hint + "\x00" + index.ReplaceAllString(d.Message, "[]")
	}
	out := issues[:1]
	counts := []int{1}
	last := key(issues[0])
	for _, d := range issues[1:] {
		k := key(d)
		if k == last {
			counts[len(counts)-1]++
			continue
		}
		last = k
		out = append(out, d)
		counts = append(counts, 1)
	}
	for i, n := range counts {
		if n > 1 {
			out[i].Message += fmt.Sprintf(" (%d times here)", n)
		}
	}
	return out
}

func HardOnly(issues []Diagnostic) []Diagnostic {
	var hard []Diagnostic
	for _, issue := range issues {
		if issue.Severity == Error {
			hard = append(hard, issue)
		}
	}
	return hard
}

func At(file string, raw []byte, offset int) Location {
	offset = max(0, min(offset, len(raw)))
	line, column := 1, 1
	for i := 0; i < offset; i++ {
		if raw[i] == '\n' || raw[i] == '\r' && (i+1 == len(raw) || raw[i+1] != '\n') {
			line++
			column = 1
		} else {
			column++
		}
	}
	return Location{file, line, column, offset}
}

// Source indexes line starts once for repeated source-position lookups.
// Columns/offsets use UTF-8 bytes, matching Go's template and JSON parsers.
type Source struct {
	file  string
	size  int
	lines []int
}

func NewSource(file string, raw []byte) Source {
	s := Source{file: file, size: len(raw), lines: []int{0}}
	for i, b := range raw {
		if b == '\n' || b == '\r' && (i+1 == len(raw) || raw[i+1] != '\n') {
			s.lines = append(s.lines, i+1)
		}
	}
	return s
}
func (s Source) At(offset int) Location {
	offset = max(0, min(offset, s.size))
	line := sort.Search(len(s.lines), func(i int) bool { return s.lines[i] > offset })
	return Location{File: s.file, Line: line, Column: offset - s.lines[line-1] + 1, Offset: offset}
}
