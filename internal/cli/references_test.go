package cli

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

	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/skills"
)

// Skills, docs, help texts and hints send readers to 'lfm rules <id>' and
// 'lfm help config <field>'. Every rule and field they name must exist, so a
// renamed rule or moved field cannot leave a dead pointer behind.
func TestReferencedRulesAndFieldsExist(t *testing.T) {
	known := map[string]bool{}
	families := map[string]bool{}
	for _, r := range compile.Rules() {
		known[r.ID] = true
		family, _, _ := strings.Cut(r.ID, ".")
		families[family] = true
	}
	for _, f := range project.Spec {
		if f.Checked != "" && !known[f.Checked] {
			t.Errorf("lfm.json field %s is checked by %q, which 'lfm rules' does not have", f.Path, f.Checked)
		}
	}

	texts := map[string]string{} // where -> text
	all, err := skills.All(skills.NewData("dev"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		texts["lfm skills "+s.Topic] = s.Text
	}
	for _, pattern := range []string{"../../SKILL.md", "../../docs/*.md", "../scaffold/files/*.md"} {
		found, _ := filepath.Glob(pattern)
		for _, path := range found {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			texts[path] = string(raw)
		}
	}
	for _, c := range commands() {
		texts["lfm help "+c.name] = c.details
	}
	// Every string literal of the program: hints, messages, help texts.
	err = filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		var b strings.Builder
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					b.WriteString(s + "\n")
				}
			}
			return true
		})
		texts[path] = b.String()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// 'lfm rules <id>' with an ID, or a family in quotes.
	commandRule := regexp.MustCompile(`lfm rules ([a-z][\w-]*\.[\w.*-]*\w|[a-z]+')`)
	helpField := regexp.MustCompile(`lfm help config ([a-zA-Z][\w.*-]*)`)
	// A rule ID standing alone: family.name, not part of a longer dotted
	// path or a file name.
	mention := regexp.MustCompile(`(?:^|[^\w./-])([a-z]+\.[a-z][a-z-]*)(?:[^\w./-]|\.(?:\s|$)|$)`)
	field := regexp.MustCompile(`(?:^|[^\w./-])((?:deliverables|seller|build|socialImage)\.[a-zA-Z][\w.]*\w)`)
	extensions := map[string]bool{"md": true, "json": true, "html": true, "tmpl": true, "css": true, "js": true, "png": true, "jpg": true, "pdf": true, "zip": true, "xlsx": true, "docx": true, "pptx": true, "epub": true, "txt": true, "ttf": true, "svg": true, "webp": true, "go": true, "sh": true, "com": true, "cash": true, "test": true}
	for where, text := range texts {
		for _, m := range commandRule.FindAllStringSubmatch(text, -1) {
			id := strings.TrimSuffix(m[1], "'")
			if !known[id] && !families[id] && !diagnostic.DynamicRule(id) && id != "lighthouse.*" {
				t.Errorf("%s: 'lfm rules %s' names no rule or family", where, id)
			}
		}
		for _, m := range helpField.FindAllStringSubmatch(text, -1) {
			if _, err := project.Explain(strings.TrimRight(m[1], ".")); err != nil {
				t.Errorf("%s: 'lfm help config %s': %v", where, m[1], err)
			}
		}
		if strings.HasSuffix(where, ".go") {
			continue // code literals hold file names and dotted paths of every kind
		}
		for _, m := range mention.FindAllStringSubmatch(text, -1) {
			family, name, _ := strings.Cut(m[1], ".")
			if families[family] && !extensions[name] && !known[m[1]] && !diagnostic.DynamicRule(m[1]) && !configField(m[1]) {
				t.Errorf("%s names rule %s, which 'lfm rules' does not have", where, m[1])
			}
		}
		for _, m := range field.FindAllStringSubmatch(text, -1) {
			if _, moved := project.Moved[m[1]]; !configField(m[1]) && !known[m[1]] && !moved {
				t.Errorf("%s names lfm.json field %s, which 'lfm help config' does not have", where, m[1])
			}
		}
	}
}

// configField reports whether path is a field of lfm.json or lies inside a
// map or free-form field.
func configField(path string) bool {
	parts := strings.Split(path, ".")
	for i := len(parts); i > 0; i-- {
		f, ok := project.Lookup(strings.Join(parts[:i], "."))
		if ok && (i == len(parts) || f.Kind == project.Map || f.Kind == project.Free) {
			return true
		}
	}
	return false
}
