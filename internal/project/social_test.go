package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXHandleValidation(t *testing.T) {
	t.Parallel()
	for _, handle := range []string{"a", "A0_", strings.Repeat("a", 15), "", "@", "@@a", "bad-handle", "bad handle", "é", strings.Repeat("a", 16)} {
		t.Run(handle, func(t *testing.T) {
			valid := handle == "a" || handle == "A0_" || handle == strings.Repeat("a", 15)
			if got := validXHandle(handle); got != valid {
				t.Fatalf("validXHandle(%q) = %v, want %v", handle, got, valid)
			}
		})
	}
}

// Property: a successfully loaded nonempty X handle is normalized, bounded and
// made only of ASCII handle characters; normalization survives another load.
func FuzzSocialHandle(f *testing.F) {
	for _, handle := range []string{"", "a", "@A_1", "@", "@@a", "bad-handle", "é", strings.Repeat("a", 15), strings.Repeat("a", 16)} {
		f.Add(handle)
	}
	f.Fuzz(func(t *testing.T, handle string) {
		if len(handle) > 1024 {
			t.Skip()
		}
		root := t.TempDir()
		p, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		load := func(handle string) Loaded {
			raw, err := json.Marshal(map[string]any{"schemaVersion": 1, "url": "https://example.com/", "language": "en", "title": "T", "description": "D", "social": map[string]string{"x": handle}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ConfigFile), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			return Load(p, nil)
		}
		l := load(handle)
		if len(l.Problems) > 0 || handle == "" {
			return
		}
		normalized := l.Config.Social.X
		if !strings.HasPrefix(normalized, "@") || len(normalized) < 2 || len(normalized) > 16 {
			t.Fatalf("invalid normalized handle %q", normalized)
		}
		for _, c := range normalized[1:] {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_", c) {
				t.Fatalf("invalid handle character %q", c)
			}
		}
		again := load(normalized)
		if len(again.Problems) > 0 || again.Config.Social.X != normalized || again.Values["social"].(map[string]any)["x"] != normalized {
			t.Fatal("handle normalization was not stable")
		}
	})
}
