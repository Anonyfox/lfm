package typeface

import (
	"compress/flate"
	"sync"

	"github.com/Anonyfox/lfm/internal/zipx"
)

// Packed is a font part deflated once for every file that embeds it.
type Packed = zipx.Packed

// Pack compresses each embedded face once per compile: every file of a
// set embeds the same fonts, and deflating them is most of a build.
type Pack struct {
	mu    sync.Mutex
	faces map[packKey]*Packed
	once  map[packKey]*sync.Once
	errs  map[packKey]error
}

// packKey is a face in one of its embedded forms.
type packKey struct {
	face *Face
	eot  bool
}

// NewPack is an empty pack.
func NewPack() *Pack {
	return &Pack{faces: map[packKey]*Packed{}, once: map[packKey]*sync.Once{}, errs: map[packKey]error{}}
}

// Obfuscated is the face obfuscated with key and deflated, as a .docx
// embeds it.
func (p *Pack) Obfuscated(face *Face, key string) *Packed {
	out, _ := p.pack(packKey{face, false}, func() ([]byte, error) { return Obfuscate(face.Data, key), nil })
	return out
}

// EOT is the face as Embedded OpenType, deflated, as a .pptx embeds it.
func (p *Pack) EOT(face *Face) (*Packed, error) {
	return p.pack(packKey{face, true}, func() ([]byte, error) { return EOT(face) })
}

func (p *Pack) pack(k packKey, form func() ([]byte, error)) (*Packed, error) {
	p.mu.Lock()
	once := p.once[k]
	if once == nil {
		once = &sync.Once{}
		p.once[k] = once
	}
	p.mu.Unlock()
	once.Do(func() {
		raw, err := form()
		if err != nil {
			p.mu.Lock()
			p.errs[k] = err
			p.mu.Unlock()
			return
		}
		packed := zipx.Pack(raw, flate.BestSpeed) // fonts gain little from slower levels
		p.mu.Lock()
		p.faces[k] = &packed
		p.mu.Unlock()
	})
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.faces[k], p.errs[k]
}
