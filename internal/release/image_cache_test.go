package release

import (
	"bytes"
	"errors"
	"testing"
)

func TestPictureContentCache(t *testing.T) {
	cacheTools(t)
	assets := map[string][]byte{"frame.css": []byte("frame"), "font.woff2": []byte("font")}
	base := imageKey("renderer/chrome1", []byte("document"), assets, 100, 200, 2)
	for _, tc := range []struct {
		name string
		key  string
		hit  bool
	}{
		{"identical", base, true},
		{"asset order", imageKey("renderer/chrome1", []byte("document"), map[string][]byte{"font.woff2": []byte("font"), "frame.css": []byte("frame")}, 100, 200, 2), true},
		{"document", imageKey("renderer/chrome1", []byte("changed"), assets, 100, 200, 2), false},
		{"frame", imageKey("renderer/chrome1", []byte("document"), map[string][]byte{"frame.css": []byte("changed")}, 100, 200, 2), false},
		{"chrome", imageKey("renderer/chrome2", []byte("document"), assets, 100, 200, 2), false},
		{"renderer", imageKey("renderer2/chrome1", []byte("document"), assets, 100, 200, 2), false},
		{"width", imageKey("renderer/chrome1", []byte("document"), assets, 101, 200, 2), false},
		{"height", imageKey("renderer/chrome1", []byte("document"), assets, 100, 201, 2), false},
		{"scale", imageKey("renderer/chrome1", []byte("document"), assets, 100, 200, 1), false},
		{"unknown identity", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cachedPicture(base, func() ([]byte, error) { return []byte("original pixels"), nil })
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			raw, err := cachedPicture(tc.key, func() ([]byte, error) { calls++; return []byte("new pixels"), nil })
			if err != nil {
				t.Fatal(err)
			}
			if tc.hit && (calls != 0 || !bytes.Equal(raw, []byte("original pixels"))) {
				t.Fatalf("cache miss or changed pixels: %d %q", calls, raw)
			}
			if !tc.hit && calls != 1 {
				t.Fatal("changed input reused render")
			}
		})
	}
	t.Run("errors are retried", func(t *testing.T) {
		calls := 0
		for range 2 {
			_, _ = cachedPicture(base+"error", func() ([]byte, error) { calls++; return nil, errors.New("render failed") })
		}
		if calls != 2 {
			t.Fatal("cached render error")
		}
	})
}

func TestPageImageCacheKeepsOrderedCompleteSets(t *testing.T) {
	cacheTools(t)
	for _, tc := range []struct {
		name      string
		images    [][]byte
		wantCalls int
	}{
		{"complete", [][]byte{[]byte("page three"), []byte("page one")}, 1},
		{"partial", [][]byte{[]byte("page three")}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			for range 2 {
				got, err := cachedPageImages(tc.name, 2, func() ([][]byte, error) { calls++; return tc.images, nil })
				if err != nil || len(got) != len(tc.images) {
					t.Fatalf("%v %q", err, got)
				}
				for i := range got {
					if !bytes.Equal(got[i], tc.images[i]) {
						t.Fatal("page order or bytes changed")
					}
				}
			}
			if calls != tc.wantCalls {
				t.Fatalf("rendered %d times, want %d", calls, tc.wantCalls)
			}
		})
	}
}
