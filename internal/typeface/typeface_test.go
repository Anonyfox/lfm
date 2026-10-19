package typeface

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/Anonyfox/lfm/internal/project"
)

const loraDir = "../document/testdata/club/fonts"

func fontData(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join(loraDir, name)
	if name == "Go-Regular.ttf" {
		path = "../fontcheck/testdata/Go-Regular.ttf"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func lora(t testing.TB, name string) *Face {
	t.Helper()
	face, err := Read("fonts/"+name, fontData(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// tableOffset is where a table starts in a font file.
func tableOffset(t testing.TB, data []byte, tag string) int {
	t.Helper()
	for i := range int(binary.BigEndian.Uint16(data[4:])) {
		rec := 12 + 16*i
		if string(data[rec:rec+4]) == tag {
			return int(binary.BigEndian.Uint32(data[rec+8:]))
		}
	}
	t.Fatalf("no %s table", tag)
	return 0
}

// retag renames a table in a copy of the font.
func retag(t testing.TB, data []byte, from, to string) []byte {
	t.Helper()
	out := bytes.Clone(data)
	for i := range int(binary.BigEndian.Uint16(out[4:])) {
		rec := 12 + 16*i
		if string(out[rec:rec+4]) == from {
			copy(out[rec:], to)
			return out
		}
	}
	t.Fatalf("no %s table", from)
	return nil
}

// withFSType sets the OS/2 embedding bits in a copy of the font.
func withFSType(t testing.TB, data []byte, fsType uint16) []byte {
	t.Helper()
	out := bytes.Clone(data)
	binary.BigEndian.PutUint16(out[tableOffset(t, out, "OS/2")+8:], fsType)
	return out
}

func TestReadLoraFaces(t *testing.T) {
	for _, tc := range []struct {
		file   string
		weight int
		italic bool
		slot   string
	}{
		{"Lora-Regular.ttf", 400, false, "Regular"},
		{"Lora-Bold.ttf", 700, false, "Bold"},
		{"Lora-Italic.ttf", 400, true, "Italic"},
		{"Lora-BoldItalic.ttf", 700, true, "BoldItalic"},
	} {
		f := lora(t, tc.file)
		if f.Family != "Lora" || f.Weight != tc.weight || f.Italic != tc.italic || f.Slot() != tc.slot {
			t.Errorf("%s: %s %d italic=%v slot %s", tc.file, f.Family, f.Weight, f.Italic, f.Slot())
		}
		if !f.Embeddable || f.Variable {
			t.Errorf("%s: embeddable %v variable %v", tc.file, f.Embeddable, f.Variable)
		}
		if f.Natural <= 0 || f.Typo <= 0 || f.AvgChar <= 0.3 || f.AvgChar >= 0.7 {
			t.Errorf("%s: natural %v typo %v avg %v", tc.file, f.Natural, f.Typo, f.AvgChar)
		}
		if f.File != "fonts/"+tc.file || len(f.Data) == 0 {
			t.Errorf("%s: file %q, %d bytes", tc.file, f.File, len(f.Data))
		}
	}
}

func TestReadGoRegularHasTabularDigits(t *testing.T) {
	f := lora(t, "Go-Regular.ttf")
	if f.Family != "Go" || !f.TabularDigits {
		t.Fatalf("family %q tabular %v", f.Family, f.TabularDigits)
	}
	if lora(t, "Lora-Regular.ttf").TabularDigits {
		t.Error("Lora's proportional figures read as tabular")
	}
}

func TestSlotThresholds(t *testing.T) {
	for _, tc := range []struct {
		weight int
		italic bool
		want   string
	}{
		{100, false, "Regular"}, {500, false, "Regular"}, {599, true, "Italic"},
		{600, false, "Bold"}, {900, false, "Bold"}, {600, true, "BoldItalic"},
	} {
		if got := (&Face{Weight: tc.weight, Italic: tc.italic}).Slot(); got != tc.want {
			t.Errorf("weight %d italic %v: %s, want %s", tc.weight, tc.italic, got, tc.want)
		}
	}
}

func TestReadRejectsWhatIsNotAFont(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":   nil,
		"text":    []byte("this is not a font file at all"),
		"cut":     fontData(t, "Lora-Regular.ttf")[:400],
		"no head": retag(t, fontData(t, "Lora-Regular.ttf"), "head", "heaX"),
	} {
		if f, err := Read(name, data); err == nil {
			t.Errorf("%s read as %+v", name, f)
		}
	}
}

func TestReadEmbeddingBits(t *testing.T) {
	data := fontData(t, "Lora-Regular.ttf")
	for fsType, want := range map[uint16]bool{0: true, 0x0004: true, 0x0008: true, 0x0002: false, 0x0200: false, 0x0208: false} {
		f, err := Read("x.ttf", withFSType(t, data, fsType))
		if err != nil {
			t.Fatal(err)
		}
		if f.Embeddable != want {
			t.Errorf("fsType %#04x: embeddable %v", fsType, f.Embeddable)
		}
	}
}

func TestReadSeesVariableFonts(t *testing.T) {
	f, err := Read("x.ttf", retag(t, fontData(t, "Lora-Regular.ttf"), "gasp", "fvar"))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Variable {
		t.Fatal("an fvar table is not read as variable")
	}
}

func TestWidthIsAdditiveAndScales(t *testing.T) {
	f := lora(t, "Lora-Regular.ttf")
	a, b := f.Width("Hamburg", 10), f.Width(" harbour", 10)
	if ab := f.Width("Hamburg harbour", 10); abs(ab-(a+b)) > 1e-9 {
		t.Errorf("width %v is not %v + %v", ab, a, b)
	}
	if w20 := f.Width("Hamburg", 20); abs(w20-2*a) > 1e-9 {
		t.Errorf("width at 20pt %v, at 10pt %v", w20, a)
	}
	prev := 0.0
	for i := 1; i <= len("Wellenbrecher"); i++ {
		w := f.Width("Wellenbrecher"[:i], 12)
		if w <= prev {
			t.Fatalf("width of %q (%v) is not more than of one letter less (%v)", "Wellenbrecher"[:i], w, prev)
		}
		prev = w
	}
	if f.Width("", 12) != 0 {
		t.Error("empty text has a width")
	}
	if f.Width("iiii", 10) >= f.Width("MMMM", 10) {
		t.Error("i is not narrower than M")
	}
}

func TestWidthCountsMissingGlyphAsM(t *testing.T) {
	f := lora(t, "Lora-Regular.ttf")
	if f.Covers('中') {
		t.Skip("Lora draws 中")
	}
	if got, want := f.Width("中", 10), f.Width("M", 10); got != want {
		t.Fatalf("missing glyph %v, M %v", got, want)
	}
}

func TestWidthWithoutFontEstimates(t *testing.T) {
	var f *Face
	if got := f.Width("äbc", 10); abs(got-16.5) > 1e-9 {
		t.Fatalf("nil face width %v", got)
	}
	if got := (&Face{}).Width("ab", 10); abs(got-11) > 1e-9 {
		t.Fatalf("empty face width %v", got)
	}
}

func TestWidthIsSafeConcurrently(t *testing.T) {
	f := lora(t, "Lora-Regular.ttf")
	want := f.Width("Sauerteig", 11)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := f.Width("Sauerteig"+strings.Repeat("x", i), 11); got < want {
				t.Errorf("width %v < %v", got, want)
			}
		}()
	}
	wg.Wait()
}

func TestCoversAndHas(t *testing.T) {
	f := lora(t, "Lora-Regular.ttf")
	for _, r := range "Aaäß€" {
		if !f.Covers(r) {
			t.Errorf("Lora does not cover %q", r)
		}
	}
	if f.Covers('中') || f.Covers('\U0001F389') {
		t.Error("Lora covers CJK or emoji")
	}
	if !f.Has('•') || !f.Has('—') {
		t.Error("Lora lacks the bullet or em dash")
	}
	if f.Has('A') {
		t.Error("Has answers for a letter outside the marks")
	}
	var none *Face
	if none.Has('•') || none.Covers('A') || (&Face{}).Covers('A') {
		t.Error("an absent face draws something")
	}
}

func TestTablesErrorsOnTruncatedData(t *testing.T) {
	data := fontData(t, "Lora-Regular.ttf")
	n := int(binary.BigEndian.Uint16(data[4:]))
	end := 0
	for i := range n {
		rec := 12 + 16*i
		end = max(end, int(binary.BigEndian.Uint32(data[rec+8:])+binary.BigEndian.Uint32(data[rec+12:])))
	}
	for _, tc := range []struct {
		cut  int
		want string
	}{
		{0, "too short"},
		{11, "too short"},
		{12 + 15, "directory is cut off"},
		{12 + 16*n, "table is cut off"},
		{end - 1, "table is cut off"},
	} {
		if _, err := Tables(data[:tc.cut]); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("cut at %d: %v, want %q", tc.cut, err, tc.want)
		}
	}
	tables, err := Tables(data)
	if err != nil || len(tables) != n || len(tables["head"]) != 54 {
		t.Fatalf("%d tables, head %d bytes, %v", len(tables), len(tables["head"]), err)
	}
}

const fontKey = "{6C2A1F8E-3B4D-4E5F-9A0B-1C2D3E4F5A6B}"

func TestObfuscateIsAnInvolution(t *testing.T) {
	data := fontData(t, "Lora-Regular.ttf")
	orig := bytes.Clone(data)
	once := Obfuscate(data, fontKey)
	if !bytes.Equal(data, orig) {
		t.Fatal("Obfuscate changed its input")
	}
	if bytes.Equal(once[:32], data[:32]) {
		t.Fatal("the first 32 bytes are unchanged")
	}
	if !bytes.Equal(once[32:], data[32:]) {
		t.Fatal("bytes after the first 32 changed")
	}
	if twice := Obfuscate(once, fontKey); !bytes.Equal(twice, data) {
		t.Fatal("obfuscating twice does not restore the font")
	}
}

func TestObfuscateXorsWithReversedKey(t *testing.T) {
	out := Obfuscate(make([]byte, 40), "00112233-4455-6677-8899-AABBCCDDEEFF")
	for i := range 32 {
		if want := byte(0xFF - 0x11*(i%16)); out[i] != want {
			t.Fatalf("byte %d is %#02x, want %#02x", i, out[i], want)
		}
	}
	for _, b := range out[32:] {
		if b != 0 {
			t.Fatal("a byte after 32 changed")
		}
	}
}

func TestObfuscateIgnoresBadKeysAndShortData(t *testing.T) {
	data := []byte("short")
	for _, key := range []string{"", "nothex", "{0011}"} {
		if got := Obfuscate(data, key); !bytes.Equal(got, data) {
			t.Errorf("key %q changed the data", key)
		}
	}
	if got := Obfuscate(data, fontKey); len(got) != len(data) || bytes.Equal(got, data) {
		t.Errorf("short data: %q", got)
	}
}

func TestEOTHeader(t *testing.T) {
	face := lora(t, "Lora-Italic.ttf")
	eot, err := EOT(face)
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	if got := le.Uint32(eot[0:]); int(got) != len(eot) {
		t.Errorf("EOTSize %d, length %d", got, len(eot))
	}
	if got := le.Uint32(eot[4:]); int(got) != len(face.Data) {
		t.Errorf("FontDataSize %d, font %d", got, len(face.Data))
	}
	if got := le.Uint32(eot[8:]); got != 0x00020001 {
		t.Errorf("version %#x", got)
	}
	if got := le.Uint32(eot[12:]); got != 0 {
		t.Errorf("flags %#x", got)
	}
	if eot[27] != 1 {
		t.Errorf("italic byte %d", eot[27])
	}
	if got := le.Uint32(eot[28:]); got != 400 {
		t.Errorf("weight %d", got)
	}
	if got := le.Uint16(eot[34:]); got != 0x504C {
		t.Errorf("magic %#x", got)
	}
	os2 := tableOffset(t, face.Data, "OS/2")
	if !bytes.Equal(eot[16:26], face.Data[os2+32:os2+42]) {
		t.Error("PANOSE differs from the font's")
	}
	family := le.Uint16(eot[82:])
	name := eot[84 : 84+int(family)]
	if want := utf16le("Lora"); !bytes.Equal(name, want) {
		t.Errorf("family name % x, want % x", name, want)
	}
	if !bytes.Contains(eot[:len(eot)-len(face.Data)], utf16le("Lora Italic")) {
		t.Error("the full name is not in the header")
	}
	if !bytes.Equal(eot[len(eot)-len(face.Data):], face.Data) {
		t.Error("the font data is not appended unchanged")
	}
}

func TestEOTOfUnreadableDataFails(t *testing.T) {
	if _, err := EOT(&Face{Data: []byte("tiny")}); err == nil {
		t.Fatal("no error")
	}
}

func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return b
}

