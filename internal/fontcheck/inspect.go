// Package fontcheck inspects font containers and CSS without loading glyphs,
// decompressing font payloads, executing code, or accessing the network.
package fontcheck

import (
	"encoding/base64"
	"encoding/binary"
	"net/url"
	"strings"
)

type Info struct {
	Source     string `json:"source"`
	Format     string `json:"format,omitempty"`
	Bytes      int    `json:"bytes"`
	SFNTBytes  uint32 `json:"declaredSFNTBytes,omitempty"`
	Variable   bool   `json:"hasVariationTable,omitempty"`
	Collection bool   `json:"collection,omitempty"`
	Issue      string `json:"issue,omitempty"`
	Unknown    string `json:"unknown,omitempty"`
}

// Kind sniffs only the signature, independently of filename/MIME hints. It
// does not promise the container, compressed payload or font is valid.
func Kind(b []byte) string {
	if len(b) < 4 {
		return ""
	}
	switch string(b[:4]) {
	case "wOF2":
		return "woff2"
	case "wOFF":
		return "woff"
	case "\x00\x01\x00\x00", "true":
		return "truetype"
	case "OTTO":
		return "opentype"
	case "ttcf":
		return "collection"
	}
	return ""
}

func MIME(kind string) string {
	switch kind {
	case "woff2":
		return "font/woff2"
	case "woff":
		return "font/woff"
	case "truetype":
		return "font/ttf"
	case "opentype":
		return "font/otf"
	case "collection":
		return "font/collection"
	}
	return ""
}

// Inspect is deliberately a bounded container screen, not a font sanitizer.
// No allocation is based on declared decompressed sizes. Directory work is
// capped at 1024 entries; WOFF2 collection internals remain opaque.
func Inspect(b []byte) Info {
	i := Info{Bytes: len(b), Format: Kind(b)}
	bad := func(s string) Info { i.Issue = s; return i }
	if i.Format == "" {
		return bad("unrecognized font signature (possibly a non-font response or unsupported legacy format)")
	}
	if i.Format == "collection" {
		i.Collection = true
		if len(b) < 12 {
			return bad("truncated font collection header")
		}
		n := uint64(binary.BigEndian.Uint32(b[8:]))
		if n == 0 || 12+4*n > uint64(len(b)) {
			return bad("font collection directory exceeds asset bounds")
		}
		i.Unknown = "collection faces and glyphs are not decoded"
		return i
	}
	if i.Format == "truetype" || i.Format == "opentype" {
		if len(b) < 12 {
			return bad("truncated SFNT header")
		}
		n := int(binary.BigEndian.Uint16(b[4:]))
		if n == 0 || 12+16*n > len(b) {
			return bad("SFNT table directory exceeds asset bounds")
		}
		if n > 1024 {
			i.Unknown = "font directory exceeds inspection budget"
			return i
		}
		for j := 0; j < n; j++ {
			r := b[12+16*j:]
			if !fits(b, binary.BigEndian.Uint32(r[8:]), binary.BigEndian.Uint32(r[12:])) {
				return bad("SFNT table exceeds asset bounds")
			}
			i.Variable = i.Variable || string(r[:4]) == "fvar"
		}
		return i
	}
	minimum := 44
	if i.Format == "woff2" {
		minimum = 48
	}
	if len(b) < minimum {
		return bad("truncated " + i.Format + " header")
	}
	if uint64(binary.BigEndian.Uint32(b[8:])) != uint64(len(b)) {
		return bad(i.Format + " declared length differs from asset bytes")
	}
	n := int(binary.BigEndian.Uint16(b[12:]))
	if n == 0 {
		return bad("font has no table directory entries")
	}
	if n > 1024 {
		i.Unknown = "font directory exceeds inspection budget"
		return i
	}
	i.SFNTBytes = binary.BigEndian.Uint32(b[16:])
	i.Collection = string(b[4:8]) == "ttcf"
	// totalSfntSize in WOFF2 is informational; reconstruction can change it.
	// A nonzero WOFF2 reserved field must not make a decoder reject the font.
	if i.Format == "woff" {
		if binary.BigEndian.Uint16(b[14:]) != 0 {
			return bad("WOFF reserved header field is nonzero")
		}
		if 44+20*n > len(b) {
			return bad("WOFF table directory exceeds asset bounds")
		}
		var total uint64 = uint64(12 + 16*n)
		for j := 0; j < n; j++ {
			r := b[44+20*j:]
			off, compressed, original := binary.BigEndian.Uint32(r[4:]), binary.BigEndian.Uint32(r[8:]), binary.BigEndian.Uint32(r[12:])
			if off < uint32(44+20*n) || compressed > original || !fits(b, off, compressed) {
				return bad("WOFF table size/offset exceeds container bounds")
			}
			total += (uint64(original) + 3) &^ 3
			i.Variable = i.Variable || string(r[:4]) == "fvar"
		}
		if total != uint64(i.SFNTBytes) {
			return bad("WOFF declared SFNT size disagrees with table lengths")
		}
	} else {
		compressed := binary.BigEndian.Uint32(b[20:])
		if compressed == 0 || uint64(compressed) > uint64(len(b)-48) {
			return bad("WOFF2 compressed block exceeds asset bounds")
		}
		if i.Collection {
			i.Unknown = "WOFF2 collection directory and compressed payload are not decoded"
			return i
		}
		pos := 48
		for j := 0; j < n; j++ {
			if pos >= len(b) {
				return bad("truncated WOFF2 directory")
			}
			flags := b[pos]
			pos++
			tag, version := int(flags&63), flags>>6
			name := ""
			if tag == 63 {
				if pos+4 > len(b) {
					return bad("truncated WOFF2 custom tag")
				}
				name = string(b[pos : pos+4])
				pos += 4
			} else {
				name = woffTags[tag]
			}
			if !base128(b, &pos) {
				return bad("invalid WOFF2 table length")
			}
			if name == "glyf" || name == "loca" {
				if version != 3 && !base128(b, &pos) {
					return bad("invalid WOFF2 transformed table length")
				}
			} else if version != 0 && !base128(b, &pos) {
				return bad("invalid WOFF2 transformed table length")
			}
			i.Variable = i.Variable || name == "fvar"
		}
		if uint64(pos)+uint64(compressed) > uint64(len(b)) {
			return bad("WOFF2 directory/compressed block exceeds asset bounds")
		}
	}
	meta := 24
	if i.Format == "woff2" {
		meta = 28
	}
	for _, at := range []int{meta, meta + 12} {
		off, size := binary.BigEndian.Uint32(b[at:]), binary.BigEndian.Uint32(b[at+4:])
		if (off == 0) != (size == 0) || !fits(b, off, size) {
			return bad("font metadata/private block exceeds asset bounds")
		}
	}
	return i
}

