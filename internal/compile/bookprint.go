package compile

import (
	"fmt"
	"sync"

	"github.com/Anonyfox/lfm/internal/pdfx"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/release"
)

// screen returns the downloadable PDF, printed twice: the first print finds
// the page each chapter starts on, the second prints those pages in the
// contents.
func (pr *press) screen(built book.Built) ([]byte, error) {
	return pr.cachedJob(printKey("screen", built.Print, built.PrintAssets), true, func(printer *release.Printer) ([]byte, error) {
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
			total := pdfx.PageCount(measure) + len(blanks) + 1
			pdf, err := printer.Print(built.Document(book.PrintOptions{Paper: true, Inside: inside, Pages: pages, Blanks: blanks, PadEnd: 1 + total%2}), built.PrintAssets, false)
			if err != nil {
				return nil, err
			}
			if n := pdfx.PageCount(pdf); book.Gutter(n) != inside {
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
	pages := pdfx.PageCount(interior)
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
