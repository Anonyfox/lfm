package compile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
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

// BenchmarkCompileLongBook compiles a book of about 60000 words, the
// length of a typical nonfiction title.
func BenchmarkCompileLongBook(b *testing.B) {
	dir := bookProject(b)
	prose, err := os.ReadFile("../aitext/testdata/generated/opus-01.md")
	if err != nil {
		b.Fatal(err)
	}
	for i, title := range []string{"Why starters die", "A week that feeds itself", "The first loaf"} {
		body := "# " + title + "\n\n" + strings.Repeat(strings.TrimSpace(string(prose[bytes.IndexByte(prose, '\n')+1:]))+"\n\n", 22)
		if err := os.WriteFile(filepath.Join(dir, "deliverables", "book", "chapters", "0"+string(rune('1'+i))+".md"), []byte(body), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Compile(context.Background(), dir, Options{Publish: true}); err != nil {
			b.Fatal(err)
		}
	}
}
