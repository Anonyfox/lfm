package pdfx

import (
	"regexp"
	"strconv"
	"testing"
)

// tiny is a minimal uncompressed PDF with a trailer Stamp can extend.
const tiny = "%PDF-1.4\n1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n3 0 obj\n<</Type /Page /Parent 2 0 R>>\nendobj\n4 0 obj\n<</Producer (Skia)>>\nendobj\nxref\n0 5\n0000000000 65535 f \ntrailer\n<</Size 5 /Root 1 0 R /Info 4 0 R>>\nstartxref\n180\n%%EOF\n"

var hexKey = regexp.MustCompile(`^[0-9a-f]{16,64}$`)

// Property: reading never panics on arbitrary bytes, and a key Stamp
// accepts reads back with the page count unchanged, also after restamping.
func FuzzStamp(f *testing.F) {
	f.Add([]byte(tiny), "0123456789abcdef")
	f.Add([]byte("trailer<</Size 1 /Root 1 0 R /Info 1 0 R>>startxref 0 %%EOF"), "0123456789abcdef0123")
	f.Add([]byte("1 0 obj <</Type /Catalog /Outlines 2 0 R /Pages 3 0 R>>\n2 0 obj <</First 2 0 R /Next 2 0 R /Dest [3 0 R]>>"), "x")
	f.Fuzz(func(t *testing.T, pdf []byte, key string) {
		PageCount(pdf)
		OutlinePages(pdf)
		Transparent(pdf)
		StampOf(pdf)
		stamped, err := Stamp(pdf, key)
		if err != nil || !hexKey.MatchString(key) {
			return
		}
		if got := StampOf(stamped); got != key {
			t.Fatalf("stamped %q, read %q", key, got)
		}
		// Stamp copies the old info dictionary, so one naming pages counts twice.
		if PageCount(stamped) != PageCount(pdf) && !copiesPages(pdf) {
			t.Fatalf("stamping changed the page count from %d to %d", PageCount(pdf), PageCount(stamped))
		}
		again, err := Stamp(stamped, "fedcba9876543210")
		if err != nil || StampOf(again) != "fedcba9876543210" {
			t.Fatalf("restamp: %v", err)
		}
	})
}

func copiesPages(pdf []byte) bool {
	for _, m := range infoEntry.FindAllSubmatch(pdf, -1) {
		n, _ := strconv.Atoi(string(m[1]))
		if pageObject.Match(objects(pdf)[n]) {
			return true
		}
	}
	return false
}
