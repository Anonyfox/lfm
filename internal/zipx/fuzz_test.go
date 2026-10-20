package zipx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"
)

// Property: reading an archive's rules and stamp never panics on arbitrary
// bytes.
func FuzzProblems(f *testing.F) {
	var buf bytes.Buffer
	w := New(&buf, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	w.Store("mimetype", []byte("application/epub+zip"))
	w.Deflate("Grüße.txt", []byte("hi"))
	w.Close()
	f.Add(buf.Bytes())
	f.Add(WithStamp(buf.Bytes(), "abc"))
	f.Add([]byte("PK\x05\x06"))
	f.Fuzz(func(t *testing.T, data []byte) {
		Problems(data)
		Stamp(data)
		WithStamp(data, "k")
	})
}

// Property: a key stamped on an archive this package writes reads back, and
// the archive still opens with its entries and no problems.
func FuzzStampRoundTrip(f *testing.F) {
	f.Add("abc123", "a.txt", []byte("hi"))
	f.Add("", "mimetype", []byte{})
	f.Add("ключ", "ü/ß.xml", []byte("<x/>"))
	f.Add(strings.Repeat("k", 300), "a", []byte{}) // a comment past the window Stamp reads
	f.Fuzz(func(t *testing.T, key, name string, data []byte) {
		if name == "" {
			t.Skip()
		}
		var buf bytes.Buffer
		w := New(&buf, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		write := w.Deflate
		if name == "mimetype" {
			write = w.Store
		}
		if err := write(name, data); err != nil {
			t.Skip()
		}
		if err := w.Close(); err != nil {
			t.Skip()
		}
		stamped := WithStamp(buf.Bytes(), key)
		if len(StampPrefix+key) > stampWindow {
			if !bytes.Equal(stamped, buf.Bytes()) {
				t.Fatalf("stamped a key of %d bytes Stamp cannot read back", len(key))
			}
			return
		}
		if got := Stamp(stamped); got != key {
			t.Fatalf("stamped %q, read %q", key, got)
		}
		if p := Problems(stamped); len(p) > 0 {
			t.Fatalf("problems %q", p)
		}
		z, err := zip.NewReader(bytes.NewReader(stamped), int64(len(stamped)))
		if err != nil || len(z.File) != 1 || z.File[0].Name != name {
			t.Fatalf("stamped archive does not read back: %v", err)
		}
	})
}
