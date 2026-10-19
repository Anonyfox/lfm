// Package buildid names the lfm binary that is running, for the keys of
// the files it writes: a file built by another lfm, whose writer may
// differ, is never taken for one this lfm would write.
package buildid

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
)

// Writer identifies this binary: its module version and VCS revision, or,
// for a build with local changes or none recorded (go test, go run), the
// executable's size and modification time, which change with every build.
var Writer = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	revision, modified := "", false
	if ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				modified = s.Value == "true"
			}
		}
	}
	if ok && revision != "" && !modified {
		return info.Main.Version + "+" + revision
	}
	if exe, err := os.Executable(); err == nil {
		if st, err := os.Stat(exe); err == nil {
			return fmt.Sprintf("local+%d+%d", st.Size(), st.ModTime().UnixNano())
		}
	}
	return "unknown"
})
