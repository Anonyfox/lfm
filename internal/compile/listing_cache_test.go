package compile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
)

func TestListingCacheReusesUnpublishedImages(t *testing.T) {
	t.Setenv("LFM_CACHE_DIR", t.TempDir())
	tool := filepath.Join(t.TempDir(), "version")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho fixture\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LFM_CHROME", tool)
	plans := []listingPlan{{Name: "published.jpg", Key: "one"}, {Name: "unpublished.jpg", Key: "two"}}
	assets := map[string][]byte{"font": []byte("font")}
	identity := release.ImageIdentity(context.Background())
	images := []deliver.Image{{Name: plans[0].Name, Data: []byte("first pixels")}, {Name: plans[1].Name, Data: []byte("second pixels")}}
	for _, tc := range []struct {
		name     string
		plans    []listingPlan
		assets   map[string][]byte
		complete bool
	}{
		{"complete set without output", plans, assets, true},
		{"changed plan", []listingPlan{{Name: "published.jpg", Key: "changed"}}, assets, false},
		{"changed asset", plans, map[string][]byte{"font": []byte("changed")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release.WriteImageCache(listingSetKey(identity, plans, assets), images)
			st := newStock(context.Background(), project.Project{Root: t.TempDir()}, true)
			pr := st.press()
			got := listingImages(pr, tc.plans, tc.assets, nil)
			if tc.complete {
				if len(got) != 2 || !bytes.Equal(got[1].Data, images[1].Data) || pr.pending || st.started {
					t.Fatalf("whole set not reused: %+v", got)
				}
			} else if len(got) != 0 || !pr.pending {
				t.Fatalf("changed recipe reused images: %+v", got)
			}
		})
	}
}
