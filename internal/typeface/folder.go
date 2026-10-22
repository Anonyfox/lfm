package typeface

import (
	"fmt"
	"os"
	"strings"

	"github.com/Anonyfox/lfm/internal/project"
)

// Folder is the fonts a deliverable ships in its fonts folder: the
// families by name and the licence files that travel with them.
type Folder struct {
	Dir      string
	Families map[string]*Family
	Licences map[string][]byte
}

// Report receives a finding about a font file: an error when fatal.
type Report func(file string, line int, fatal bool, message, hint string)

// Load reads every font and licence file in dir, a folder below the
// project; a face lfm cannot embed is reported and left out.
func Load(p project.Project, dir string, report Report) Folder {
	out := Folder{Dir: dir, Families: map[string]*Family{}, Licences: map[string][]byte{}}
	path, err := p.Path(dir)
	if err != nil {
		return out
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		report(dir, 1, true, err.Error()+".", "Make the folder readable.")
		return out
	}
	for _, e := range entries {
		name := e.Name()
		file := dir + "/" + name
		lower := strings.ToLower(name)
		switch {
		case e.IsDir():
		case strings.HasSuffix(lower, ".ttf") || strings.HasSuffix(lower, ".otf"):
			data, err := p.Read(file)
			if err != nil {
				report(file, 1, true, err.Error()+".", "Make the file readable inside the project.")
				continue
			}
			face, err := Read(file, data)
			if err != nil {
				report(file, 1, true, fmt.Sprintf("%s is not a font lfm can read: %v.", name, err), "Download the static TrueType files of the family from Google Fonts.")
				continue
			}
			if face.Variable {
				report(file, 1, true, fmt.Sprintf("%s is a variable font.", name), fmt.Sprintf("Office apps need the static files: one per weight and style, %s-Regular.ttf, %s-Bold.ttf and so on.", fileFamily(face.Family), fileFamily(face.Family)))
				continue
			}
			if !face.Embeddable {
				report(file, 1, true, fmt.Sprintf("%s forbids embedding.", name), "Use a Google Fonts family under the Open Font License; those allow embedding.")
				continue
			}
			f := out.Families[face.Family]
			if f == nil {
				f = &Family{Name: face.Family, Faces: map[string]*Face{}}
				out.Families[face.Family] = f
			}
			if _, dup := f.Faces[face.Slot()]; dup {
				report(file, 1, false, fmt.Sprintf("%s is a second %s %s face.", name, face.Family, face.Slot()), "Keep one file per weight and style.")
				continue
			}
			f.Faces[face.Slot()] = face
		case strings.HasPrefix(lower, "ofl") || strings.HasPrefix(lower, "license") || strings.HasPrefix(lower, "licence"):
			data, err := p.Read(file)
			if err != nil {
				report(file, 1, true, err.Error()+".", "Make the file readable inside the project.")
				continue
			}
			out.Licences[name] = data
		}
	}
	if len(out.Families) > 0 && len(out.Licences) == 0 {
		report(dir, 1, true, "The fonts have no licence file.", "Add the family's OFL.txt to "+dir+"; the Open Font License requires it with every copy, and lfm ships it with the fonts.")
	}
	return out
}

// Want is the family a theme names for role (text or heading), reported
// at the concept's Look line when it is missing or lacks a face.
func (f Folder) Want(name, role, file string, line int, report Report) *Family {
	fam := f.Families[name]
	hint := fmt.Sprintf("Download the static files of %s from Google Fonts into %s: the regular, bold and italic faces (curl the TrueType links of https://fonts.googleapis.com/css2?family=%s:ital,wght@0,400;0,700;1,400 ), with its OFL.txt.", name, f.Dir, strings.ReplaceAll(name, " ", "+"))
	switch {
	case fam == nil || fam.Regular() == nil:
		report(file, line, true, fmt.Sprintf("The %s font %s is not in %s.", role, name, f.Dir), hint)
		return nil
	case fam.Faces["Bold"] == nil:
		report(file, line, false, fmt.Sprintf("%s has no bold face.", name), hint)
	case role == "text" && fam.Faces["Italic"] == nil:
		report(file, line, false, fmt.Sprintf("%s has no italic face.", name), hint)
	}
	return fam
}

// fileFamily is a family as static font files name it, or a slot when the
// file names none.
func fileFamily(family string) string {
	if family = strings.ReplaceAll(strings.TrimSpace(family), " ", ""); family == "" {
		return "<Family>"
	}
	return family
}
