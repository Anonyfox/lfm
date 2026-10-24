package compile

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/buildid"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/release"
)

func TestDocumentsAreBuiltCheckedAndDelivered(t *testing.T) {
	t.Parallel()
	dir := documentProject(t)
	t.Cleanup(func() { strictArchives(t, dir) })
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v\n%v", err, findings(result, ""))
	}
	if got := findings(result, "document."); len(got) > 0 {
		t.Fatalf("findings: %v", got)
	}
	d := result.Document
	if d == nil || d.Documents != 3 || len(d.Editions) != 2 || len(d.Files) != 2 || !d.Pending {
		t.Fatalf("summary %+v", d)
	}
	out := filepath.Join(dir, "output", docPath)
	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !bytes.Contains(page, []byte("allotment-society-kit-en-letter.zip")) {
		t.Fatalf("download page lists no ZIP:\n%s", page)
	}
	if _, ok := result.Previews[docPath+"/preview/en-a4.html"]; !ok {
		t.Fatal("no preview")
	}
	first, _ := os.ReadFile(filepath.Join(out, "allotment-society-kit-en-a4.zip"))
	if _, err := Compile(context.Background(), dir, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(out, "allotment-society-kit-en-a4.zip"))
	if !bytes.Equal(first, second) {
		t.Fatal("an unchanged set rebuilds different bytes")
	}
	if _, err := office.Find(); err != nil || testing.Short() {
		t.Skip("no LibreOffice")
	}
	result, err = Compile(context.Background(), dir, Options{Publish: true, Release: true})
	if err != nil || result.Document.Pending || result.Document.Images < 4 || !strings.HasPrefix(result.Document.Layout, "6 files print as planned") {
		t.Fatalf("release: %v %+v %v", err, result.Document, findings(result, "document."))
	}
	z, err := zip.OpenReader(filepath.Join(out, "allotment-society-kit-en-a4.zip"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	z.Close()
	for _, want := range []string{"Plot Fee Invoice.docx", "PDF/Plot Fee Invoice.pdf", "Start here.pdf", "Fonts/Lora-Regular.ttf", "Fonts/OFL.txt"} {
		if !strings.Contains(strings.Join(names, "|"), want) {
			t.Errorf("the ZIP holds no %s: %v", want, names)
		}
	}
	reused, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil || reused.Document.Pending || reused.Document.Images != result.Document.Images {
		t.Fatalf("prints not reused: %v %+v", err, reused.Document)
	}
}

func TestOptionalHelpersNameWhatIsMissing(t *testing.T) {
	t.Setenv("LFM_SOFFICE", "none")
	got := optionalHelpers([]string{".docx"})
	if len(got) != 1 || !strings.HasPrefix(got[0], "LibreOffice") {
		t.Fatalf("optional = %q", got)
	}
	if got := optionalHelpers(nil); len(got) != 0 {
		t.Fatalf("a page without files needs no LibreOffice: %q", got)
	}
}

// TestANewerLfmRebuildsFilesButKeepsItsPrints compiles a released set as a
// newer lfm would: the documents are written again by the new writer, the
// PDFs whose layout did not change are reused, so the published ZIPs keep
// them without a release.
func TestANewerLfmRebuildsFilesButKeepsItsPrints(t *testing.T) {
	if _, err := release.BrowserPath(); err != nil || testing.Short() {
		t.Skip("needs Chrome")
	}
	dir := documentProject(t)
	if _, err := Compile(context.Background(), dir, Options{Publish: true, Release: true}); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "output", docPath, "allotment-society-kit-en-a4.zip")
	before, _ := os.ReadFile(zipPath)
	was := buildid.Writer
	buildid.Writer = func() string { return "a newer lfm" }
	defer func() { buildid.Writer = was }()
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	after, _ := os.ReadFile(zipPath)
	if err != nil || result.Document.Pending {
		zz, _ := zip.NewReader(bytes.NewReader(after), int64(len(after)))
		var names []string
		for _, f := range zz.File {
			names = append(names, f.Name)
		}
		imgs, _ := os.ReadDir(filepath.Join(dir, "distribution", "document", "images"))
		t.Fatalf("%v %+v\nzip: %v\nimages: %d", err, result.Document, names, len(imgs))
	}
	if bytes.Equal(before, after) {
		t.Fatal("the newer lfm reused the old files")
	}
	z, err := zip.NewReader(bytes.NewReader(after), int64(len(after)))
	if err != nil {
		t.Fatal(err)
	}
	pdfs := 0
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "PDF/") {
			pdfs++
		}
	}
	if pdfs != 3 {
		t.Fatalf("the ZIP lost its prints: %d PDFs", pdfs)
	}
}
