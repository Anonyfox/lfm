package printable

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Calendar is a run of months for planner pages, computed rather than typed:
// month lengths, leap years, weekdays and ISO week numbers come from the
// standard library. Every month, week and day has an ID to link to.
type Calendar struct {
	Start    time.Weekday // the first day of every week row
	Weekdays []Weekday    // the week's day names from Start
	Months   []Month
	Weeks    []Week // every week touching the range, in order
	Days     []Day  // every day of the range
}

// Weekday is a day name in the calendar's language.
type Weekday struct{ Name, Short string }

// Month is one month with its calendar grid.
type Month struct {
	ID          string // m2027-01
	Year, Num   int
	Name, Short string
	Rows        []Week // the grid: weeks from the one holding the 1st to the one holding the last day
	Days        []Day
}

// Week is seven days from the calendar's start day.
type Week struct {
	ID     string // w2027-05: ISO year and week of the week's Thursday
	Number int    // ISO week number (KW)
	Year   int    // ISO year
	Days   []Day
}

// Day is one date. InRange is false for days a month grid shows from the
// neighbouring months.
type Day struct {
	ID      string // d2027-01-05
	Date    time.Time
	Day     int
	Month   int
	Year    int
	Weekday Weekday
	Week    string // the ID of its week
	InRange bool
}

// IsoDate is the date as 2027-01-05.
func (d Day) IsoDate() string { return d.Date.Format("2006-01-02") }

// NewCalendar builds months from the first day of from (2027 or 2027-08) for
// the given number of months (12 by default), with weeks starting on start
// (monday or sunday) and names in language.
func NewCalendar(from, start string, months int, language string) (Calendar, error) {
	year, month := 0, 1
	y, m, hasMonth := strings.Cut(from, "-")
	var err error
	if year, err = strconv.Atoi(y); err != nil || year < 1900 || year > 2200 {
		return Calendar{}, fmt.Errorf("calendar: %q is not a year like 2027 or a month like 2027-08", from)
	}
	if hasMonth {
		if month, err = strconv.Atoi(m); err != nil || month < 1 || month > 12 {
			return Calendar{}, fmt.Errorf("calendar: %q is not a month like 2027-08", from)
		}
	}
	if months == 0 {
		months = 12
	}
	if months < 1 || months > 36 {
		return Calendar{}, fmt.Errorf("calendar: %d months; use 1 to 36", months)
	}
	var first time.Weekday
	switch strings.ToLower(start) {
	case "monday", "mon", "":
		first = time.Monday
	case "sunday", "sun":
		first = time.Sunday
	default:
		return Calendar{}, fmt.Errorf("calendar: start %q is not monday or sunday", start)
	}
	names := monthNames(language)
	days := weekdayNames(language)
	c := Calendar{Start: first}
	for i := range 7 {
		c.Weekdays = append(c.Weekdays, days[(int(first)+i)%7])
	}
	begin := time.Date(year, time.Month(month), 1, 12, 0, 0, 0, time.UTC)
	end := begin.AddDate(0, months, 0) // exclusive
	day := func(t time.Time) Day {
		return Day{ID: t.Format("d2006-01-02"), Date: t, Day: t.Day(), Month: int(t.Month()), Year: t.Year(),
			Weekday: days[t.Weekday()], Week: weekID(t, first), InRange: !t.Before(begin) && t.Before(end)}
	}
	weekOf := func(t time.Time) Week {
		back := (int(t.Weekday()) - int(first) + 7) % 7
		s := t.AddDate(0, 0, -back)
		w := Week{ID: weekID(s, first)}
		w.Year, w.Number = thursday(s, first).ISOWeek()
		for i := range 7 {
			w.Days = append(w.Days, day(s.AddDate(0, 0, i)))
		}
		return w
	}
	for t := begin; t.Before(end); t = t.AddDate(0, 0, 1) {
		c.Days = append(c.Days, day(t))
		if len(c.Weeks) == 0 || c.Weeks[len(c.Weeks)-1].ID != weekID(t, first) {
			c.Weeks = append(c.Weeks, weekOf(t))
		}
	}
	for t := begin; t.Before(end); t = t.AddDate(0, 1, 0) {
		next := t.AddDate(0, 1, 0)
		mo := Month{ID: t.Format("m2006-01"), Year: t.Year(), Num: int(t.Month()), Name: names[t.Month()-1].Name, Short: names[t.Month()-1].Short}
		for d := t; d.Before(next); d = d.AddDate(0, 0, 1) {
			mo.Days = append(mo.Days, day(d))
		}
		for w := weekOf(t); ; {
			for i := range w.Days {
				w.Days[i].InRange = w.Days[i].Month == mo.Num
			}
			mo.Rows = append(mo.Rows, w)
			following := w.Days[6].Date.AddDate(0, 0, 1)
			if !following.Before(next) {
				break
			}
			w = weekOf(following)
		}
		c.Months = append(c.Months, mo)
	}
	return c, nil
}

// weekID names the week holding t by the ISO week of its Thursday, so Monday
// and Sunday weeks of the same span share an ID.
func weekID(t time.Time, first time.Weekday) string {
	back := (int(t.Weekday()) - int(first) + 7) % 7
	y, w := thursday(t.AddDate(0, 0, -back), first).ISOWeek()
	return fmt.Sprintf("w%d-%02d", y, w)
}

// thursday is the Thursday of the week starting at s.
func thursday(s time.Time, first time.Weekday) time.Time {
	return s.AddDate(0, 0, (int(time.Thursday)-int(first)+7)%7)
}

type name struct{ Name, Short string }

