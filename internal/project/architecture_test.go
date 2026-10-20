package project

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestArchitectureNamesEveryPackage holds docs/architecture.md to the
// packages under internal/, so a new package cannot go undocumented.
func TestArchitectureNamesEveryPackage(t *testing.T) {
	doc, err := os.ReadFile("../../docs/architecture.md")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() && !strings.Contains(string(doc), "`"+e.Name()+"`") {
			t.Errorf("docs/architecture.md does not name internal/%s", e.Name())
		}
	}
}

// TestDependenciesPointOneWay keeps the layering docs/architecture.md
// describes: nothing imports cli, and only cli and serve drive compile.
func TestDependenciesPointOneWay(t *testing.T) {
	const module = "github.com/Anonyfox/lfm/internal/"
	files, err := filepath.Glob("../*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		pkg := filepath.Base(filepath.Dir(file))
		for _, imp := range f.Imports {
			switch path, _ := strconv.Unquote(imp.Path.Value); {
			case path == module+"cli":
				t.Errorf("%s imports cli", file)
			case path == module+"compile" && pkg != "cli" && pkg != "serve":
				t.Errorf("%s imports compile", file)
			}
		}
	}
}
