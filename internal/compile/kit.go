package compile

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Anonyfox/lfm/internal/buildid"
	"github.com/Anonyfox/lfm/internal/cache"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/scratch"
	"github.com/Anonyfox/lfm/internal/term"
)

// The steps every deliverable of editable files shares (documents, decks,
// and for listing images spreadsheets): printing with checks on fresh
// prints, second opinions from other apps, start guides, reused ZIPs and
// listing images.

// finding is a release finding about one source file.
type finding struct{ File, Message string }

// warnings collects findings in delivery order.
type warnings struct {
	out []diagnostic.Diagnostic
}

func (w *warnings) add(file, rule, message, hint string) {
	w.out = append(w.out, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: file, Line: 1, Column: 1}, Message: message, Hint: hint})
}

// printJob is one PDF a built file ships.
type printJob struct {
	key, slot  string        // its print key; where its PDF goes
	file, name string        // the source, and the file the PDF belongs to
	doc        func() []byte // what Chrome prints
	// checks hold a fresh print to its plan; printer runs page checks
	// such as boxes that cut their text off.
	checks func(pdf []byte, printer *release.Printer) []finding
}

// printAll prints the jobs in order, reusing every stamped PDF, and
// returns the PDFs by slot and what the checks found on fresh prints.
func printAll(pr *press, jobs []printJob, fonts map[string][]byte, rule string, w *warnings) (map[string][]byte, []finding) {
	pdfs := map[string][]byte{}
	var found []finding

	for _, j := range jobs {
		fresh := pr.lookup(j.key) == nil
		pdf, err := pr.job(j.key, func(printer *release.Printer) ([]byte, error) {
			return printer.Print(j.doc(), fonts, false)
		})
		if err == nil && pdf != nil && fresh && j.checks != nil {
			printer, _ := pr.printerFor()
			found = append(found, j.checks(pdf, printer)...)
		}

		switch {
		case err != nil:
			w.add(j.file, rule, "The PDF of "+j.name+" could not be printed: "+err.Error(), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release'.")
		case pdf != nil:
			pdfs[j.slot] = pdf
		}
	}

	// Keep findings ordered by source and message.
	slices.SortFunc(found, func(a, b finding) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Message, b.Message))
	})
	return pdfs, found
}

// opinion is another app's view of the real files: LibreOffice for every
// user who has it, Word or PowerPoint for lfm's own tests.
type opinion struct {
	app        string                    // as the report names it
	run        func() ([]finding, error) // nil when the app is not there
	failHint   string                    // when the app itself failed
	differHint string                    // when it shows the files differently
}

// secondOpinions runs each app that is there and returns the report's
// words for each: "LibreOffice agrees", "2 findings in Word".
func secondOpinions(dir, rule string, apps []opinion, w *warnings) []string {
	var out []string
	for _, a := range apps {
		if a.run == nil {
			continue
		}
		cross, err := a.run()
		switch {
		case err != nil:
			w.add(dir, rule, a.app+" did not check the files: "+err.Error(), a.failHint)
		case len(cross) == 0:
			out = append(out, a.app+" agrees")
		default:
			for _, f := range cross {
				w.add(f.File, rule, f.Message, a.differHint)
			}
			out = append(out, term.Plural(len(cross), "finding")+" in "+a.app)
		}
	}
	return out
}

// libreOffice is the LibreOffice opinion when it is installed.
func libreOffice(check func(soffice string) ([]finding, error), differ string) opinion {
	o := opinion{app: "LibreOffice", failHint: "Run 'lfm compile --release' again; reinstall LibreOffice if it keeps failing.", differHint: differ}
	if soffice, err := office.Find(); err == nil {
		o.run = func() ([]finding, error) { return check(soffice) }
	}
	return o
}

// editionZip is a published edition's ZIP when every part of it is there
// and its key is unchanged, else the ZIP pack writes, stamped only when
// complete so a later compile can reuse it whole.
func editionZip(pr *press, key string, complete bool, pack func(key string) ([]byte, error)) ([]byte, error) {
	if !complete {
		key = ""
	}
	if key != "" {
		if data := pr.lookup(key); data != nil {
			return data, nil
		}
	}
	return pack(key)
}

// listingPlan is one listing image: its key, and the HTML to render it from.
type listingPlan struct {
	Name, Key string
	Doc       []byte
}

// listingImages materializes demanded plans in order, preserving individual hits.
// Dependencies are prepared only for missing images. Failed work stays pending.
func listingImages(pr *press, kind, sell string, planned []listingPlan, render func(*release.Printer, []listingPlan) ([]listingPlan, map[string][]byte, error)) []deliver.Image {
	if sell == deliver.SellBundle {
		planned = planned[:min(pr.demand[kind], len(planned))]
	}
	out := make([]deliver.Image, len(planned))
	indices := map[string]int{}
	var missing []listingPlan
	for i, plan := range planned {
		out[i] = deliver.Image{Name: plan.Name, Data: pr.lookup(plan.Key)}
		if out[i].Data == nil {
			missing = append(missing, plan)
			indices[plan.Name] = i
		}
	}
	if len(missing) > 0 && pr.release && render != nil {
		printer, err := pr.printerFor()
		if err == nil {
			plans, assets, err := render(printer, missing)
			if err == nil {
				docs := make([][]byte, len(plans))
				overhead := 0
				for i, plan := range plans {
					docs[i] = plan.Doc
					overhead = max(overhead, len(deliver.StampJPEG([]byte{0xff, 0xd8}, planned[indices[plan.Name]].Key))-2)
				}
				_ = printer.Pictures(docs, assets, imageWidth, imageHeight, imageScale, false, (1<<20)-overhead, func(i int, jpg []byte) {
					index := indices[plans[i].Name]
					key := planned[index].Key
					out[index].Data = deliver.StampJPEG(jpg, key)
					pr.previous[key] = out[index].Data
				})
			}
		}
	}
	absent := func(image deliver.Image) bool { return image.Data == nil }
	if slices.ContainsFunc(out, absent) {
		pr.wait()
	}
	return slices.DeleteFunc(out, absent)
}

