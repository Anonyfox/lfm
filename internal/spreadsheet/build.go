package spreadsheet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Anonyfox/lfm/internal/buildid"
)

// Built is one file lfm built for an edition.
type Built struct {
	Name    string
	Edition Edition
	Dataset string
	Data    []byte
}

// created is the fixed date every file carries, so a rebuild of an
// unchanged workbook is byte for byte the same.
func (r Report) created() string {
	if r.Workbook.Today != "" {
		return r.Workbook.Today + "T00:00:00Z"
	}
	return "2026-01-01T00:00:00Z"
}

// key identifies a file by everything that goes into it: the workbook, the
// look, the edition and the dataset.
func (r Report) key(edition, dataset string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00", buildid.Writer(), r.Plan.Accent, edition, dataset, r.created())
	if err := json.NewEncoder(h).Encode(r.Workbook); err != nil {
		return "" // unstamped, so never reused
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// Build writes the .xlsx files: per edition the blank template and, when the
// workbook has an example, the filled example. Files build side by side;
// previous returns a file built from the same inputs before, stamped with
// its key, which is used as is.
func (r Report) Build(previous func(key string) []byte) ([]Built, error) {
	var jobs []Built
	for _, ed := range r.Editions {
		jobs = append(jobs, Built{Name: r.FileName(ed.Edition, false), Edition: ed.Edition, Dataset: Empty})
		if _, ok := r.Workbook.Datasets[Example]; ok {
			jobs = append(jobs, Built{Name: r.FileName(ed.Edition, true), Edition: ed.Edition, Dataset: Example})
		}
	}
	pal := palette(r.Plan.Accent)
	errs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i := range jobs {
		wg.Go(func() {
			ed := r.edition(jobs[i].Edition.Key)
			key := r.key(ed.Key, jobs[i].Dataset)
			if data := previous(key); key != "" && data != nil {
				jobs[i].Data = data
				return
			}
			jobs[i].Data, errs[i] = writeXLSX(ed, r.Books[ed.Key][jobs[i].Dataset], pal, r.created(), key)
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("%s: %w", jobs[i].Name, err)
		}
	}
	return jobs, nil
}

func (r Report) edition(key string) *edition {
	for _, ed := range r.Editions {
		if ed.Key == key {
			return ed
		}
	}
	return nil
}

// Preview is an edition as one HTML page with its tabs, computed with the
// example (or the blank template), for lfm serve.
func (r Report) Preview(key string) []byte {
	ed := r.edition(key)
	b := r.Books[key][Example]
	if b == nil {
		b = r.Books[key][Empty]
	}
	return previewHTML(ed, b, palette(r.Plan.Accent), r.Plan.Name)
}

// Compat recalculates the built files in LibreOffice and returns where its
// values differ from lfm's.
func (r Report) Compat(office string, files []Built) ([]string, error) {
	data := map[string][]byte{}
	for _, f := range files {
		data[f.Name] = f.Data
	}
	got, err := Recalculate(office, data)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		for _, d := range compare(r.edition(f.Edition.Key), r.Books[f.Edition.Key][f.Dataset], got[f.Name], "LibreOffice") {
			out = append(out, f.Name+": "+d)
		}
	}
	return out, nil
}

// StartDocs are the start PDFs to print, by file name.
func (r Report) StartDocs() map[string][]byte {
	out := map[string][]byte{}
	for _, ed := range r.Editions {
		out[r.StartFileName(ed.Edition)] = r.StartDoc(ed)
	}
	return out
}
