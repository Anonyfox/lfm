package imaging

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

func tiffOrientation(value uint16) []byte {
	b := make([]byte, 26)
	copy(b, []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0})
	binary.LittleEndian.PutUint16(b[18:], value)
	return b
}

func TestMetadataDimensionsAndEXIFOrientation(t *testing.T) {
	var jpg, pngData, gifData bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	if err := jpeg.Encode(&jpg, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&gifData, img, nil); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{jpg.Bytes(), pngData.Bytes(), gifData.Bytes()} {
		info := Inspect(raw)
		if info.Width != 12 || info.Height != 8 || info.Issue != "" || !info.Raster {
			t.Fatal(info)
		}
	}
	for orientation := uint16(1); orientation <= 8; orientation++ {
		tiff := tiffOrientation(orientation)
		exif := append([]byte("Exif\x00\x00"), tiff...)
		app := append([]byte{0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)
		raw := append(bytes.Clone(jpg.Bytes()[:2]), app...)
		raw = append(raw, jpg.Bytes()[2:]...)
		wantWidth, wantHeight := 12, 8
		if orientation >= 5 {
			wantWidth, wantHeight = 8, 12
		}
		info := Inspect(raw)
		if info.Width != wantWidth || info.Height != wantHeight || info.Issue != "" {
			t.Fatalf("JPEG orientation %d: %+v", orientation, info)
		}
		chunk := make([]byte, len(tiff)+12)
		binary.BigEndian.PutUint32(chunk, uint32(len(tiff)))
		copy(chunk[4:], "eXIf")
		copy(chunk[8:], tiff)
		binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
		original := pngData.Bytes()
		raw = append(bytes.Clone(original[:len(original)-12]), chunk...)
		raw = append(raw, original[len(original)-12:]...)
		info = Inspect(raw)
		if info.Width != 12 || info.Height != 8 || info.Issue != "" || (info.Unknown != "") != (orientation != 1) {
			t.Fatalf("PNG orientation should stay unguessed %d: %+v", orientation, info)
		}
	}
	webp, err := os.ReadFile("../../testdata/example/assets/images/share.webp")
	if err != nil {
		t.Fatal(err)
	}
	info := Inspect(webp)
	if info.Format != "webp" || info.Width != 1200 || info.Height != 630 || info.Issue != "" {
		t.Fatal(info)
	}
}

func TestSVGOnlyFixedUnambiguousPixelDimensions(t *testing.T) {
	for _, svg := range []string{`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60"><path d="M0 0h1z"/></svg>`, `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="120px" height="60px" viewBox="0 0 2 1"/>`} {
		info := Inspect([]byte(svg))
		if info.Width != 120 || info.Height != 60 || info.Raster || info.Issue != "" {
			t.Fatal(info)
		}
	}
	for _, svg := range []string{`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 60"/>`, `<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="60"/>`, `<svg xmlns="http://www.w3.org/2000/svg" width="120.5" height="60"/>`, `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60" style="width:40px"/>`, `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60"><style>svg{width:40px}</style></svg>`, `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60"><animate attributeName="width"/></svg>`, `<svg width="120" height="60"/>`, `<html width="120" height="60"/>`} {
		info := Inspect([]byte(svg))
		if info.Width != 0 || info.Height != 0 || info.Unknown == "" {
			t.Fatal(info)
		}
	}
}

func TestInvalidOrUnsupportedMetadataDoesNotInventDimensions(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("not an image"), []byte("\x89PNG\r\n\x1a\n"), []byte("\xff\xd8"), []byte("RIFF\x00\x00\x00\x00WEBP"), []byte("GIF89a"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"><`)} {
		info := Inspect(raw)
		if info.Width != 0 || info.Height != 0 || info.Issue == "" && info.Unknown == "" {
			t.Fatal(info)
		}
	}
	info := Inspect([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60">` + strings.Repeat(" ", metadataReadLimit) + `</svg>`))
	if info.Width != 0 || info.Unknown == "" {
		t.Fatal(info)
	}
}

func FuzzInspect(f *testing.F) {
	for _, raw := range [][]byte{[]byte("GIF89a"), []byte("\x89PNG\r\n\x1a\n"), []byte("\xff\xd8"), []byte("RIFF\x00\x00\x00\x00WEBP"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="12" height="8"/>`)} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 128*1024 {
			t.Skip()
		}
		before := bytes.Clone(raw)
		info := Inspect(raw)
		if info.Width < 0 || info.Height < 0 || !bytes.Equal(raw, before) {
			t.Fatal("invalid dimensions or mutated input")
		}
	})
}

func BenchmarkInspectPNG(b *testing.B) {
	var data bytes.Buffer
	png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 32, 16)))
	raw := data.Bytes()
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		Inspect(raw)
	}
}
