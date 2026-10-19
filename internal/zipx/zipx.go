// Package zipx writes the ZIP archives lfm ships (EPUB, .docx, .pptx,
// .xlsx, download and shop ZIPs) the way the strictest readers want them:
// every entry carries its sizes in its header, no data descriptors, no
// extra fields, UTF-8 names flagged, one fixed timestamp, so the same
// input gives the same bytes. EPUB forbids an extra field on its mimetype
// entry; LibreOffice refuses stored entries followed by a data descriptor.
package zipx

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"fmt"
	"hash/crc32"
	"io"
	"sync"
	"time"
	"unicode/utf8"
)

// Writer appends entries to one archive.
type Writer struct {
	z        *zip.Writer
	date     uint16
	clock    uint16
	modified time.Time
}

// New starts an archive on w whose entries all carry modified.
func New(w io.Writer, modified time.Time) *Writer {
	date, clock := DosTime(modified)
	return &Writer{z: zip.NewWriter(w), date: date, clock: clock, modified: modified}
}

// Packed is data compressed once, for entries many archives share, such
// as a font every document embeds.
type Packed struct {
	Data       []byte // deflated
	CRC        uint32
	Size       uint64 // uncompressed
	Compressed uint64
}

// Pack deflates data for Raw.
func Pack(data []byte, level int) Packed {
	var buf bytes.Buffer
	fw := flaters.get(level, &buf)
	fw.Write(data)
	fw.Close()
	flaters.put(level, fw)
	return Packed{Data: buf.Bytes(), CRC: crc32.ChecksumIEEE(data), Size: uint64(len(data)), Compressed: uint64(buf.Len())}
}

// Store writes an uncompressed entry: for data compressed already (PDF,
// PNG, JPEG, fonts in a ZIP) and for EPUB's mimetype.
func (w *Writer) Store(name string, data []byte) error {
	return w.raw(name, zip.Store, crc32.ChecksumIEEE(data), uint64(len(data)), data)
}

// Deflate writes a compressed entry.
func (w *Writer) Deflate(name string, data []byte) error {
	p := Pack(data, flate.DefaultCompression)
	return w.Raw(name, p)
}

// Raw writes an entry compressed before.
func (w *Writer) Raw(name string, p Packed) error {
	return w.raw(name, zip.Deflate, p.CRC, p.Size, p.Data)
}

func (w *Writer) raw(name string, method uint16, crc uint32, size uint64, data []byte) error {
	h := &zip.FileHeader{Name: name, Method: method, CRC32: crc, CompressedSize64: uint64(len(data)), UncompressedSize64: size,
		CreatorVersion: 20, ReaderVersion: 20}
	//lint:ignore SA1019 CreateRaw writes the DOS fields as given and ignores Modified.
	h.ModifiedDate, h.ModifiedTime = w.date, w.clock
	if !ascii(name) && utf8.ValidString(name) {
		h.Flags |= 0x800 // the name is UTF-8; CreateRaw does not set it
	}
	fw, err := w.z.CreateRaw(h)
	if err != nil {
		return err
	}
	_, err = fw.Write(data)
	return err
}

// Copy writes an entry of another archive with this archive's header
// rules, keeping its compressed bytes.
func (w *Writer) Copy(f *zip.File) error {
	r, err := f.OpenRaw()
	if err != nil {
		return err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	switch f.Method {
	case zip.Store, zip.Deflate:
		return w.raw(f.Name, f.Method, f.CRC32, f.UncompressedSize64, data)
	}
	return fmt.Errorf("%s: compression method %d", f.Name, f.Method)
}

// Comment sets the archive's comment.
func (w *Writer) Comment(s string) error { return w.z.SetComment(s) }

// Close writes the central directory.
func (w *Writer) Close() error { return w.z.Close() }

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// DosTime is t as a zip entry's MS-DOS date and time.
func DosTime(t time.Time) (date, clock uint16) {
	return uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9), uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
}

// StampPrefix starts the zip comment carrying a file's input key; apps
// never show the comment.
const StampPrefix = "lfm-input:"

// Stamp is the input key an archive carries in its comment, read from the
// end of the file; tail may be the whole file or its last bytes.
func Stamp(tail []byte) string {
	for i := len(tail) - 22; i >= max(0, len(tail)-22-256); i-- {
		if tail[i] != 'P' || tail[i+1] != 'K' || tail[i+2] != 5 || tail[i+3] != 6 {
			continue
		}
		n := int(tail[i+20]) | int(tail[i+21])<<8
		comment := tail[i+22:]
		if n != len(comment) || !bytes.HasPrefix(comment, []byte(StampPrefix)) {
			return ""
		}
		return string(comment[len(StampPrefix):])
	}
	return ""
}

// WithStamp sets the comment of an archive without one to the key.
func WithStamp(data []byte, key string) []byte {
	// The end record is the last 22 bytes while the comment is empty.
	if len(data) < 22 || !bytes.Equal(data[len(data)-22:len(data)-18], []byte("PK\x05\x06")) {
		return data
	}
	comment := StampPrefix + key
	out := make([]byte, 0, len(data)+len(comment))
	out = append(out, data[:len(data)-2]...)
	out = append(out, byte(len(comment)), byte(len(comment)>>8))
	return append(out, comment...)
}

// flaters reuses deflate writers: allocating one costs more than
// compressing a small XML part.
var flaters flaterPool

type flaterPool struct {
	mu    sync.Mutex
	pools map[int]*sync.Pool
}

func (p *flaterPool) pool(level int) *sync.Pool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pools == nil {
		p.pools = map[int]*sync.Pool{}
	}
	if p.pools[level] == nil {
		p.pools[level] = &sync.Pool{}
	}
	return p.pools[level]
}

func (p *flaterPool) get(level int, w io.Writer) *flate.Writer {
	if fw, ok := p.pool(level).Get().(*flate.Writer); ok {
		fw.Reset(w)
		return fw
	}
	fw, _ := flate.NewWriter(w, level)
	return fw
}

func (p *flaterPool) put(level int, fw *flate.Writer) { p.pool(level).Put(fw) }
