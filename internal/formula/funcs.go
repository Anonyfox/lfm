package formula

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// function is one function of the portable set: what it takes, the prefix
// the xlsx file stores before its name, and how it computes.
type function struct {
	min, max int    // argument counts; max -1 is unbounded
	prefix   string // "_xlfn." for functions newer than Excel 2007
	fn       func(c *Context, args []Node) Value
}

func (f function) arity(n int) error {
	switch {
	case n < f.min:
		return fmt.Errorf("needs at least %d arguments, has %d", f.min, n)
	case f.max >= 0 && n > f.max:
		return fmt.Errorf("takes at most %d arguments, has %d", f.max, n)
	}
	return nil
}

// portableHint names the alternatives to the functions left out.
const portableHint = "INDEX/MATCH instead of XLOOKUP, SUMIFS or SUMPRODUCT instead of FILTER or QUERY, and cell formats instead of TEXT"

// Definable reports whether a name can be an Excel defined name: not a
// cell address (tax1 is column TAX, row 1), not R1C1 notation, not a
// logical value or a function's name. Google Sheets and LibreOffice accept
// the same names.
func Definable(name string) bool {
	lower := strings.ToLower(name)
	if !definedName.MatchString(lower) || lower == "true" || lower == "false" || rc.MatchString(lower) {
		return false
	}
	if m := cellLike.FindStringSubmatch(lower); m != nil && ColumnNumber(strings.ToUpper(m[1])) <= 16384 {
		return false
	}
	_, isFunction := functions[strings.ToUpper(lower)]
	return !isFunction
}

var (
	definedName = regexp.MustCompile(`^[a-z_][a-z0-9_.]{0,254}$`)
	cellLike    = regexp.MustCompile(`^([a-z]{1,3})([0-9]+)$`)
	rc          = regexp.MustCompile(`^(r[0-9]*c?[0-9]*|c[0-9]*)$`)
)

