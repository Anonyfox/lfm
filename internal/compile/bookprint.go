package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
)

// printFormat names how lfm prints books; changing the passes below changes it,
// so files printed by an older lfm are printed again.
const printFormat = "lfm-book-print-1"

// press prints the book's PDFs, or finds them already printed. Chrome takes
// seconds, so only a release prints. Every printed file is stamped with the
// key of the inputs it was printed from, and any compile reuses a file from
// the last publish in output/ or distribution/ whose key still matches. An
// unchanged book therefore compiles without a browser; a changed one keeps
// its EPUB and pages current and waits for the next release for its prints.
type press struct {
	previous map[string][]byte // stamp key -> PDF
	release  bool
	pending  bool // a print was due but this compile does not print

	mu       sync.Mutex
	printer  *release.Printer
	started  bool
	startErr error
}

func newPress(p project.Project, downloadPath string, isRelease bool) *press {
	pr := &press{previous: map[string][]byte{}, release: isRelease}
	for _, dir := range []string{filepath.Join(p.Root, project.OutputDir, downloadPath), filepath.Join(p.Root, project.DistributionDir)} {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".pdf") {
				return nil
			}
			if key := stampOfFile(path); key != "" && pr.previous[key] == nil {
				if pdf, err := os.ReadFile(path); err == nil && book.StampOf(pdf) == key {
					pr.previous[key] = pdf
				}
			}
			return nil
		})
	}
	return pr
}

// stampOfFile reads only the end of a file, where the stamp is.
func stampOfFile(path string) string {
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
	return book.StampOf(tail)
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

// job returns the PDF for key: the stamped file from the last publish, or, in
// a release, a fresh print stamped with key. Otherwise it returns nil and
// marks the press pending.
func (pr *press) job(key string, run func(*release.Printer) ([]byte, error)) ([]byte, error) {
	if pdf := pr.previous[key]; pdf != nil {
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
	return book.Stamp(pdf, key)
}

// screen returns the downloadable PDF, printed twice: the first print finds
// the page each chapter starts on, the second prints those pages in the
// contents.
func (pr *press) screen(built book.Built) ([]byte, error) {
	return pr.job(printKey("screen", built.Print, built.PrintAssets), func(printer *release.Printer) ([]byte, error) {
		first, err := printer.Print(built.Print, built.PrintAssets, true)
		if err != nil {
			return nil, err
		}
		starts := built.ChapterStarts(first)
		if starts == nil {
			return first, nil
		}
		return printer.Print(built.Document(book.PrintOptions{Pages: starts}), built.PrintAssets, true)
	})
}

// paperback returns the paperback interior and the cover wraps the channels
// taking it lay out. A measuring
// print finds where the chapters start, blank pages move each onto a
// right-hand page, and the end is padded to an even count with a blank last
// page, as printers want. The inside margin follows the final page count, so
// a count that crosses into a wider gutter prints again.
func (pr *press) paperback(r *book.Report, built book.Built) book.Printed {
	printed := book.Printed{Wraps: map[string][]byte{}, Spines: map[string]float64{}}
	base := built.Document(book.PrintOptions{Paper: true, Inside: book.Gutter(0)})
	interior, err := pr.job(printKey("paper", base, built.PrintAssets), func(printer *release.Printer) ([]byte, error) {
		inside := book.Gutter(0)
		for range 3 {
			measure, err := printer.Print(built.Document(book.PrintOptions{Paper: true, Inside: inside}), built.PrintAssets, true)
			if err != nil {
				return nil, err
			}
			starts := built.ChapterStarts(measure)
			if starts == nil {
				return nil, fmt.Errorf("the chapter pages could not be read from the measuring print")
			}
			blanks, pages := built.Recto(starts)
			total := book.PageCount(measure) + len(blanks) + 1
			pdf, err := printer.Print(built.Document(book.PrintOptions{Paper: true, Inside: inside, Pages: pages, Blanks: blanks, PadEnd: 1 + total%2}), built.PrintAssets, false)
			if err != nil {
				return nil, err
			}
			if n := book.PageCount(pdf); book.Gutter(n) != inside {
				inside = book.Gutter(n)
				continue
			}
			return pdf, nil
		}
		return nil, fmt.Errorf("the page count does not settle between two inside margins")
	})
	switch {
	case err != nil:
		printed.Problems = append(printed.Problems, "The paperback interior could not be printed: "+err.Error())
		return printed
	case interior == nil:
		return printed
	}
	pages := book.PageCount(interior)
	printed.Interior, printed.InteriorPages, printed.Inside = interior, pages, book.Gutter(pages)
	for _, key := range book.Wraps(*r, pages) {
		switch key {
		case "kdp":
			printed.Spines[key] = book.KDPSpine(pages, r.Config.Paper)
		case "lulu":
			printed.Spines[key] = book.LuluSpine(pages)
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for key, spine := range printed.Spines {
		wg.Go(func() {
			// KDP prints spine text from 79 pages; narrower spines stay blank.
			doc := built.CoverDocument(spine, pages >= 79)
			wrap, err := pr.job(printKey("wrap", doc, built.PrintAssets), func(printer *release.Printer) ([]byte, error) {
				return printer.Print(doc, built.PrintAssets, false)
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				printed.Problems = append(printed.Problems, "The "+key+" cover wrap could not be printed: "+err.Error())
			case wrap != nil:
				printed.Wraps[key] = wrap
			}
		})
	}
	wg.Wait()
	return printed
}
