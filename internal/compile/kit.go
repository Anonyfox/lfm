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

// printGuides prints every start guide in order, reusing stamped ones.
func printGuides(pr *press, keys []string, docs [][]byte, names []string, fonts map[string][]byte, dir, rule string, w *warnings) [][]byte {
	jobs := make([]printJob, len(docs))
	for i := range docs {
		jobs[i] = printJob{key: keys[i], slot: keys[i], file: dir, name: "start guide " + names[i], doc: func() []byte { return docs[i] }}
	}
	pdfs, _ := printAll(pr, jobs, fonts, rule, w)
	out := make([][]byte, len(docs))
	for i := range out {
		out[i] = pdfs[keys[i]]
	}
	return out
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
	data := make(map[string][]byte, len(planned))
	keys := map[string]string{}
	var missing []listingPlan
	for _, plan := range planned {
		if jpg := pr.lookup(plan.Key); jpg != nil {
			data[plan.Name] = jpg
		} else {
			missing = append(missing, plan)
			keys[plan.Name] = plan.Key
		}
	}
	if len(missing) > 0 {
		if pr.release && render != nil {
			if printer, err := pr.printerFor(); err == nil {
				plans, assets, err := render(printer, missing)
				if err == nil {
					for _, plan := range plans {
						key := keys[plan.Name]
						if key == "" {
							continue
						}
						jpg, err := pr.job(key, func(printer *release.Printer) ([]byte, error) {
							png, err := printer.Picture(plan.Doc, assets, imageWidth, imageHeight, imageScale)
							if err != nil {
								return nil, err
							}
							return jpegUnder(png, 1<<20, key)
						})
						if err == nil && jpg != nil {
							data[plan.Name] = jpg
						}
					}
				}
			}
		}
		if len(data) != len(planned) {
			pr.wait()
		}
	}
	var out []deliver.Image
	for _, plan := range planned {
		if jpg := data[plan.Name]; jpg != nil {
			out = append(out, deliver.Image{Name: plan.Name, Data: jpg})
		}
	}
	return out
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
	for _, shot := range shots {
		marker := []byte("data:image/png;base64," + base64.StdEncoding.EncodeToString(shot.marker))
		if !slices.ContainsFunc(plans, func(p listingPlan) bool { return bytes.Contains(p.Doc, marker) }) {
			continue
		}
		png, err := printer.Picture(shot.doc, assets, shot.width, shot.height, shot.scale)
		if err != nil {
			return nil, err
		}
		data := []byte("data:image/png;base64," + base64.StdEncoding.EncodeToString(png))
		for i := range plans {
			plans[i].Doc = bytes.ReplaceAll(plans[i].Doc, marker, data)
		}
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
	cache.Prune(filepath.Dir(path))
	var out T
	if raw, err := os.ReadFile(path); err == nil && json.Unmarshal(raw, &out) == nil {
		return out, nil
	}
	out, err := run()
	if err != nil {
		return out, err
	}
	if raw, err := json.Marshal(out); err == nil && os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		// A rename makes the verdict appear whole to a parallel release.
		if tmp, err := scratch.File(filepath.Dir(path), ".verdict-*"); err == nil {
			_, werr := tmp.Write(raw)
			if tmp.Close() == nil && werr == nil {
				os.Rename(tmp.Name(), path)
			}
			scratch.Remove(tmp.Name())
		}
	}
	cache.Prune(filepath.Dir(path))
	return out, nil
}

func builtData[B any](built []B, data func(B) []byte) [][]byte {
	out := make([][]byte, len(built))
	for i, b := range built {
		out[i] = data(b)
	}
	return out
}
