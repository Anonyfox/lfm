package skills

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// Every skill must load in any Agent Skills client and stay small enough to be
// read in full: https://agentskills.io/specification.
func TestSkillsFollowTheSpec(t *testing.T) {
	all, err := All(NewData("1.2.3"))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("no skills embedded")
	}
	name := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	for _, s := range all {
		t.Run(s.Topic, func(t *testing.T) {
			if !name.MatchString(s.Name) || len(s.Name) > 64 || s.Name != "lfm-"+s.Topic {
				t.Errorf("name %q must be lfm-<topic>, lowercase and hyphenated", s.Name)
			}
			if n := utf8.RuneCountInString(s.Description); n > 1024 || n < 200 {
				t.Errorf("description has %d characters; keep it between 200 and 1024", n)
			}
			if !strings.Contains(s.Description, "Use ") || !strings.Contains(s.Description, "Not for") {
				t.Error("description must say when to use the skill and when not")
			}
			header, body, _ := strings.Cut(strings.TrimPrefix(s.Text, "---\n"), "\n---\n")
			for _, line := range strings.Split(header, "\n") {
				key, value, _ := strings.Cut(line, ": ")
				if strings.Contains(value, ": ") || strings.ContainsAny(value, "<>") {
					t.Errorf("frontmatter %s holds YAML-unsafe text: %q", key, value)
				}
			}
			lines := strings.Count(body, "\n")
			if lines > 250 {
				t.Errorf("body has %d lines; keep skills under 250 so they are read in full", lines)
			}
			if tokens := len(body) / 4; tokens > 5000 {
				t.Errorf("body is about %d tokens; the spec budget is 5000", tokens)
			}
			for _, banned := range []string{"[[", "]]", "—", "TODO", "MUST", "NEVER", "mcp__", "WebSearch", "WebFetch"} {
				if strings.Contains(s.Text, banned) {
					t.Errorf("skill contains %q", banned)
				}
			}
			if !strings.Contains(s.Text, "lfm 1.2.3") {
				t.Error("skill does not name the version that printed it")
			}
		})
	}
}

func TestGet(t *testing.T) {
	s, err := Get("research", NewData("dev"))
	if err != nil || s.Name != "lfm-research" || !strings.Contains(s.Text, "research/human-voices.md") {
		t.Fatalf("%+v %v", s.Name, err)
	}
	for _, topic := range []string{"nope", "../research", "research/SKILL.md", ""} {
		if _, err := Get(topic, NewData("dev")); err == nil {
			t.Errorf("Get(%q) succeeded", topic)
		}
	}
}

func TestResearchSkillCarriesItsNonNegotiablesEarly(t *testing.T) {
	s, err := Get("research", NewData("dev"))
	if err != nil {
		t.Fatal(err)
	}
	// Harnesses may keep only the start of a skill after compaction.
	head := strings.Join(strings.SplitN(s.Text, "\n", 45)[:44], "\n")
	for _, rule := range []string{"Verbatim or nothing", "Never write from memory", "Read before write"} {
		if !strings.Contains(head, rule) {
			t.Errorf("%q must appear in the first 44 lines", rule)
		}
	}
	for _, fact := range []string{"60 voices", "5 platforms", "12 peers", "research/competitor-analysis.md"} {
		if !strings.Contains(s.Text, fact) {
			t.Errorf("rendered skill lacks %q", fact)
		}
	}
}
