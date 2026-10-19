package compile

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/ooxml"
	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
)

// printFormat names how lfm prints deliverables; changing the passes below changes it,
// so files printed by an older lfm are printed again.
const printFormat = "lfm-book-print-1"

// press prints a deliverable's PDFs and images, or finds them already
// printed. Chrome takes seconds, so only a release prints. Every printed file
// is stamped with the key of the inputs it was printed from, and any compile
// reuses a file from the last publish in output/ or distribution/ whose key
// still matches. An unchanged deliverable therefore compiles without a
// browser; a changed one keeps everything else current and waits for the
// next release for its prints.
type press struct {
	previous map[string][]byte // stamp key -> PDF, JPEG or workbook
	zipped   map[string]zipRef // stamp key -> a stored file inside a published ZIP, read on use
	release  bool
	pending  bool // a print was due but this compile does not print

	mu       sync.Mutex
	printer  *release.Printer
	started  bool
	startErr error
}

func newPress(p project.Project, isRelease bool) *press {
	pr := &press{previous: map[string][]byte{}, zipped: map[string]zipRef{}, release: isRelease}
	// Stamped prints live in the deliverables' download folders in output/
	// and in distribution/; the page's own assets carry no stamp.
	for _, dir := range []string{filepath.Join(p.Root, project.OutputDir), filepath.Join(p.Root, project.DistributionDir)} {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			var key string
			switch {
			case strings.HasSuffix(path, ".pdf"):
				key = stampOfFile(path)
			case strings.HasSuffix(path, ".jpg"):
				key = jpegStampOfFile(path)
			case strings.HasSuffix(path, ".xlsx"):
				key = tailStampOfFile(path, spreadsheet.Stamp)
			case strings.HasSuffix(path, ".docx"), strings.HasSuffix(path, ".pptx"):
				key = tailStampOfFile(path, ooxml.Stamp)
			case strings.HasSuffix(path, ".zip"):
				pr.scanZip(path)
				return nil
			}
			if key == "" || pr.previous[key] != nil {
				return nil
			}
			if data, err := os.ReadFile(path); err == nil && stampOf(data) == key {
				pr.previous[key] = data
			}
			return nil
		})
	}
	return pr
}

// stampOf is the key any stamped print or build carries.
func stampOf(data []byte) string {
	for _, of := range []func([]byte) string{pdfx.StampOf, deliver.JPEGStamp, spreadsheet.Stamp, ooxml.Stamp} {
		if key := of(data); key != "" {
			return key
		}
	}
	return ""
}

// zipRef locates a stored file inside a ZIP, or the whole ZIP.
type zipRef struct {
	path         string
	offset, size int64
}

// scanZip indexes the stamped files packed in a published ZIP, such as the
// documents and PDFs of an edition's download, and the ZIP's own stamp. It
// reads only their ends, where the stamps are; lookup reads a file when a
// compile uses it.
func (pr *press) scanZip(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return
	}
	tail := func(offset, size int64) string {
		buf := make([]byte, min(size, 4096))
		if _, err := f.ReadAt(buf, offset+size-int64(len(buf))); err != nil {
			return ""
		}
		return stampOf(buf)
	}
	if key := tail(0, info.Size()); key != "" {
		pr.zipped[key] = zipRef{path, 0, info.Size()}
	}
	z, err := zip.NewReader(f, info.Size())
	if err != nil {
		return
	}
	for _, e := range z.File {
		switch strings.ToLower(filepath.Ext(e.Name)) {
		case ".pdf", ".docx", ".xlsx":
		default:
			continue
		}
		offset, err := e.DataOffset()
		if err != nil || e.Method != zip.Store {
			continue
		}
		if key := tail(offset, int64(e.CompressedSize64)); key != "" {
			if _, seen := pr.zipped[key]; !seen {
				pr.zipped[key] = zipRef{path, offset, int64(e.CompressedSize64)}
			}
		}
	}
}

