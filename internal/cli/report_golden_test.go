package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/golden"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/testproject"
)

// goldenProjects are the projects whose compile report is held to
// testdata/golden: the example page and one per deliverable.
var goldenProjects = map[string]func(testing.TB) string{
	"page":        examplePage,
	"book":        testproject.Book,
	"printable":   testproject.Printable,
	"spreadsheet": testproject.Spreadsheet,
	"document":    testproject.Document,
	"deck":        testproject.Deck,
	"bundle":      testproject.DeckDocumentBundle,
}

func examplePage(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(testproject.Example())); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(dir, "output"))
	return dir
}

// TestReportGolden compiles every golden project and holds the terminal
// report and the --json output to what they printed before.
func TestReportGolden(t *testing.T) {
	t.Parallel()
	for name, project := range goldenProjects {
		for _, format := range []string{"txt", "json"} {
			t.Run(name+"."+format, func(t *testing.T) {
				t.Parallel()
				dir := project(t)
				if name == "book" {
					file := filepath.Join(dir, "lfm.json")
					data, err := os.ReadFile(file)
					if err != nil {
						t.Fatal(err)
					}
					var config map[string]any
					if err := json.Unmarshal(data, &config); err != nil {
						t.Fatal(err)
					}
					config["deliverables"].(map[string]any)["book"].(map[string]any)["formats"] = []string{"epub"}
					data, err = json.MarshalIndent(config, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				settle(t, dir)
				args := []string{"compile", dir}
				if format == "json" {
					args = append(args, "--json")
				}
				code, out, errs := run(t, args...)
				if code != 0 || errs != "" {
					t.Fatalf("exit %d, stderr %q\n%s", code, errs, out)
				}
				golden.Text(t, "report-"+name+"."+format, normalize(out, dir))
			})
		}
	}
}

// TestReleaseReport runs a release of the deck project in Chrome and checks
// the report's release lines by their shape: frames, timings and Lighthouse
// scores differ from machine to machine.
func TestReleaseReport(t *testing.T) {
	t.Parallel()
	if _, err := release.BrowserPath(); err != nil || testing.Short() {
		t.Skip("no browser")
	}
	dir := testproject.Deck(t)
	code, out, errs := run(t, "compile", dir, "--release")
	if code != 0 || errs != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errs, out)
	}
	out = normalize(out, dir)
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`(?m)^  release      (critical CSS \S+ \S+ of \S+ \S+ blocks rendering; the rest loads after|\S.*)$`),
		regexp.MustCompile(`(?m)^  browser      \S.*, \d+ runtime problems? in (<1 ms|\d+ ms|\d+\.\d s)$`),
		regexp.MustCompile(`(?m)^  frames       \d+ screenshots?`),
		regexp.MustCompile(`(?m)^  decks        Fire Safety Briefing · 1 deck, 11 slides`),
	} {
		if !want.MatchString(out) {
			t.Errorf("the release report lacks %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "print with --release") {
		t.Errorf("a release still says the files print with --release:\n%s", out)
	}
}

// settle dates every source of the project to one fixed time: a book's
// EPUB records when its sources last changed.
func settle(t *testing.T, dir string) {
	t.Helper()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chtimes(path, at, at)
	})
	if err != nil {
		t.Fatal(err)
	}
}

var (
	ansi        = regexp.MustCompile("\x1b\\[[0-9;]*m")
	projectName = regexp.MustCompile(`(?m)^([✓✗]) \S+ `)
	took        = regexp.MustCompile(`(compiled(?: → output/)?) in (<1 ms|\d+ ms|\d+\.\d s)`)
	durationMS  = regexp.MustCompile(`"durationMs": \d+`)
)

// normalize replaces what changes from run to run: the project's temp
// directory, its name, and how long the compile took.
func normalize(out, root string) string {
	out = ansi.ReplaceAllString(out, "")
	// The resolved directory first: on macOS /tmp is /private/tmp, and the
	// compile reports the resolved path.
	roots := []string{root}
	if real, err := filepath.EvalSymlinks(root); err == nil && real != root {
		roots = []string{real, root}
	}
	for _, r := range roots {
		out = strings.ReplaceAll(out, r, "<root>")
		out = strings.ReplaceAll(out, filepath.ToSlash(r), "<root>")
	}
	out = projectName.ReplaceAllString(out, "$1 <project> ")
	out = took.ReplaceAllString(out, "$1 in <duration>")
	return durationMS.ReplaceAllString(out, `"durationMs": 0`)
}
