package book

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // cover sources
	"math"

	_ "golang.org/x/image/webp" // cover sources
)

// MaxCoverBytes is the largest store cover KDP takes.
const MaxCoverBytes = 5 << 20

// StoreCover returns the cover as the RGB JPEG every store accepts: the
// source itself when it already is one under 5 MB, otherwise re-encoded.
// A re-encoded cover carries the hash of its source in a comment, so the
// cover published last time is reused while the source is unchanged:
// re-encoding takes a tenth of a second.
func StoreCover(src, previous []byte) ([]byte, error) {
	// The header tells format and color model; most covers are used as they are.
	config, format, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("the cover cannot be decoded: %w", err)
	}
	if format == "jpeg" && config.ColorModel != color.CMYKModel && len(src) <= MaxCoverBytes {
		return src, nil
	}
	sum := sha256.Sum256(src)
	comment := append([]byte{0xff, 0xfe, 0, 0}, "lfm-cover "+hex.EncodeToString(sum[:16])...)
	binary.BigEndian.PutUint16(comment[2:], uint16(len(comment)-2))
	if len(previous) > 2+len(comment) && bytes.Equal(previous[2:2+len(comment)], comment) {
		return previous, nil
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("the cover cannot be decoded: %w", err)
	}
	rgb := image.NewRGBA(img.Bounds())
	draw.Draw(rgb, rgb.Bounds(), img, img.Bounds().Min, draw.Src)
	for quality := 92; quality >= 60; quality -= 8 {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, rgb, &jpeg.Options{Quality: quality}); err != nil {
			return nil, err
		}
		if out.Len()+len(comment) <= MaxCoverBytes {
			jpg := out.Bytes()
			return append(append(append([]byte{}, jpg[:2]...), comment...), jpg[2:]...), nil
		}
	}
	return nil, fmt.Errorf("the cover stays above 5 MB as a JPEG; export it smaller")
}

// PrintDPI is the resolution an image of the given pixels reaches when it
// fills a print area of the given inches, cropped to cover it.
func PrintDPI(pixels [2]int, inches [2]float64) int {
	if pixels[0] == 0 || inches[0] == 0 {
		return 0
	}
	return int(math.Floor(math.Min(float64(pixels[0])/inches[0], float64(pixels[1])/inches[1])))
}
