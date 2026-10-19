package compile

import (
	"context"
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
		if _, err := Compile(context.Background(), dir, Options{Publish: true}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRecompileReleasedDeck compiles a deck whose release is published:
// every print is reused from output/ and distribution/.
func BenchmarkRecompileReleasedDeck(b *testing.B) {
	dir := deckProject(b)
	if _, err := Compile(context.Background(), dir, Options{Publish: true, Release: true}); err != nil {
		b.Skip("no release:", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Compile(context.Background(), dir, Options{Publish: true}); err != nil {
			b.Fatal(err)
		}
	}
}
