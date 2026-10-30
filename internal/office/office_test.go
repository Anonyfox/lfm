package office

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFindHonoursTurnedOff(t *testing.T) {
	for _, v := range []string{"none", "off"} {
		t.Setenv("LFM_SOFFICE", v)
		p, err := Find()
		if p != "" || !Missing(err) {
			t.Errorf("LFM_SOFFICE=%s: %q, %v", v, p, err)
		}
	}
}

func TestFindHonoursExplicitPath(t *testing.T) {
	t.Setenv("LFM_SOFFICE", "/opt/libreoffice/program/soffice")
	if p, err := Find(); p != "/opt/libreoffice/program/soffice" || err != nil {
		t.Fatalf("%q, %v", p, err)
	}
}

func TestFindLooksInPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs an executable without extension")
	}
	dir := t.TempDir()
	soffice := filepath.Join(dir, "soffice")
	if err := os.WriteFile(soffice, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LFM_SOFFICE", "")
	t.Setenv("PATH", dir)
	if p, err := Find(); p != soffice || err != nil {
		t.Fatalf("%q, %v", p, err)
	}
}

func TestMissing(t *testing.T) {
	if !Missing(errNotFound) || !Missing(fmt.Errorf("check: %w", errNotFound)) {
		t.Error("not found is not missing")
	}
	if Missing(nil) || Missing(errors.New("LibreOffice is not installed")) {
		t.Error("another error is missing")
	}
}

// fakeSoffice is a script that converts like soffice: every input file
// becomes <name>.<filter> in --outdir.
func fakeSoffice(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell script")
	}
	path := filepath.Join(t.TempDir(), "soffice")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

const converts = `filter=""; out=""
while [ $# -gt 0 ]; do
  case "$1" in
    --convert-to) filter="$2"; shift ;;
    --outdir) out="$2"; mkdir -p "$out"; shift ;;
    -*) ;;
    *) base=$(basename "$1"); cp "$1" "$out/${base%.*}.$filter" ;;
  esac
  shift
done
`

func TestConvertReturnsEveryWrittenFile(t *testing.T) {
	got, err := Convert(context.Background(), fakeSoffice(t, converts), map[string][]byte{"a.docx": []byte("one"), "b.pptx": []byte("two")}, "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || string(got["a.pdf"]) != "one" || string(got["b.pdf"]) != "two" {
		t.Fatalf("got %v", got)
	}
}

func TestConvertReportsFailures(t *testing.T) {
	if _, err := Convert(context.Background(), fakeSoffice(t, "echo conversion-failed >&2; exit 3\n"), map[string][]byte{"a.docx": nil}, "pdf"); err == nil || (!strings.Contains(err.Error(), "LibreOffice: exit status 3") || !strings.Contains(err.Error(), "conversion-failed")) {
		t.Errorf("failing run: %v", err)
	}
	if _, err := Convert(context.Background(), filepath.Join(t.TempDir(), "absent"), nil, "pdf"); err == nil || !strings.Contains(err.Error(), "start LibreOffice") {
		t.Errorf("absent binary: %v", err)
	}
}

