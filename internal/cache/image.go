package cache

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func imagePath(contents []byte) string {
	return filepath.Join(Dir("img"), fmt.Sprintf("%x", sha256.Sum256(contents)))
}

// ImageReusable reports whether target is linked or needs a cross-device copy.
// The caller must still compare its bytes before reusing it.
func ImageReusable(target string, contents []byte) bool {
	a, err := os.Stat(target)
	if err != nil {
		return false
	}
	b, err := os.Stat(imagePath(contents))
	return err == nil && (os.SameFile(a, b) || differentDevices(a, b))
}

// LinkImage publishes immutable image bytes from the shared content cache.
// Only a cross-device link falls back to a copy; other errors remain visible.
func LinkImage(target string, contents []byte) error {
	dir := Dir("img")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := imagePath(contents)
	got, err := os.ReadFile(path)
	if err == nil && !bytes.Equal(got, contents) {
		// Never rewrite an inode that another published output may retain.
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	if err != nil || !bytes.Equal(got, contents) {
		f, err := os.CreateTemp(dir, ".image-")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		_, err = f.Write(contents)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if err := os.Chmod(f.Name(), 0o444); err != nil {
			return err
		}
		if err := os.Link(f.Name(), path); err != nil && !os.IsExist(err) {
			return err
		}
	}
	Touch(path)
	if err := os.Link(path, target); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return os.WriteFile(target, contents, 0o444)
		}
		return err
	}
	return nil
}
