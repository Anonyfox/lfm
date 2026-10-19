package compile

import (
	"fmt"
	"sort"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/bundle"
	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
	"github.com/Anonyfox/lfm/internal/templatecheck"
	"github.com/Anonyfox/lfm/internal/urlcheck"
)

// Rules lists every rule any stage of the lifecycle can report, sorted by ID.
func Rules() []diagnostic.RuleInfo {
	rules := templatecheck.Catalog()
	rules = append(rules, commercecheck.Catalog()...)
	rules = append(rules, diagnostic.OperationalRules()...)
	rules = append(rules, urlcheck.Catalog()...)
	rules = append(rules, research.Rules()...)
	rules = append(rules, concept.Rules()...)
	rules = append(rules, book.Rules()...)
	rules = append(rules, printable.Rules()...)
	rules = append(rules, bundle.Rules()...)
	rules = append(rules, spreadsheet.Rules()...)
	rules = append(rules, document.Rules()...)
	rules = append(rules, deck.Rules()...)
	for _, rule := range check.DefaultRules() {
		rules = append(rules, rule.RuleInfo)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules
}

// ruleCheck validates one build.validation.rules entry: the rule must exist
// (or belong to the dynamic Lighthouse family), the level must be off, warn or
// error, and required rules stay errors.
func ruleCheck(id string, level diagnostic.Severity) error {
	var known *diagnostic.RuleInfo
	for _, rule := range Rules() {
		if rule.ID == id {
			known = &rule
			break
		}
	}
	switch {
	case known == nil && !diagnostic.DynamicRule(id):
		return fmt.Errorf("unknown rule %q; run 'lfm rules' for the list", id)
	case known != nil && known.Required && level != diagnostic.Error:
		return fmt.Errorf("%s is required and must stay error", id)
	case level != diagnostic.Off && level != diagnostic.Warning && level != diagnostic.Error:
		return fmt.Errorf("level for %s must be off, warn or error", id)
	}
	return nil
}
