package document

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/ooxml"
)

// docxFormat names how lfm writes documents; changing the writer changes it,
// so files an older lfm wrote are written again.
const docxFormat = "lfm-docx-6"

// created is the date every file carries, so a rebuild is byte-identical.
const created = "2026-01-01"

// Built is one .docx of one edition.
type Built struct {
	Edition Edition
	Doc     *Doc
	Name    string // the file name buyers see: the document's title
	Key     string // the input key it is stamped with
	Data    []byte
}

// Stamp is the input key a built .docx or .zip carries in its zip comment.
func Stamp(tail []byte) string { return ooxml.Stamp(tail) }

var withStamp = ooxml.WithStamp

// FileName is a document's file name: its title, safe on every system.
func FileName(title, ext string) string { return ooxml.FileName(title, ext) }

// inputs hashes what every file of the set shares: the theme and the
// faces and images it embeds.
func (r Report) inputs() []byte {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%+v\x00", docxFormat, r.Theme)
	for _, f := range []*family{r.text, r.head} {
		if f == nil {
			continue
		}
		for _, slot := range sortedKeys(f.Faces) {
			fmt.Fprintf(h, "%s/%s:", f.Name, slot)
			h.Write(f.Faces[slot].Data)
		}
	}
	for _, name := range sortedKeys(r.licences) {
		fmt.Fprintf(h, "%s:", name)
		h.Write(r.licences[name])
	}
	for _, src := range sortedKeys(r.images) {
		fmt.Fprintf(h, "%s:", src)
		h.Write(r.images[src].Data)
	}
	return h.Sum(nil)
}

// Digest is the hash of what every file of the set shares: the theme, the
// fonts and their licences, and the images.
func (r Report) Digest() []byte {
	if r.digest != nil {
		return r.digest
	}
	return r.inputs()
}

// render lays out a document for an edition.
func (r Report) render(d *Doc, e Edition) render {
	return render{doc: d, m: layoutTheme(r.Theme, Papers[e.Paper], r.text, r.head), images: r.images, created: created, embed: true, pack: r.pack, seller: r.Config.Seller, product: r.Plan.Name}
}

// Jobs are every document of every edition with the key of its inputs,
// not yet built.
func (r Report) Jobs() []Built {
	shared := r.Digest()
	var jobs []Built
	for _, e := range r.Editions {
		for _, d := range r.Docs[e.Lang] {
			src, _ := json.Marshal(d)
			h := sha256.New()
			h.Write(shared)
			fmt.Fprintf(h, "\x00%s\x00", e.Paper)
			h.Write(src)
			jobs = append(jobs, Built{Edition: e, Doc: d, Name: FileName(d.Title, "docx"), Key: hex.EncodeToString(h.Sum(nil))[:32]})
		}
	}
	return jobs
}

// Build writes every document of every edition. previous returns a file
// built from the same inputs before, which is used as is; the rest build
// side by side.
func (r Report) Build(previous func(key string) []byte) ([]Built, error) {
	jobs := r.Jobs()
	errs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i := range jobs {
		wg.Go(func() {
			if data := previous(jobs[i].Key); data != nil {
				jobs[i].Data = data
				return
			}
			data, err := docx(r.render(jobs[i].Doc, jobs[i].Edition))
			jobs[i].Data, errs[i] = withStamp(data, jobs[i].Key), err
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("%s (%s): %w", jobs[i].Doc.File, jobs[i].Edition.Key, err)
		}
	}
	return jobs, nil
}

// Preview is an edition's documents as one HTML page for lfm serve.
func (r Report) Preview(e Edition) []byte {
	var rs []render
	for _, d := range r.Docs[e.Lang] {
		rs = append(rs, r.render(d, e))
	}
	return previewHTML(r.Plan.Name, rs)
}

// PreviewAssets are the fonts and images a preview loads, by path below
// its folder.
func (r Report) PreviewAssets() map[string][]byte {
	out := map[string][]byte{}
	for _, f := range []*family{r.text, r.head} {
		if f == nil {
			continue
		}
		for _, face := range f.Faces {
			out["fonts/"+baseName(face.File)] = face.Data
		}
	}
	for src, p := range r.images {
		out[src] = p.Data
	}
	return out
}

// ZipName is an edition's download.
func (r Report) ZipName(e Edition) string { return r.Slug() + "-" + e.Key + ".zip" }

// Zip packs an edition: its documents, their PDFs when printed, the start
// guide, and the fonts with their licence, so Pages users can install them.
// A key stamps a complete ZIP, so later compiles reuse it as is.
func (r Report) Zip(e Edition, built []Built, pdfs map[string][]byte, guide []byte, key string) ([]byte, error) {
	w := wording(e.Lang)
	files := map[string][]byte{}
	for _, b := range built {
		if b.Edition != e {
			continue
		}
		files[b.Name] = b.Data
		if pdf := pdfs[b.Key]; pdf != nil {
			files[w.PDFFolder+"/"+FileName(b.Doc.Title, "pdf")] = pdf
		}
	}
	if guide != nil {
		files[FileName(w.GuideTitle, "pdf")] = guide
	}
	for _, f := range []*family{r.text, r.head} {
		if f == nil {
			continue
		}
		for _, face := range f.Faces {
			files[w.FontsFolder+"/"+baseName(face.File)] = face.Data
		}
	}
	for name, data := range r.licences {
		files[w.FontsFolder+"/"+name] = data
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	modified, _ := time.Parse("2006-01-02", created)
	for _, name := range sortedKeys(files) {
		// .docx and .pdf are compressed already; fonts are not worth the time.
		if strings.HasSuffix(name, ".txt") {
			fw, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified})
			if err != nil {
				return nil, err
			}
			if _, err := fw.Write(files[name]); err != nil {
				return nil, err
			}
			continue
		}
		if err := ooxml.Store(z, name, files[name], modified); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	if key != "" {
		return withStamp(buf.Bytes(), key), nil
	}
	return buf.Bytes(), nil
}
