package check

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// testRule wraps a single check for fixtures that do not need catalog help.
func testRule(id string, check func(*Document) []Diagnostic) Rule {
	return Rule{RuleInfo: diagnostic.RuleInfo{ID: id}, Check: check}
}

// Every rule a user or agent can be told about must explain itself in
// `lfm rules <id>`: a one-line summary and a short help paragraph.
func TestCatalogRulesHaveStandardHelp(t *testing.T) {
	seen := map[string]bool{}
	sentence := regexp.MustCompile(`[.!?](\s|$)`)
	for _, rule := range Catalog() {
		if rule.ID == "" || seen[rule.ID] {
			t.Errorf("empty or duplicate rule ID %q", rule.ID)
		}
		seen[rule.ID] = true
		summary, help := rule.Summary, rule.Help
		switch {
		case strings.TrimSpace(summary) == "":
			t.Errorf("%s has no Summary", rule.ID)
		case utf8.RuneCountInString(summary) > 90:
			t.Errorf("%s Summary is %d characters; keep it to 90: %q", rule.ID, utf8.RuneCountInString(summary), summary)
		case strings.HasSuffix(summary, "."):
			t.Errorf("%s Summary ends with a period: %q", rule.ID, summary)
		case strings.TrimSpace(summary) != summary || strings.Contains(summary, "\n"):
			t.Errorf("%s Summary must be one trimmed line: %q", rule.ID, summary)
		}
		if strings.TrimSpace(help) == "" {
			t.Errorf("%s has no Help", rule.ID)
			continue
		}
		if n := len(sentence.FindAllString(help, -1)); n < 2 || n > 5 {
			t.Errorf("%s Help has %d sentences; use 2 to 5", rule.ID, n)
		}
		if !strings.HasSuffix(help, ".") || strings.Contains(help, "\n") {
			t.Errorf("%s Help must be one paragraph ending with a period", rule.ID)
		}
		for _, banned := range []string{"\u2014", "Note:"} {
			if strings.Contains(summary+" "+help, banned) {
				t.Errorf("%s help text contains %q", rule.ID, banned)
			}
		}
	}
}

// Keep future diagnostics and the CLI's catalog/config controls in sync. Dynamic
// Lighthouse IDs belong to the installed tool and have their own namespace.
func TestCatalogCoversEmittedRuleIDs(t *testing.T) {
	known := map[string]bool{}
	for _, entry := range Catalog() {
		if known[entry.ID] || entry.ID == "" || entry.Summary == "" {
			t.Fatal("invalid catalog entry", entry)
		}
		known[entry.ID] = true
	}
	literal := func(expr ast.Expr) string {
		if s, ok := expr.(*ast.BasicLit); ok && s.Kind == token.STRING {
			v, _ := strconv.Unquote(s.Value)
			return v
		}
		return ""
	}
	check := func(id, file string) {
		if id != "" && !known[id] && !diagnostic.DynamicRule(id) {
			t.Errorf("%s emits uncatalogued rule %q", file, id)
		}
	}
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		node, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(node, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.KeyValueExpr:
				if key, ok := node.Key.(*ast.Ident); ok && key.Name == "Rule" {
					check(literal(node.Value), path)
				}
			case *ast.CallExpr:
				name := ""
				switch f := node.Fun.(type) {
				case *ast.Ident:
					name = f.Name
				case *ast.SelectorExpr:
					name = f.Sel.Name
				}
				if (name == "notice" || name == "warn" || name == "issue" || name == "jsonldRule" || name == "fontRule" || name == "commerceRule" || name == "commerceAdd" || name == "add" && strings.Contains(filepath.ToSlash(path), "/commercecheck/")) && len(node.Args) > 0 {
					check(literal(node.Args[0]), path)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEveryCatalogRuleHasAnHonestConfigControl(t *testing.T) {
	for _, entry := range Catalog() {
		for _, level := range []Severity{Off, Warning, Error} {
			err := ValidateOptions(map[string]Severity{entry.ID: level})
			if entry.Required && level != Error {
				if err == nil {
					t.Fatal("execution failure can be disabled", entry.ID)
				}
			} else if err != nil {
				t.Fatal(entry.ID, err)
			}
		}
	}
	for _, id := range []string{"made.up", "lighthouse.", "lighthouse.Bad ID", "lighthouse.foo.bar"} {
		if ValidateOptions(map[string]Severity{id: Off}) == nil {
			t.Fatal("accepted unknown ID", id)
		}
	}
	if err := ValidateOptions(map[string]Severity{"lighthouse.new-audit-2027": Off}); err != nil {
		t.Fatal(err)
	}
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
