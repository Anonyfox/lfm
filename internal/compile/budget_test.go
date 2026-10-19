//go:build !race

package compile

import (
	"context"
	"os"
	"runtime"
	"testing"
)

// TestCompileKeepsItsBudget holds an ordinary compile of the example to
// its allocations, which time on a busy machine cannot measure: half again
// what it takes today. A change that needs more should say why here.
func TestCompileKeepsItsBudget(t *testing.T) {
	// Not parallel: memory statistics are the whole process's.
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../testdata/example")); err != nil {
		t.Fatal(err)
	}
	compile := func() {
		if _, err := Compile(context.Background(), dir, Options{Publish: true}); err != nil {
			t.Fatal(err)
		}
	}
	compile() // the first compile publishes; later ones find output/ current
	const runs = 3
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range runs {
		compile()
	}
	runtime.ReadMemStats(&after)
	allocs, bytes := (after.Mallocs-before.Mallocs)/runs, (after.TotalAlloc-before.TotalAlloc)/runs
	t.Logf("%d allocations, %.1f MB a compile", allocs, float64(bytes)/1e6)
	if allocs > 50_000 || bytes > 12e6 {
		t.Errorf("a compile takes %d allocations and %.1f MB; the budget is 50000 and 12 MB", allocs, float64(bytes)/1e6)
	}
}