func TestConvertWithoutOutputIsEmpty(t *testing.T) {
	got, err := Convert(context.Background(), fakeSoffice(t, "exit 0\n"), map[string][]byte{"a.docx": nil}, "pdf")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestRefusalMatchesWordAndPowerPointLogLines(t *testing.T) {
	for _, tc := range []struct{ line, text string }{
		{`10/19/2026 12:00:01.123 WORD[1234:5678] [Doc] Failed to open docx package: error 0x80004005`, ""},
		{`10/19/2026 12:00:01.123 POWERPOINT[1234:5678] Failed to open pptx package`, ""},
		{`10/19/2026 12:00:02.000 WORD[1234] Alert {"Id":"1","TraceText":"Word found unreadable content in 00.docx. Do you want to recover the contents?"}`, "Word found unreadable content in 00.docx. Do you want to recover the contents?"},
		{`10/19/2026 12:00:02.000 POWERPOINT[1] Alert {"TraceText":"PowerPoint found a problem with content in 00.pptx. PowerPoint can attempt to repair the presentation."}`, "PowerPoint found a problem with content in 00.pptx. PowerPoint can attempt to repair the presentation."},
		{`10/19/2026 12:00:02.000 EXCEL[1] Alert {"TraceText":"We found a problem with some content in 00.xlsx."}`, "We found a problem with some content in 00.xlsx."},
	} {
		m := refusal.FindStringSubmatch(tc.line)
		if m == nil {
			t.Errorf("no match: %s", tc.line)
			continue
		}
		if m[1] != tc.text {
			t.Errorf("captured %q, want %q", m[1], tc.text)
		}
	}
}

func TestRefusalIgnoresOrdinaryLines(t *testing.T) {
	for _, line := range []string{
		`10/19/2026 12:00:01.123 WORD[1234] Opened docx package 00.docx`,
		`10/19/2026 12:00:01.123 WORD[1234] Failed to open xlsx package`,
		`10/19/2026 12:00:02.000 WORD[1234] Alert {"TraceText":"Do you want to save the changes?"}`,
		`10/19/2026 12:00:02.000 WORD[1234] Telemetry {"TraceText":"repair skipped"}`,
		`10/19/2026 12:00:02.000 WORD[1234] Recovered 3 autosave files`,
		``,
	} {
		if refusal.MatchString(line) {
			t.Errorf("matched: %s", line)
		}
	}
}

// diagnostics makes the app's log folder below a fake home.
func diagnostics(t *testing.T, a app) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Library", "Containers", a.bundle, "Data", "Library", "Logs", "Diagnostics", a.log)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeLog(t *testing.T, path string, mod time.Time, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func stamp(t time.Time) string { return t.UTC().Format("01/02/2006 15:04:05.000") }

func TestRefusedReadsOnlyLinesAfterStart(t *testing.T) {
	dir := diagnostics(t, word)
	start := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	log := filepath.Join(dir, "word.log")
	writeLog(t, log, start,
		stamp(start.Add(-time.Minute))+` WORD[1] Failed to open docx package`,
		stamp(start.Add(-time.Second))+` WORD[1] Alert {"TraceText":"Word found unreadable content in old.docx."}`,
		stamp(start.Add(time.Second))+` WORD[1] Opened 00.docx`,
		"short line",
	)
	if why := word.refused(start); why != "" {
		t.Fatalf("an earlier refusal counted: %q", why)
	}
	writeLog(t, log, start.Add(time.Minute),
		stamp(start.Add(-time.Minute))+` WORD[1] Failed to open docx package`,
		stamp(start.Add(2*time.Second))+` WORD[1] Alert {"TraceText":"Word found unreadable content in 00.docx."}`,
	)
	if why := word.refused(start); why != "Microsoft Word refused the file: Word found unreadable content in 00.docx." {
		t.Fatalf("refused %q", why)
	}
	writeLog(t, log, start.Add(time.Minute), stamp(start)+` WORD[1] Failed to open docx package`)
	if why := word.refused(start); why != "Microsoft Word could not open the package and offered to repair it" {
		t.Fatalf("refused %q", why)
	}
}

func TestRefusedReadsTheNewestLog(t *testing.T) {
	dir := diagnostics(t, powerpoint)
	start := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	writeLog(t, filepath.Join(dir, "a.log"), start.Add(-time.Hour), stamp(start.Add(time.Second))+` POWERPOINT[1] Failed to open pptx package`)
	writeLog(t, filepath.Join(dir, "b.log"), start.Add(time.Second), stamp(start.Add(time.Second))+` POWERPOINT[1] Opened 00.pptx`)
	if why := powerpoint.refused(start); why != "" {
		t.Fatalf("an older log counted: %q", why)
	}
	writeLog(t, filepath.Join(dir, "a.log"), start.Add(time.Minute), stamp(start.Add(time.Second))+` POWERPOINT[1] Failed to open pptx package`)
	if why := powerpoint.refused(start); !strings.HasPrefix(why, "Microsoft PowerPoint could not open") {
		t.Fatalf("refused %q", why)
	}
}

func TestRefusedWithoutLogs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if why := excel.refused(time.Now()); why != "" {
		t.Fatalf("refused %q", why)
	}
}

func TestRefusedIgnoresLastYearsLines(t *testing.T) {
	dir := diagnostics(t, word)
	start := time.Date(2027, 1, 1, 0, 0, 5, 0, time.UTC)
	writeLog(t, filepath.Join(dir, "word.log"), start,
		stamp(time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC))+` WORD[1] Failed to open docx package`)
	if why := word.refused(start); why != "" {
		t.Fatalf("refused %q", why)
	}
}

func TestAppsNeedTheirContainer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if HasWord() || HasPowerPoint() || HasExcel() {
		t.Fatal("an app counts as installed without its container")
	}
	for name, open := range map[string]func(map[string][]byte) (map[string]Opened, error){"Word": OpenInWord, "PowerPoint": OpenInPowerPoint, "Excel": OpenInExcel} {
		if got, err := open(map[string][]byte{"a": nil}); err == nil || got != nil || !strings.Contains(err.Error(), "for Mac is not installed") {
			t.Errorf("%s: %v, %v", name, got, err)
		}
	}
}

func TestAppsKeepTheirFilesInTheirSandbox(t *testing.T) {
	t.Setenv("HOME", "/Users/test")
	for _, a := range []app{word, powerpoint, excel} {
		if got, want := a.data(), "/Users/test/Library/Containers/"+a.bundle+"/Data"; got != want {
			t.Errorf("%s: %s, want %s", a.name, got, want)
		}
		if !strings.Contains(a.script, `tell application "`+a.name+`"`) {
			t.Errorf("%s: the script tells another app", a.name)
		}
	}
}

// A missing regional locale makes LibreOffice encode non-ASCII sheet filenames
// with the C locale. Linux's built-in C.UTF-8 must work without locale generation.
func TestConvertUsesAvailableUnicodeLocale(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux locale fallback")
	}
	t.Setenv("LC_ALL", "lfm_missing_locale.UTF-8")
	body := `test "$(locale charmap)" = UTF-8 || exit 2
` + converts
	got, err := Convert(context.Background(), fakeSoffice(t, body), map[string][]byte{"Übersicht.xlsx": []byte("cells")}, "csv")
	if err != nil {
		t.Fatal(err)
	}
	if string(got["Übersicht.csv"]) != "cells" {
		t.Fatalf("Unicode output lost: %v", got)
	}
}
