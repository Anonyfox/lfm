package compile

import (
	"os"
	"testing"
)

// BenchmarkCompile compiles testdata/example, or the project in LFM_BENCH_DIR.
func BenchmarkCompile(b *testing.B) {
	dir := os.Getenv("LFM_BENCH_DIR")
	if dir == "" {
		dir = b.TempDir()
		if err := os.CopyFS(dir, os.DirFS("../../testdata/example")); err != nil {
			b.Fatal(err)
		}
	}
	for b.Loop() {
		if _, err := Compile(dir, Options{Publish: true}); err != nil {
			b.Fatal(err)
		}
	}
}
