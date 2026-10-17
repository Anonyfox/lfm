// Package imaging reads bounded image metadata, never pixel buffers or networks.
package imaging

import (
	"bytes"
	"encoding/xml"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strconv"
	"strings"

	"github.com/bep/imagemeta"
)

// Info contains metadata dimensions. Unknown/Issue makes them unsuitable for
// inferred layout, but known pixel counts still support absolute asset budgets.
type Info struct {
	Source  string `json:"source"`
	Format  string `json:"format,omitempty"`
	Width   int    `json:"intrinsicWidth,omitempty"`
	Height  int    `json:"intrinsicHeight,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	Raster  bool   `json:"raster,omitempty"`
	Issue   string `json:"issue,omitempty"`
	Unknown string `json:"unknown,omitempty"`
}

// Decision corresponds to one HTML img in document order. It carries source
// attribution through URL packing into the exact-final-byte validation stage.
type Decision struct {
	Info
	Added                         []string `json:"added,omitempty"`
	Reason                        string   `json:"reason"`
	Candidates                    []Info   `json:"candidates,omitempty"`
	AuthoredWidth, AuthoredHeight int      `json:"-"`
	CompareSize                   bool     `json:"-"`
}

const metadataReadLimit = 512 * 1024

var errBudget = errors.New("image metadata exceeds the bounded inspection budget")

type boundedReader struct {
	*bytes.Reader
	left, operations int
	exhausted        bool
}

func (r *boundedReader) Read(p []byte) (int, error) {
	r.operations--
	if r.left <= 0 || r.operations <= 0 {
		r.exhausted = true
		return 0, errBudget
	}
	p = p[:min(len(p), r.left)]
	n, err := r.Reader.Read(p)
	r.left -= n
	return n, err
}
func (r *boundedReader) Seek(offset int64, whence int) (int64, error) {
	r.operations--
	if r.operations <= 0 {
		r.exhausted = true
		return 0, errBudget
	}
	return r.Reader.Seek(offset, whence)
}

func Inspect(raw []byte) (info Info) {
	info.Bytes = len(raw)
	var format imagemeta.ImageFormat
	switch {
	case bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")):
		info.Format, format = "png", imagemeta.PNG
	case bytes.HasPrefix(raw, []byte("\xff\xd8")):
		info.Format, format = "jpeg", imagemeta.JPEG
	case bytes.HasPrefix(raw, []byte("GIF87a")) || bytes.HasPrefix(raw, []byte("GIF89a")):
		info.Format, format = "gif", imagemeta.GIF
	case len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		info.Format, format = "webp", imagemeta.WebP
	case bytes.HasPrefix(bytes.TrimSpace(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))), []byte("<")):
		return inspectSVG(raw)
	default:
		info.Unknown = "unsupported or unrecognizable image header; dimensions were left authored"
		return
	}
	info.Raster = true
	if format != imagemeta.WebP {
		config, _, err := image.DecodeConfig(io.LimitReader(bytes.NewReader(raw), metadataReadLimit))
		// GIF headers may declare a zero logical screen; never report half a size.
		if err != nil || config.Width <= 0 || config.Height <= 0 {
			info.Issue = "image header cannot be read; re-export the source image"
			return
		}
		info.Width, info.Height = config.Width, config.Height
	}
	if format == imagemeta.GIF {
		return
	}
	r := &boundedReader{Reader: bytes.NewReader(raw), left: metadataReadLimit, operations: 4096}
	orientation, seen, uncertain, requested := uint16(1), false, false, false
	tags := 0
	sources := imagemeta.EXIF
	if format == imagemeta.WebP {
		sources |= imagemeta.CONFIG
	}
	result, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: format, Sources: sources, LimitNumTags: 1024, LimitTagSize: 32,
		ShouldHandleTag: func(tag imagemeta.TagInfo) bool {
			tags++
			if tags > 512 {
				uncertain = true
			}
			if tag.Namespace == "IFD0" && tag.Tag == "Orientation" {
				requested = true
				return true
			}
			return false
		},
		HandleTag: func(tag imagemeta.TagInfo) error {
			value, ok := tag.Value.(uint16)
			if !ok || seen || value < 1 || value > 8 {
				uncertain = true
			} else {
				orientation = value
				seen = true
			}
			return nil
		},
		Warnf: func(string, ...any) { uncertain = true },
	})
	if format == imagemeta.WebP {
		info.Width, info.Height = result.ImageConfig.Width, result.ImageConfig.Height
	}
	if err != nil || uncertain || requested && !seen || r.exhausted || info.Width <= 0 || info.Height <= 0 {
		info.Issue = "image dimensions/orientation could not be established from bounded metadata; re-export with normalized orientation"
		info.Width, info.Height = 0, 0
		return
	}
	if orientation != 1 && format != imagemeta.JPEG {
		info.Unknown = "non-JPEG EXIF orientation is not portable across browsers; dimensions were left authored"
		return
	}
	if orientation >= 5 {
		info.Width, info.Height = info.Height, info.Width
	}
	return
}

func inspectSVG(raw []byte) (info Info) {
	info.Bytes, info.Format = len(raw), "svg"
	if len(raw) > metadataReadLimit {
		info.Unknown = "SVG exceeds the bounded XML inspection budget"
		return
	}
	d := xml.NewDecoder(bytes.NewReader(raw))
	root, depth, unsafe := false, 0, false
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			info.Issue = "SVG XML cannot be read; fix or re-export the source"
			info.Width, info.Height = 0, 0
			return
		}
		switch t := token.(type) {
		case xml.ProcInst:
			if t.Target != "xml" {
				unsafe = true
			}
		case xml.Directive:
			unsafe = true
		case xml.StartElement:
			depth++
			if !root {
				root = true
				if t.Name.Local != "svg" || t.Name.Space != "http://www.w3.org/2000/svg" {
					info.Unknown = "XML is not an SVG image with the SVG namespace"
					return
				}
				seen := map[string]bool{}
				for _, a := range t.Attr {
					if a.Name.Space != "" {
						continue
					}
					switch a.Name.Local {
					case "width":
						if seen["width"] {
							unsafe = true
						}
						seen["width"] = true
						info.Width = svgPixels(a.Value)
					case "height":
						if seen["height"] {
							unsafe = true
						}
						seen["height"] = true
						info.Height = svgPixels(a.Value)
					case "style":
						unsafe = true
					}
				}
			} else if depth == 1 {
				unsafe = true
			}
			switch t.Name.Local {
			case "style", "script", "animate", "animateTransform", "set":
				unsafe = true
			}
		case xml.EndElement:
			depth--
		}
	}
	if unsafe || !root || info.Width <= 0 || info.Height <= 0 {
		info.Width, info.Height = 0, 0
		info.Unknown = "SVG has no unambiguous fixed pixel dimensions; viewBox alone is an aspect ratio, not a pixel size"
	}
	return
}

func svgPixels(value string) int {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "px"))
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || n <= 0 || n > 1<<30 || n != float64(int(n)) {
		return 0
	}
	return int(n)
}

// PositiveAttribute deliberately rejects zero, signed/relative/malformed values.
// Their presence is author intent; inference must never repair or replace them.
func PositiveAttribute(value string) int {
	if value == "" {
		return 0
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0
		}
	}
	n, err := strconv.ParseUint(value, 10, 31)
	if err != nil || n == 0 {
		return 0
	}
	return int(n)
}