// Functions lists the portable function names, sorted.
func Functions() []string {
	names := make([]string, 0, len(functions))
	for n := range functions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

var functions map[string]function

func init() {
	functions = map[string]function{
		// Math and aggregation.
		"SUM":        {1, -1, "", aggregate(func(xs []float64) Value { return NumberValue(sum(xs)) })},
		"AVERAGE":    {1, -1, "", aggregate(average)},
		"MIN":        {1, -1, "", aggregate(func(xs []float64) Value { return NumberValue(extreme(xs, -1)) })},
		"MAX":        {1, -1, "", aggregate(func(xs []float64) Value { return NumberValue(extreme(xs, 1)) })},
		"COUNT":      {1, -1, "", count(func(v Value) bool { return v.Kind == Number })},
		"COUNTA":     {1, -1, "", count(func(v Value) bool { return v.Kind != Blank })},
		"COUNTBLANK": {1, 1, "", count(func(v Value) bool { return v.Kind == Blank || v.Kind == Text && v.Str == "" })},
		"SUMIF":      {2, 3, "", sumif},
		"SUMIFS":     {3, -1, "", ifs("sum")},
		"COUNTIF":    {2, 2, "", countif},
		"COUNTIFS":   {2, -1, "", ifs("count")},
		"AVERAGEIF":  {2, 3, "", averageif},
		"AVERAGEIFS": {3, -1, "", ifs("average")},
		"MAXIFS":     {3, -1, "_xlfn.", ifs("max")},
		"MINIFS":     {3, -1, "_xlfn.", ifs("min")},
		"SUMPRODUCT": {1, -1, "", sumproduct},
		"ROUND":      {2, 2, "", rounding(roundHalfAway)},
		"ROUNDUP":    {2, 2, "", rounding(func(f float64) float64 { return math.Copysign(math.Ceil(math.Abs(f)-1e-9), f) })},
		"ROUNDDOWN":  {2, 2, "", rounding(func(f float64) float64 { return math.Copysign(math.Floor(math.Abs(f)+1e-9), f) })},
		"ABS":        {1, 1, "", numeric1(math.Abs)},
		"INT":        {1, 1, "", numeric1(math.Floor)},
		"SQRT":       {1, 1, "", func(c *Context, a []Node) Value { return numberOp(c, a, func(x float64) Value { return sqrt(x) }) }},
		"MOD":        {2, 2, "", mod},
		"POWER":      {2, 2, "", func(c *Context, a []Node) Value { return binary("^", arg(c, a, 0), arg(c, a, 1)) }},
		// Logic.
		"IF":      {1, 3, "", ifFn},
		"IFS":     {2, -1, "_xlfn.", ifsFn},
		"IFERROR": {2, 2, "", iferror(func(v Value) bool { return v.Kind == Error })},
		"IFNA":    {2, 2, "_xlfn.", iferror(func(v Value) bool { return v.Kind == Error && v.Str == ErrNA })},
		"AND":     {1, -1, "", logic(true)},
		"OR":      {1, -1, "", logic(false)},
		"NOT":     {1, 1, "", notFn},
		"CHOOSE":  {2, -1, "", choose},
		// Lookup.
		"INDEX":   {2, 3, "", index},
		"MATCH":   {2, 3, "", match},
		"VLOOKUP": {3, 4, "", vlookup},
		"ROWS":    {1, 1, "", func(c *Context, a []Node) Value { r, _ := dims(arg(c, a, 0)); return NumberValue(float64(r)) }},
		"ROW":     {0, 0, "", func(c *Context, _ []Node) Value { return NumberValue(float64(c.Row)) }},
		// Dates.
		"DATE":       {3, 3, "", date},
		"YEAR":       {1, 1, "", datePart(func(t time.Time) int { return t.Year() })},
		"MONTH":      {1, 1, "", datePart(func(t time.Time) int { return int(t.Month()) })},
		"DAY":        {1, 1, "", datePart(func(t time.Time) int { return t.Day() })},
		"WEEKDAY":    {1, 2, "", weekday},
		"ISOWEEKNUM": {1, 1, "_xlfn.", datePart(func(t time.Time) int { _, w := t.ISOWeek(); return w })},
		"EOMONTH":    {2, 2, "", monthShift(true)},
		"EDATE":      {2, 2, "", monthShift(false)},
		"DAYS":       {2, 2, "_xlfn.", func(c *Context, a []Node) Value { return binary("-", dateArg(c, a, 0), dateArg(c, a, 1)) }},
		"TODAY": {0, 0, "", func(c *Context, _ []Node) Value {
			if c.Volatile != nil {
				*c.Volatile = true
			}
			return NumberValue(Serial(c.Today))
		}},
		// Money.
		"PMT":  {3, 5, "", pmt},
		"FV":   {3, 5, "", fv},
		"NPER": {3, 5, "", nper},
		// Text.
		"LEN": {1, 1, "", func(c *Context, a []Node) Value {
			return textOp(c, a, func(s string) Value { return NumberValue(float64(utf8.RuneCountInString(s))) })
		}},
		"LOWER": {1, 1, "", func(c *Context, a []Node) Value {
			return textOp(c, a, func(s string) Value { return TextValue(strings.ToLower(s)) })
		}},
		"UPPER": {1, 1, "", func(c *Context, a []Node) Value {
			return textOp(c, a, func(s string) Value { return TextValue(strings.ToUpper(s)) })
		}},
		"TRIM": {1, 1, "", func(c *Context, a []Node) Value {
			return textOp(c, a, func(s string) Value { return TextValue(strings.Join(strings.Fields(s), " ")) })
		}},
		"LEFT":        {1, 2, "", leftRight(true)},
		"RIGHT":       {1, 2, "", leftRight(false)},
		"MID":         {3, 3, "", mid},
		"REPT":        {2, 2, "", rept},
		"CONCATENATE": {1, -1, "", concatenate},
		"SUBSTITUTE":  {3, 3, "", substitute},
		"SEARCH":      {2, 3, "", find(true)},
		"FIND":        {2, 3, "", find(false)},
		// Information.
		"ISBLANK":  {1, 1, "", is(func(v Value) bool { return v.Kind == Blank })},
		"ISNUMBER": {1, 1, "", is(func(v Value) bool { return v.Kind == Number })},
		"ISTEXT":   {1, 1, "", is(func(v Value) bool { return v.Kind == Text })},
		"ISERROR":  {1, 1, "", is(func(v Value) bool { return v.Kind == Error })},
		"NA":       {0, 0, "", func(*Context, []Node) Value { return ErrorValue(ErrNA) }},
	}
}

func arg(c *Context, args []Node, i int) Value {
	if i >= len(args) || args[i] == nil {
		return Value{}
	}
	return Eval(args[i], c)
}

func present(args []Node, i int) bool { return i < len(args) && args[i] != nil }

// scalar takes the value of an argument as one value: a single-cell range
// is its cell; a larger range is #VALUE!.
func scalar(v Value) Value {
	if v.Kind == Array {
		if len(v.Rows) == 1 && len(v.Rows[0]) == 1 {
			return v.Rows[0][0]
		}
		return ErrorValue(ErrValue)
	}
	return v
}

func numArg(c *Context, args []Node, i int) (float64, Value, bool) {
	v := scalar(arg(c, args, i))
	f, e := toNumber(v)
	if e != "" {
		return 0, ErrorValue(e), false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, ErrorValue(ErrNum), false
	}
	return f, Value{}, true
}

// aggregate collects numbers as SUM does: from ranges only numbers count,
// a direct argument converts; any error is the result.
func aggregate(f func([]float64) Value) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		var xs []float64
		for i := range args {
			v := arg(c, args, i)
			if v.Kind == Array {
				for _, x := range flat(v) {
					switch x.Kind {
					case Error:
						return x
					case Number:
						xs = append(xs, x.Num)
					}
				}
				continue
			}
			n, e := toNumber(v)
			if e != "" {
				return ErrorValue(e)
			}
			xs = append(xs, n)
		}
		return f(xs)
	}
}

func sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

func average(xs []float64) Value {
	if len(xs) == 0 {
		return ErrorValue(ErrDiv0)
	}
	return NumberValue(sum(xs) / float64(len(xs)))
}

func extreme(xs []float64, sign float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if (x-m)*sign > 0 {
			m = x
		}
	}
	return m
}

func count(pred func(Value) bool) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		n := 0
		for i := range args {
			each(arg(c, args, i), func(v Value) {
				if pred(v) {
					n++
				}
			})
		}
		return NumberValue(float64(n))
	}
}

// criterion is a *IF(S) condition such as ">=100", "Food", "<>", "Fo*".
func criterion(v Value) func(Value) bool {
	v = scalar(v)
	if v.Kind != Text {
		return func(x Value) bool { return x.Kind != Blank && compare(x, v) == 0 }
	}
	s := v.Str
	op := "="
	for _, o := range []string{">=", "<=", "<>", "=", ">", "<"} {
		if strings.HasPrefix(s, o) {
			op, s = o, s[len(o):]
			break
		}
	}
	if n, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && s != "" {
		return func(x Value) bool {
			if x.Kind != Number {
				return op == "<>"
			}
			switch op {
			case "=":
				return x.Num == n
			case "<>":
				return x.Num != n
			case ">":
				return x.Num > n
			case "<":
				return x.Num < n
			case ">=":
				return x.Num >= n
			}
			return x.Num <= n
		}
	}
	if b := strings.ToUpper(s); b == "TRUE" || b == "FALSE" {
		want := b == "TRUE"
		return func(x Value) bool {
			eq := x.Kind == Logical && (x.Num != 0) == want
			if op == "<>" {
				return !eq
			}
			return eq
		}
	}
	switch op {
	case "=", "<>":
		var match func(string) bool
		switch {
		case s == "":
			match = func(t string) bool { return t == "" }
		case strings.ContainsAny(s, "*?"):
			re := wildcard(s)
			match = re.MatchString
		default:
			key := fold(s)
			match = func(t string) bool { return sameText(t, s, key) }
		}
		return func(x Value) bool {
			var t string
			switch x.Kind {
			case Blank:
				t = ""
			case Text:
				t = x.Str
			default:
				return op == "<>" && s != "" || op == "<>" && s == ""
			}
			if op == "<>" {
				return !match(t)
			}
			return match(t)
		}
	}
	return func(x Value) bool {
		if x.Kind != Text {
			return false
		}
		c := strings.Compare(strings.ToLower(x.Str), strings.ToLower(s))
		switch op {
		case ">":
			return c > 0
		case "<":
			return c < 0
		case ">=":
			return c >= 0
		}
		return c <= 0
	}
}