// has reports whether the last publish holds the stamped file of key,
// without reading it.
func (pr *press) has(key string) bool {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	_, zipped := pr.zipped[key]
	return pr.previous[key] != nil || zipped
}

// lookup returns the stamped print or build of key from the last publish:
// a file in output/ or distribution/, or one packed in a ZIP there.
func (pr *press) lookup(key string) []byte {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if data := pr.previous[key]; data != nil {
		return data
	}
	ref, ok := pr.zipped[key]
	if !ok {
		return nil
	}
	f, err := os.Open(ref.path)
	if err != nil {
		return nil
	}
	defer f.Close()
	data := make([]byte, ref.size)
	if _, err := f.ReadAt(data, ref.offset); err != nil || stampOf(data) != key {
		return nil
	}
	pr.previous[key] = data
	return data
}

// jpegStampOfFile reads only the start of a JPEG, where the stamp is.
func jpegStampOfFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 128)
	n, _ := f.Read(head)
	return deliver.JPEGStamp(head[:n])
}

// stampOfFile reads only the end of a file, where the stamp is.
func stampOfFile(path string) string { return tailStampOfFile(path, pdfx.StampOf) }

func tailStampOfFile(path string, stampOf func([]byte) string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	tail := make([]byte, min(info.Size(), 4096))
	if _, err := f.ReadAt(tail, info.Size()-int64(len(tail))); err != nil {
		return ""
	}
	return stampOf(tail)
}

func (pr *press) printerFor() (*release.Printer, error) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if !pr.started {
		pr.started = true
		pr.printer, pr.startErr = release.NewPrinter()
	}
	return pr.printer, pr.startErr
}

func (pr *press) close() {
	if pr.printer != nil {
		pr.printer.Close()
	}
}

// printKey identifies a print job by everything that goes into it.
func printKey(kind string, doc []byte, assets map[string][]byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", printFormat, kind)
	h.Write(doc)
	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(h, "\x00%s\x00%d\x00", name, len(assets[name]))
		h.Write(assets[name])
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// job returns the PDF (or stamped JPEG) for key: the stamped file from the last publish, or, in
// a release, a fresh print stamped with key. Otherwise it returns nil and
// marks the press pending.
func (pr *press) job(key string, run func(*release.Printer) ([]byte, error)) ([]byte, error) {
	if pdf := pr.lookup(key); pdf != nil {
		return pdf, nil
	}
	if !pr.release {
		pr.mu.Lock()
		pr.pending = true
		pr.mu.Unlock()
		return nil, nil
	}
	printer, err := pr.printerFor()
	if err != nil {
		return nil, err
	}
	pdf, err := run(printer)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(pdf, []byte{0xff, 0xd8}) {
		return pdf, nil // a JPEG, stamped by jpegUnder
	}
	return pdfx.Stamp(pdf, key)
}

// offerPrices returns the page's offers by key with their prices, which a
// deliverable's listing must match.
func offerPrices(commerce commercecheck.Facts) map[string]*deliver.Price {
	offers := map[string]*deliver.Price{}
	for _, offer := range commerce.Offers {
		var price *deliver.Price
		if m := offer.Price; m != nil && m.Scale > 0 {
			amount := new(big.Rat).SetFrac64(m.AmountMinor, m.Scale)
			currency := strings.ToUpper(m.Currency)
			price = &deliver.Price{Text: amount.FloatString(2) + " " + currency, Amount: amount, Currency: currency}
		}
		offers[strings.TrimPrefix(offer.Path, "data.commerce.offers.")] = price
	}
	return offers
}

// jpegUnder encodes a PNG as a JPEG of at most limit bytes, lowering the
// quality as needed, stamped with key.
func jpegUnder(png []byte, limit int, key string) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	for quality := 90; quality >= 50; quality -= 8 {
		out.Reset()
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, err
		}
		if out.Len() <= limit {
			break
		}
	}
	return deliver.StampJPEG(out.Bytes(), key), nil
}
