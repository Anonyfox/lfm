package office

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Opened is a file as a Microsoft Office app for Mac opened it.
type Opened struct {
	Result string // what the script returned: Word's page count
	Data   []byte // what the app wrote: Word's PDF, Excel's recalculated workbook
	Error  string // why the app could not open it as is
}

// app is a Microsoft Office app for Mac and its sandbox, the one folder it
// reads and writes without asking the user.
type app struct {
	name, bundle, log, ext, out string
	script                      string
}

var (
	word = app{name: "Microsoft Word", bundle: "com.microsoft.Word", log: "WORD", ext: ".docx", out: ".pdf", script: `on run argv
	tell application "Microsoft Word"
		open POSIX file (item 1 of argv)
		delay 1
		set d to active document
		set n to (compute statistics d statistic statistic pages)
		save as d file name (item 2 of argv) file format format PDF
		close d saving no
		return n
	end tell
end run`}
	powerpoint = app{name: "Microsoft PowerPoint", bundle: "com.microsoft.Powerpoint", log: "POWERPOINT", ext: ".pptx", out: ".pdf", script: `on run argv
	tell application "Microsoft PowerPoint"
		open POSIX file (item 1 of argv)
		delay 1
		set p to active presentation
		set n to count slides of p
		save p in POSIX file (item 2 of argv) as save as PDF
		close p saving no
		return n
	end tell
end run`}
	excel = app{name: "Microsoft Excel", bundle: "com.microsoft.Excel", log: "EXCEL", ext: ".xlsx", out: ".xlsx", script: `on run argv
	tell application "Microsoft Excel"
		open POSIX file (item 1 of argv)
		delay 1
		set wb to active workbook
		calculate full
		save wb
		close wb saving no
		return "saved"
	end tell
end run`}
)

func (a app) data() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Containers", a.bundle, "Data")
}

func (a app) installed() bool {
	_, err := os.Stat("/Applications/" + a.name + ".app")
	info, derr := os.Stat(a.data())
	return err == nil && derr == nil && info.IsDir()
}

// HasWord reports whether Microsoft Word for Mac is installed.
func HasWord() bool { return word.installed() }

// HasPowerPoint reports whether Microsoft PowerPoint for Mac is installed.
func HasPowerPoint() bool { return powerpoint.installed() }

// OpenInPowerPoint opens each .pptx in PowerPoint as a buyer would and
// returns its slide count and PowerPoint's PDF of it.
func OpenInPowerPoint(files map[string][]byte) (map[string]Opened, error) {
	return powerpoint.open(files)
}

// HasExcel reports whether Microsoft Excel for Mac is installed.
func HasExcel() bool { return excel.installed() }

// OpenInWord opens each .docx in Word as a buyer would and returns its
// page count and Word's PDF of it.
func OpenInWord(files map[string][]byte) (map[string]Opened, error) { return word.open(files) }

// OpenInExcel opens each .xlsx in Excel, recalculates every formula and
// returns the workbook as Excel saved it, with Excel's values.
func OpenInExcel(files map[string][]byte) (map[string]Opened, error) { return excel.open(files) }

// open runs the app's script on each file, one at a time. The app shows a
// dialog for a file it has to repair; that is caught in its log and the
// app is quit at once, so no dialog waits for anyone.
func (a app) open(files map[string][]byte) (map[string]Opened, error) {
	if !a.installed() {
		return nil, errors.New(a.name + " for Mac is not installed")
	}
	dir := filepath.Join(a.data(), "lfm-check")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	script := filepath.Join(dir, "open.applescript")
	if err := os.WriteFile(script, []byte(a.script), 0o644); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	out := map[string]Opened{}
	for i, name := range names {
		in := filepath.Join(dir, fmt.Sprintf("%02d%s", i, a.ext))
		target := strings.TrimSuffix(in, a.ext) + "-out" + a.out
		if a.out == a.ext {
			target = in // saved in place
		}
		if err := os.WriteFile(in, files[name], 0o644); err != nil {
			return nil, err
		}
		out[name] = a.run(script, in, target)
	}
	return out, nil
}

func (a app) run(script, in, target string) Opened {
	start := time.Now().UTC()
	cmd := exec.Command("osascript", script, in, target)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		return Opened{Error: err.Error()}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.After(60 * time.Second)
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				return Opened{Error: strings.TrimSpace(stderr.String())}
			}
			data, _ := os.ReadFile(target)
			return Opened{Result: strings.TrimSpace(stdout.String()), Data: data}
		case <-tick.C:
			if why := a.refused(start); why != "" {
				cmd.Process.Kill()
				a.quit()
				return Opened{Error: why}
			}
		case <-deadline:
			cmd.Process.Kill()
			a.quit()
			return Opened{Error: a.name + " did not finish within 60 seconds"}
		}
	}
}

// quit ends the app, closing any dialog it shows.
func (a app) quit() {
	exec.Command("pkill", "-x", a.name).Run()
	time.Sleep(time.Second)
}

// refusal is a log line of an app refusing a file: Word failing to open
// the package, or any app alerting about a problem it offers to repair.
var refusal = regexp.MustCompile(`Failed to open (?:docx|pptx) package|Alert.*TraceText":"([^"]*(problem|unreadable|repair|recover)[^"]*)`)

// refused reads the app's diagnostic log for a refusal since start.
func (a app) refused(start time.Time) string {
	logs, _ := filepath.Glob(filepath.Join(a.data(), "Library", "Logs", "Diagnostics", a.log, "*.log"))
	sort.Slice(logs, func(i, j int) bool { return modTime(logs[i]).After(modTime(logs[j])) })
	if len(logs) == 0 {
		return ""
	}
	data, err := os.ReadFile(logs[0])
	if err != nil {
		return ""
	}
	since := start.Truncate(time.Second)
	for _, line := range strings.Split(string(data), "\n") {
		// Lines start with an MM/DD/YYYY stamp, which only sorts as a time.
		if len(line) < 19 {
			continue
		}
		at, err := time.ParseInLocation("01/02/2006 15:04:05", line[:19], start.Location())
		if err != nil || at.Before(since) {
			continue
		}
		if m := refusal.FindStringSubmatch(line); m != nil {
			if m[1] != "" {
				return a.name + " refused the file: " + m[1]
			}
			return a.name + " could not open the package and offered to repair it"
		}
	}
	return ""
}

func modTime(p string) time.Time {
	info, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
