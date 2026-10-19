package compile

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// TestEveryReportedRuleIsInTheCatalog finds every rule ID the program can
// report (Rule constants, Rule: fields of diagnostics, the rule argument of
// notices) and holds it to 'lfm rules', which build.validation.rules is
// checked against: a rule missing there could be reported but never set.
func TestEveryReportedRuleIsInTheCatalog(t *testing.T) {
	known := map[string]bool{}
	for _, r := range Rules() {
		known[r.ID] = true
	}
	id := regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z0-9-]+)+$`)
	report := func(fset *token.FileSet, at token.Pos, lit ast.Expr) {
		b, ok := lit.(*ast.BasicLit)
		if !ok || b.Kind != token.STRING {
			return
		}
		s, err := strconv.Unquote(b.Value)
		if err != nil || !id.MatchString(s) || known[s] || diagnostic.DynamicRule(s) {
			return
		}
		t.Errorf("%s: rule %q is reported but not in 'lfm rules'", fset.Position(at), s)
	}
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ValueSpec:
				for i, name := range n.Names {
					if strings.HasPrefix(name.Name, "Rule") && i < len(n.Values) {
						report(fset, name.Pos(), n.Values[i])
					}
				}
			case *ast.KeyValueExpr:
				if k, ok := n.Key.(*ast.Ident); ok && k.Name == "Rule" {
					report(fset, n.Pos(), n.Value)
				}
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "notice" && len(n.Args) > 0 {
					report(fset, n.Pos(), n.Args[0])
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