func inflate(t *testing.T, p *Packed) []byte {
	t.Helper()
	raw, err := io.ReadAll(flate.NewReader(bytes.NewReader(p.Data)))
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(raw)) != p.Size || uint64(len(p.Data)) != p.Compressed {
		t.Fatalf("sizes %d/%d, packed says %d/%d", len(raw), len(p.Data), p.Size, p.Compressed)
	}
	return raw
}

func TestPackDeflatesEachFormOnce(t *testing.T) {
	face := lora(t, "Lora-Regular.ttf")
	p := NewPack()
	obf := p.Obfuscated(face, fontKey)
	if again := p.Obfuscated(face, fontKey); again != obf {
		t.Error("a second Obfuscated deflated again")
	}
	if !bytes.Equal(inflate(t, obf), Obfuscate(face.Data, fontKey)) {
		t.Error("the packed font is not the obfuscated one")
	}
	eot, err := p.EOT(face)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := p.EOT(face); again != eot || eot == obf {
		t.Error("EOT is not packed once, apart from the obfuscated form")
	}
	want, _ := EOT(face)
	if !bytes.Equal(inflate(t, eot), want) {
		t.Error("the packed EOT differs")
	}
	if other := p.Obfuscated(lora(t, "Lora-Bold.ttf"), fontKey); other == obf {
		t.Error("two faces share a packed part")
	}
}

