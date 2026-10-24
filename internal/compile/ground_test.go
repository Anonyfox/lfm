package compile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

func TestWarmGroundFailsBeforePublish(t *testing.T) {
	for _, css := range []string{
		`:root{--ground:#faf7f2}body{background:var(--ground)}`,
		`:root{--ground:#F4EFE6;--accent:#8F3A26}body{background:var(--ground);color:var(--accent)}`,
	} {
		t.Run(css, func(t *testing.T) {
			root := fixture(t)
			write(t, root, "assets/styles/main.css", css)
			result, err := Compile(context.Background(), root, Options{Publish: true})
			if err == nil {
				t.Fatal("warm ground compiled")
			}
			found := false
			if result.Validation != nil {
				for _, d := range result.Validation.Diagnostics {
					if d.Rule == "design.ground" && d.Severity == diagnostic.Error {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("missing ground error: %+v (%v)", result.Validation, err)
			}
			if _, err := os.Stat(filepath.Join(root, "output")); !os.IsNotExist(err) {
				t.Fatalf("published before verify: %v", err)
			}

		})
	}
}
