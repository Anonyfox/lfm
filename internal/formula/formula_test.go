package formula

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// grid is a test book: cells by "Sheet!A1".
type grid map[string]Value

func (g grid) Cell(sheet string, col, row int) Value { return g[sheet+"!"+CellName(col, row)] }

func book() grid {
	g := grid{}
	cats := []string{"Food", "Rent", "Food", "Fun", "food", ""}
	amounts := []float64{-12.5, -800, -30.25, -20, -5, 1500}
	for i := range cats {
		if cats[i] != "" {
			g[fmt.Sprintf("Log!A%d", i+2)] = TextValue(cats[i])
		}
		g[fmt.Sprintf("Log!B%d", i+2)] = NumberValue(amounts[i])
		g[fmt.Sprintf("Log!C%d", i+2)] = NumberValue(Serial(time.Date(2026, time.Month(9+i%2), 3+i, 0, 0, 0, 0, time.UTC)))
	}
	g["Log!D2"] = TextValue("12")
	g["Log!E2"], g["Log!E3"], g["Log!E4"] = TextValue("Äpfel"), TextValue("ÄPFEL"), TextValue("Apfel")
	return g
}

func eval(t *testing.T, g grid, src string) Value {
	t.Helper()
	n, err := Parse(src, nil)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return Eval(n, &Context{Book: g, Sheet: "Log", Today: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)})
}

func TestEvaluatesLikeTheApps(t *testing.T) {
	g := book()
	cases := map[string]string{
		`=SUM(Log!B2:B7)`:                                                            "632.25",
		`=SUMIFS(B2:B7,A2:A7,"Food")`:                                                "-47.75", // case-insensitive: Food, Food, food
		`=SUMIF(A2:A7,"F*",B2:B7)`:                                                   "-67.75",
		`=SUMIFS(B2:B7,B2:B7,"<0",A2:A7,"<>Rent")`:                                   "-67.75",
		`=COUNTIFS(B2:B7,">=-20",B2:B7,"<0")`:                                        "3",
		`=COUNTIF(A2:A7,"")`:                                                         "1",
		`=COUNTIFS(A2:A7,"")&COUNTIFS(A2:A7,"=")&COUNTIFS(A2:A7,"=FOOD")`:            "113",
		`=SUMIFS(B2:B7,A2:A7,"food",B2:B7,"<-10")`:                                   "-42.75", // the index picks the rows, the second criterion tests them
		`=COUNTIFS(E2:E7,"äpfel")&COUNTIFS(E2:E7,"ÄPF?L")&COUNTIFS(E2:E7,"<>äpfel")`: "224",
		`=COUNTIFS(A2:A7,">E")&COUNTIFS(B2:B7,"-800")&COUNTIFS(D2:D7,"12")`:          "510",
		`=COUNTIF(A2:A7,"<>")`:                                                       "5",
		`=AVERAGEIFS(B2:B7,A2:A7,"Food")`:                                            "-15.9166666666667",
		`=SUMPRODUCT((A2:A7="Food")*B2:B7)`:                                          "-47.75",
		`=SUMPRODUCT(--(B2:B7<0))`:                                                   "5",
		`=ROUND(2.5,0)&" "&ROUND(-2.5,0)&" "&ROUND(1.005,2)`:                         "3 -3 1.01",
		`=ROUNDUP(1.21,1)+ROUNDDOWN(-1.29,1)`:                                        "0.1",
		`=INDEX(A2:B7,2,2)`:                                                          "-800",
		`=MATCH("rent",A2:A7,0)`:                                                     "2",
		`=VLOOKUP("Fun",A2:B7,2,FALSE)`:                                              "-20",
		`=IFERROR(1/0,"none")`:                                                       "none",
		`=IF(D2>10,"big","small")`:                                                   "big", // text that reads as a number compares as text in Excel: "12">10 is TRUE
		`=YEAR(C2)*100+MONTH(C2)`:                                                    "202609",
		`=EOMONTH(DATE(2028,1,31),1)-DATE(2028,2,1)`:                                 "28",
		`=EDATE(DATE(2026,1,31),1)`:                                                  "46081",
		`=DATE(2026,13,1)`:                                                           "46388",
		`=WEEKDAY(DATE(2026,10,19),2)`:                                               "1",
		`=ISOWEEKNUM(DATE(2027,1,1))`:                                                "53",
		`=TODAY()`:                                                                   "46314",
		`=ROUND(PMT(0.05/12,36,10000),2)`:                                            "-299.71",
		`=ROUND(NPER(0.01,-200,5000),2)`:                                             "28.91",
		`=ROUND(FV(0.04/12,120,-100),2)`:                                             "14724.98",
		`=LEFT("Budget",3)&MID("Budget",4,2)&RIGHT("Budget")`:                        "Budget",
		`=REPT("█",3)&LEN("Größe")`:                                                  "███5",
		`=SEARCH("ö?e","Größe")&SEARCH("~?","a?b")`:                                  "32",
		`=SEARCH("ge*","Budget")`:                                                    "4",
		`=MAXIFS(B2:B7,A2:A7,"Food")+MINIFS(B2:B7,A2:A7,"Food")`:                     "-35.25",
		`=IFS(B2>0,"in",B2<0,"out")`:                                                 "out",
		`=AND(TRUE,B2<0)&OR(FALSE,B3>0)&NOT(B2>0)`:                                   "TRUEFALSETRUE",
		`=COUNTA(A2:A7)&"/"&COUNTBLANK(A2:A7)&"/"&COUNT(A2:B7)`:                      "5/1/6",
		`=MOD(-3,5)`:             "2",
		`=2^3^2`:                 "512",
		`=-2^2`:                  "4",
		`=10%*50`:                "5",
		`=1/0`:                   "#DIV/0!",
		`=SUM(B2,"x")`:           "#VALUE!",
		`=MATCH("nope",A2:A7,0)`: "#N/A",
	}
	for src, want := range cases {
		got := eval(t, g, src)
		if got.String() != want {
			t.Errorf("%s = %s, want %s", src, got, want)
		}
	}
}

