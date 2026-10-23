package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/testproject"
)

func TestCompileJSONManifest(t *testing.T) {
	t.Parallel()
	dir := testproject.Book(t)
	testproject.Configure(t, dir, func(c map[string]any) {
		c["url"] = "https://example.com/"
		c["social"] = map[string]any{"x": "example"}
	})
	code, out, errs := run(t, "compile", dir, "--json")
	var got struct {
		Files []struct {
			Name   string `json:"name"`
			Bytes  int    `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
		SocialMeta []string `json:"socialMetaRepairs"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 || errs != "" {
		t.Fatalf("exit %d, stderr %q, JSON error %v\n%s", code, errs, err, out)
	}
	seen := map[string]bool{}
	for _, f := range got.Files {
		data, err := os.ReadFile(filepath.Join(dir, "output", filepath.FromSlash(f.Name)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if seen[f.Name] || f.Bytes != len(data) || f.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("invalid manifest entry %+v", f)
		}
		seen[f.Name] = true
	}
	err := filepath.WalkDir(filepath.Join(dir, "output"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(filepath.Join(dir, "output"), path)
		if err == nil && !seen[filepath.ToSlash(rel)] {
			t.Errorf("output file absent from JSON manifest: %s", rel)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "robots.txt", "sitemap.xml", testproject.BookPath + "/index.html", testproject.BookPath + "/the-weekend-starter.epub"} {
		if !seen[name] {
			t.Errorf("missing output %s", name)
		}
	}
	if strings.Join(got.SocialMeta, ",") != "twitter:site,twitter:creator" {
		t.Fatal("JSON report lacks social repairs", got.SocialMeta)
	}
	code, out, errs = run(t, "compile", dir)
	if code != 0 || errs != "" || !strings.Contains(out, "added twitter:site, twitter:creator from social.x") {
		t.Fatalf("human report lacks social repairs: exit %d, stderr %q\n%s", code, errs, out)
	}
}
