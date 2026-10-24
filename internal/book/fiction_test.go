package book

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

func fictionConcept(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/fiction/concept.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func fictionPlan(t *testing.T, text string) (Plan, []string) {
	t.Helper()
	var messages []string
	p := checkConcept(text, nil, nil, func(rule string, line int, message, hint string) { messages = append(messages, rule+": "+message) }, "fiction")
	return p, messages
}

func TestFictionConcept(t *testing.T) {
	text := fictionConcept(t)
	p, msgs := fictionPlan(t, text)
	if len(msgs) != 1 || !strings.Contains(msgs[0], RuleOpen) {
		t.Fatal(msgs)
	}
	if p.Kind != "fiction" || strings.Join(p.Cast, ",") != "Nora,Jonas" || p.POV != "third person" || p.Tense != "past" || len(p.Matter) != 1 || !p.CoverOpen || p.Cover != "images/cover.png" {
		t.Fatalf("%+v", p)
	}
	for _, name := range []string{"Reader", "Premise", "Cast", "World", "Arc", "Hooks", "Voice", "Chapters", "Imagery", "Look"} {
		_, messages := fictionPlan(t, strings.Replace(text, "## "+name+"\n", "", 1))
		if !strings.Contains(strings.Join(messages, "\n"), name) {
			t.Fatalf("missing %s: %v", name, messages)
		}
	}
	_, msgs = fictionPlan(t, strings.Replace(text, "short stories.", "short stories. V999", 1))
	if !strings.Contains(strings.Join(msgs, "\n"), "V999 does not exist") {
		t.Fatal(msgs)
	}
}

func TestQuotedHooks(t *testing.T) {
	for _, kind := range []string{"fiction", "nonfiction"} {
		for _, title := range []string{"\"Exact\" trailing notes", "“Exact” trailing notes", "„Exact“ trailing notes"} {
			var msgs []string
			text := strings.Replace(fictionConcept(t), "“The Signal”", title, 1)
			p := checkConcept(text, nil, nil, func(_ string, _ int, message, _ string) { msgs = append(msgs, message) }, kind)
			if p.Title != "Exact" || !strings.Contains(strings.Join(msgs, "\n"), "Text after the quoted title is ignored: trailing notes") {
				t.Fatal(p.Title, msgs)
			}
		}
	}
}

func TestChapterJobsAndStatus(t *testing.T) {
	text := fictionConcept(t)
	_, msgs := fictionPlan(t, strings.Replace(text, "Nora hears the strange signal", "Listen", 1))
	if !strings.Contains(strings.Join(msgs, "\n"), "job \"Listen\" is too short to plan from (1 words, write at least 4)") {
		t.Fatal(msgs)
	}
	text = strings.ReplaceAll(text, "Jonas follows the signal home", "Nora hears the strange signal")
	text = strings.ReplaceAll(text, "Both neighbours identify the source", "Nora hears the strange signal")
	_, msgs = fictionPlan(t, text)
	n := 0
	for _, m := range msgs {
		if strings.Contains(m, "share the job") {
			n++
			if !strings.Contains(m, "Chapters 1, 2 and 3") {
				t.Fatal(m)
			}
		}
	}
	if n != 1 {
		t.Fatal(msgs)
	}
	_, msgs = fictionPlan(t, strings.Replace(text, "| planned |", "| someday |", 1))
	if !strings.Contains(strings.Join(msgs, "\n"), "invalid Status") {
		t.Fatal(msgs)
	}
	s := Summary{Chapters: []Chapter{{N: 1, Status: "planned"}, {N: 2, Status: "draft"}}}
	if s.Next().N != 2 {
		t.Fatal(s.Next())
	}
	s.Chapters[1].Exists = true
	if s.Next().N != 1 {
		t.Fatal(s.Next())
	}
}

func TestFictionCheckPlannedMatterAndListing(t *testing.T) {
	root := t.TempDir()
	text := strings.Replace(fictionConcept(t), "- Avoid:", "- Use: \"quiet signal\"\n- Avoid:", 1)
	writeFile(t, root, ConceptFile, []byte(text))
	writeFile(t, root, Dir+"/chapters/one.md", []byte("# Arrival\n\nNora listened. The radio fell silent."))
	writeFile(t, root, Dir+"/chapters/three.md", []byte("TODO miraculous solution"))
	writeFile(t, root, Dir+"/matter/dedication.md", []byte("# Dedication\n\nFor everyone."))
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Kind: "fiction", Sell: "shops"}
	r := Check(p, cfg, true, nil, "", nil, nil)
	if r.Summary.Planned != 1 || r.Summary.Complete || r.Summary.Next().N != 2 || r.Summary.Target != 900 || !r.Plan.Matter[0].Exists {
		t.Fatalf("%+v", r.Summary)
	}
	if _, ok := r.Sources["matter/dedication.md"]; !ok {
		t.Fatal("matter not read")
	}
	for _, d := range r.Diagnostics {
		if strings.Contains(d.Location.File, "three.md") || d.Rule == RuleEvidence || d.Rule == RuleImages || d.Rule == RuleValue || strings.Contains(d.Message, "listing never says") {
			t.Fatal(d)
		}
	}
	writeFile(t, root, ListingFile, []byte(strings.Replace(testListing, "## Description", "## Description\nA quiet signal.", 1)))
	p, err = project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r = Check(p, cfg, true, nil, "", nil, nil)
	for _, d := range r.Diagnostics {
		if strings.Contains(d.Message, "listing never says") {
			t.Fatal(d)
		}
	}
	writeFile(t, root, ListingFile, []byte(testListing))
	p, err = project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r = Check(p, cfg, true, nil, "", nil, nil)
	found := false
	for _, d := range r.Diagnostics {
		found = found || strings.Contains(d.Message, "The listing never says \"quiet signal\"")
	}
	if !found {
		t.Fatalf("missing listing Use warning: Use=%q diagnostics=%+v", r.Plan.Use, r.Diagnostics)
	}
	cfg.Kind = "poetry"
	p, err = project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r = Check(p, cfg, true, nil, "", nil, nil)
	found = false
	for _, d := range r.Diagnostics {
		found = found || d.Rule == RuleConfig && d.Path == "deliverables.book.kind" && d.Severity == diagnostic.Error
	}
	if !found {
		t.Fatal("missing kind error")
	}
}

