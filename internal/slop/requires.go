package slop

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/andybalholm/cascadia"
)

// Require describes one page-wide occurrence constraint. Pointers distinguish
// omitted bounds from explicit zero.
type Require struct {
	Phrase   string `json:"phrase,omitempty"`
	Regex    string `json:"regex,omitempty"`
	Snippet  string `json:"snippet,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Before   string `json:"before,omitempty"`
	Exactly  *int   `json:"exactly,omitempty"`
	Min      *int   `json:"min,omitempty"`
	Max      *int   `json:"max,omitempty"`
	Selector string `json:"selector,omitempty"`
	Severity string `json:"severity"`
	Hint     string `json:"hint,omitempty"`
}

// Validate returns the offending JSON field and its problem.
func (r Require) Validate() (string, error) {
	n := 0
	for _, s := range []string{r.Phrase, r.Regex, r.Snippet} {
		if s != "" {
			n++
		}
	}
	if n != 1 || strings.TrimSpace(r.Phrase+r.Regex+r.Snippet) == "" {
		return "phrase", fmt.Errorf("set exactly one nonblank phrase, regex or snippet")
	}
	if r.Scope != "" && r.Scope != "copy" && r.Scope != "html" {
		return "scope", fmt.Errorf("scope must be copy or html")
	}
	if r.Snippet != "" && r.Scope != "html" {
		return "snippet", fmt.Errorf("snippet requires html scope")
	}
	if r.Before != "" && (r.Scope != "html" || r.Before != "</body>" && r.Before != "</head>") {
		return "before", fmt.Errorf("before requires html scope and must be </body> or </head>")
	}
	if r.Selector != "" {
		if r.Scope == "html" {
			return "selector", fmt.Errorf("selector requires copy scope")
		}
		if _, err := cascadia.ParseGroup(r.Selector); err != nil {
			return "selector", fmt.Errorf("invalid CSS selector: %w", err)
		}
	}
	for _, b := range []struct {
		k string
		v *int
	}{{"exactly", r.Exactly}, {"min", r.Min}, {"max", r.Max}} {
		if b.v != nil && *b.v < 0 {
			return b.k, fmt.Errorf("count must be nonnegative")
		}
	}
	if r.Exactly != nil && (r.Min != nil || r.Max != nil) {
		return "exactly", fmt.Errorf("exactly cannot be combined with min or max")
	}
	lo := 1
	if r.Min != nil {
		lo = *r.Min
	}
	if r.Max != nil && *r.Max < lo {
		return "max", fmt.Errorf("max must be at least min (default 1)")
	}
	if r.Severity != "error" && r.Severity != "warning" {
		return "severity", fmt.Errorf("severity must be error or warning")
	}
	if r.Regex != "" {
		if _, err := regexp.Compile(r.Regex); err != nil {
			return "regex", fmt.Errorf("invalid Go regular expression: %w", err)
		}
	}
	return "", nil
}

// Count uses the ban matcher for copy, and exact rendered bytes for HTML.
func (r Require) Count(blocks []string, raw string) int {
	if r.Scope == "html" {
		if r.Before != "" {
			// Fold ASCII tag names without changing byte offsets, including in
			// documents containing invalid UTF-8 or non-ASCII uppercase letters.
			folded := []byte(raw)
			for i, c := range folded {
				if c >= 'A' && c <= 'Z' {
					folded[i] = c + ('a' - 'A')
				}
			}
			if i := strings.Index(string(folded), r.Before); i >= 0 {
				raw = raw[:i]
			}
		}
		if r.Regex != "" {
			re, err := regexp.Compile(r.Regex)
			if err != nil {
				return 0
			}
			return len(re.FindAllStringIndex(raw, -1))
		}
		literal := r.Snippet
		if literal == "" {
			literal = r.Phrase
		}
		return strings.Count(raw, literal)
	}
	return len(MatchBans(blocks, []Ban{{Phrase: r.Phrase, Regex: r.Regex, Severity: "error"}}))
}

// Expectation returns the readable constraint and whether count satisfies it.
func (r Require) Expectation(count int) (string, bool) {
	if r.Exactly != nil {
		return fmt.Sprintf("exactly %d", *r.Exactly), count == *r.Exactly
	}
	lo := 1
	if r.Min != nil {
		lo = *r.Min
	}
	if r.Max != nil {
		return fmt.Sprintf("between %d and %d", lo, *r.Max), count >= lo && count <= *r.Max
	}
	return fmt.Sprintf("at least %d", lo), count >= lo
}