// picturePlan is a source raster referenced by a listing layout. Markers are
// rendered through the same templates as PNGs, then replaced only on demand.
type picturePlan struct {
	marker, doc   []byte
	width, height int64
	scale         float64
}

func captureShots(printer *release.Printer, plans, missing []listingPlan, assets map[string][]byte, shots []picturePlan) ([]listingPlan, error) {
	plans = slices.DeleteFunc(plans, func(p listingPlan) bool {
		return !slices.ContainsFunc(missing, func(m listingPlan) bool { return m.Name == p.Name })
	})
	shots = slices.DeleteFunc(shots, func(s picturePlan) bool {
		s.marker = []byte("data:image/png;base64," + base64.StdEncoding.EncodeToString(s.marker))
		return !slices.ContainsFunc(plans, func(p listingPlan) bool { return bytes.Contains(p.Doc, s.marker) })
	})
	for len(shots) > 0 {
		shot := shots[0]
		n := 1
		for n < len(shots) && shots[n].width == shot.width && shots[n].height == shot.height && shots[n].scale == shot.scale {
			n++
		}
		docs := make([][]byte, n)
		for i := range docs {
			docs[i] = shots[i].doc
		}
		err := printer.Pictures(docs, assets, shot.width, shot.height, shot.scale, true, 0, func(i int, png []byte) {
			marker := []byte("data:image/png;base64," + base64.StdEncoding.EncodeToString(shots[i].marker))
			data := []byte("data:image/png;base64," + base64.StdEncoding.EncodeToString(png))
			for j := range plans {
				plans[j].Doc = bytes.ReplaceAll(plans[j].Doc, marker, data)
			}
		})
		if err != nil {
			return nil, err
		}
		shots = shots[n:]
	}
	return plans, nil
}

// Listing images are 1500 × 1125 CSS pixels at twice the density: the
// 3000 × 2250 Etsy and Gumroad show sharpest.
const imageWidth, imageHeight, imageScale = 1500, 1125, 2

// sale is how a deliverable reaches buyers in this compile.
type sale struct {
	page bool   // through the page's checkout and an unlisted download page
	skip bool   // sells through the page but has no valid path: nothing to publish
	root string // the folder its previews live under
}

// startSale decides how a deliverable sells and stops it when the page
// would leak its download path.
func startSale(sell, path, url string, page []byte, rule, kind string) (sale, []diagnostic.Diagnostic) {
	s := sale{root: kind}
	s.page = sell == deliver.SellPage && deliver.ValidPath(path) && url != ""
	if sell == deliver.SellPage && !s.page {
		s.skip = true
		return s, nil
	}
	if s.page {
		if leak := leaks(page, path, rule, kind); leak != nil {
			return s, leak
		}
		s.root = path
	}
	return s, nil
}

// open starts the press beside the previews of every edition, which lfm
// serve shows under the deliverable's folder.
func (s sale) open(st *stock, result *Result, editions []string, preview func(i int) []byte, assets map[string][]byte) *press {
	pr := st.press()
	if result.Previews == nil {
		result.Previews = map[string][]byte{}
	}
	for i, key := range editions {
		result.Previews[s.root+"/preview/"+key+".html"] = preview(i)
	}
	for name, data := range assets {
		result.Previews[s.root+"/preview/"+name] = data
	}
	return pr
}

// remembered is what LibreOffice found in files the last time it opened
// exactly these bytes with this lfm and this LibreOffice, or run's fresh
// result, kept for next time. Opening them takes seconds a release with
// unchanged files does not need. The verdicts live beside the release
// frames in the user cache; only successful runs are kept.
func remembered[T any](soffice, kind string, files [][]byte, run func() (T, error)) (T, error) {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", kind, buildid.Writer(), soffice)
	if real, err := filepath.EvalSymlinks(soffice); err == nil {
		if info, err := os.Stat(real); err == nil {
			fmt.Fprintf(h, "%s\x00%d\x00%d\x00", real, info.Size(), info.ModTime().UnixNano())
		}
	}
	for _, f := range files {
		fmt.Fprintf(h, "%d\x00", len(f))
		h.Write(f)
	}
	path := filepath.Join(cache.Dir("libreoffice"), hex.EncodeToString(h.Sum(nil))[:32]+".json")
	cache.Touch(path)
	var out T
	if raw, err := os.ReadFile(path); err == nil && json.Unmarshal(raw, &out) == nil {
		return out, nil
	}
	out, err := run()
	if err != nil {
		return out, err
	}
	if raw, err := json.Marshal(out); err == nil && os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_ = scratch.WriteFile(path, raw)
	}
	return out, nil
}

func builtData[B, T any](built []B, data func(B) T) []T {
	out := make([]T, len(built))
	for i, b := range built {
		out[i] = data(b)
	}
	return out
}
