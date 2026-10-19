package zipx

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
	"time"
)

func build(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := New(&buf, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := w.Store("mimetype", []byte("application/epub+zip")); err != nil {
		t.Fatal(err)
	}
	w.Deflate("OEBPS/chapter.xhtml", bytes.Repeat([]byte("<p>Sauerteig</p>"), 200))
	w.Store("Schriften/Lora-Regular.ttf", []byte{0, 1, 0, 0})
	w.Raw("Brandschutzunterweisung – Sprechernotizen.pdf", Pack([]byte("%PDF-1.7"), 1))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestArchivesAreStrictAndReadable(t *testing.T) {
	data := build(t)
	if p := Problems(data); len(p) > 0 {
		t.Fatalf("problems: %v", p)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mimetype", "OEBPS/chapter.xhtml", "Schriften/Lora-Regular.ttf", "Brandschutzunterweisung – Sprechernotizen.pdf"}
	for i, f := range z.File {
		if f.Name != want[i] {
			t.Errorf("entry %d is %q", i, f.Name)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(f.Name, err)
		}
		if _, err := io.ReadAll(r); err != nil {
			t.Errorf("%s does not read back: %v", f.Name, err)
		}
		if !f.Modified.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("%s modified %v", f.Name, f.Modified)
		}
	}
	if !bytes.Equal(data, build(t)) {
		t.Error("the same entries give other bytes")
	}
}

func TestProblemsSeeWhatArchiveZipWritesByDefault(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store, Modified: time.Now()})
	w.Write([]byte("application/epub+zip"))
	w, _ = z.CreateHeader(&zip.FileHeader{Name: "Grüße.txt", Method: zip.Deflate, NonUTF8: true})
	w.Write([]byte("hi"))
	z.Close()
	p := Problems(buf.Bytes())
	if len(p) != 5 {
		t.Fatalf("problems %q", p)
	}
}

func TestStampsRoundTrip(t *testing.T) {
	data := WithStamp(build(t), "abc123")
	if Stamp(data) != "abc123" || Stamp(data[len(data)-60:]) != "abc123" {
		t.Fatal("stamp lost")
	}
	if Stamp(build(t)) != "" {
		t.Fatal("stamp without one")
	}
	if _, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkDeflateSmallParts(b *testing.B) {
	part := bytes.Repeat([]byte(`<w:p><w:r><w:t>Plot fee invoice</w:t></w:r></w:p>`), 40)
	for b.Loop() {
		var buf bytes.Buffer
		w := New(&buf, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		for range 30 {
			w.Deflate("word/document.xml", part)
		}
		w.Close()
	}
}
