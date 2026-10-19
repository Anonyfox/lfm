package compile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestComparisonsOrderNumbersByValue(t *testing.T) {
	t.Parallel()
	n := func(s string) json.Number { return json.Number(s) }
	for _, tc := range []struct {
		name string
		a, b any
		want int
	}{
		{"json numbers", n("10"), n("6"), 1},
		{"json and int literal", n("10"), 6, 1},
		{"decimals", n("9.90"), n("9.9"), 0},
		{"exact beyond float", n("9007199254740993"), n("9007199254740992"), 1},
		{"negative", n("-1"), 0, -1},
		{"strings stay bytes", "10", "6", -1},
		{"number text and string", n("10"), "10", 0},
	} {
		got, err := order(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Errorf("%s: order = %d, %v; want %d", tc.name, got, err, tc.want)
		}
	}
	if _, err := order(n("1"), true); err == nil {
		t.Error("a number and a bool cannot be ordered")
	}
	eq := comparisons["eq"].(func(any, ...any) (bool, error))
	if same, err := eq(n("2.50"), 1, n("2.5")); !same || err != nil {
		t.Error("eq matches any of several arguments by value", same, err)
	}
	if same, err := eq(true, true); !same || err != nil {
		t.Error("bools compare", same, err)
	}
}

func TestTemplatesCompareConfigNumbersByValue(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "<h1>{{.title}}</h1>", `<h1>{{.title}}</h1><p id="cmp">{{if ge .data.quiz.value 6}}big{{else}}small{{end}} {{if eq .data.quiz.value 42}}exact{{end}} {{if lt .data.quiz.value 100}}below{{end}}</p>`, 1))
	if _, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	if page := read(t, root, "output/index.html"); !strings.Contains(page, "big exact below") {
		t.Fatal("42 compared as text:", page[strings.Index(page, "cmp"):][:60])
	}
}

func TestSameDocumentFragmentsNeedNoRegistry(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	write(t, root, "index.html.tmpl", strings.Replace(fixtureHTML, "<h1>{{.title}}</h1>", `<h1>{{.title}}</h1><a href="#faq-one">Read the answer</a><form action="#explanation"><button>Show</button></form>`, 1))
	result, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, result.Validation.Diagnostics)
	}
	for _, d := range result.Validation.Diagnostics {
		if strings.HasPrefix(d.Rule, "links.") {
			t.Errorf("%s: %s", d.Rule, d.Message)
		}
	}
}
