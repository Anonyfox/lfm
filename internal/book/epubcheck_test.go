package book

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/testtmp"
	"testing"
	"time"
)

func buildChecked(t *testing.T, r Report, read Reader, modified time.Time) (Built, error) {
	t.Helper()
	b, err := Build(r, read, modified)
	if err == nil {
		t.Run("epubcheck", func(t *testing.T) {
			if !testtmp.EPUBCheck(t, b.EPUB) {
				t.Skip("epubcheck unavailable")
			}
		})
	}
	return b, err
}

func TestSVGValidation(t *testing.T) {
	for _, tc := range []struct {
		svg   string
		valid bool
	}{
		{`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 175"><path d="M0 0"/></svg>`, true},
		{`<svg width="10" height="20"><use href="#x"/></svg>`, true},
		{`<svg width="10" height="20"><image href="data:image/png;base64,AA"/></svg>`, true},
		{`<svg width="10cm" height="17.5cm"><style>rect {fill:url(#paint)}</style></svg>`, true},
		{`<svg width="1" height="1"><style>rect {fill:url(https://example.org/x)}</style></svg>`, false},
		{`<svg width="1" height="1"><rect fill="url(other.svg)"/></svg>`, false},
		{`<svg width="Inf" height="Inf"/>`, false},
		{`text<svg width="1" height="1"/>`, false},
		{`<svg/>`, false}, {`<html/>`, false}, {`<svg width="1" height="1"><script/></svg>`, false},
		{`<svg width="1" height="1" onload="x"/>`, false},
		{`<svg width="1" height="1"><foreignObject/></svg>`, false},
		{`<svg width="1" height="1"><use href="other.svg#x"/></svg>`, false},
		{`<svg width="1" height="1"><use href="https://example.org/x"/></svg>`, false},
	} {
		_, _, err := SVGSize([]byte(tc.svg))
		if (err == nil) != tc.valid {
			t.Errorf("%s: %v", tc.svg, err)
		}
	}
}

func FuzzSVG(f *testing.F) {
	f.Add([]byte(`<svg viewBox="0 0 100 175"/>`))
	f.Fuzz(func(t *testing.T, data []byte) { _, _, _ = SVGSize(data) })
}

func TestFictionMatterEPUB(t *testing.T) {
	p, voices, peers := fixture(t)
	r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	r.Config.Kind = "fiction"
	r.Plan.Matter = []Matter{{Part: "Dedication", Place: "front", File: "matter/dedication.md", Exists: true}, {Part: "Afterword", Place: "back", File: "matter/afterword.md", Exists: true}}
	r.Sources["matter/dedication.md"] = []byte("# Dedication\n\nFor everyone.\n")
	r.Sources["matter/afterword.md"] = []byte("# Afterword\n\nA closing note.\n")
	r.Sources["images/tall.svg"] = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 175"><rect width="100" height="175" fill="red"/></svg>`)
	r.Sources[r.Plan.Chapters[0].File] = append(r.Sources[r.Plan.Chapters[0].File], []byte("\n![Tall diagram](../images/tall.svg)\n")...)
	built, err := buildChecked(t, r, func(rel string) ([]byte, error) { return p.Read(Dir + "/" + rel) }, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(built.EPUB), int64(len(built.EPUB)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range z.File {
		rd, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(rd)
		rd.Close()
		if e != nil {
			t.Fatal(e)
		}
		files[f.Name] = string(data)
	}
	if !strings.Contains(files["OEBPS/text/matter01.xhtml"], `epub:type="dedication"`) || !strings.Contains(files["OEBPS/text/matter02.xhtml"], `epub:type="afterword"`) {
		t.Fatal("matter types missing")
	}
	for _, name := range []string{"OEBPS/text/nav.xhtml", "OEBPS/toc.ncx"} {
		text := files[name]
		if !strings.Contains(text, "Dedication") || strings.Index(text, "Dedication") > strings.Index(text, r.Plan.Chapters[0].Title) || strings.Index(text, "Afterword") < strings.Index(text, r.Plan.Chapters[2].Title) {
			t.Fatal(name, "matter order")
		}
	}
	for _, doc := range [][]byte{built.Print, built.Document(PrintOptions{Paper: true, Inside: 0.75})} {
		text := string(doc)
		front := strings.Index(text, `class="matter front"`)
		back := strings.Index(text, `class="matter back"`)
		chapter := strings.Index(text, `class="chapter"`)
		if front < 0 || front > chapter || back < chapter {
			t.Fatal("print matter order")
		}
		if strings.Contains(text, `class="chapter-label">Chapter -`) {
			t.Fatal("matter has a chapter label")
		}
	}
}

func TestChapterSVGFindings(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		missing    bool
	}{{"valid", `<svg viewBox="0 0 10 17.5"/>`, false}, {"invalid", `<svg viewBox="0 0 10 17.5"><script/></svg>`, false}, {"missing", "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			p, voices, peers := fixture(t)
			file := Dir + "/chapters/01.md"
			// Use the plan's source file rather than depending on fixture numbering.
			initial := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
			file = Dir + "/" + initial.Plan.Chapters[0].File
			source, err := p.Read(file)
			if err != nil {
				t.Fatal(err)
			}
			source = append(source, []byte("\n![Diagram](../images/check.svg)\n")...)
			writeFile(t, p.Root, file, source)
			if !tc.missing {
				writeFile(t, p.Root, Dir+"/images/check.svg", []byte(tc.data))
			}
			r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
			count := 0
			for _, d := range r.Diagnostics {
				if d.Severity == diagnostic.Error && (d.Rule == RuleImages || d.Rule == RuleChapter) {
					count++
					if d.Location.File != file || d.Location.Line < 2 {
						t.Fatal(d)
					}
				}
			}
			if tc.name == "valid" {
				if count != 0 || !r.Buildable {
					t.Fatal(r.Diagnostics)
				}
			} else if count != 1 || r.Buildable {
				t.Fatal(count, r.Buildable, r.Diagnostics)
			}
		})
	}
}

func TestMissingCoverOnce(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "open"}[open], func(t *testing.T) {
			p, voices, peers := fixture(t)
			if err := os.Remove(filepath.Join(p.Root, Dir, "images/cover.png")); err != nil {
				t.Fatal(err)
			}
			if open {
				raw, err := p.Read(ConceptFile)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, p.Root, ConceptFile, bytes.ReplaceAll(raw, []byte("the jar on a fridge shelf"), []byte("Open: waiting")))
			}
			r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
			count := 0
			for _, d := range r.Diagnostics {
				if strings.Contains(d.Message, "cover") && (d.Rule == RuleImages || d.Rule == RuleChannels) {
					count++
					if d.Message != "No cover yet: images/cover.png is missing; the stores need it." {
						t.Fatal(d)
					}
				}
			}
			if count != 1 {
				t.Fatal(count, r.Diagnostics)
			}
		})
	}
}

func TestEPUBReferenceSourceLine(t *testing.T) {
	p, voices, peers := fixture(t)
	r := Check(p, testConfig, true, nil, "https://example.org/", voices, peers)
	file := r.Plan.Chapters[0].File
	source := r.Sources[file]
	line := bytes.Count(source, []byte("\n")) + 2
	r.Sources[file] = append(source, []byte("\n[Missing part](missing.md)\n")...)
	_, err := Build(r, func(rel string) ([]byte, error) { return p.Read(Dir + "/" + rel) }, time.Unix(1700000000, 0))
	var located *SourceError
	if !errors.As(err, &located) || located.File != file || located.Line != line {
		t.Fatal(err, located, line)
	}
}