func wildcard(s string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("(?is)^")
	rs := []rune(s) // by character: an umlaut is one, as ? counts it
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; c {
		case '~':
			if i+1 < len(rs) {
				i++
				b.WriteString(regexp.QuoteMeta(string(rs[i])))
			}
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// ifs implements SUMIFS, COUNTIFS, AVERAGEIFS, MAXIFS and MINIFS: every
// criteria range has the size of the value range.
func ifs(mode string) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		start := 1
		var values []Value
		if mode == "count" {
			start = 0
		} else {
			values = flat(arg(c, args, 0))
		}
		if (len(args)-start)%2 != 0 {
			return ErrorValue(ErrValue)
		}
		type test struct {
			cells []Value
			ok    func(Value) bool
		}
		var tests []test
		var rows []int32 // the candidates a text index found
		indexed := false
		n := len(values)
		for i := start; i+1 < len(args); i += 2 {
			rv := arg(c, args, i)
			r := flat(rv)
			if i == start && values == nil {
				n = len(r)
			}
			if len(r) != n {
				return ErrorValue(ErrValue)
			}
			cv := arg(c, args, i+1)
			// One text criterion on a kept area picks the rows from its
			// index; the others test only those.
			if key, ok := textKey(cv); ok && !indexed && rv.cells != nil {
				rows, indexed = rv.cells.positions(key), true
				continue
			}
			tests = append(tests, test{r, criterion(cv)})
		}
		var kept []int32
		check := func(j int32) {
			for _, t := range tests {
				if !t.ok(t.cells[j]) {
					return
				}
			}
			kept = append(kept, j)
		}
		if indexed {
			for _, j := range rows {
				check(j)
			}
		} else {
			for j := range int32(n) {
				check(j)
			}
		}
		return finishAt(mode, values, kept)
	}
}

// textKey is the folded text a criterion matches exactly, as "Rent" or
// "=Rent" do; criteria with operators, wildcards, numbers or logicals
// have none.
func textKey(v Value) (string, bool) {
	v = scalar(v)
	if v.Kind != Text {
		return "", false
	}
	// The same prefixes criterion strips: any < or > is a comparison.
	if strings.HasPrefix(v.Str, "<") || strings.HasPrefix(v.Str, ">") {
		return "", false
	}
	s := strings.TrimPrefix(v.Str, "=")
	if strings.ContainsAny(s, "*?") {
		return "", false
	}
	if _, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && s != "" {
		return "", false
	}
	if b := strings.ToUpper(s); b == "TRUE" || b == "FALSE" {
		return "", false
	}
	return fold(s), true
}

