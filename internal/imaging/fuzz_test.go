package imaging

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
	"time"
)

// Property: Inspect is deterministic and bounded, and it reports either a
// complete, unqualified pixel size or no size at all; it never invents one.
func FuzzInspectDimensions(f *testing.F) {
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	var jpg, pngData, gifData bytes.Buffer
	if jpeg.Encode(&jpg, img, nil) != nil || png.Encode(&pngData, img) != nil || gif.Encode(&gifData, img, nil) != nil {
		f.Fatal("cannot encode seeds")
	}
	exif := append([]byte("Exif\x00\x00"), tiffOrientation(6)...)
	rotated := append(append(bytes.Clone(jpg.Bytes()[:2]), append([]byte{0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)...), jpg.Bytes()[2:]...)
	for _, raw := range [][]byte{jpg.Bytes(), pngData.Bytes(), gifData.Bytes(), rotated, []byte("RIFF\x00\x00\x00\x00WEBPVP8X"),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="12" height="8"/>`), []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="1e3" height="8px"/>`)} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 128*1024 {
			t.Skip()
		}
		start := time.Now()
		info := Inspect(raw)
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("Inspect took %v", elapsed)
		}
		if again := Inspect(raw); again != info {
			t.Fatalf("not deterministic: %+v then %+v", info, again)
		}
		sized := info.Width > 0 && info.Height > 0
		unsized := info.Width == 0 && info.Height == 0
		if info.Bytes != len(raw) || !(sized && info.Issue == "" && info.Unknown == "" || unsized) {
			t.Fatalf("partial or qualified dimensions: %+v", info)
		}
		if unsized && info.Issue == "" && info.Unknown == "" {
			t.Fatalf("no dimensions and no explanation: %+v", info)
		}
	})
}
