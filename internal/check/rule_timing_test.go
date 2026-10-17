package check

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestRuleTiming prints the cost of every rule on LFM_TIMING_DIR/index.html,
// a compiled output folder. It is a measuring tool, skipped by default.
func TestRuleTiming(t *testing.T) {
	dir := os.Getenv("LFM_TIMING_DIR")
	if dir == "" {
		t.Skip("set LFM_TIMING_DIR to a compiled output folder")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Stylesheets: map[string][]byte{}, Assets: map[string][]byte{}}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		options.Assets[e.Name()] = b
		if strings.HasSuffix(e.Name(), ".css") {
			options.Stylesheets["./"+e.Name()] = b
		}
	}
	const runs = 20
	cost := map[string]time.Duration{}
	var parse time.Duration
	for i := 0; i < runs; i++ {
		start := time.Now()
		d, err := Parse(raw, options)
		if err != nil {
			t.Fatal(err)
		}
		parse += time.Since(start)
		for _, r := range DefaultRules() {
			start := time.Now()
			r.Check(d)
			cost[r.ID] += time.Since(start)
		}
	}
	ids := make([]string, 0, len(cost))
	var total time.Duration
	for id, c := range cost {
		ids = append(ids, id)
		total += c
	}
	sort.Slice(ids, func(i, j int) bool { return cost[ids[i]] > cost[ids[j]] })
	t.Logf("parse %v, rules %v per run", parse/runs, total/runs)
	for _, id := range ids[:15] {
		t.Logf("%-28s %v", id, cost[id]/runs)
	}
}