// sameText compares t with s case-insensitively as fold does; key is
// fold(s). ASCII, the common case, needs no folded copy.
func sameText(t, s, key string) bool {
	if ascii(t) && ascii(s) {
		return strings.EqualFold(t, s)
	}
	return fold(t) == key
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// finishAt aggregates values at the kept positions, which ascend.
func finishAt(mode string, values []Value, kept []int32) Value {
	if mode == "count" {
		return NumberValue(float64(len(kept)))
	}
	xs := make([]float64, 0, len(kept))
	for _, j := range kept {
		switch values[j].Kind {
		case Error:
			return values[j]
		case Number:
			xs = append(xs, values[j].Num)
		}
	}
	switch mode {
	case "sum":
		return NumberValue(sum(xs))
	case "average":
		return average(xs)
	case "max":
		return NumberValue(extreme(xs, 1))
	}
	return NumberValue(extreme(xs, -1))
}

func finish(mode string, values []Value, keep []bool) Value {
	if mode == "count" {
		n := 0
		for _, k := range keep {
			if k {
				n++
			}
		}
		return NumberValue(float64(n))
	}
	var xs []float64
	for j, k := range keep {
		if !k {
			continue
		}
		switch values[j].Kind {
		case Error:
			return values[j]
		case Number:
			xs = append(xs, values[j].Num)
		}
	}
	switch mode {
	case "sum":
		return NumberValue(sum(xs))
	case "average":
		return average(xs)
	case "max":
		return NumberValue(extreme(xs, 1))
	}
	return NumberValue(extreme(xs, -1))
}

func sumif(c *Context, args []Node) Value {
	r := flat(arg(c, args, 0))
	values := r
	if present(args, 2) {
		values = flat(arg(c, args, 2))
		if len(values) != len(r) {
			return ErrorValue(ErrValue)
		}
	}
	ok := criterion(arg(c, args, 1))
	keep := make([]bool, len(r))
	for i, x := range r {
		keep[i] = ok(x)
	}
	return finish("sum", values, keep)
}

func averageif(c *Context, args []Node) Value {
	r := flat(arg(c, args, 0))
	values := r
	if present(args, 2) {
		values = flat(arg(c, args, 2))
	}
	ok := criterion(arg(c, args, 1))
	keep := make([]bool, len(r))
	for i, x := range r {
		keep[i] = ok(x) && i < len(values)
	}
	return finish("average", values, keep)
}

func countif(c *Context, args []Node) Value {
	ok := criterion(arg(c, args, 1))
	n := 0
	each(arg(c, args, 0), func(x Value) {
		if ok(x) {
			n++
		}
	})
	return NumberValue(float64(n))
}

func sumproduct(c *Context, args []Node) Value {
	var arrays []Value
	for i := range args {
		arrays = append(arrays, arg(c, args, i))
	}
	rows, cols := dims(arrays[0])
	for _, a := range arrays[1:] {
		if r, k := dims(a); r != rows || k != cols {
			return ErrorValue(ErrValue)
		}
	}
	total := 0.0
	for i := range rows {
		for j := range cols {
			p := 1.0
			for _, a := range arrays {
				v := at(a, i, j)
				switch v.Kind {
				case Error:
					return v
				case Number:
					p *= v.Num
				case Logical:
					// SUMPRODUCT treats logical values in arrays as 0 unless coerced (--).
					p *= 0
				default:
					p *= 0
				}
			}
			total += p
		}
	}
	return NumberValue(total)
}

func roundHalfAway(f float64) float64 { return math.Copysign(math.Floor(math.Abs(f)+0.5+1e-12), f) }

func rounding(r func(float64) float64) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		x, e, ok := numArg(c, args, 0)
		if !ok {
			return e
		}
		d, e, ok := numArg(c, args, 1)
		if !ok {
			return e
		}
		scale := math.Pow(10, math.Trunc(d))
		return NumberValue(r(x*scale) / scale)
	}
}

func numeric1(f func(float64) float64) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		return numberOp(c, args, func(x float64) Value { return NumberValue(f(x)) })
	}
}

func numberOp(c *Context, args []Node, f func(float64) Value) Value {
	x, e, ok := numArg(c, args, 0)
	if !ok {
		return e
	}
	return f(x)
}

func sqrt(x float64) Value {
	if x < 0 {
		return ErrorValue(ErrNum)
	}
	return NumberValue(math.Sqrt(x))
}

func mod(c *Context, args []Node) Value {
	x, e, ok := numArg(c, args, 0)
	if !ok {
		return e
	}
	d, e, ok := numArg(c, args, 1)
	if !ok {
		return e
	}
	if d == 0 {
		return ErrorValue(ErrDiv0)
	}
	return NumberValue(x - d*math.Floor(x/d))
}

func ifFn(c *Context, args []Node) Value {
	b, e := toBool(scalar(arg(c, args, 0)))
	if e != "" {
		return ErrorValue(e)
	}
	if b {
		if !present(args, 1) {
			return BoolValue(true)
		}
		return arg(c, args, 1)
	}
	if len(args) < 3 {
		return BoolValue(false)
	}
	return arg(c, args, 2)
}

func ifsFn(c *Context, args []Node) Value {
	if len(args)%2 != 0 {
		return ErrorValue(ErrValue)
	}
	for i := 0; i < len(args); i += 2 {
		b, e := toBool(scalar(arg(c, args, i)))
		if e != "" {
			return ErrorValue(e)
		}
		if b {
			return arg(c, args, i+1)
		}
	}
	return ErrorValue(ErrNA)
}

func iferror(caught func(Value) bool) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		v := arg(c, args, 0)
		if caught(scalar(v)) {
			return arg(c, args, 1)
		}
		return v
	}
}

func logic(all bool) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		result := all
		for i := range args {
			var bad Value
			each(arg(c, args, i), func(v Value) {
				if v.Kind == Blank || v.Kind == Text && present(args, i) && args[i] != nil && isRange(args[i]) {
					return
				}
				b, e := toBool(v)
				if e != "" {
					bad = ErrorValue(e)
					return
				}
				if all {
					result = result && b
				} else {
					result = result || b
				}
			})
			if bad.Kind == Error {
				return bad
			}
		}
		return BoolValue(result)
	}
}

