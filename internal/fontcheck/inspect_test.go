package fontcheck

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"testing"
)

// Minimal directory fixtures exercise the container screen, not payload decoding.
func woffFixture(v2 bool) []byte {
	if v2 {
		b := make([]byte, 53)
		copy(b, "wOF2")
		binary.BigEndian.PutUint32(b[4:], 0x10000)
		binary.BigEndian.PutUint32(b[8:], uint32(len(b)))
		binary.BigEndian.PutUint16(b[12:], 1)
		binary.BigEndian.PutUint32(b[16:], 32)
		binary.BigEndian.PutUint32(b[20:], 3)
		b[48] = 47
		b[49] = 4
		return b
	}
	b := make([]byte, 68)
	copy(b, "wOFF")
	binary.BigEndian.PutUint32(b[4:], 0x10000)
	binary.BigEndian.PutUint32(b[8:], uint32(len(b)))
	binary.BigEndian.PutUint16(b[12:], 1)
	binary.BigEndian.PutUint32(b[16:], 32)
	copy(b[44:], "fvar")
	binary.BigEndian.PutUint32(b[48:], 64)
	binary.BigEndian.PutUint32(b[52:], 4)
	binary.BigEndian.PutUint32(b[56:], 4)
	return b
}

func TestInspectContainers(t *testing.T) {
	raw, err := os.ReadFile("testdata/Go-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	i := Inspect(raw)
	if i.Issue != "" || i.Format != "truetype" || i.Variable || i.Bytes != len(raw) {
		t.Fatal(i)
	}
	woff2, err := os.ReadFile("testdata/Go-Regular.woff2")
	if err != nil {
		t.Fatal(err)
	}
	i = Inspect(woff2)
	if i.Issue != "" || i.Format != "woff2" || i.Variable || i.SFNTBytes != uint32(len(raw)) {
		t.Fatal(i)
	}
	for _, v2 := range []bool{false, true} {
		i := Inspect(woffFixture(v2))
		if i.Issue != "" || !i.Variable || i.SFNTBytes != 32 {
			t.Fatal(i)
		}
	}
	i = Inspect([]byte("<html>not a font</html>"))
	if i.Issue == "" {
		t.Fatal(i)
	}
}

func TestMalformedFontBounds(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		valid := woffFixture(v2)
		for n := 0; n < len(valid); n++ {
			if i := Inspect(valid[:n]); i.Issue == "" {
				t.Fatalf("accepted truncation %d: %+v", n, i)
			}
		}
		meta := 24
		if v2 {
			meta = 28
		}
		for _, offset := range []int{8, 12, meta} {
			b := append([]byte{}, valid...)
			binary.BigEndian.PutUint32(b[offset:], 0xffffffff)
			i := Inspect(b)
			if i.Issue == "" && i.Unknown == "" {
				t.Fatal(offset, i)
			}
		}
	}
	b := woffFixture(true)
	binary.BigEndian.PutUint32(b[16:], 0xffffffff)
	if i := Inspect(b); i.Issue != "" {
		t.Fatal("informational WOFF2 size rejected", i)
	}
	b[14] = 1
	if i := Inspect(b); i.Issue != "" {
		t.Fatal("WOFF2 reserved rejected", i)
	}
	b = woffFixture(false)
	b[14] = 1
	if i := Inspect(b); i.Issue == "" {
		t.Fatal("WOFF reserved accepted")
	}
	b = woffFixture(true)
	b[49] = 0x80
	if i := Inspect(b); i.Issue == "" {
		t.Fatal("nonminimal base128 accepted")
	}
}

func TestDataAndFormats(t *testing.T) {
	i := Data("data:font/woff2;base64," + base64.StdEncoding.EncodeToString(woffFixture(true)))
	if i.Issue != "" || i.Format != "woff2" {
		t.Fatal(i)
	}
	if Data("data:font/woff2;base64,!!!").Issue == "" || Data("data:font/woff2,%xy").Issue == "" {
		t.Fatal("bad encodings accepted")
	}
	for _, pair := range [][2]string{{"opentype", "truetype"}, {"truetype-variations", "opentype"}, {"woff2-variations", "woff2"}} {
		if !MatchesFormat(pair[0], pair[1]) {
			t.Fatal(pair)
		}
	}
	if MatchesFormat("woff", "woff2") {
		t.Fatal("mismatch")
	}
}

func FuzzInspect(f *testing.F) {
	f.Add(woffFixture(false))
	f.Add(woffFixture(true))
	f.Add([]byte("ttcf"))
	f.Add([]byte("OTTO"))
	f.Fuzz(func(t *testing.T, b []byte) { Inspect(b) })
}

func BenchmarkInspectWOFF2(b *testing.B) {
	raw := woffFixture(true)
	for b.Loop() {
		Inspect(raw)
	}
}
