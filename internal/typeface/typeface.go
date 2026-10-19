// Package typeface reads the font files a deliverable ships and embeds:
// what each face is, how wide its text runs, whether it draws a character,
// and the forms Word (obfuscated TrueType) and PowerPoint (Embedded
// OpenType) embed, each compressed once per compile.
package typeface

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Face is one font file of the project: what it is and the metrics the
// layout and the design checks need.
type Face struct {
	File          string // path below the deliverable folder
	Family        string // typographic family, as apps list it
	Weight        int    // 400 regular, 700 bold
	Italic        bool
	Embeddable    bool    // OS/2 fsType allows embedding in documents
	Natural       float64 // Word's single line height in em: (winAscent + winDescent) / unitsPerEm
	Typo          float64 // the typographic line in em, which Keynote and LibreOffice read
	AvgChar       float64 // average advance of running text in em
	TabularDigits bool    // 0-9 share one advance
	Serif         bool    // OS/2 family class 1-7
	Data          []byte
	Variable      bool // an fvar table: Word and Pages need static fonts
	glyphs        map[rune]bool
	font          *sfnt.Font
	upm           int
	mu            sync.Mutex
	advances      map[rune]float64 // in em, measured once
}

// Slot is where a face goes in a family: the four faces Word knows.
func (f *Face) Slot() string {
	switch {
	case f.Weight >= 600 && f.Italic:
		return "BoldItalic"
	case f.Weight >= 600:
		return "Bold"
	case f.Italic:
		return "Italic"
	}
	return "Regular"
}

// sample weighs letters as running English and German text does, spaces
// included, for the average character width a measure is counted in.
const sample = "the quick brown fox jumps over the lazy dog and then rests in the shade; " +
	"eine Vorlage, die sich leicht bearbeiten lässt, spart Zeit beim Schreiben, Planen und Drucken. "

// Read parses a font file; file is its path for messages.
func Read(file string, data []byte) (*Face, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, err
	}
	var buf sfnt.Buffer
	face := &Face{File: file, Data: data, Weight: 400, font: f}
	name := func(id sfnt.NameID) string {
		s, err := f.Name(&buf, id)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(s)
	}
	face.Family = name(sfnt.NameIDTypographicFamily)
	if face.Family == "" {
		face.Family = name(sfnt.NameIDFamily)
	}
	tables, err := Tables(data)
	if err != nil {
		return nil, err
	}
	_, face.Variable = tables["fvar"]
	head, ok := tables["head"]
	if !ok || len(head) < 54 {
		return nil, errors.New("the font has no head table")
	}
	upm := binary.BigEndian.Uint16(head[18:])
	face.upm = int(upm)
	os2, ok := tables["OS/2"]
	if !ok || len(os2) < 78 {
		return nil, errors.New("the font has no OS/2 table")
	}
	face.Weight = int(binary.BigEndian.Uint16(os2[4:]))
	fsType := binary.BigEndian.Uint16(os2[8:])
	// Restricted licence embedding (0x0002) forbids it; bitmap-only (0x0200) is useless.
	face.Embeddable = fsType&0x0002 == 0 && fsType&0x0200 == 0
	// The family class says serif or sans; without one, PANOSE's serif
	// style does (11 to 13 are sans).
	class, panose := os2[30], os2[32:42]
	face.Serif = class >= 1 && class <= 7 || class == 0 && panose[0] == 2 && panose[1] >= 2 && panose[1] <= 10
	fsSelection := binary.BigEndian.Uint16(os2[62:])
	face.Italic = fsSelection&0x0001 != 0
	winAscent, winDescent := binary.BigEndian.Uint16(os2[74:]), binary.BigEndian.Uint16(os2[76:])
	face.Natural = float64(winAscent+winDescent) / float64(upm)
	typoAsc, typoDesc, typoGap := int16(binary.BigEndian.Uint16(os2[68:])), int16(binary.BigEndian.Uint16(os2[70:])), int16(binary.BigEndian.Uint16(os2[72:]))
	face.Typo = float64(int(typoAsc)-int(typoDesc)+int(typoGap)) / float64(upm)

	ppem := fixed.Int26_6(upm) << 6
	advance := func(r rune) (float64, bool) {
		idx, err := f.GlyphIndex(&buf, r)
		if err != nil || idx == 0 {
			return 0, false
		}
		a, err := f.GlyphAdvance(&buf, idx, ppem, font.HintingNone)
		if err != nil {
			return 0, false
		}
		return float64(a) / 64 / float64(upm), true
	}
	total, n := 0.0, 0
	for _, r := range sample {
		if a, ok := advance(r); ok {
			total += a
			n++
		}
	}
	if n > 0 {
		face.AvgChar = total / float64(n)
	}
	face.glyphs = map[rune]bool{}
	for _, r := range Symbols {
		if idx, err := f.GlyphIndex(&buf, r); err == nil && idx != 0 {
			face.glyphs[r] = true
		}
	}
	first, _ := advance('0')
	face.TabularDigits = first > 0
	for r := '1'; r <= '9'; r++ {
		if a, _ := advance(r); a != first {
			face.TabularDigits = false
		}
	}
	return face, nil
}

// Symbols are the marks lfm sets: bullets, checkboxes, ornaments. Each
// falls back to one the font has.
const Symbols = "•–◦·☐☒□■❦❧◆—"

