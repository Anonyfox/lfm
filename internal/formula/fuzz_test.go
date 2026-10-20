package formula

import (
	"testing"
	"time"
)

// Property: parsing never panics; a parsed formula prints to source that
// parses back to the same print, and evaluates over a small grid without
// panicking.
func FuzzParseEval(f *testing.F) {
	for _, src := range []string{
		`=SUM(Log!B2:B7)`, `=SUMIFS(B2:B7,A2:A7,"Food")`, `=SUMPRODUCT((A2:A7="Food")*B2:B7)`,
		`=INDEX(A2:B7,2,2)`, `=VLOOKUP("Fun",A2:B7,2,FALSE)`, `=IFERROR(1/0,"none")`, `=2^3^2`, `=-2^2`, `=10%*50`,
		`=EOMONTH(DATE(2028,1,31),1)-DATE(2028,2,1)`, `=ROUND(PMT(0.05/12,36,10000),2)`, `=REPT("█",3)&LEN("Größe")`,
		`=SEARCH("ö?e","Größe")&SEARCH("~?","a?b")`, `=IFS(B2>0,"in",B2<0,"out")`, `='a''b'!$A$1:B$2`, `=MID("abc",2,)`,
		`=CHOOSE(2,A1,B1)`, `=MATCH("rent",A2:A7,0)`, `=#N/A`, `="say ""hi"""`, `=SUBSTITUTE("aa","a","bb")`,
		`=SUM(A1:XFD1048576)`, `=SUMPRODUCT(A1:XFD1*A1:A1048576)`, `=SUBSTITUTE(REPT("a",30000),"a","bb")`, `=MID("abc",1,1E+300)`,
		`=CHOOSE(1E+300,1)`, `="inf"*1`, `=YEAR(DATE(2300,1,1))`,
	} {
		f.Add(src)
	}
	g := book()
	today := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, src string) {
		n, err := Parse(src, nil)
		if err != nil {
			return
		}
		printed := Print(n)
		again, err := Parse(printed, nil)
		if err != nil {
			t.Fatalf("%q printed %q, which does not parse: %v", src, printed, err)
		}
		if p := Print(again); p != printed {
			t.Fatalf("%q printed %q, then %q", src, printed, p)
		}
		Eval(n, &Context{Book: g, Sheet: "Log", Row: 3, Today: today, Unportable: func(string) {}})
	})
}