func isRange(n Node) bool {
	_, ok := n.(Area)
	return ok
}

func notFn(c *Context, args []Node) Value {
	b, e := toBool(scalar(arg(c, args, 0)))
	if e != "" {
		return ErrorValue(e)
	}
	return BoolValue(!b)
}

func choose(c *Context, args []Node) Value {
	i, e, ok := numArg(c, args, 0)
	if !ok {
		return e
	}
	n := whole(i)
	if n < 1 || n >= len(args) {
		return ErrorValue(ErrValue)
	}
	return arg(c, args, n)
}

func index(c *Context, args []Node) Value {
	a := arg(c, args, 0)
	if a.Kind != Array {
		a = Value{Kind: Array, Rows: [][]Value{{a}}}
	}
	r, e, ok := numArg(c, args, 1)
	if !ok {
		return e
	}
	col := 1.0
	if present(args, 2) {
		if col, e, ok = numArg(c, args, 2); !ok {
			return e
		}
	}
	rows, cols := dims(a)
	// A single row or column takes its one index as the position along it.
	if rows == 1 && !present(args, 2) {
		r, col = 1, r
	}
	ri, ci := whole(r), whole(col)
	if ri < 1 || ci < 1 || ri > rows || ci > cols {
		return ErrorValue(ErrRef)
	}
	return a.Rows[ri-1][ci-1]
}

func match(c *Context, args []Node) Value {
	want := scalar(arg(c, args, 0))
	if want.Kind == Error {
		return want
	}
	list := flat(arg(c, args, 1))
	mode := 1.0
	if present(args, 2) {
		var e Value
		var ok bool
		if mode, e, ok = numArg(c, args, 2); !ok {
			return e
		}
	}
	switch {
	case mode == 0:
		ok := criterion(want)
		if want.Kind != Text {
			ok = func(x Value) bool { return x.Kind != Blank && compare(x, want) == 0 }
		}
		for i, x := range list {
			if ok(x) {
				return NumberValue(float64(i + 1))
			}
		}
	case mode > 0:
		best := -1
		for i, x := range list {
			if x.Kind == Blank {
				continue
			}
			if compare(x, want) <= 0 && sameKind(x, want) {
				best = i
			} else if sameKind(x, want) {
				break
			}
		}
		if best >= 0 {
			return NumberValue(float64(best + 1))
		}
	default:
		best := -1
		for i, x := range list {
			if x.Kind == Blank {
				continue
			}
			if compare(x, want) >= 0 && sameKind(x, want) {
				best = i
			} else if sameKind(x, want) {
				break
			}
		}
		if best >= 0 {
			return NumberValue(float64(best + 1))
		}
	}
	return ErrorValue(ErrNA)
}

func sameKind(a, b Value) bool { return a.Kind == b.Kind || a.Kind == Number && b.Kind == Logical }

func vlookup(c *Context, args []Node) Value {
	want := scalar(arg(c, args, 0))
	table := arg(c, args, 1)
	col, e, ok := numArg(c, args, 2)
	if !ok {
		return e
	}
	approx := true
	if present(args, 3) {
		b, e := toBool(scalar(arg(c, args, 3)))
		if e != "" {
			return ErrorValue(e)
		}
		approx = b
	}
	if table.Kind != Array {
		return ErrorValue(ErrValue)
	}
	ci := whole(col)
	if ci < 1 || ci > len(table.Rows[0]) {
		return ErrorValue(ErrRef)
	}
	first := make([]Value, len(table.Rows))
	for i, row := range table.Rows {
		first[i] = row[0]
	}
	mode := Num(0)
	if approx {
		mode = 1
	}
	pos := match(c, []Node{literal(want), arrayNode{Value{Kind: Array, Rows: columnRows(first)}}, mode})
	if pos.Kind == Error {
		return pos
	}
	return table.Rows[int(pos.Num)-1][ci-1]
}

// literal and arrayNode let one function call another with values already
// computed.
type arrayNode struct{ v Value }

func (arrayNode) node() {}

func literal(v Value) Node { return arrayNode{v} }

func columnRows(vs []Value) [][]Value {
	rows := make([][]Value, len(vs))
	for i, v := range vs {
		rows[i] = []Value{v}
	}
	return rows
}

