package testtmp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// EPUBCheck validates an EPUB with the optional external validator.
func EPUBCheck(t *testing.T, data []byte) bool {
	t.Helper()
	tool := os.Getenv("LFM_EPUBCHECK")
	if tool == "" {
		tool, _ = exec.LookPath("epubcheck")
	}
	if tool == "" {
		return false
	}
	file := filepath.Join(t.TempDir(), "book.epub")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, tool, file).CombinedOutput()
	if err != nil {
		t.Fatalf("epubcheck: %v\n%s", err, output)
	}
	t.Logf("%s", output)
	return true
}