// Has reports whether the face draws one of lfm's marks.
func (f *Face) Has(r rune) bool { return f != nil && f.glyphs[r] }

// Covers reports whether the face draws r.
func (f *Face) Covers(r rune) bool {
	if f == nil || f.font == nil {
		return false
	}
	var buf sfnt.Buffer
	idx, err := f.font.GlyphIndex(&buf, r)
	return err == nil && idx != 0
}

// Tables maps a font's table tags to their bytes.
func Tables(data []byte) (map[string][]byte, error) {
	if len(data) < 12 {
		return nil, errors.New("the file is too short for a font")
	}
	n := int(binary.BigEndian.Uint16(data[4:]))
	out := map[string][]byte{}
	for i := range n {
		rec := 12 + 16*i
		if rec+16 > len(data) {
			return nil, errors.New("the font's table directory is cut off")
		}
		tag := string(data[rec : rec+4])
		off, size := binary.BigEndian.Uint32(data[rec+8:]), binary.BigEndian.Uint32(data[rec+12:])
		if uint64(off)+uint64(size) > uint64(len(data)) {
			return nil, errors.New("the font's " + tag + " table is cut off")
		}
		out[tag] = data[off : off+size]
	}
	return out, nil
}

// Obfuscate prepares a font for embedding in a .docx as ECMA-376 Part 1,
// 17.8.1 requires: the first 32 bytes XORed with the key's bytes in
// reverse order.
func Obfuscate(data []byte, key string) []byte {
	raw, _ := hex.DecodeString(strings.NewReplacer("{", "", "}", "", "-", "").Replace(key))
	out := append([]byte{}, data...)
	for i := 0; i < 32 && i < len(out) && len(raw) == 16; i++ {
		out[i] ^= raw[15-i%16]
	}
	return out
}

// Width is the advance of s at size in points, as apps set it without
// kerning: a little wider than kerned text, so a fit it allows is real.
// A character the face lacks counts as wide as an "M".
func (f *Face) Width(s string, size float64) float64 {
	if f == nil || f.font == nil {
		return float64(len([]rune(s))) * 0.55 * size
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.advances == nil {
		f.advances = map[rune]float64{}
	}
	var buf sfnt.Buffer
	measure := func(r rune) float64 {
		if a, ok := f.advances[r]; ok {
			return a
		}
		idx, err := f.font.GlyphIndex(&buf, r)
		if err != nil || idx == 0 {
			idx, _ = f.font.GlyphIndex(&buf, 'M')
		}
		adv, err := f.font.GlyphAdvance(&buf, idx, fixed.Int26_6(f.upm)<<6, font.HintingNone)
		a := 0.6
		if err == nil && f.upm > 0 {
			a = float64(adv) / 64 / float64(f.upm)
		}
		f.advances[r] = a
		return a
	}
	total := 0.0
	for _, r := range s {
		total += measure(r)
	}
	return total * size
}

// Family is the faces of one font family a deliverable ships.
type Family struct {
	Name  string
	Faces map[string]*Face // by slot: Regular, Bold, Italic, BoldItalic
}

// Regular is the family's regular face; nil when it has none.
func (f *Family) Regular() *Face {
	if f == nil {
		return nil
	}
	return f.Faces["Regular"]
}

// Face is the face a run in bold and italic is set in, the regular one
// where the family lacks it.
func (f *Family) Face(bold, italic bool) *Face {
	if f == nil {
		return nil
	}
	slot := "Regular"
	switch {
	case bold && italic:
		slot = "BoldItalic"
	case bold:
		slot = "Bold"
	case italic:
		slot = "Italic"
	}
	if face := f.Faces[slot]; face != nil {
		return face
	}
	return f.Regular()
}

// Glyph picks the first of the marks the family's regular face draws.
func Glyph(f *Family, marks ...rune) string {
	for _, r := range marks {
		if f.Regular().Has(r) {
			return string(r)
		}
	}
	return string(marks[len(marks)-1])
}

// Checkboxes are the open and done marks of task lists, and the font that
// draws them: the family's when it has them, else Arial, whose WGL4 set has
// □ and ■ in every app (LibreOffice maps it to Liberation Sans).
func Checkboxes(f *Family) (open, done, font string) {
	if r := f.Regular(); r.Has('☐') && r.Has('☒') {
		return "☐", "☒", f.Name
	}
	return "□", "■", "Arial"
}

// FontFaces are the CSS @font-face rules of the families' faces, each
// family once, loaded from prefix plus the file's base name.
func FontFaces(prefix string, families ...*Family) string {
	var b strings.Builder
	seen := map[string]bool{}
	for _, f := range families {
		if f == nil || seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		slots := make([]string, 0, len(f.Faces))
		for slot := range f.Faces {
			slots = append(slots, slot)
		}
		slices.Sort(slots)
		for _, slot := range slots {
			face := f.Faces[slot]
			style := "normal"
			if face.Italic {
				style = "italic"
			}
			name := face.File
			if i := strings.LastIndexByte(name, '/'); i >= 0 {
				name = name[i+1:]
			}
			fmt.Fprintf(&b, `@font-face{font-family:%q;src:url(%q);font-weight:%d;font-style:%s}`, f.Name, prefix+name, face.Weight, style)
		}
	}
	return b.String()
}