func dateArg(c *Context, args []Node, i int) Value {
	v := scalar(arg(c, args, i))
	if v.Kind == Text {
		return ErrorValue(ErrValue) // text dates read differently in every locale
	}
	return v
}

func date(c *Context, args []Node) Value {
	var parts [3]float64
	for i := range parts {
		f, e, ok := numArg(c, args, i)
		if !ok {
			return e
		}
		parts[i] = f
	}
	t := time.Date(int(parts[0]), time.Month(int(parts[1])), int(parts[2]), 0, 0, 0, 0, time.UTC)
	if s := Serial(t); s >= 61 {
		return NumberValue(s)
	}
	return ErrorValue(ErrNum) // before March 1900 the apps disagree
}

func datePart(f func(time.Time) int) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		v := dateArg(c, args, 0)
		n, e := toNumber(v)
		if e != "" {
			return ErrorValue(e)
		}
		return NumberValue(float64(f(Date(n))))
	}
}

func weekday(c *Context, args []Node) Value {
	n, e := toNumber(dateArg(c, args, 0))
	if e != "" {
		return ErrorValue(e)
	}
	kind := 1.0
	if present(args, 1) {
		var ev Value
		var ok bool
		if kind, ev, ok = numArg(c, args, 1); !ok {
			return ev
		}
	}
	d := int(Date(n).Weekday()) // Sunday 0
	switch whole(kind) {
	case 1:
		return NumberValue(float64(d + 1))
	case 2:
		return NumberValue(float64((d+6)%7 + 1))
	case 3:
		return NumberValue(float64((d + 6) % 7))
	}
	return ErrorValue(ErrNum)
}

func monthShift(end bool) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		n, e := toNumber(dateArg(c, args, 0))
		if e != "" {
			return ErrorValue(e)
		}
		m, ev, ok := numArg(c, args, 1)
		if !ok {
			return ev
		}
		t := Date(n)
		months := whole(m)
		if end {
			first := time.Date(t.Year(), t.Month()+time.Month(months)+1, 1, 0, 0, 0, 0, time.UTC)
			return NumberValue(Serial(first.AddDate(0, 0, -1)))
		}
		target := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
		last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		return NumberValue(Serial(time.Date(target.Year(), target.Month(), min(t.Day(), last), 0, 0, 0, 0, time.UTC)))
	}
}

// money reads (rate, n, value, [future], [type]) style arguments.
func money(c *Context, args []Node) ([5]float64, Value, bool) {
	var x [5]float64
	for i := range 5 {
		if !present(args, i) {
			continue
		}
		f, e, ok := numArg(c, args, i)
		if !ok {
			return x, e, false
		}
		x[i] = f
	}
	return x, Value{}, true
}

func pmt(c *Context, args []Node) Value {
	x, e, ok := money(c, args)
	if !ok {
		return e
	}
	rate, n, pv, fv, typ := x[0], x[1], x[2], x[3], x[4]
	if n == 0 {
		return ErrorValue(ErrNum)
	}
	if rate == 0 {
		return NumberValue(-(pv + fv) / n)
	}
	f := math.Pow(1+rate, n)
	return NumberValue(-(rate * (pv*f + fv)) / ((1 + rate*typ) * (f - 1)))
}

func fv(c *Context, args []Node) Value {
	x, e, ok := money(c, args)
	if !ok {
		return e
	}
	rate, n, payment, pv, typ := x[0], x[1], x[2], x[3], x[4]
	if rate == 0 {
		return NumberValue(-(pv + payment*n))
	}
	f := math.Pow(1+rate, n)
	return NumberValue(-(pv*f + payment*(1+rate*typ)*(f-1)/rate))
}

func nper(c *Context, args []Node) Value {
	x, e, ok := money(c, args)
	if !ok {
		return e
	}
	rate, payment, pv, fv, typ := x[0], x[1], x[2], x[3], x[4]
	if rate == 0 {
		if payment == 0 {
			return ErrorValue(ErrNum)
		}
		return NumberValue(-(pv + fv) / payment)
	}
	num := payment*(1+rate*typ) - fv*rate
	den := pv*rate + payment*(1+rate*typ)
	if num/den <= 0 {
		return ErrorValue(ErrNum)
	}
	return NumberValue(math.Log(num/den) / math.Log(1+rate))
}

