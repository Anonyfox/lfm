package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func actions(changes []Change) string {
	var parts []string
	for _, c := range changes {
		parts = append(parts, c.Name+"="+c.Action)
	}
	return strings.Join(parts, " ")
}

func TestStampIsVerifiable(t *testing.T) {
	s, err := Get("design", NewData("1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if _, state := inspect(s.Text, s.Text); state != Current {
		t.Fatalf("fresh copy is %s", state)
	}
	newer, _ := Get("design", NewData("1.0.1"))
	if v, state := inspect(s.Text, newer.Text); state != Outdated || v != "1.0.0" {
		t.Fatalf("older copy is %s %s", v, state)
	}
	if _, state := inspect(strings.Replace(s.Text, "## ", "## Our ", 1), newer.Text); state != Modified {
		t.Fatalf("edited copy is %s", state)
	}
	if _, state := inspect("---\nname: lfm-design\n---\nmine\n", newer.Text); state != Foreign {
		t.Fatalf("unstamped copy is %s", state)
	}
}

func TestInstallUpdatesOnlyUntouchedCopies(t *testing.T) {
	dir := filepath.Join(t.TempDir(), AgentsDir)
	changes, err := Install(dir, NewData("1.0.0"), false)
	if err != nil || actions(changes) != "lfm-research=created lfm-design=created lfm-implementation=created lfm-book=created lfm-printable=created lfm-spreadsheet=created lfm-document=created lfm-deck=created" {
		t.Fatalf("%s %v", actions(changes), err)
	}
	if changes, _ := Install(dir, NewData("1.0.0"), false); actions(changes) != "lfm-research=unchanged lfm-design=unchanged lfm-implementation=unchanged lfm-book=unchanged lfm-printable=unchanged lfm-spreadsheet=unchanged lfm-document=unchanged lfm-deck=unchanged" {
		t.Fatalf("second run: %s", actions(changes))
	}
	design := filepath.Join(dir, "lfm-design", "SKILL.md")
	raw, _ := os.ReadFile(design)
	edited := strings.Replace(string(raw), "## ", "## Our ", 1)
	os.WriteFile(design, []byte(edited), 0o644)
	os.WriteFile(filepath.Join(dir, "lfm-research", "SKILL.md"), []byte("---\nname: lfm-research\n---\nhand written\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "lfm-design", "notes.md"), []byte("ours"), 0o644)
	// A skill an older lfm shipped and this one dropped.
	gone := filepath.Join(dir, "lfm-retired", "SKILL.md")
	write(gone, stamp("---\nname: lfm-retired\ndescription: old\n---\nbody\n", "0.9.0"))

	changes, err = Install(dir, NewData("1.0.1"), false)
	if err != nil || actions(changes) != "lfm-research=kept lfm-design=kept lfm-implementation=updated lfm-book=updated lfm-printable=updated lfm-spreadsheet=updated lfm-document=updated lfm-deck=updated lfm-retired=removed" {
		t.Fatalf("update: %s %v", actions(changes), err)
	}
	if changes[0].Reason != "not written by lfm; --force replaces it" || changes[1].Reason != "changed in this project; --force replaces it" || changes[2].Reason != "was lfm 1.0.0" {
		t.Fatalf("reasons: %+v", changes)
	}
	if raw, _ := os.ReadFile(design); string(raw) != edited {
		t.Fatal("an edited copy was overwritten")
	}
	if _, err := os.Stat(filepath.Dir(gone)); !os.IsNotExist(err) {
		t.Fatal("the dropped skill is still installed")
	}
	changes, _ = Install(dir, NewData("1.0.1"), true)
	if actions(changes) != "lfm-research=replaced lfm-design=replaced lfm-implementation=unchanged lfm-book=unchanged lfm-printable=unchanged lfm-spreadsheet=unchanged lfm-document=unchanged lfm-deck=unchanged" {
		t.Fatalf("force: %s", actions(changes))
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "lfm-design", "notes.md")); string(raw) != "ours" {
		t.Fatal("other files in a skill folder were touched")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*", ".SKILL.md.*"))
	if len(leftovers) > 0 {
		t.Fatalf("temporary files left: %v", leftovers)
	}
}

func TestFindWalksUpAndCountsLinkedFoldersOnce(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "sites", "page")
	os.MkdirAll(project, 0o755)
	if _, err := Install(filepath.Join(root, AgentsDir), NewData("1.0.0"), false); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(root, ".claude"), 0o755)
	os.Symlink(filepath.Join(root, AgentsDir), filepath.Join(root, ClaudeDir))
	found := Find(project, NewData("1.0.1"))
	if len(found) != 8 || found[0].State != Outdated || found[0].Version != "1.0.0" {
		t.Fatalf("%+v", found)
	}
}
