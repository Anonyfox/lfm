package book

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/Anonyfox/lfm/internal/zipx"
)

// verifyEPUB checks the package the way reading systems depend on it: the
// mimetype entry first, stored and without an extra field, a container pointing at the package
// document, every manifest item present and every file declared, spine entries
// that exist, a navigation document, unique IDs per document and every href and
// src inside the content resolving to a packaged file.
func verifyEPUB(data []byte) error {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if len(z.File) == 0 || z.File[0].Name != "mimetype" || z.File[0].Method != zip.Store {
		return fmt.Errorf("the mimetype entry must come first and uncompressed")
	}
	// Store validators read the local headers too: no extra field on
	// mimetype, no data descriptors.
	if problems := zipx.Problems(data); len(problems) > 0 {
		return fmt.Errorf("the archive: %s", strings.Join(problems, "; "))
	}
	files := map[string][]byte{}
	for _, f := range z.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		files[f.Name] = b
	}
	if string(files["mimetype"]) != "application/epub+zip" {
		return fmt.Errorf("mimetype must be application/epub+zip")
	}
	var container struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(files["META-INF/container.xml"], &container); err != nil || len(container.Rootfiles) != 1 {
		return fmt.Errorf("META-INF/container.xml must name one package document")
	}
	opfPath := container.Rootfiles[0].FullPath
	var pkg struct {
		Manifest []struct {
			ID    string `xml:"id,attr"`
			Href  string `xml:"href,attr"`
			Media string `xml:"media-type,attr"`
			Props string `xml:"properties,attr"`
		} `xml:"manifest>item"`
		Spine []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"spine>itemref"`
		Modified []struct {
			Property string `xml:"property,attr"`
			Value    string `xml:",chardata"`
		} `xml:"metadata>meta"`
	}
	if err := xml.Unmarshal(files[opfPath], &pkg); err != nil {
		return fmt.Errorf("%s: %w", opfPath, err)
	}
	base := path.Dir(opfPath)
	declared := map[string]string{} // zip path -> media type
	ids := map[string]bool{}
	nav := false
	for _, it := range pkg.Manifest {
		full := path.Join(base, it.Href)
		if _, ok := files[full]; !ok {
			return fmt.Errorf("manifest item %s is missing from the package", it.Href)
		}
		if ids[it.ID] {
			return fmt.Errorf("manifest id %s is used twice", it.ID)
		}
		ids[it.ID] = true
		declared[full] = it.Media
		if strings.Contains(" "+it.Props+" ", " nav ") {
			nav = true
		}
	}
	if !nav {
		return fmt.Errorf("the package has no navigation document")
	}
	modified := false
	for _, m := range pkg.Modified {
		modified = modified || m.Property == "dcterms:modified" && strings.HasSuffix(m.Value, "Z")
	}
	if !modified {
		return fmt.Errorf("the package has no dcterms:modified date")
	}
	for name := range files {
		if name == "mimetype" || strings.HasPrefix(name, "META-INF/") || name == opfPath {
			continue
		}
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("%s is packaged but not in the manifest", name)
		}
	}
	for _, ref := range pkg.Spine {
		if !ids[ref.IDRef] {
			return fmt.Errorf("spine item %s is not in the manifest", ref.IDRef)
		}
	}
	for name, media := range declared {
		if media != "application/xhtml+xml" {
			continue
		}
		if err := checkReferences(name, files[name], files); err != nil {
			return err
		}
	}
	return nil
}

// checkReferences resolves every href and src of one XHTML document and
// checks that its IDs are unique.
func checkReferences(name string, doc []byte, files map[string][]byte) error {
	d := xml.NewDecoder(bytes.NewReader(doc))
	d.Strict = true
	seen := map[string]bool{}
	dir := path.Dir(name)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, a := range el.Attr {
			switch a.Name.Local {
			case "id":
				if seen[a.Value] {
					return fmt.Errorf("%s uses the id %q twice", name, a.Value)
				}
				seen[a.Value] = true
			case "href", "src":
				v := a.Value
				if v == "" || strings.HasPrefix(v, "#") || strings.Contains(v, "://") || strings.HasPrefix(v, "mailto:") {
					continue
				}
				file, _, _ := strings.Cut(v, "#")
				if _, ok := files[path.Join(dir, file)]; !ok {
					return fmt.Errorf("%s references %s, which is not in the package", name, v)
				}
			}
		}
	}
}
