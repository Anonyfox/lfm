package typeface

import (
	"bytes"
	"encoding/binary"
	"errors"
	"unicode/utf16"

	"golang.org/x/image/font/sfnt"
)

// EOT wraps a TrueType font as Embedded OpenType 2.1, uncompressed: the
// .fntdata a .pptx embeds. PowerPoint for Windows and Mac show the deck in
// its fonts from it; the header repeats the font's OS/2 and name facts.
func EOT(face *Face) ([]byte, error) {
	tables, err := Tables(face.Data)
	if err != nil {
		return nil, err
	}
	os2, head := tables["OS/2"], tables["head"]
	if len(os2) < 86 || len(head) < 12 {
		return nil, errors.New("the font's OS/2 table lacks the code page ranges")
	}
	be := binary.BigEndian
	var buf sfnt.Buffer
	name := func(id sfnt.NameID) []byte {
		s, _ := face.font.Name(&buf, id)
		var b bytes.Buffer
		for _, u := range utf16.Encode([]rune(s)) {
			binary.Write(&b, binary.LittleEndian, u)
		}
		return b.Bytes()
	}
	var h bytes.Buffer
	le := func(v any) { binary.Write(&h, binary.LittleEndian, v) }
	le(uint32(0)) // EOTSize, set below
	le(uint32(len(face.Data)))
	le(uint32(0x00020001))
	le(uint32(0)) // flags: not compressed, not XORed
	h.Write(os2[32:42])
	italic := uint8(0)
	if face.Italic {
		italic = 1
	}
	le(uint8(1)) // DEFAULT_CHARSET
	le(italic)
	le(uint32(be.Uint16(os2[4:])))
	le(be.Uint16(os2[8:])) // fsType
	le(uint16(0x504C))
	for _, off := range []int{42, 46, 50, 54} {
		le(be.Uint32(os2[off:]))
	}
	le(be.Uint32(os2[78:]))
	le(be.Uint32(os2[82:]))
	le(be.Uint32(head[8:])) // checkSumAdjustment
	for range 4 {
		le(uint32(0))
	}
	le(uint16(0))
	for i, id := range []sfnt.NameID{sfnt.NameIDFamily, sfnt.NameIDSubfamily, sfnt.NameIDVersion, sfnt.NameIDFull} {
		if i > 0 {
			le(uint16(0))
		}
		s := name(id)
		le(uint16(len(s)))
		h.Write(s)
	}
	le(uint16(0))
	le(uint16(0)) // no root string
	out := append(h.Bytes(), face.Data...)
	binary.LittleEndian.PutUint32(out, uint32(len(out)))
	return out, nil
}