func TestPackKeepsTheError(t *testing.T) {
	p := NewPack()
	bad := &Face{Data: []byte("tiny")}
	for range 2 {
		if packed, err := p.EOT(bad); err == nil || packed != nil {
			t.Fatalf("packed %v, err %v", packed, err)
		}
	}
}

func TestPackIsSafeConcurrently(t *testing.T) {
	faces := []*Face{lora(t, "Lora-Regular.ttf"), lora(t, "Lora-Bold.ttf")}
	p := NewPack()
	const n = 16
	got := make([][2]*Packed, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			face := faces[i%2]
			eot, err := p.EOT(face)
			if err != nil {
				t.Error(err)
			}
			got[i] = [2]*Packed{p.Obfuscated(face, fontKey), eot}
		}()
	}
	wg.Wait()
	for i := 2; i < n; i++ {
		if got[i] != got[i%2] {
			t.Fatalf("goroutine %d got other parts than %d", i, i%2)
		}
	}
}

// folder writes files into a project's fonts folder.
func folder(t *testing.T, files map[string][]byte) project.Project {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "deliverables", "document", "fonts")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type finding struct {
	file    string
	fatal   bool
	message string
}

func collect(out *[]finding) Report {
	return func(file string, line int, fatal bool, message, hint string) {
		*out = append(*out, finding{file, fatal, message})
	}
}