var months = map[string][12]name{
	"en": {{"January", "Jan"}, {"February", "Feb"}, {"March", "Mar"}, {"April", "Apr"}, {"May", "May"}, {"June", "Jun"}, {"July", "Jul"}, {"August", "Aug"}, {"September", "Sep"}, {"October", "Oct"}, {"November", "Nov"}, {"December", "Dec"}},
	"de": {{"Januar", "Jan"}, {"Februar", "Feb"}, {"März", "Mär"}, {"April", "Apr"}, {"Mai", "Mai"}, {"Juni", "Jun"}, {"Juli", "Jul"}, {"August", "Aug"}, {"September", "Sep"}, {"Oktober", "Okt"}, {"November", "Nov"}, {"Dezember", "Dez"}},
	"fr": {{"janvier", "janv."}, {"février", "févr."}, {"mars", "mars"}, {"avril", "avr."}, {"mai", "mai"}, {"juin", "juin"}, {"juillet", "juil."}, {"août", "août"}, {"septembre", "sept."}, {"octobre", "oct."}, {"novembre", "nov."}, {"décembre", "déc."}},
	"es": {{"enero", "ene"}, {"febrero", "feb"}, {"marzo", "mar"}, {"abril", "abr"}, {"mayo", "may"}, {"junio", "jun"}, {"julio", "jul"}, {"agosto", "ago"}, {"septiembre", "sept"}, {"octubre", "oct"}, {"noviembre", "nov"}, {"diciembre", "dic"}},
	"it": {{"gennaio", "gen"}, {"febbraio", "feb"}, {"marzo", "mar"}, {"aprile", "apr"}, {"maggio", "mag"}, {"giugno", "giu"}, {"luglio", "lug"}, {"agosto", "ago"}, {"settembre", "set"}, {"ottobre", "ott"}, {"novembre", "nov"}, {"dicembre", "dic"}},
	"pt": {{"janeiro", "jan"}, {"fevereiro", "fev"}, {"março", "mar"}, {"abril", "abr"}, {"maio", "mai"}, {"junho", "jun"}, {"julho", "jul"}, {"agosto", "ago"}, {"setembro", "set"}, {"outubro", "out"}, {"novembro", "nov"}, {"dezembro", "dez"}},
	"nl": {{"januari", "jan"}, {"februari", "feb"}, {"maart", "mrt"}, {"april", "apr"}, {"mei", "mei"}, {"juni", "jun"}, {"juli", "jul"}, {"augustus", "aug"}, {"september", "sep"}, {"oktober", "okt"}, {"november", "nov"}, {"december", "dec"}},
	"pl": {{"styczeń", "sty"}, {"luty", "lut"}, {"marzec", "mar"}, {"kwiecień", "kwi"}, {"maj", "maj"}, {"czerwiec", "cze"}, {"lipiec", "lip"}, {"sierpień", "sie"}, {"wrzesień", "wrz"}, {"październik", "paź"}, {"listopad", "lis"}, {"grudzień", "gru"}},
}

// weekdays start on Sunday, as time.Weekday does.
var weekdays = map[string][7]name{
	"en": {{"Sunday", "Sun"}, {"Monday", "Mon"}, {"Tuesday", "Tue"}, {"Wednesday", "Wed"}, {"Thursday", "Thu"}, {"Friday", "Fri"}, {"Saturday", "Sat"}},
	"de": {{"Sonntag", "So"}, {"Montag", "Mo"}, {"Dienstag", "Di"}, {"Mittwoch", "Mi"}, {"Donnerstag", "Do"}, {"Freitag", "Fr"}, {"Samstag", "Sa"}},
	"fr": {{"dimanche", "dim."}, {"lundi", "lun."}, {"mardi", "mar."}, {"mercredi", "mer."}, {"jeudi", "jeu."}, {"vendredi", "ven."}, {"samedi", "sam."}},
	"es": {{"domingo", "dom"}, {"lunes", "lun"}, {"martes", "mar"}, {"miércoles", "mié"}, {"jueves", "jue"}, {"viernes", "vie"}, {"sábado", "sáb"}},
	"it": {{"domenica", "dom"}, {"lunedì", "lun"}, {"martedì", "mar"}, {"mercoledì", "mer"}, {"giovedì", "gio"}, {"venerdì", "ven"}, {"sabato", "sab"}},
	"pt": {{"domingo", "dom"}, {"segunda-feira", "seg"}, {"terça-feira", "ter"}, {"quarta-feira", "qua"}, {"quinta-feira", "qui"}, {"sexta-feira", "sex"}, {"sábado", "sáb"}},
	"nl": {{"zondag", "zo"}, {"maandag", "ma"}, {"dinsdag", "di"}, {"woensdag", "wo"}, {"donderdag", "do"}, {"vrijdag", "vr"}, {"zaterdag", "za"}},
	"pl": {{"niedziela", "nd"}, {"poniedziałek", "pn"}, {"wtorek", "wt"}, {"środa", "śr"}, {"czwartek", "cz"}, {"piątek", "pt"}, {"sobota", "sb"}},
}

func base(language string) string {
	b, _, _ := strings.Cut(strings.ToLower(language), "-")
	return b
}

func monthNames(language string) [12]name {
	if m, ok := months[base(language)]; ok {
		return m
	}
	return months["en"]
}

func weekdayNames(language string) [7]Weekday {
	src, ok := weekdays[base(language)]
	if !ok {
		src = weekdays["en"]
	}
	var out [7]Weekday
	for i, n := range src {
		out[i] = Weekday(n)
	}
	return out
}
