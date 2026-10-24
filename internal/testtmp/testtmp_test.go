package testtmp

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	marker := os.Getenv("LFM_TESTTMP_CHILD")
	if marker == "" {
		Main(m)
		return
	}
	Main(m, func(dir string) error {
		if err := os.WriteFile(marker, []byte(dir), 0600); err != nil {
			return err
		}
		if os.Getenv("LFM_TESTTMP_FAIL") == "1" {
			return errors.New("fixture setup failed")
		}
		return nil
	})
}

func TestMainChild(t *testing.T) {
	if os.Getenv("LFM_TESTTMP_CHILD") == "" {
		t.Skip("subprocess fixture")
	}
	if _, err := os.Stat(os.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestMainRemovesFixturesOnSuccessAndSetupFailure(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, fail := range []string{"0", "1"} {
		marker := filepath.Join(t.TempDir(), "root")
		cmd := exec.Command(executable, "-test.run=^TestMainChild$")
		cmd.Env = append(os.Environ(), "LFM_TESTTMP_CHILD="+marker, "LFM_TESTTMP_FAIL="+fail)
		output, err := cmd.CombinedOutput()
		if (err != nil) != (fail == "1") {
			t.Fatalf("%s: %v %s", fail, err, output)
		}
		if fail == "1" && !strings.Contains(string(output), "fixture setup failed") {
			t.Fatal(string(output))
		}
		root, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(string(root)); !os.IsNotExist(err) {
			t.Fatalf("fixture root remains: %s (%v)", root, err)
		}
	}
}

func TestLighthouseFixture(t *testing.T) {
	t.Setenv("LFM_LIGHTHOUSE", "")
	dir := t.TempDir()
	if err := Lighthouse(dir); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(os.Getenv("LFM_LIGHTHOUSE")).Output()
	if err != nil || !json.Valid(output) || !strings.Contains(string(output), `"performance":{"score":1}`) {
		t.Fatalf("%v %s", err, output)
	}
	if err := Lighthouse(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("accepted missing fixture directory")
	}
}