func fits(b []byte, off, size uint32) bool { return uint64(off)+uint64(size) <= uint64(len(b)) }
func base128(b []byte, pos *int) bool {
	var v uint32
	for j := 0; j < 5; j++ {
		if *pos >= len(b) {
			return false
		}
		c := b[*pos]
		*pos++
		if j == 0 && c == 0x80 || v&0xfe000000 != 0 {
			return false
		}
		v = v<<7 | uint32(c&0x7f)
		if c&0x80 == 0 {
			return true
		}
	}
	return false
}

var woffTags = strings.Split("cmap,head,hhea,hmtx,maxp,name,OS/2,post,cvt ,fpgm,glyf,loca,prep,CFF ,VORG,EBDT,EBLC,gasp,hdmx,kern,LTSH,PCLT,VDMX,vhea,vmtx,BASE,GDEF,GPOS,GSUB,EBSC,JSTF,MATH,CBDT,CBLC,COLR,CPAL,SVG ,sbix,acnt,avar,bdat,bloc,bsln,cvar,fdsc,feat,fmtx,fvar,gvar,hsty,just,lcar,mort,morx,opbd,prop,trak,Zapf,Silf,Glat,Gloc,Feat,Sill", ",")

// Data inspects authored data URLs within the existing 2 MB HTML budget.
func Data(ref string) Info {
	base, _, _ := strings.Cut(ref, "#")
	header, payload, ok := strings.Cut(base, ",")
	if !ok || len(payload) > 2_000_000 {
		return Info{Unknown: "data font exceeds inspection budget or has no payload"}
	}
	s, err := url.PathUnescape(payload)
	if err != nil {
		return Info{Issue: "invalid data font percent encoding"}
	}
	raw := []byte(s)
	if strings.HasSuffix(strings.ToLower(header), ";base64") {
		raw, err = base64.StdEncoding.DecodeString(s)
		if err != nil {
			return Info{Issue: "invalid data font base64 encoding"}
		}
	}
	return Inspect(raw)
}

func MatchesFormat(hint, kind string) bool {
	hint = strings.TrimSuffix(strings.ToLower(hint), "-variations")
	if hint == "truetype" || hint == "opentype" {
		return kind == "truetype" || kind == "opentype"
	}
	return hint == kind || hint == "collection" && kind == "collection"
}