const fontsDir = "deliverables/document/fonts"

func TestLoadReadsFamiliesAndLicences(t *testing.T) {
	p := folder(t, map[string][]byte{
		"Lora-Regular.ttf":    fontData(t, "Lora-Regular.ttf"),
		"Lora-Bold.TTF":       fontData(t, "Lora-Bold.ttf"),
		"Lora-Italic.ttf":     fontData(t, "Lora-Italic.ttf"),
		"Lora-BoldItalic.ttf": fontData(t, "Lora-BoldItalic.ttf"),
		"Go-Regular.ttf":      fontData(t, "Go-Regular.ttf"),
		"OFL.txt":             []byte("licence"),
		"LICENSE-Go.txt":      []byte("bsd"),
		"README.md":           []byte("ignored"),
	})
	var found []finding
	f := Load(p, fontsDir, collect(&found))
	if len(found) > 0 {
		t.Fatalf("findings %+v", found)
	}
	if f.Dir != fontsDir || len(f.Families) != 2 || len(f.Families["Lora"].Faces) != 4 || f.Families["Go"].Regular() == nil {
		t.Fatalf("folder %+v", f)
	}
	if len(f.Licences) != 2 || string(f.Licences["OFL.txt"]) != "licence" {
		t.Errorf("licences %v", f.Licences)
	}
	if got := f.Families["Lora"].Faces["Bold"].File; got != fontsDir+"/Lora-Bold.TTF" {
		t.Errorf("bold file %q", got)
	}
}

