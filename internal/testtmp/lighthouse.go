package testtmp

import (
	"os"
	"path/filepath"
)

// Lighthouse supplies a deterministic report to tests of the compiler and CLI.
// Chrome still runs normally. Runner failures and audit interpretation belong
// to release's focused tests, rather than depending on a global Node install or
// a machine's current load in every release-compile test.
func Lighthouse(dir string) error {
	const report = `{"lighthouseVersion":"fixture","categories":{"performance":{"score":1},"accessibility":{"score":1},"best-practices":{"score":1},"seo":{"score":1}},"audits":{"document-title":{"score":1}}}`
	tool := filepath.Join(dir, "lighthouse")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nprintf '%s\\n' '"+report+"'\n"), 0700); err != nil {
		return err
	}
	return os.Setenv("LFM_LIGHTHOUSE", tool)
}
