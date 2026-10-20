package zipx

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

// Problems lists where an archive breaks the rules this package writes
// by, reading every entry's local header through the central directory:
// a data descriptor, an extra field, a UTF-8 name without its flag, an
// EPUB mimetype entry that is compressed. Tests hold every archive lfm
// builds to it.
func Problems(data []byte) []string {
	le := binary.LittleEndian
	end := -1
	for i := len(data) - 22; i >= max(0, len(data)-22-65535); i-- {
		if le.Uint32(data[i:]) == 0x06054b50 {
			end = i
			break
		}
	}
	if end < 0 {
		return []string{"no end of central directory"}
	}
	count := int(le.Uint16(data[end+10:]))
	at := int(le.Uint32(data[end+16:]))
	var out []string
	for n := 0; n < count; n++ {
		if at+46 > len(data) || le.Uint32(data[at:]) != 0x02014b50 {
			return append(out, "the central directory is cut off")
		}
		nameLen, extraLen, commentLen := int(le.Uint16(data[at+28:])), int(le.Uint16(data[at+30:])), int(le.Uint16(data[at+32:]))
		local := int(le.Uint32(data[at+42:]))
		if at+46+nameLen > len(data) {
			return append(out, "the central directory is cut off")
		}
		name := string(data[at+46 : at+46+nameLen])
		if extraLen > 0 {
			out = append(out, fmt.Sprintf("%s: an extra field of %d bytes in the directory", name, extraLen))
		}
		at += 46 + nameLen + extraLen + commentLen
		if local+30 > len(data) || le.Uint32(data[local:]) != 0x04034b50 {
			out = append(out, name+": no local header")
			continue
		}
		flags := le.Uint16(data[local+6:])
		method := le.Uint16(data[local+8:])
		if flags&0x8 != 0 {
			out = append(out, name+": followed by a data descriptor")
		}
		if x := int(le.Uint16(data[local+28:])); x > 0 {
			out = append(out, fmt.Sprintf("%s: an extra field of %d bytes", name, x))
		}
		if !ascii(name) && utf8.ValidString(name) && flags&0x800 == 0 {
			out = append(out, name+": a UTF-8 name without the UTF-8 flag")
		}
		if n == 0 && name == "mimetype" && method != 0 {
			out = append(out, "mimetype: compressed")
		}
	}
	return out
}
