package aitext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func fixtures(t *testing.T, dir string) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", dir, "*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures in %s", dir)
	}
	out := map[string]string{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(f)] = string(raw)
	}
	return out
}

func langOfFixture(name string) string {
	for _, l := range []string{"de", "fr", "es", "it", "pt", "nl", "pl"} {
		if strings.HasPrefix(name, l+"-") {
			return l
		}
	}
	for _, n := range []string{"opus-06", "opus-07", "sonnet-08", "haiku-09"} {
		if strings.HasPrefix(name, n) {
			return "de"
		}
	}
	return "en"
}

// Human writing must never be flagged: a false alarm sends a writer to
// rewrite good prose.
func TestHumanFixturesPass(t *testing.T) {
	for name, text := range fixtures(t, "human") {
		if r := Analyze(Paragraphs(text), langOfFixture(name)); r.Likely {
			t.Errorf("%s flagged (score %.2f): %+v", name, r.Score, r.Signals)
		}
	}
}

// Generated text from current models. The estimate is tuned for few false
// alarms; on unseen sources it catches 45 to 76% of current-model 500-word
// windows depending on the language and most whole documents (TestTrain).
// These fixtures pin that level so a change that loses it fails.
func TestGeneratedFixturesCaught(t *testing.T) {
	flagged := 0
	for name, text := range fixtures(t, "generated") {
		r := Analyze(Paragraphs(text), langOfFixture(name))
		if r.Likely {
			flagged++
			if len(r.Signals) == 0 {
				t.Errorf("%s flagged without reasons", name)
			}
		}
		if r.Score <= 0 {
			t.Errorf("%s has no score", name)
		}
		t.Logf("%-14s score %.2f likely %v %v", name, r.Score, r.Likely, r.Signals)
	}
	if total := len(fixtures(t, "generated")); flagged*10 < total*7 {
		t.Errorf("only %d of %d generated fixtures flagged", flagged, total)
	}
}

func TestShortTextIsNotJudged(t *testing.T) {
	if r := Analyze([]string{"A short paragraph with a few words only."}, "en"); r.Likely || r.Score != 0 {
		t.Errorf("short text judged: %+v", r)
	}
}

func TestUnknownLanguageStillScores(t *testing.T) {
	text := fixtures(t, "generated")["opus-01.md"]
	if r := Analyze(Paragraphs(text), "fr"); r.Words < MinWords {
		t.Errorf("words %d", r.Words)
	}
}

func BenchmarkAnalyzeChapter(b *testing.B) {
	raw, _ := os.ReadFile("testdata/generated/opus-01.md")
	text := Paragraphs(strings.Repeat(string(raw)+"\n\n", 3))
	for b.Loop() {
		Analyze(text, "en")
	}
}

// Paragraphs splits plain text or Markdown into prose paragraphs: headings,
// list markers and emphasis are dropped, lines of one paragraph joined.
func Paragraphs(text string) []string {
	var out []string
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		var lines []string
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "```") {
				continue
			}
			line = strings.TrimLeft(line, "-*+> ")
			line = strings.NewReplacer("**", "", "__", "").Replace(line)
			lines = append(lines, line)
		}
		if p := strings.Join(lines, " "); len(strings.FieldsFunc(p, unicode.IsSpace)) > 0 {
			out = append(out, p)
		}
	}
	return out
}
