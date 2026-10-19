package compile

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/bundle"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/scaffold"
)

// Every place that names the deliverables must name all of them: a unit
// missing from one list is built and then dropped, such as a distribution/
// folder written and deleted again.
func TestEveryListNamesEveryDeliverable(t *testing.T) {
	t.Parallel()
	var names []string
	for _, k := range kinds {
		names = append(names, k.name)
		if !units[k.name] {
			t.Errorf("%s has no distribution/ folder", k.name)
		}
		for _, field := range []string{"deliverables." + k.name, "deliverables." + k.name + ".sell"} {
			if _, ok := project.Lookup(field); !ok {
				t.Errorf("lfm.json has no %s", field)
			}
		}
	}
	if !slices.Equal(names, bundle.Kinds) {
		t.Errorf("compile builds %v, a bundle holds %v", names, bundle.Kinds)
	}
	if len(units) != len(kinds)+1 || !units["bundle"] {
		t.Errorf("distribution/ units %v", units)
	}
	if f, _ := project.Lookup("deliverables.bundle.units"); !slices.Equal(f.Values, bundle.Kinds) {
		t.Errorf("deliverables.bundle.units takes %v, a bundle holds %v", f.Values, bundle.Kinds)
	}
}

// A deliverable sold only in the bundle is held to it, whichever it is.
func TestEveryDeliverableCanSellInTheBundle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	configure(t, dir, func(c map[string]any) {
		d := map[string]any{}
		for _, kind := range bundle.Kinds {
			d[kind] = map[string]any{"sell": "bundle"}
			if err := os.MkdirAll(filepath.Join(dir, "deliverables", kind), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		c["deliverables"] = d
	})
	p, err := project.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, found := checkBundle(p, project.Load(p, ruleCheck), commercecheck.Facts{})
	var messages []string
	for _, d := range found {
		messages = append(messages, d.Message)
	}
	for _, kind := range bundle.Kinds {
		if want := "deliverables." + kind + ".sell is bundle, but lfm.json has no deliverables.bundle."; !slices.Contains(messages, want) {
			t.Errorf("no finding %q in %s", want, strings.Join(messages, "; "))
		}
	}
}
