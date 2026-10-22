package release

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

// Delivery is how the page's text travels: as hosts send it, compressed,
// and what leaving compression off would cost. Every common static host
// compresses text by default; the audit measures the page that way.
type Delivery struct {
	TextBytes       int `json:"textBytes"`       // HTML, CSS, scripts, SVG and data as built
	CompressedBytes int `json:"compressedBytes"` // the same with gzip, as hosts send them
	// UncompressedMS is about how much longer the text takes to arrive
	// uncompressed on Lighthouse's mobile network.
	UncompressedMS int `json:"uncompressedMs"`
}

// mobileBytesPerSecond is the throughput Lighthouse simulates for mobile:
// 1.6 Mbit/s, as 1638.4 Kbit/s.
const mobileBytesPerSecond = 1638.4 * 1024 / 8

// compressible are the files hosts compress; images, fonts and archives
// are compressed already.
func compressible(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm", ".css", ".js", ".mjs", ".json", ".svg", ".xml", ".txt", ".map", ".webmanifest", ".ico":
		return true
	}
	return false
}

// texts gzips each text file of an artifact once, for the audit's server
// and for the delivery it reports.
type texts struct {
	mu     sync.Mutex
	zipped map[string][]byte
}

// gzipped is content compressed as a host would, or nil when compression
// would not make it smaller.
func (t *texts) gzipped(name string, content []byte) []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	if z, ok := t.zipped[name]; ok {
		return z
	}
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write(content) // writes to memory cannot fail
	_ = w.Close()
	z := b.Bytes()
	if len(z) >= len(content) {
		z = nil
	}
	if t.zipped == nil {
		t.zipped = map[string][]byte{}
	}
	t.zipped[name] = z
	return z
}

// delivery sums the artifact's text as built and as hosts send it.
func (t *texts) delivery(page []byte, assets map[string][]byte) Delivery {
	var d Delivery
	add := func(name string, content []byte) {
		if !compressible(name) {
			return
		}
		d.TextBytes += len(content)
		if z := t.gzipped(name, content); z != nil {
			d.CompressedBytes += len(z)
		} else {
			d.CompressedBytes += len(content)
		}
	}
	add("index.html", page)
	for name, content := range assets {
		add(name, content)
	}
	d.UncompressedMS = int(float64(d.TextBytes-d.CompressedBytes) / mobileBytesPerSecond * 1000)
	return d
}

// acceptsGzip reports whether a request takes gzip, as every browser does.
func acceptsGzip(r *http.Request) bool {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(coding), "gzip") || strings.TrimSpace(coding) == "*" {
			return strings.ReplaceAll(strings.TrimSpace(params), " ", "") != "q=0"
		}
	}
	return false
}
