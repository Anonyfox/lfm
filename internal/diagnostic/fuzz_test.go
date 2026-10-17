package diagnostic

import "testing"

// Property: the indexed Source.At agrees with the direct At scan for every offset.
func FuzzSourceAt(f *testing.F) {
	for _, raw := range []string{"", "a\nb\r\nc\rd", "é🐺\nend\n", "\r", "\r\n\r", "\n\n"} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			t.Skip()
		}
		indexed := NewSource("input", []byte(raw))
		for offset := -2; offset <= len(raw)+2; offset++ {
			if got, want := indexed.At(offset), At("input", []byte(raw), offset); got != want {
				t.Fatalf("offset %d: Source.At %+v, At %+v", offset, got, want)
			}
		}
	})
}
