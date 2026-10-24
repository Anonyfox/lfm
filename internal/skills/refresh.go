package skills

import (
	"strconv"
	"strings"
)

// Refresh rewrites the untouched copies (state Outdated) that an older lfm
// release wrote, so every folder an agent loads from carries this binary's
// skills. Copies a project changed, foreign files, copies from a newer lfm and
// builds without a release version are left alone. It returns the copies with
// their new state and the changes it made.
func Refresh(copies []Copy, data Data) ([]Copy, []Change, error) {
	if !isRelease(data.Version) {
		return copies, nil, nil
	}
	current, err := byName(data)
	if err != nil {
		return copies, nil, err
	}
	out := make([]Copy, len(copies))
	copy(out, copies)
	var changes []Change
	for i, c := range out {
		skill, ok := current[c.Name]
		if !ok || c.State != Outdated || !older(c.Version, data.Version) {
			continue
		}
		if err := write(c.Path, skill.Text); err != nil {
			return out, changes, err
		}
		changes = append(changes, Change{Name: c.Name, Path: c.Path, Action: "updated", Reason: "was lfm " + c.Version})
		out[i].Version, out[i].State = data.Version, Current
	}
	return out, changes, nil
}

// isRelease reports whether v is a plain release version such as 0.1.9.
func isRelease(v string) bool {
	_, ok := parseVersion(v)
	return ok
}

// older reports whether release version a precedes release version b. An
// unparseable a counts as older, since only lfm writes the stamp.
func older(a, b string) bool {
	pb, ok := parseVersion(b)
	if !ok {
		return false
	}
	pa, ok := parseVersion(a)
	if !ok {
		return true
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