func textOp(c *Context, args []Node, f func(string) Value) Value {
	v := scalar(arg(c, args, 0))
	if v.Kind == Error {
		return v
	}
	return f(toText(v))
}

func leftRight(left bool) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		v := scalar(arg(c, args, 0))
		if v.Kind == Error {
			return v
		}
		n := 1.0
		if present(args, 1) {
			var e Value
			var ok bool
			if n, e, ok = numArg(c, args, 1); !ok {
				return e
			}
		}
		if n < 0 {
			return ErrorValue(ErrValue)
		}
		r := []rune(toText(v))
		k := min(whole(n), len(r))
		if left {
			return TextValue(string(r[:k]))
		}
		return TextValue(string(r[len(r)-k:]))
	}
}

func mid(c *Context, args []Node) Value {
	v := scalar(arg(c, args, 0))
	if v.Kind == Error {
		return v
	}
	start, e, ok := numArg(c, args, 1)
	if !ok {
		return e
	}
	n, e, ok := numArg(c, args, 2)
	if !ok {
		return e
	}
	if start < 1 || n < 0 {
		return ErrorValue(ErrValue)
	}
	r := []rune(toText(v))
	s := min(whole(start)-1, len(r))
	return TextValue(string(r[s:min(s+whole(n), len(r))]))
}

func rept(c *Context, args []Node) Value {
	v := scalar(arg(c, args, 0))
	if v.Kind == Error {
		return v
	}
	n, e, ok := numArg(c, args, 1)
	if !ok {
		return e
	}
	if n < 0 {
		return ErrorValue(ErrValue)
	}
	t := toText(v)
	if float64(utf8.RuneCountInString(t))*n > maxText {
		return ErrorValue(ErrValue)
	}
	return TextValue(strings.Repeat(t, whole(n)))
}

func concatenate(c *Context, args []Node) Value {
	var b strings.Builder
	for i := range args {
		v := scalar(arg(c, args, i))
		if v.Kind == Error {
			return v
		}
		if v.Kind == Logical && c.Unportable != nil {
			c.Unportable("joins TRUE or FALSE into text, which LibreOffice writes as 1 or 0; use IF(cond, \"yes\", \"no\")")
		}
		b.WriteString(toText(v))
	}
	return text(b.String())
}

func substitute(c *Context, args []Node) Value {
	var s [3]string
	for i := range s {
		v := scalar(arg(c, args, i))
		if v.Kind == Error {
			return v
		}
		s[i] = toText(v)
	}
	if s[1] == "" {
		return TextValue(s[0])
	}
	if grow := utf8.RuneCountInString(s[2]) - utf8.RuneCountInString(s[1]); grow > 0 && utf8.RuneCountInString(s[0])+strings.Count(s[0], s[1])*grow > maxText {
		return ErrorValue(ErrValue)
	}
	return text(strings.ReplaceAll(s[0], s[1], s[2]))
}

func find(fold bool) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value {
		needle, hay := scalar(arg(c, args, 0)), scalar(arg(c, args, 1))
		for _, v := range []Value{needle, hay} {
			if v.Kind == Error {
				return v
			}
		}
		start := 1.0
		if present(args, 2) {
			var e Value
			var ok bool
			if start, e, ok = numArg(c, args, 2); !ok {
				return e
			}
		}
		h, n := []rune(toText(hay)), toText(needle)
		if start < 1 || whole(start) > len(h)+1 {
			return ErrorValue(ErrValue)
		}
		rest := string(h[whole(start)-1:])
		var i int
		if fold {
			i = -1
			if loc := wildcardFind(n).FindStringIndex(rest); loc != nil {
				i = loc[0]
			}
		} else {
			i = strings.Index(rest, n)
		}
		if i < 0 {
			return ErrorValue(ErrValue)
		}
		return NumberValue(float64(utf8.RuneCountInString(rest[:i]) + whole(start)))
	}
}

func wildcardFind(s string) *regexp.Regexp {
	re := wildcard(s).String()
	return regexp.MustCompile(strings.TrimSuffix(strings.Replace(re, "^", "", 1), "$"))
}

func is(pred func(Value) bool) func(*Context, []Node) Value {
	return func(c *Context, args []Node) Value { return BoolValue(pred(scalar(arg(c, args, 0)))) }
}
