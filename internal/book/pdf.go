package book

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

var (
	pagesCount = regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+(\d+)|/Count\s+(\d+)[^>]*?/Type\s*/Pages\b`)
	pageObject = regexp.MustCompile(`/Type\s*/Page\b`)
)

// PageCount returns the number of pages of a PDF: the largest /Count of a
// page tree node, or, without one, the number of page objects. Chrome writes
// its page tree uncompressed, which is all lfm needs to read.
func PageCount(pdf []byte) int {
	best := 0
	for _, m := range pagesCount.FindAllSubmatch(pdf, -1) {
		v := m[1]
		if len(v) == 0 {
			v = m[2]
		}
		if n, err := strconv.Atoi(string(v)); err == nil && n > best {
			best = n
		}
	}
	if best > 0 {
		return best
	}
	return len(pageObject.FindAll(pdf, -1)) // /Page\b never matches /Pages
}

var (
	objectStart = regexp.MustCompile(`(?m)^(\d+) 0 obj\b`)
	reference   = regexp.MustCompile(`(\d+) 0 R`)
)

type pdfObjects map[int][]byte

// objects indexes the uncompressed objects of a PDF by number, each up to its
// endobj; stream bodies are included but never interpreted.
func objects(pdf []byte) pdfObjects {
	out := pdfObjects{}
	for _, m := range objectStart.FindAllSubmatchIndex(pdf, -1) {
		n, _ := strconv.Atoi(string(pdf[m[2]:m[3]]))
		body := pdf[m[1]:]
		if end := bytes.Index(body, []byte("endobj")); end >= 0 {
			body = body[:end]
		}
		if stream := bytes.Index(body, []byte("stream")); stream >= 0 {
			body = body[:stream] // the dictionary only
		}
		out[n] = body
	}
	return out
}

// ref returns the object a dictionary key points to.
func (o pdfObjects) ref(n int, key string) int {
	body := o[n]
	i := bytes.Index(body, []byte(key+" "))
	if i < 0 {
		return 0
	}
	m := reference.FindSubmatch(body[i+len(key):])
	if m == nil || bytes.Index(body[i+len(key):], m[0]) > 2 {
		return 0
	}
	v, _ := strconv.Atoi(string(m[1]))
	return v
}

// OutlinePages returns the page number (from 1) of every top-level bookmark of
// a PDF Chrome printed with a document outline, in order. Chrome builds the
// outline from the headings, so a book's chapters are its last entries. It
// returns nil when the structure cannot be read.
func OutlinePages(pdf []byte) []int {
	o := objects(pdf)
	catalog := 0
	for n, body := range o {
		if bytes.Contains(body, []byte("/Type /Catalog")) {
			catalog = n
		}
	}
	if catalog == 0 {
		return nil
	}
	index := map[int]int{}
	var walk func(n, depth int)
	walk = func(n, depth int) {
		body := o[n]
		if depth > 32 || body == nil {
			return
		}
		if !bytes.Contains(body, []byte("/Type /Pages")) {
			index[n] = len(index) + 1
			return
		}
		i := bytes.Index(body, []byte("/Kids ["))
		if i < 0 {
			return
		}
		kids := body[i+7:]
		end := bytes.IndexByte(kids, ']')
		if end < 0 {
			return
		}
		kids = kids[:end]
		for _, m := range reference.FindAllSubmatch(kids, -1) {
			kid, _ := strconv.Atoi(string(m[1]))
			walk(kid, depth+1)
		}
	}
	walk(o.ref(catalog, "/Pages"), 0)
	outline := o.ref(catalog, "/Outlines")
	if outline == 0 {
		return nil
	}
	var pages []int
	seen := map[int]bool{}
	for item := o.ref(outline, "/First"); item != 0 && !seen[item]; item = o.ref(item, "/Next") {
		seen[item] = true
		body := o[item]
		i := bytes.Index(body, []byte("/Dest ["))
		if i < 0 {
			return nil
		}
		m := reference.FindSubmatch(body[i:])
		if m == nil {
			return nil
		}
		page, _ := strconv.Atoi(string(m[1]))
		if index[page] == 0 {
			return nil
		}
		pages = append(pages, index[page])
	}
	return pages
}

var partialAlpha = regexp.MustCompile(`^(?:/(?:ca|CA) (?:0?\.\d+|0)\b|/SMask \d+ 0 R)`)

// Transparent reports whether a PDF Chrome printed contains transparency:
// partial opacity or images with an alpha channel. Printers want it flattened.
// It looks only where an opacity or mask key starts, so a large PDF is
// scanned at memory speed.
func Transparent(pdf []byte) bool {
	for _, key := range [][]byte{[]byte("/SMask "), []byte("/ca "), []byte("/CA ")} {
		for rest := pdf; ; {
			i := bytes.Index(rest, key)
			if i < 0 {
				break
			}
			if partialAlpha.Match(rest[i:min(len(rest), i+32)]) {
				return true
			}
			rest = rest[i+len(key):]
		}
	}
	return false
}

var (
	stampPattern = regexp.MustCompile(`/LfmBuild \(([0-9a-f]{16,64})\)`)
	startXref    = regexp.MustCompile(`startxref\s+(\d+)\s+%%EOF\s*$`)
	trailerDict  = regexp.MustCompile(`(?s)trailer\s*<<(.*?)>>\s*startxref`)
	sizeEntry    = regexp.MustCompile(`/Size\s+(\d+)`)
	rootEntry    = regexp.MustCompile(`/Root\s+\d+\s+0\s+R`)
	infoEntry    = regexp.MustCompile(`/Info\s+(\d+)\s+0\s+R`)
)

// Stamp records key in a PDF's document information, the inputs it was
// printed from, as an incremental update: the printed bytes stay untouched
// and the file stays a valid PDF. A later compile reuses the file when its
// key still matches instead of printing it again.
func Stamp(pdf []byte, key string) ([]byte, error) {
	xref := startXref.FindSubmatch(pdf)
	trailers := trailerDict.FindAllSubmatch(pdf, -1)
	if xref == nil || len(trailers) == 0 {
		return nil, fmt.Errorf("the PDF has no readable trailer")
	}
	trailer := trailers[len(trailers)-1][1]
	size := sizeEntry.FindSubmatch(trailer)
	root := rootEntry.Find(trailer)
	if size == nil || root == nil {
		return nil, fmt.Errorf("the PDF trailer has no /Size or /Root")
	}
	n, _ := strconv.Atoi(string(size[1]))
	info := ""
	if m := infoEntry.FindSubmatch(trailer); m != nil {
		old, _ := strconv.Atoi(string(m[1]))
		body := bytes.TrimSpace(objects(pdf)[old])
		body = bytes.TrimSuffix(bytes.TrimPrefix(body, []byte("<<")), []byte(">>"))
		info = string(stampPattern.ReplaceAll(body, nil))
	}
	var b bytes.Buffer
	b.Write(pdf)
	if !bytes.HasSuffix(pdf, []byte("\n")) {
		b.WriteByte('\n')
	}
	offset := b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<<%s\n/LfmBuild (%s)>>\nendobj\n", n, info, key)
	xrefAt := b.Len()
	fmt.Fprintf(&b, "xref\n%d 1\n%010d 00000 n \ntrailer\n<</Size %d\n%s\n/Info %d 0 R\n/Prev %s>>\nstartxref\n%d\n%%%%EOF\n", n, offset, n+1, root, n, xref[1], xrefAt)
	return b.Bytes(), nil
}

// StampOf returns the key a PDF was stamped with, or "".
func StampOf(pdf []byte) string {
	tail := pdf[max(0, len(pdf)-4096):]
	if m := stampPattern.FindAllSubmatch(tail, -1); len(m) > 0 {
		return string(m[len(m)-1][1])
	}
	return ""
}