func TestPrintsTheStoredSyntax(t *testing.T) {
	resolve := func(name string) (Node, error) {
		switch name {
		case "Log.amount":
			return Area{Sheet: "Buchungen 2027", From: Ref{Col: 2, Row: 2, AbsCol: true, AbsRow: true}, To: Ref{Col: 2, Row: 1001, AbsCol: true, AbsRow: true}}, nil
		case ".amount":
			return Ref{Col: 2, Row: 5}, nil
		}
		return nil, fmt.Errorf("unknown %s", name)
	}
	for src, want := range map[string]string{
		`=SUMIFS([Log.amount],[Log.amount],"<0")`: `SUMIFS('Buchungen 2027'!$B$2:$B$1001,'Buchungen 2027'!$B$2:$B$1001,"<0")`,
		`=IFS([.amount]>0, "in", TRUE, "out")`:    `_xlfn.IFS(B5>0,"in",TRUE,"out")`,
		`=maxifs(Log!A1:A3, Log!B1:B3, ">1")`:     `_xlfn.MAXIFS(Log!A1:A3,Log!B1:B3,">1")`,
		`=(1+2)*3-(4-5)`:                          `(1+2)*3-(4-5)`,
		`="say ""hi"""`:                           `"say ""hi"""`,
	} {
		n, err := Parse(src, resolve)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got := Print(n); got != want {
			t.Errorf("%s printed %s, want %s", src, got, want)
		}
	}
	for _, bad := range []string{`=XLOOKUP(1,A1:A2,B1:B2)`, `=FILTER(A1:A9,B1:B9>0)`, `=TEXT(A1,"dd.mm.yyyy")`, `=SUM(A1;A2)`, `=SUM(A1`, `=[nope]`, `=ROUND(1)`} {
		if _, err := Parse(bad, resolve); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestSerialsMatchExcel(t *testing.T) {
	if s := Serial(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)); s != 46314 {
		t.Fatal(s)
	}
	if d := Date(46314); !d.Equal(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)) {
		t.Fatal(d)
	}
	if !strings.HasPrefix(formatNumber(1.0/3), "0.333333333333333") || formatNumber(1e20) != "1e+20" || formatNumber(math.Copysign(0, -1)) != "0" {
		t.Fatal(formatNumber(1.0 / 3))
	}
}

func TestDefinableKeepsNamesThatAreNotAddresses(t *testing.T) {
	for name, want := range map[string]bool{
		"income": true, "per_day": true, "month_end": true, "goal": true, "x": true,
		"tax1": false, "ab12": false, "r": false, "c": false, "r1c1": false, "c3": false, "true": false, "sum": false, "xfe1": true,
	} {
		if Definable(name) != want {
			t.Errorf("Definable(%q) = %v", name, !want)
		}
	}
}