func TestLoadReportsMissingLicence(t *testing.T) {
	var found []finding
	Load(folder(t, map[string][]byte{"Lora-Regular.ttf": fontData(t, "Lora-Regular.ttf")}), fontsDir, collect(&found))
	if len(found) != 1 || !found[0].fatal || found[0].file != fontsDir || !strings.Contains(found[0].message, "no licence file") {
		t.Fatalf("findings %+v", found)
	}
}

func TestLoadWarnsAboutDuplicateFaces(t *testing.T) {
	var found []finding
	f := Load(folder(t, map[string][]byte{
		"A-Lora-Regular.ttf": fontData(t, "Lora-Regular.ttf"),
		"B-Lora-Copy.ttf":    fontData(t, "Lora-Regular.ttf"),
		"OFL.txt":            nil,
	}), fontsDir, collect(&found))
	if len(found) != 1 || found[0].fatal || !strings.Contains(found[0].message, "B-Lora-Copy.ttf is a second Lora Regular face") {
		t.Fatalf("findings %+v", found)
	}
	if got := f.Families["Lora"].Regular().File; got != fontsDir+"/A-Lora-Regular.ttf" {
		t.Errorf("kept %q", got)
	}
}

func TestLoadRejectsUnusableFonts(t *testing.T) {
	data := fontData(t, "Lora-Regular.ttf")
	var found []finding
	f := Load(folder(t, map[string][]byte{
		"Broken.ttf":     []byte("not a font"),
		"Variable.ttf":   retag(t, data, "gasp", "fvar"),
		"Restricted.otf": withFSType(t, data, 0x0002),
		"OFL.txt":        nil,
	}), fontsDir, collect(&found))
	if len(f.Families) != 0 {
		t.Errorf("families %v", f.Families)
	}
	want := map[string]string{
		fontsDir + "/Broken.ttf":     "is not a font lfm can read",
		fontsDir + "/Variable.ttf":   "is a variable font",
		fontsDir + "/Restricted.otf": "forbids embedding",
	}
	if len(found) != len(want) {
		t.Fatalf("findings %+v", found)
	}
	for _, x := range found {
		if !x.fatal || !strings.Contains(x.message, want[x.file]) {
			t.Errorf("finding %+v", x)
		}
	}
}

func TestLoadOfMissingOrOutsideFolder(t *testing.T) {
	p := folder(t, nil)
	var found []finding
	for _, dir := range []string{"deliverables/none", "../outside"} {
		if f := Load(p, dir, collect(&found)); len(f.Families) != 0 || len(f.Licences) != 0 {
			t.Errorf("%s: %+v", dir, f)
		}
	}
	if len(found) != 0 {
		t.Errorf("findings %+v", found)
	}
}

func loraFamily(t *testing.T, slots ...string) *Family {
	t.Helper()
	files := map[string]string{"Regular": "Lora-Regular.ttf", "Bold": "Lora-Bold.ttf", "Italic": "Lora-Italic.ttf", "BoldItalic": "Lora-BoldItalic.ttf"}
	f := &Family{Name: "Lora", Faces: map[string]*Face{}}
	for _, s := range slots {
		f.Faces[s] = lora(t, files[s])
	}
	return f
}

func TestWantReportsMissingFamilyAndFaces(t *testing.T) {
	folderOf := func(f *Family) Folder {
		return Folder{Dir: fontsDir, Families: map[string]*Family{"Lora": f}}
	}
	for _, tc := range []struct {
		name, role string
		family     *Family
		fatal      bool
		want       string
	}{
		{"Inter", "text", loraFamily(t, "Regular"), true, "The text font Inter is not in " + fontsDir},
		{"Lora", "heading", loraFamily(t, "Bold"), true, "The heading font Lora is not in"},
		{"Lora", "heading", loraFamily(t, "Regular"), false, "Lora has no bold face."},
		{"Lora", "text", loraFamily(t, "Regular", "Bold"), false, "Lora has no italic face."},
		{"Lora", "heading", loraFamily(t, "Regular", "Bold"), false, ""},
		{"Lora", "text", loraFamily(t, "Regular", "Bold", "Italic"), false, ""},
	} {
		var found []finding
		var line int
		got := folderOf(tc.family).Want(tc.name, tc.role, "deliverables/document/concept.md", 7, func(file string, l int, fatal bool, message, hint string) {
			line = l
			found = append(found, finding{file, fatal, message})
			if !strings.Contains(hint, "family="+strings.ReplaceAll(tc.name, " ", "+")) {
				t.Errorf("hint %q", hint)
			}
		})
		switch {
		case tc.want == "":
			if len(found) != 0 || got == nil {
				t.Errorf("%s %s: %+v, family %v", tc.name, tc.role, found, got)
			}
		case len(found) != 1 || found[0].fatal != tc.fatal || !strings.Contains(found[0].message, tc.want) || line != 7 || found[0].file != "deliverables/document/concept.md":
			t.Errorf("%s %s: %+v at line %d", tc.name, tc.role, found, line)
		case tc.fatal != (got == nil):
			t.Errorf("%s %s: family %v", tc.name, tc.role, got)
		}
	}
}

