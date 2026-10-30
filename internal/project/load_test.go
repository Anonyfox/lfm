package project

import (
	"errors"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

const validConfig = `{"schemaVersion":1,"url":"https://example.com/page/","language":"en","title":"T","description":"D","data":{"price":29}}`

func TestLoadValidConfigAndData(t *testing.T) {
	p := newProject(t)
	write(t, p.Root, ConfigFile, validConfig)
	write(t, p.Root, "data/faq.json", `[{"q":"Why?","a":"Because."}]`)
	l := Load(p, nil)
	if len(l.Problems) != 0 {
		t.Fatalf("%+v", l.Problems)
	}
	if l.Config.Build.Assets.Default != Auto || l.Values["title"] != "T" || l.Values["data"].(map[string]any)["faq"] == nil {
		t.Fatalf("values: %+v", l.Values)
	}
	if string(l.Sources["data/faq.json"]) == "" {
		t.Fatal("data source not kept for locations")
	}
}

func TestLoadCollectsEveryProblem(t *testing.T) {
	p := newProject(t)
	write(t, p.Root, ConfigFile, `{"schemaVersion":2,"url":"http://example.com/page?x#y","language":"","title":"T","description":" ",
		"Title":"case","custom":1,"build":{"assets":{"default":"bundle","overrides":{"../x.png":"file","a.png":"lazy"}},"validation":{"rules":{"nope":"warn"}}}}`)
	write(t, p.Root, "data/notes.txt", "x")
	write(t, p.Root, "data/a.b.json", "{}")
	l := Load(p, func(id string, level diagnostic.Severity) error {
		if id == "nope" {
			return errors.New("unknown rule")
		}
		return nil
	})
	var paths []string
	for _, problem := range l.Problems {
		paths = append(paths, problem.Path)
	}
	got := strings.Join(paths, " ")
	for _, want := range []string{"schemaVersion", "url", "language", "description", "Title", "custom",
		"build.assets.default", `build.assets.overrides["../x.png"]`, `build.assets.overrides["a.png"]`, "build.validation.rules.nope", "data", `data["a.b"]`} {
		if !strings.Contains(" "+got+" ", " "+want+" ") {
			t.Errorf("missing problem at %s; got %s", want, got)
		}
	}
	for _, problem := range l.Problems {
		if problem.Path == "Title" && !strings.Contains(problem.Hint, "Use the exact key title") {
			t.Errorf("case hint missing: %v", problem.Hint)
		}
	}
	if l.Values != nil {
		t.Fatal("values must be withheld while problems exist")
	}
}

func TestLoadRejectsDataCollisionsAndBrokenJSON(t *testing.T) {
	p := newProject(t)
	write(t, p.Root, ConfigFile, validConfig)
	write(t, p.Root, "data/price.json", "30")
	write(t, p.Root, "data/broken.json", "{")
	l := Load(p, nil)
	var messages []string
	for _, problem := range l.Problems {
		messages = append(messages, problem.File+": "+problem.Err.Error())
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "data/price.json: data.price is defined in both lfm.json and data/price.json") || !strings.Contains(joined, "data/broken.json") {
		t.Fatal(joined)
	}
}

func TestLoadMissingConfig(t *testing.T) {
	l := Load(newProject(t), nil)
	if len(l.Problems) != 1 || l.Problems[0].File != ConfigFile {
		t.Fatalf("%+v", l.Problems)
	}
}

func TestBrowserData(t *testing.T) {
	values := map[string]any{"data": map[string]any{"offer": map[string]any{"price": 29, "name": "</script>"}, "faq": []any{1}}}
	raw, err := BrowserData(values, []string{"data.offer"})
	if err != nil || string(raw) != `{"data":{"offer":{"name":"\u003c/script\u003e","price":29}}}` {
		t.Fatalf("%s %v", raw, err)
	}
	for _, path := range []string{"data.missing", "data.faq.0", "data..offer", ""} {
		if _, err := BrowserData(values, []string{path}); err == nil {
			t.Errorf("BrowserData(%q) accepted", path)
		}
	}
}

func TestAssetsMode(t *testing.T) {
	a := Assets{Default: Auto, Overrides: map[string]string{"hero.webp": File}}
	if a.Mode("hero.webp") != File || a.Mode("other.png") != Auto {
		t.Fatal("mode")
	}
}

func TestLoadBudgets(t *testing.T) {
	p := newProject(t)
	write(t, p.Root, ConfigFile, strings.Replace(validConfig, `"data"`, `"build":{"validation":{"budgets":{"cssKiB":80,"elements":3000,"imageKiB":4000,"videoKiB":9000,"firstScreenKiB":500}}},"data"`, 1))
	l := Load(p, nil)
	if b := l.Config.Build.Validation.Budgets; len(l.Problems) != 0 || b.CSSKiB != 80 || b.Elements != 3000 || b.JSKiB != 0 || b.ImageKiB != 4000 || b.VideoKiB != 9000 || b.FirstScreenKiB != 500 {
		t.Fatalf("%+v %+v", l.Problems, b)
	}
	problems := func(budgets string) string {
		write(t, p.Root, ConfigFile, strings.Replace(validConfig, `"data"`, `"build":{"validation":{"budgets":`+budgets+`}},"data"`, 1))
		var paths []string
		for _, problem := range Load(p, nil).Problems {
			paths = append(paths, problem.Path)
		}
		return " " + strings.Join(paths, " ") + " "
	}
	// The shape first, then the ranges a page may raise its budgets to.
	for budgets, want := range map[string][]string{
		`{"imageKiB":-1,"videoKiB":1048577,"firstScreenKiB":-1}`: {"build.validation.budgets.imageKiB", "build.validation.budgets.videoKiB", "build.validation.budgets.firstScreenKiB"},
		`{"firstScreenKiB":"big"}`:                               {"build.validation.budgets.firstScreenKiB"},
		`{"jsKiB":"big","fonts":3}`:                              {"build.validation.budgets.jsKiB", "build.validation.budgets.fonts"},
		`{"cssKiB":-1,"depth":99999}`:                            {"build.validation.budgets.cssKiB", "build.validation.budgets.depth"},
		`{"cssKiB":10240,"elements":1e6}`:                        {"build.validation.budgets.elements"},
	} {
		got := problems(budgets)
		for _, path := range want {
			if !strings.Contains(got, " "+path+" ") {
				t.Errorf("%s: missing problem at %s; got%s", budgets, path, got)
			}
		}
	}
}

func TestCopyAllowConfig(t *testing.T) {
	for _, tc := range []struct{ allow, path string }{
		{`[{"rule":"copy.superlative","phrase":"Traumhaft Hotel","reason":"Registered name"}]`, ""},
		{`[{"rule":"copy.superlative"}]`, "build.validation.copyAllow[0].reason"},
		{`[{"rule":"copy.superlative","reason":"  "}]`, "build.validation.copyAllow[0].reason"},
		{`[{"rule":"typo","reason":"Registered name"}]`, "build.validation.copyAllow[0].rule"},
		{`[{"rule":"copy.superlative","reason":"Registered name","typo":true}]`, "build.validation.copyAllow[0].typo"},
		{`["copy.superlative"]`, "build.validation.copyAllow[0]"},
	} {
		p := newProject(t)
		config := strings.TrimSuffix(validConfig, "}") + `,"build":{"validation":{"copyAllow":` + tc.allow + `}}}`
		write(t, p.Root, ConfigFile, config)
		loaded := Load(p, nil)
		if tc.path == "" {
			if len(loaded.Problems) > 0 {
				t.Fatal(loaded.Problems)
			}
			continue
		}
		found := false
		for _, problem := range loaded.Problems {
			if problem.Path == tc.path {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected %s, got %+v", tc.allow, tc.path, loaded.Problems)
		}
	}
}
