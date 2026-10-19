package ooxml

import (
	"archive/zip"
	"bytes"
	"hash/crc32"
	"strings"
	"time"
)

// StampPrefix starts the zip comment carrying a file's input key; apps
// never show the comment.
const StampPrefix = "lfm-input:"

// Stamp is the input key a built .docx, .pptx or .zip carries in its zip
// comment, read from the end of the file; tail may be the whole file.
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

// WithStamp sets a zip's comment to the key.
func WithStamp(data []byte, key string) []byte {
	// The end record is the last 22 bytes while the comment is empty.
	if len(data) < 22 || !bytes.Equal(data[len(data)-22:len(data)-18], []byte("PK\x05\x06")) {
		return data
	}
	comment := StampPrefix + key
	out := append([]byte{}, data[:len(data)-2]...)
	out = append(out, byte(len(comment)), byte(len(comment)>>8))
	return append(out, comment...)
}

// DosTime is t as a zip entry's MS-DOS date and time.
func DosTime(t time.Time) (date, clock uint16) {
	return uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9), uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
}

// Store writes an uncompressed zip entry with its sizes in the header, as
// zip tools write it: some readers, LibreOffice among them, refuse a stored
// entry followed by a data descriptor, which zip.CreateHeader writes.
func Store(z *zip.Writer, name string, data []byte, modified time.Time) error {
	return Raw(z, name, zip.Store, crc32.ChecksumIEEE(data), uint64(len(data)), data, modified)
}

// Raw writes an entry already compressed with method, such as a font
// deflated once for every file that embeds it. CreateRaw leaves the
// versions and DOS time unset, and Word refuses entries that need
// "version 0.0" to extract.
func Raw(z *zip.Writer, name string, method uint16, crc uint32, size uint64, data []byte, modified time.Time) error {
	h := &zip.FileHeader{Name: name, Method: method, Modified: modified, CRC32: crc, CompressedSize64: uint64(len(data)), UncompressedSize64: size, CreatorVersion: 20, ReaderVersion: 20}
	//lint:ignore SA1019 CreateRaw writes these DOS fields and ignores Modified.
	h.ModifiedDate, h.ModifiedTime = DosTime(modified)
	fw, err := z.CreateRaw(h)
	if err != nil {
		return err
	}
	_, err = fw.Write(data)
	return err
}

// FileName is a file name buyers see: a title, safe on every system.
func FileName(title, ext string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return -1
		}
		return r
	}, title)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		clean = "Document"
	}
	return clean + "." + ext
}
