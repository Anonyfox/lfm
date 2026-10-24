package compile

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
	"golang.org/x/net/html"
)

// socialCard bundles the card with the same source boundary and resource resolver
// as the page. Its relative URLs are resolved beside the authored HTML.
func (b *builder) socialCard(source string) ([]byte, error) {
	path, err := b.project.Path(source)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b.recordInput(path)
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	card := &builder{ctx: b.ctx, project: b.project, root: filepath.Dir(path), outDir: b.outDir, config: b.config, files: map[string]emitted{}, external: map[string]bool{}, inlineSources: map[[32]byte][]string{}}
	card.config.SocialImage.Src = ""
	card.config.Build.BrowserData = nil
	if err := card.processHTML(doc, nil, nil); err != nil {
		return nil, err
	}
	for _, input := range card.inputList() {
		b.recordInput(input)
	}
	var page bytes.Buffer
	if err := html.Render(&page, doc); err != nil {
		return nil, err
	}
	assets := card.sidecars()
	inputs := maps.Clone(assets)
	inputs["source:"+b.project.Rel(path)] = raw
	for _, input := range card.inputList() {
		source, err := os.ReadFile(input)
		if err != nil {
			return nil, err
		}
		inputs["source:"+b.project.Rel(input)] = source
	}
	key := printKey("social-1200x630", page.Bytes(), inputs)
	st := newStock(b.ctx, b.project, true)
	defer st.close()
	pr := st.press()
	if previous := pr.lookup(key); previous != nil {
		return previous, nil
	}
	printer, err := st.printerFor()
	if err != nil {
		return nil, err
	}
	png, err := printer.Picture(page.Bytes(), assets, 1200, 630, 1)
	if err != nil {
		return nil, err
	}
	jpg, err := jpegUnder(png, len(png), key)
	if err != nil {
		return nil, err
	}
	if len(jpg) < len(png) {
		return jpg, nil
	}
	return stampCardPNG(png, key), nil
}

// The input key lives in a standard PNG text chunk just before IEND, so
// stock can read it from the tail without decoding the image.
func stampCardPNG(png []byte, key string) []byte {
	const prefix = "tEXtlfm-key\x00"
	chunk := make([]byte, 4+len(prefix)+len(key)+4)
	binary.BigEndian.PutUint32(chunk, uint32(len(prefix)-4+len(key)))
	copy(chunk[4:], prefix+key)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	out := append([]byte(nil), png[:len(png)-12]...)
	out = append(out, chunk...)
	return append(out, png[len(png)-12:]...)
}

func socialPNGStamp(raw []byte) string {
	const prefix = "tEXtlfm-key\x00"
	const size = 4 + len(prefix) + 32 + 4
	if len(raw) < size+12 {
		return ""
	}
	chunk := raw[len(raw)-12-size : len(raw)-12]
	if binary.BigEndian.Uint32(chunk) != uint32(len(prefix)-4+32) || string(chunk[4:4+len(prefix)]) != prefix || binary.BigEndian.Uint32(chunk[size-4:]) != crc32.ChecksumIEEE(chunk[4:size-4]) {
		return ""
	}
	return string(chunk[4+len(prefix) : size-4])
}

func socialCardFailure(source string, err error) diagnostic.Diagnostic {
	reason := err.Error()
	if errors.Is(err, context.DeadlineExceeded) {
		reason = "did not render within 60 s"
	}
	return diagnostic.Diagnostic{Rule: "build.failed", Severity: diagnostic.Error, Location: diagnostic.Location{File: project.ConfigFile, Line: 1, Column: 1}, Path: "socialImage.src", Message: fmt.Sprintf("Social card %s: %s", source, reason), Hint: "Set LFM_CHROME to a working Chrome executable, or point socialImage.src at a 1200x630 PNG/JPEG."}
}

func (b *builder) cardResource(path string) (string, error) {
	if ref, ok := b.cards[path]; ok {
		return ref, nil
	}
	if b.cards == nil {
		b.cards = map[string]string{}
	}
	b.cards[path] = ""
	raw, err := b.socialCard(path)
	if err != nil {
		b.notices = append(b.notices, socialCardFailure(b.config.SocialImage.Src, err))
		// Continue assembly so the ordinary diagnostic gate reports this failure.
		return "", nil
	}
	ext := ".png"
	if bytes.HasPrefix(raw, []byte{0xff, 0xd8}) {
		ext = ".jpg"
	}
	name := fmt.Sprintf("%s-%s%s", strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), printKey("social-image", raw, nil)[:12], ext)
	if err := b.emit(name, path, raw); err != nil {
		return "", err
	}
	b.cards[path] = fileURL(name)
	return b.cards[path], nil
}
