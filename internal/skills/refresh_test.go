package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshUpdatesUntouchedOlderCopies(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, AgentsDir)
	if _, err := Install(dir, NewData("0.1.7"), false); err != nil {
		t.Fatal(err)
	}
	// One copy changed by the project must survive.
	changed := filepath.Join(dir, "lfm-design", "SKILL.md")
	raw, err := os.ReadFile(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, append(raw, "\nlocal note\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	pages := filepath.Join(root, "pages", "home")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	data := NewData("0.1.9")
	found := Find(pages, data)
	copies, changes, err := Refresh(found, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) == 0 || len(changes) != len(found)-1 {
		t.Fatalf("changes %d of %d copies", len(changes), len(found))
	}
	for _, c := range copies {
		want := Current
		if c.Name == "lfm-design" {
			want = Modified
		}
		if c.State != want {
			t.Errorf("%s: %s, want %s", c.Name, c.State, want)
		}
	}
	for _, c := range Find(pages, data) {
		if c.Name != "lfm-design" && c.State != Current {
			t.Errorf("%s on disk: %s", c.Name, c.State)
		}
	}
	if raw, _ := os.ReadFile(changed); !strings.Contains(string(raw), "local note") {
		t.Error("modified copy overwritten")
	}
}

func TestRefreshLeavesNewerAndDevCopies(t *testing.T) {
	dir := filepath.Join(t.TempDir(), AgentsDir)
	if _, err := Install(dir, NewData("0.2.0"), false); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.1.9", "dev", "0.1.9-rc1"} {
		data := NewData(version)
		found, err := Scan(dir, data)
		if err != nil {
			t.Fatal(err)
		}
		if _, changes, err := Refresh(found, data); err != nil || len(changes) != 0 {
			t.Errorf("%s: %d changes, %v", version, len(changes), err)
		}
	}
}

func TestOlder(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"0.1.7", "0.1.8", true}, {"0.1.10", "0.1.9", false}, {"0.1.9", "0.1.9", false},
		{"v0.1.7", "0.2.0", true}, {"", "0.1.9", true}, {"0.1.7", "dev", false}, {"1.0.0", "0.9.9", false},
	} {
		if got := older(tc.a, tc.b); got != tc.want {
			t.Errorf("older(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}
