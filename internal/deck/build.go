package deck

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/buildid"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/typeface"
	"github.com/Anonyfox/lfm/internal/zipx"
)

// created is the date every file carries, so a rebuild is byte-identical.
const created = "2026-01-01"

// render is one deck laid out: what the .pptx and its prints need.
type render struct {
	deck                     *Deck
	k                        design
	paper                    Paper // the notes page
	images                   map[string]*picture
	pack                     *typeface.Pack
	embed, inline            bool
	created, seller, product string
}

func (r Report) render(d *Deck, paper string) render {
	return render{deck: d, k: r.design(), paper: Papers[paper], images: r.images, pack: r.pack, embed: true, created: created, seller: r.Config.Seller, product: r.Plan.Name}
}

// Built is one .pptx of one edition.
type Built struct {
	Edition Edition
	Deck    *Deck
	Name    string // the file name buyers see: the deck's title
	Key     string // the input key it is stamped with
	Data    []byte
}

// Stamp is the input key a built .pptx or .zip carries in its zip comment.
func Stamp(tail []byte) string { return zipx.Stamp(tail) }

// inputs hashes what every file of the set shares: the theme and the
// faces and images it embeds.
func (r Report) inputs() []byte {
	h := sha256.New()
	fmt.Fprintf(h, "%+v\x00", r.Theme)
	for _, f := range []*typeface.Family{r.text, r.head} {
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

// Digest is the hash of what every file of the set shares.
func (r Report) Digest() []byte {
	if r.digest != nil {
		return r.digest
	}
	return r.inputs()
}

// Jobs are every deck of every edition with the key of its inputs, not
// yet built.
func (r Report) Jobs() []Built {
	shared := r.Digest()
	var jobs []Built
	for _, e := range r.Editions {
		for _, d := range r.Decks[e.Lang] {
			src, _ := json.Marshal(d.Blocks)
			h := sha256.New()
			// The binary's identity: another lfm may write the same input
			// otherwise. Prints key on what they print instead (Digest).
			fmt.Fprintf(h, "%s\x00", buildid.Writer())
			h.Write(shared)
			fmt.Fprintf(h, "\x00%s\x00%s\x00%d\x00%s\x00", d.Title, e.Papers[0], d.Minutes, d.Lang)
			h.Write(src)
			jobs = append(jobs, Built{Edition: e, Deck: d, Name: deliver.FileName(d.Title, "pptx"), Key: hex.EncodeToString(h.Sum(nil))[:32]})
		}
	}
	return jobs
}

// Build writes every deck of every edition. previous returns a file built
// from the same inputs before, which is used as is; the rest build side by
// side.
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
			// The notes pages take the edition's first paper.
			data, err := pptx(r.render(jobs[i].Deck, jobs[i].Edition.Papers[0]))
			jobs[i].Data, errs[i] = zipx.WithStamp(data, jobs[i].Key), err
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("%s (%s): %w", jobs[i].Deck.File, jobs[i].Edition.Key, err)
		}
	}
	return jobs, nil
}

// Print names one PDF of a built deck: its slides, or its notes or quiz
// sheet on a paper.
type Print struct {
	Kind  string // slides, notes, quiz
	Paper string // for notes and quiz
}

// Prints are the PDFs a built deck ships: the slides, the notes on each
// paper, and a quiz sheet on each paper when the deck asks questions.
func (b Built) Prints() []Print {
	out := []Print{{Kind: "slides"}}
	for _, p := range b.Edition.Papers {
		out = append(out, Print{Kind: "notes", Paper: p})
	}
	if quizzes(b.Deck) > 0 {
		for _, p := range b.Edition.Papers {
			out = append(out, Print{Kind: "quiz", Paper: p})
		}
	}
	return out
}

// Key is the key of one print of a built deck.
func (b Built) PrintKey(p Print) string { return b.Key + "/" + p.Kind + "/" + p.Paper }

// PrintDoc is the HTML Chrome prints for one PDF of a built deck.
func (r Report) PrintDoc(b Built, p Print) []byte {
	switch p.Kind {
	case "notes":
		return notesPrint(r.render(b.Deck, p.Paper))
	case "quiz":
		return quizPrint(r.render(b.Deck, p.Paper))
	}
	return slidesPrint(r.render(b.Deck, b.Edition.Papers[0]))
}

// pdfName is the file name of one print of a deck in the ZIP.
func (b Built) pdfName(p Print) string {
	w := wording(b.Edition.Lang)
	paper := ""
	if len(b.Edition.Papers) > 1 {
		paper = ", " + paperNames[p.Paper]
	}
	switch p.Kind {
	case "notes":
		return deliver.FileName(b.Deck.Title+" ("+w.Notes+paper+")", "pdf")
	case "quiz":
		return deliver.FileName(b.Deck.Title+" ("+w.Quiz+paper+")", "pdf")
	}
	return deliver.FileName(b.Deck.Title, "pdf")
}

func quizzes(d *Deck) int {
	n := 0
	for _, s := range d.Slides {
		if s.Quiz != nil {
			n++
		}
	}
	return n
}

// ZipName is an edition's download.
func (r Report) ZipName(e Edition) string { return r.Slug() + "-" + e.Key + ".zip" }

// Zip packs an edition: its decks, their PDFs when printed, the start
// guide, and the fonts with their licence, so Keynote users can install
// them. A key stamps a complete ZIP, so later compiles reuse it as is.
func (r Report) Zip(e Edition, built []Built, pdfs map[string][]byte, guide []byte, key string) ([]byte, error) {
	w := wording(e.Lang)
	files := map[string][]byte{}
	for _, b := range built {
		if b.Edition.Key != e.Key {
			continue
		}
		files[b.Name] = b.Data
		for _, p := range b.Prints() {
			if pdf := pdfs[b.PrintKey(p)]; pdf != nil {
				files[w.PDFFolder+"/"+b.pdfName(p)] = pdf
			}
		}
	}
	if guide != nil {
		files[deliver.FileName(w.GuideTitle, "pdf")] = guide
	}
	for _, f := range []*typeface.Family{r.text, r.head} {
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
	modified, _ := time.Parse("2006-01-02", created)
	z := zipx.New(&buf, modified)
	for _, name := range sortedKeys(files) {
		// .pptx and .pdf are compressed already; fonts are not worth the time.
		write := z.Store
		if strings.HasSuffix(name, ".txt") {
			write = z.Deflate
		}
		if err := write(name, files[name]); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	if key != "" {
		return zipx.WithStamp(buf.Bytes(), key), nil
	}
	return buf.Bytes(), nil
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
