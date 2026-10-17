package compile

import (
	"compress/flate"
	"encoding/base64"
	"io"
	"strings"
	"sync"
)

// One cheap DEFLATE comparison per SVG source, reused across its references.
// Pooling avoids allocating a compressor for every reference. This is a transfer
// heuristic at the same speed setting as the cost report, not a promise about
// every host's gzip/Brotli settings or the complete document's shared dictionary.
var encodingWriters = sync.Pool{New: func() any {
	w, _ := flate.NewWriter(io.Discard, flate.BestSpeed)
	return w
}}

type byteCounter int

func (n *byteCounter) Write(p []byte) (int, error) { *n += byteCounter(len(p)); return len(p), nil }

func inlineData(contents []byte, kind string) (uri, encoding string) {
	base64URI := "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(contents)
	if kind != "image/svg+xml" {
		return base64URI, "base64"
	}
	textURI := svgDataURI(contents)
	w := encodingWriters.Get().(*flate.Writer)
	defer encodingWriters.Put(w)
	score := func(value string) int {
		var n byteCounter
		w.Reset(&n)
		_, _ = io.WriteString(w, value)
		_ = w.Close()
		return int(n)
	}
	textScore, base64Score := score(textURI), score(base64URI)
	if textScore < base64Score || textScore == base64Score && len(textURI) < len(base64URI) {
		return textURI, "percent"
	}
	return base64URI, "base64"
}

// Encode bytes, not XML: decoding must recover the original file byte-for-byte,
// including Unicode/encoding declarations, whitespace, IDs and fragment refs.
// This alphabet survives HTML attributes, srcset, either JS/CSS string delimiter
// and unquoted CSS url(). In particular %, #, comma, &, quotes and backslashes
// must never become ambiguous payload, markup, escapes or URL fragments.
func svgDataURI(contents []byte) string {
	const hex = "0123456789ABCDEF"
	var out strings.Builder
	out.Grow(len(contents) + len(contents)/2 + 19)
	out.WriteString("data:image/svg+xml,")
	for _, c := range contents {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~:/=;", rune(c)) {
			out.WriteByte(c)
		} else {
			out.WriteByte('%')
			out.WriteByte(hex[c>>4])
			out.WriteByte(hex[c&15])
		}
	}
	return out.String()
}
