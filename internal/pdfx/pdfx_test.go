package pdfx

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPageCountOfAChromePDF(t *testing.T) {
	pdf, err := os.ReadFile("testdata/print.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if n := PageCount(pdf); n != 9 {
		t.Fatalf("PageCount = %d, want 9", n)
	}
}

func TestOutlinePages(t *testing.T) {
	pdf, err := os.ReadFile("testdata/print.pdf")
	if err != nil {
		t.Fatal(err)
	}
	got := OutlinePages(pdf)
	// The title page heading, then the three chapters.
	if fmt.Sprint(got) != "[2 4 6 8]" {
		t.Fatalf("outline pages = %v", got)
	}
}

func TestTransparent(t *testing.T) {
	if !Transparent([]byte("<</ca .5>>")) || !Transparent([]byte("/SMask 12 0 R")) || Transparent([]byte("<</ca 1 /CA 1>>")) {
		t.Error("transparency detection")
	}
}

func TestStampKeepsThePDFValid(t *testing.T) {
	pdf, _ := os.ReadFile("testdata/print.pdf")
	stamped, err := Stamp(pdf, "0123456789abcdef0123")
	if err != nil {
		t.Fatal(err)
	}
	if StampOf(stamped) != "0123456789abcdef0123" || StampOf(pdf) != "" || PageCount(stamped) != PageCount(pdf) || fmt.Sprint(OutlinePages(stamped)) != "[2 4 6 8]" {
		t.Fatal("stamp lost the key or changed the document")
	}
	again, _ := Stamp(stamped, "fedcba9876543210")
	if StampOf(again) != "fedcba9876543210" {
		t.Fatal("restamp")
	}
	if dir := os.Getenv("LFM_STAMP_OUT"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "stamped.pdf"), again, 0o644)
	}
}

func TestFigurePages(t *testing.T) {
	pdf := []byte(`%PDF-1.7
1 0 obj
<</Type /StructElem /S /Figure /Alt (split) /Pg 10 0 R /K [0 <</Type /MCR /Pg 11 0 R /MCID 0>>]>>
endobj
2 0 obj
<</Type /StructElem /S /Figure /Alt (whole) /Pg 10 0 R /K [3 0 R]>>
endobj
3 0 obj
<</Type /StructElem /Pg 10 0 R /K [2 0 R]>>
endobj
`)
	got := FigurePages(pdf)
	if got["split"] != 2 || got["whole"] != 1 {
		t.Fatal(got)
	}
}
