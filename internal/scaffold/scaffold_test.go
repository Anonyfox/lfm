package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/compile"
)

func TestCreatedProjectCompilesWithoutFindings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tea-shop")
	created, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lfm.json", "index.html.tmpl", "AGENTS.md", ".gitignore", "research/human-voices.md"} {
		if !contains(created, want) {
			t.Fatalf("%s not created: %v", want, created)
		}
	}
	config, _ := os.ReadFile(filepath.Join(dir, "lfm.json"))
	if !strings.Contains(string(config), `"title": "Tea shop"`) || !strings.Contains(string(config), "https://example.com/tea-shop/") {
		t.Fatalf("placeholders not filled:\n%s", config)
	}
	result, err := compile.Compile(context.Background(), dir, compile.Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Errors()+result.Warnings() != 0 {
		t.Fatalf("starter is not clean: %+v", result.Validation.Diagnostics)
	}
	if _, err := os.Stat(filepath.Join(dir, "output", "index.html")); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lfm.json"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("unrelated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dir); err == nil || !strings.Contains(err.Error(), "lfm.json") {
		t.Fatalf("got %v", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "lfm.json")); string(raw) != "mine" {
		t.Fatal("existing file overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html.tmpl")); err == nil {
		t.Fatal("partial project written despite a conflict")
	}
}

func TestCreateAcceptsUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dir); err != nil {
		t.Fatal(err)
	}
}

func TestTitleAndSlug(t *testing.T) {
	for in, want := range map[string][2]string{
		"my-tea_shop": {"My tea shop", "my-tea-shop"},
		"Café Ölberg": {"Café Ölberg", "cafe-olberg"},
		"---":         {"New page", "page"},
		"v2.launch":   {"V2 launch", "v2-launch"},
	} {
		if got := Title(in); got != want[0] {
			t.Errorf("Title(%q) = %q, want %q", in, got, want[0])
		}
		if got := Slug(in); got != want[1] {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want[1])
		}
	}
}

func FuzzSlugIsURLSafe(f *testing.F) {
	f.Add("My Page!")
	f.Add("ünïcode -- name")
	valid := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	f.Fuzz(func(t *testing.T, name string) {
		if got := Slug(name); !valid.MatchString(got) {
			t.Fatalf("Slug(%q) = %q", name, got)
		}
	})
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
