package check

import (
	"os"
	"regexp"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// testRule wraps a single check for fixtures that do not need catalog help.
func testRule(id string, check func(*Document) []Diagnostic) Rule {
	return Rule{RuleInfo: diagnostic.RuleInfo{ID: id}, Check: check}
}

func TestDocumentedHTMLRulesMatchCatalog(t *testing.T) {
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]int{}
	for _, match := range regexp.MustCompile("(?m)^\\| `([^`]+)` \\|").FindAllSubmatch(raw, -1) {
		documented[string(match[1])]++
	}
	for _, rule := range DefaultRules() {
		if documented[rule.ID] != 1 {
			t.Errorf("%s needs exactly one default-rule table row, got %d", rule.ID, documented[rule.ID])
		}
		delete(documented, rule.ID)
	}
	for id := range documented {
		t.Errorf("documented HTML rule %s is not in DefaultRules", id)
	}
}