func TestFictionOutline(t *testing.T) {
	plan, _ := fictionPlan(t, fictionConcept(t))
	ch := plan.Chapters[0]
	for _, src := range []string{"# Arrival\n\nNoras radio sounded. The radio fell silent.", "# Arrival\n\nNora's radio sounded. The radio fell silent."} {
		_, ds := checkChapter(ch, []byte(src), plan, Config{Language: "en"}, func(string) ([]byte, error) { return nil, nil })
		for _, d := range ds {
			if strings.Contains(d.Message, "outline names") || strings.Contains(d.Message, "planned hook") {
				t.Fatal(d)
			}
		}
	}
	_, ds := checkChapter(ch, []byte("# Arrival\n\nNoralee listened."), plan, Config{Language: "en"}, func(string) ([]byte, error) { return nil, nil })
	count := 0
	for _, d := range ds {
		if strings.Contains(d.Message, "outline names") || strings.Contains(d.Message, "planned hook") {
			count++
		}
	}
	if count != 2 {
		t.Fatal(ds)
	}
	ch.Status = "planned"
	_, ds = checkChapter(ch, []byte("TODO"), plan, Config{}, func(string) ([]byte, error) { return nil, os.ErrNotExist })
	if len(ds) != 0 {
		t.Fatal(ds)
	}
}

func TestMatterOpenAndAIStatement(t *testing.T) {
	text := strings.Replace(fictionConcept(t), "[dedication](matter/dedication.md)", "Open: dedication?", 1)
	p, msgs := fictionPlan(t, text)
	if len(p.Matter) != 0 || !strings.Contains(strings.Join(msgs, "\n"), "dedication?") {
		t.Fatal(p.Matter, msgs)
	}
	text = testListing[:strings.Index(testListing, "Tools:")] + "Tools: Codex CLI (gpt-6.1-sol)\nStatement: Codex CLI drafted the text.\n"
	var messages []string
	checkListing(text, Plan{}, nil, "en", func(_ string, _ int, msg, _ string) { messages = append(messages, msg) })
	if strings.Contains(strings.Join(messages, "\n"), "statement does not name") {
		t.Fatal(messages)
	}
	text = strings.Replace(text, "Statement: Codex CLI drafted the text.", "Statement: Open: owner decides", 1)
	printed := false
	checkListing(text, Plan{}, nil, "en", func(rule string, _ int, _, hint string) {
		if rule == RuleOpen && strings.Contains(hint, "printed") {
			printed = true
		}
	})
	if !printed {
		t.Fatal("missing printed statement warning")
	}
}

func TestFictionQualityScope(t *testing.T) {
	// A measured profile isolates the kind-dependent thresholds from text scoring.
	q := quality{
		words: 1000, sentences: 100, paragraphs: 20, oneLiners: 20, fragments: 90,
		paragraphMean: 10, paragraphCV: 1, firstValue: -1, nominal: 900,
		opening: "opens on a generalization", ending: "ends on a recap",
		hedges:      []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		stock:       []found{{text: "Sarah", line: 1}},
		hard:        []found{{text: "a known tell", line: 1}},
		readability: -100, readLow: 50, readHigh: 70, reading: readingStats{words: 1000},
		wstf: 20, meanSentence: 60,
	}
	for _, kind := range []string{"nonfiction", "fiction"} {
		var messages []string
		value := false
		qualityChecks(q, prose{}, Plan{Kind: kind}, "de", "", func(rule string, _ int, msg, _ string) {
			value = value || rule == RuleValue
			messages = append(messages, msg)
		})
		joined := strings.Join(messages, "\n")
		if !strings.Contains(joined, "known tell") || !strings.Contains(joined, "Sentences average") {
			t.Fatal(messages)
		}
		for _, marker := range []string{"hedged promises", "stock name", "chapter ends", "abstract nouns", "single short sentence", "Paragraphs average", "four words or fewer", "Readability", "Wiener Sachtextformel"} {
			if strings.Contains(joined, marker) != (kind == "nonfiction") {
				t.Fatalf("%s %s: %v", kind, marker, messages)
			}
		}
		if value != (kind == "nonfiction") {
			t.Fatal(kind, messages)
		}
	}
	q.paragraphMean = 500
	var messages []string
	qualityChecks(q, prose{}, Plan{Kind: "fiction"}, "en", "", func(_ string, _ int, msg, _ string) { messages = append(messages, msg) })
	if !strings.Contains(strings.Join(messages, "\n"), "Paragraphs average 500") {
		t.Fatal(messages)
	}
}

