package book

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// SVGSize validates a self-contained SVG and returns its intrinsic dimensions.
func SVGSize(data []byte) (float64, float64, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	var w, h float64
	styleDepth := 0
	var style strings.Builder
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || t.Name.Local != "svg" {
					return 0, 0, fmt.Errorf("requires an svg root")
				}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "width":
						w, _ = svgLength(a.Value)
					case "height":
						h, _ = svgLength(a.Value)
					}
				}
				for _, a := range t.Attr {
					if a.Name.Local == "viewBox" {
						fields := strings.Fields(strings.ReplaceAll(a.Value, ",", " "))
						if len(fields) == 4 {
							w, _ = strconv.ParseFloat(fields[2], 64)
							h, _ = strconv.ParseFloat(fields[3], 64)
						}
					}
				}
			}
			depth++
			name := strings.ToLower(t.Name.Local)
			if name == "style" {
				styleDepth = depth
				style.Reset()
			}
			if name == "script" || name == "foreignobject" {
				return 0, 0, fmt.Errorf("%s is not allowed", t.Name.Local)
			}
			for _, a := range t.Attr {
				name := strings.ToLower(a.Name.Local)
				value := strings.TrimSpace(a.Value)
				if strings.HasPrefix(name, "on") {
					return 0, 0, fmt.Errorf("event attribute %s is not allowed", a.Name.Local)
				}
				if name == "href" && !svgReference(value) {
					return 0, 0, fmt.Errorf("external reference %q is not allowed", value)
				}
				if err := svgCSS(value); err != nil {
					return 0, 0, err
				}
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return 0, 0, fmt.Errorf("text outside the svg root")
			}
			if styleDepth > 0 {
				style.Write(t)
			}
		case xml.ProcInst:
			if t.Target == "xml-stylesheet" {
				return 0, 0, fmt.Errorf("external stylesheets are not allowed")
			}
		case xml.EndElement:
			if depth == styleDepth {
				if err := svgCSS(style.String()); err != nil {
					return 0, 0, err
				}
				styleDepth = 0
			}
			depth--
		}
	}
	if roots != 1 || depth != 0 || !(w > 0 && h > 0) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return 0, 0, fmt.Errorf("requires a positive viewBox or width and height")
	}
	return w, h, nil
}

func svgLength(value string) (float64, error) {
	for _, unit := range []struct {
		suffix string
		scale  float64
	}{{"px", 1}, {"pt", 96.0 / 72}, {"pc", 16}, {"mm", 96.0 / 25.4}, {"cm", 96.0 / 2.54}, {"in", 96}} {
		if strings.HasSuffix(value, unit.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, unit.suffix)), 64)
			return n * unit.scale, err
		}
	}
	return strconv.ParseFloat(strings.TrimSpace(value), 64)
}

func svgReference(value string) bool {
	return value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, "data:")
}

func svgCSS(value string) error {
	if strings.Contains(strings.ToLower(value), "@import") {
		return fmt.Errorf("CSS imports are not allowed")
	}
	for _, m := range cssURL.FindAllStringSubmatch(value, -1) {
		if !svgReference(strings.TrimSpace(m[2])) {
			return fmt.Errorf("external reference %q is not allowed", m[2])
		}
	}
	return nil
}
