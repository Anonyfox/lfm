package document

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/pdfx"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// Finding is a release finding about one source file.
type Finding struct{ File, Message string }

var (
	baseFont = regexp.MustCompile(`/BaseFont\s*/(?:[A-Z]{6}\+)?([A-Za-z0-9_-]+)`)
	mediaBox = regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([\d.]+)\s+([\d.]+)\s*\]`)
)

// PrintFormat names the print layout; changing printHTML changes it, so
// PDFs an older lfm printed are printed again.
const PrintFormat = "lfm-print-1"

// PrintDoc is a built document's print layout for Chrome.
func (r Report) PrintDoc(b Built) []byte { return printHTML(r.render(b.Doc, b.Edition)) }

// ShotDoc is a built document's first page for a listing image.
func (r Report) ShotDoc(b Built) []byte { return shotHTML(r.render(b.Doc, b.Edition)) }

// PagePixels is an edition's paper in CSS pixels, for a first-page shot.
func PagePixels(e Edition) (int64, int64) { return pagePixels(Papers[e.Paper]) }

// Inspect holds Chrome's print of a document to its plan: pages, paper and
// the theme's fonts.
func (r Report) Inspect(b Built, pdf []byte) []Finding { return r.inspect(b, pdf, "Chrome's print") }

// Verify opens every built file in LibreOffice, one run for all, and holds
// its print to the plan as Inspect does Chrome's: another app's view of
// the real .docx.
func (r Report) Verify(soffice string, built []Built) ([]Finding, error) {
	files := map[string][]byte{}
	for _, b := range built {
		files[b.Key+".docx"] = b.Data
	}
	pdfs, err := office.Convert(soffice, files, "pdf")
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, b := range built {
		pdf := pdfs[b.Key+".pdf"]
		if pdf == nil {
			findings = append(findings, Finding{b.Doc.File, fmt.Sprintf("LibreOffice could not print %s (%s).", b.Name, b.Edition.Key)})
			continue
		}
		findings = append(findings, r.inspect(b, pdf, "LibreOffice")...)
	}
	return findings, nil
}

// Word opens every built file in Microsoft Word for Mac and holds Word's
// own PDF of it to the plan, as Verify does LibreOffice's: a file Word has
// to repair, other pages, another paper, other fonts.
func (r Report) Word(built []Built) ([]Finding, error) {
	files := map[string][]byte{}
	for _, b := range built {
		files[b.Key] = b.Data
	}
	opened, err := office.OpenInWord(files)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, b := range built {
		o := opened[b.Key]
		if o.Error != "" {
			out = append(out, Finding{b.Doc.File, fmt.Sprintf("%s (%s): %s.", b.Doc.Title, b.Edition.Key, o.Error)})
			continue
		}
		out = append(out, r.inspect(b, o.Data, "Word")...)
	}
	return out, nil
}

// inspect holds one printed document to its plan.
func (r Report) inspect(b Built, pdf []byte, app string) []Finding {
	var out []Finding
	add := func(format string, args ...any) {
		out = append(out, Finding{b.Doc.File, fmt.Sprintf("%s (%s): ", b.Doc.Title, b.Edition.Key) + fmt.Sprintf(format, args...)})
	}
	if pages := pdfx.PageCount(pdf); b.Doc.Pages > 0 && pages != b.Doc.Pages {
		add("prints on %d pages; its front matter plans %d.", pages, b.Doc.Pages)
	}
	if m := mediaBox.FindSubmatch(pdf); m != nil {
		w, _ := strconv.ParseFloat(string(m[1]), 64)
		h, _ := strconv.ParseFloat(string(m[2]), 64)
		p := Papers[b.Edition.Paper]
		if math.Abs(w-p.Width*72/25.4) > 2 || math.Abs(h-p.Height*72/25.4) > 2 {
			add("prints on %.0f × %.0f mm, not %s.", w*25.4/72, h*25.4/72, paperNames[b.Edition.Paper])
		}
	}
	allowed := []string{}
	for _, f := range []*family{r.text, r.head} {
		if f != nil {
			allowed = append(allowed, strings.ReplaceAll(f.Name, " ", ""))
		}
	}
	if _, _, font := typeface.Checkboxes(r.text); font == "Arial" || app == "Word" {
		// The checkboxes' fallback; Word also sets the gap after a list
		// marker in Arial, which draws nothing.
		allowed = append(allowed, "Arial", "LiberationSans")
	}
	seen := map[string]bool{}
	for _, m := range baseFont.FindAllSubmatch(pdf, -1) {
		name := string(m[1])
		ok := false
		for _, a := range allowed {
			if strings.HasPrefix(name, a) {
				ok = true
			}
		}
		if !ok && !seen[name] {
			seen[name] = true
			add("%s sets text in %s, so a character is missing from the theme's fonts or a font was not embedded.", app, name)
		}
	}
	return out
}