func TestMatterChecks(t *testing.T) {
	for _, src := range []string{"", "# Wrong\n\nmiraculous solution", "# Dedication\n\nFor everyone."} {
		root := t.TempDir()
		writeFile(t, root, ConceptFile, []byte(fictionConcept(t)))
		if src != "" {
			writeFile(t, root, Dir+"/matter/dedication.md", []byte(src))
		}
		p, err := project.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		r := Check(p, Config{Kind: "fiction", Sell: "shops"}, true, nil, "", nil, nil)
		var matter []diagnostic.Diagnostic
		for _, d := range r.Diagnostics {
			if strings.Contains(d.Location.File, "matter/") || strings.Contains(d.Message, "Matter file") {
				matter = append(matter, d)
			}
		}
		switch src {
		case "":
			if len(matter) != 1 || matter[0].Rule != RuleConcept {
				t.Fatal(matter)
			}
		case "# Wrong\n\nmiraculous solution":
			if len(matter) != 2 || matter[0].Rule != RuleChapter || matter[1].Rule != RuleCopy {
				t.Fatal(matter)
			}
		default:
			if len(matter) != 0 {
				t.Fatal(matter)
			}
		}
	}
}

func TestPlannedFilesDoNotRepeat(t *testing.T) {
	root := t.TempDir()
	text := fictionConcept(t)
	writeFile(t, root, ConceptFile, []byte(text))
	repeated := "This passage contains enough words to be detected when two chapters repeat exactly the same sentence."
	writeFile(t, root, Dir+"/chapters/one.md", []byte("# Arrival\n\nNora listened. "+repeated))
	writeFile(t, root, Dir+"/chapters/two.md", []byte("# Search\n\nJonas listened. "+repeated))
	writeFile(t, root, Dir+"/chapters/three.md", []byte("# Answer\n\n"+repeated))
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r := Check(p, Config{Kind: "fiction", Sell: "shops"}, true, nil, "", nil, nil)
	found := false
	for _, d := range r.Diagnostics {
		if d.Rule == RuleValue || strings.Contains(d.Location.File, "three.md") {
			t.Fatal(d)
		}
		if strings.Contains(d.Message, "word for word") {
			found = true
			if d.Rule != RuleCopy {
				t.Fatal(d)
			}
		}
	}
	if !found {
		t.Fatal("missing fiction repetition")
	}
}

func TestFictionInvalidStatusIsIncomplete(t *testing.T) {
	root := t.TempDir()
	text := strings.Replace(fictionConcept(t), "| planned |", "| someday |", 1)
	writeFile(t, root, ConceptFile, []byte(text))
	for _, row := range []struct{ file, title string }{{"one", "Arrival"}, {"two", "Search"}, {"three", "Answer"}} {
		writeFile(t, root, Dir+"/chapters/"+row.file+".md", []byte("# "+row.title+"\n\nSome text."))
	}
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r := Check(p, Config{Kind: "fiction", Sell: "shops"}, true, nil, "", nil, nil)
	if r.Summary.Complete || r.Buildable {
		t.Fatal("invalid status completed the book")
	}
}

func TestDuplicateJobsAfterMalformedRow(t *testing.T) {
	text := strings.Replace(fictionConcept(t), "| 1 | Arrival | Nora hears the strange signal | Hook: „The radio fell silent.“ | 300 | [one](chapters/one.md) | done |", "| broken |", 1)
	text = strings.Replace(text, "Both neighbours identify the source", "Jonas follows the signal home", 1)
	_, msgs := fictionPlan(t, text)
	if !strings.Contains(strings.Join(msgs, "\n"), "Chapters 2 and 3 share the job") {
		t.Fatal(msgs)
	}
}

func TestBookKindConfig(t *testing.T) {
	for _, kind := range []string{"nonfiction", "fiction"} {
		var cfg project.BookConfig
		if err := json.Unmarshal([]byte("{\"kind\":\""+kind+"\"}"), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Kind != kind {
			t.Fatal(cfg.Kind)
		}
	}
	p, err := project.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := Check(p, Config{Sell: "shops"}, true, nil, "", nil, nil)
	if r.Config.Kind != "nonfiction" {
		t.Fatal(r.Config.Kind)
	}
}
