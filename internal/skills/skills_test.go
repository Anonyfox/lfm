package skills

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/research"
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

// The skill teaches the exact format the compiler checks; its examples must
// parse cleanly, so the two can never drift apart.
func TestResearchExamplesParse(t *testing.T) {
	s, err := Get("research", NewData("dev"))
	if err != nil {
		t.Fatal(err)
	}
	examples := regexp.MustCompile(`(?s)<example>\n(.*?)</example>`).FindAllStringSubmatch(s.Text, -1)
	if len(examples) < 3 {
		t.Fatalf("found %d examples", len(examples))
	}
	for _, ex := range examples {
		body := ex[1]
		switch {
		case strings.HasPrefix(body, "### V"):
			file := "# Human voices: X\n\n## Batch 2026-10-17 · from zero · queries: \"q\" · unreachable: none\n\n" + body
			if _, problems := research.ParseVoices(file); len(problems) != 0 {
				t.Errorf("voice example does not parse: %+v\n%s", problems, body)
			}
		case strings.HasPrefix(body, "### P"):
			if _, problems := research.ParseCompetitors("# Competitor analysis: X\n\n## Batch 2026-10-17 · from zero\n\n" + body); len(problems) != 0 {
				t.Errorf("peer example does not parse: %+v", problems)
			}
		case strings.HasPrefix(body, "## Batch"):
			if _, problems := research.ParseVoices("# Human voices: X\n\n" + body); len(problems) != 0 {
				t.Errorf("batch example does not parse: %+v", problems)
			}
		default:
			t.Errorf("unrecognized example: %q", body)
		}
	}
}