func TestFamilyFaceFallsBackToRegular(t *testing.T) {
	f := loraFamily(t, "Regular", "Bold")
	if f.Face(true, false) != f.Faces["Bold"] || f.Face(false, true) != f.Faces["Regular"] || f.Face(true, true) != f.Faces["Regular"] {
		t.Fatal("wrong faces")
	}
	var none *Family
	if none.Face(true, true) != nil || none.Regular() != nil {
		t.Fatal("a nil family has faces")
	}
}

func TestGlyphPicksFirstDrawnMark(t *testing.T) {
	f := loraFamily(t, "Regular")
	if got := Glyph(f, '☐', '•', '-'); got != "•" {
		t.Errorf("glyph %q", got)
	}
	if got := Glyph(nil, '☐', '-'); got != "-" {
		t.Errorf("nil family glyph %q", got)
	}
}

func TestCheckboxesFallBackToArial(t *testing.T) {
	f := loraFamily(t, "Regular")
	if f.Regular().Has('☐') && f.Regular().Has('☒') {
		t.Skip("Lora draws the ballot boxes")
	}
	if open, done, font := Checkboxes(f); open != "□" || done != "■" || font != "Arial" {
		t.Errorf("checkboxes %q %q %q", open, done, font)
	}
	if open, done, font := Checkboxes(nil); open != "□" || done != "■" || font != "Arial" {
		t.Errorf("nil family: %q %q %q", open, done, font)
	}
	f.Regular().glyphs['☐'], f.Regular().glyphs['☒'] = true, true
	if open, done, font := Checkboxes(f); open != "☐" || done != "☒" || font != "Lora" {
		t.Errorf("family with boxes: %q %q %q", open, done, font)
	}
}

func TestFontFacesAreDeterministic(t *testing.T) {
	lora := loraFamily(t, "Regular", "Bold", "Italic", "BoldItalic")
	for slot, face := range lora.Faces {
		face.File = fontsDir + "/Lora-" + slot + ".ttf"
	}
	css := FontFaces("fonts/", lora, nil, lora)
	for range 5 {
		if again := FontFaces("fonts/", lora, nil, lora); again != css {
			t.Fatal("FontFaces changes between calls")
		}
	}
	want := `@font-face{font-family:"Lora";src:url("fonts/Lora-Bold.ttf");font-weight:700;font-style:normal}` +
		`@font-face{font-family:"Lora";src:url("fonts/Lora-BoldItalic.ttf");font-weight:700;font-style:italic}` +
		`@font-face{font-family:"Lora";src:url("fonts/Lora-Italic.ttf");font-weight:400;font-style:italic}` +
		`@font-face{font-family:"Lora";src:url("fonts/Lora-Regular.ttf");font-weight:400;font-style:normal}`
	if css != want {
		t.Fatalf("got  %s\nwant %s", css, want)
	}
	if FontFaces("x/") != "" {
		t.Error("no families gives rules")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func FuzzRead(f *testing.F) {
	for _, name := range []string{"Lora-Regular.ttf", "Go-Regular.ttf"} {
		data := fontData(f, name)
		f.Add(data)
		f.Add(data[:2048])
	}
	f.Add([]byte{0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 'h', 'e', 'a', 'd', 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 16})
	f.Add([]byte("OTTO"))
	f.Fuzz(func(t *testing.T, data []byte) {
		Tables(data)
		face, err := Read("x.ttf", data)
		if err != nil {
			return
		}
		face.Width("Hamburg 12 €", 10)
		face.Covers('A')
		face.Slot()
		EOT(face)
	})
}
