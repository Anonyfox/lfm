package compile

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/scaffold"
)

const docPath = "k3n8q2w7x5r9t4m6p1"

func documentProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"document": map[string]any{"path": docPath, "version": "2027.1"}}
		c["seller"] = map[string]any{"name": "Riverside Press"}
	})
	src := filepath.Join("..", "document", "testdata", "club")
	dst := filepath.Join(dir, filepath.FromSlash(document.Dir))
	filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		os.MkdirAll(filepath.Join(dst, filepath.Dir(rel)), 0o755)
		data, _ := os.ReadFile(path)
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	os.WriteFile(filepath.Join(dst, "listing.md"), []byte("# Listing: Allotment Society Kit\n\n## Title\nAllotment Society Templates for Word and Google Docs, Minutes, Invoice, Fair Program\n\n## Description\n"+strings.Repeat("Run your community garden's paperwork in one evening: minutes, plot fee invoices and the harvest fair program, ready to fill. ", 3)+"\n\n## Tags\n- allotment templates\n\n## Price\n9.90 USD\n\n## AI\nText: assisted\nImages: none\nTools: Claude\nStatement: Texts edited with Claude; layout and checks by software.\n"), 0o644)
	return dir
}

func TestDocumentsAreBuiltCheckedAndDelivered(t *testing.T) {
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
	got := optionalHelpers([]string{".docx"}, &release.AuditResult{Lighthouse: &release.LighthouseResult{Missing: true}})
	if len(got) != 2 || !strings.HasPrefix(got[0], "LibreOffice") || !strings.HasPrefix(got[1], "Lighthouse") {
		t.Fatalf("optional = %q", got)
	}
	if got := optionalHelpers(nil, nil); len(got) != 0 {
		t.Fatalf("a page without files needs no LibreOffice: %q", got)
	}
}
