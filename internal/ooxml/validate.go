package ooxml

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/scratch"
)

// Validate runs Microsoft's Open XML SDK over the files, as Office reads
// them, and returns its findings. dll is the built validator (see the
// validator folder); tests pass LFM_OOXML_VALIDATOR.
func Validate(dll string, files map[string][]byte) (string, error) {
	dir, err := scratch.Dir("ooxml")
	if err != nil {
		return "", err
	}
	defer scratch.Remove(dir)
	args := []string{dll}
	for name, data := range files {
		path := filepath.Join(dir, filepath.Base(name))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return "", err
		}
		args = append(args, path)
	}
	out, err := exec.Command("dotnet", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
