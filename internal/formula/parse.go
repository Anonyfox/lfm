// Package formula parses, prints and evaluates the spreadsheet formulas lfm
// builds: the portable subset Excel 2019+, Google Sheets and LibreOffice all
// compute the same way. Placeholders in square brackets ([Log.amount],
// [.amount], [income]) name columns and cells by meaning; a resolver turns
// them into references for one edition, so a sheet can be renamed per
// language without touching a formula.
package formula

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// errorLiterals are the error values a formula may name.
var errorLiterals = []string{"#DIV/0!", "#N/A", "#VALUE!", "#REF!", "#NAME?", "#NUM!", "#NULL!"}

// Node is a parsed formula.
type Node interface{ node() }

// Num, Str, Bool and Err are literals.
type (
	Num  float64
	Str  string
	Bool bool
	Err  string
)

// Ref is one cell; Col and Row count from 1. A Here reference is in the row
// of the cell the formula belongs to: one parsed formula serves every row of
// a table column.
type Ref struct {
	Sheet          string
	Col, Row       int
	AbsCol, AbsRow bool
	Here           bool
	Name           string // a defined name the file stores instead of the address
}

// Area is a rectangle of cells on one sheet.
type Area struct {
	Sheet    string
	From, To Ref
}

// Unary is -x, +x or x%.
type Unary struct {
	Op string
	X  Node
}

// Binary is an operator between two operands.
type Binary struct {
	Op   string
	L, R Node
}

// Call is a function call.
type Call struct {
	Name string // upper case, without prefix
	Args []Node
}

func (Num) node()    {}
func (Str) node()    {}
func (Bool) node()   {}
func (Err) node()    {}
func (Ref) node()    {}
func (Area) node()   {}
func (Unary) node()  {}
func (Binary) node() {}
func (Call) node()   {}

// Resolver turns a placeholder's text (without brackets) into a reference.
type Resolver func(name string) (Node, error)

// Parse parses src, which may start with "=". Placeholders go through
// resolve; functions outside the portable set are an error.
func Parse(src string, resolve Resolver) (Node, error) {
	src = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(src), "="))
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, resolve: resolve}
	n, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, fmt.Errorf("unexpected %q", p.peek().text)
	}
	return n, nil
}

type kind int

const (
	tEOF kind = iota
	tNum
	tStr
	tIdent // a name: function, TRUE/FALSE, cell reference, sheet-qualified reference
	tPlace
	tOp
	tLParen
	tRParen
	tComma
	tColon
	tErr
)

type token struct {
	kind kind
	text string
}

func lex(s string) ([]token, error) {
	var out []token
	r := []rune(s)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '"':
			var b strings.Builder
			i++
			for {
				if i >= len(r) {
					return nil, fmt.Errorf("unclosed text")
				}
				if r[i] == '"' {
					if i+1 < len(r) && r[i+1] == '"' {
						b.WriteRune('"')
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteRune(r[i])
				i++
			}
			out = append(out, token{tStr, b.String()})
		case c == '[':
			j := i + 1
			for j < len(r) && r[j] != ']' {
				j++
			}
			if j >= len(r) {
				return nil, fmt.Errorf("unclosed placeholder")
			}
			out = append(out, token{tPlace, string(r[i+1 : j])})
			i = j + 1
		case c == '#':
			rest := string(r[i:])
			found := ""
			for _, e := range errorLiterals {
				if strings.HasPrefix(strings.ToUpper(rest), e) {
					found = e
					break
				}
			}
			if found == "" {
				return nil, fmt.Errorf("unknown error value in %q", rest)
			}
			out = append(out, token{tErr, found})
			i += len([]rune(found))
		case unicode.IsDigit(c) || c == '.' && i+1 < len(r) && unicode.IsDigit(r[i+1]):
			j := i
			for j < len(r) && (unicode.IsDigit(r[j]) || r[j] == '.') {
				j++
			}
			if j < len(r) && (r[j] == 'e' || r[j] == 'E') && j+1 < len(r) && (unicode.IsDigit(r[j+1]) || (r[j+1] == '-' || r[j+1] == '+') && j+2 < len(r) && unicode.IsDigit(r[j+2])) {
				j += 2
				for j < len(r) && unicode.IsDigit(r[j]) {
					j++
				}
			}
			// 1:1 row ranges are not supported; a digit run is a number.
			out = append(out, token{tNum, string(r[i:j])})
			i = j
		case c == '\'':
			j := i + 1
			var b strings.Builder
			for {
				if j >= len(r) {
					return nil, fmt.Errorf("unclosed sheet name")
				}
				if r[j] == '\'' {
					if j+1 < len(r) && r[j+1] == '\'' {
						b.WriteRune('\'')
						j += 2
						continue
					}
					break
				}
				b.WriteRune(r[j])
				j++
			}
			j++
			if j >= len(r) || r[j] != '!' {
				return nil, fmt.Errorf("a quoted sheet name must be followed by an exclamation mark")
			}
			k := j + 1
			for k < len(r) && (unicode.IsLetter(r[k]) || unicode.IsDigit(r[k]) || r[k] == '$') {
				k++
			}
			out = append(out, token{tIdent, "'" + b.String() + "'!" + string(r[j+1:k])})
			i = k
		case unicode.IsLetter(c) || c == '_' || c == '$':
			j := i
			for j < len(r) && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_' || r[j] == '.' || r[j] == '$') {
				j++
			}
			if j < len(r) && r[j] == '!' {
				k := j + 1
				for k < len(r) && (unicode.IsLetter(r[k]) || unicode.IsDigit(r[k]) || r[k] == '$') {
					k++
				}
				out = append(out, token{tIdent, "'" + string(r[i:j]) + "'!" + string(r[j+1:k])})
				i = k
				continue
			}
			out = append(out, token{tIdent, string(r[i:j])})
			i = j
		case c == '(':
			out = append(out, token{tLParen, "("})
			i++
		case c == ')':
			out = append(out, token{tRParen, ")"})
			i++
		case c == ',' || c == ';':
			if c == ';' {
				return nil, fmt.Errorf("use , between arguments; lfm stores formulas in the file's English syntax, every app shows its own")
			}
			out = append(out, token{tComma, ","})
			i++
		case c == ':':
			out = append(out, token{tColon, ":"})
			i++
		case strings.ContainsRune("+-*/^&=<>%", c):
			op := string(c)
			if i+1 < len(r) {
				two := string(r[i : i+2])
				if two == "<=" || two == ">=" || two == "<>" {
					op = two
				}
			}
			out = append(out, token{tOp, op})
			i += len([]rune(op))
		default:
			return nil, fmt.Errorf("unexpected character %q", c)
		}
	}
	return append(out, token{tEOF, ""}), nil
}

type parser struct {
	toks    []token
	i       int
	resolve Resolver
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token {
	t := p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}

var precedence = map[string]int{"=": 1, "<>": 1, "<": 1, ">": 1, "<=": 1, ">=": 1, "&": 2, "+": 3, "-": 3, "*": 4, "/": 4, "^": 5}

func (p *parser) expr(min int) (Node, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		prec, ok := precedence[t.text]
		if t.kind != tOp || !ok || prec < min {
			return left, nil
		}
		p.next()
		next := prec + 1
		if t.text == "^" {
			next = prec // right-associative
		}
		right, err := p.expr(next)
		if err != nil {
			return nil, err
		}
		left = Binary{t.text, left, right}
	}
}

func (p *parser) unary() (Node, error) {
	if t := p.peek(); t.kind == tOp && (t.text == "-" || t.text == "+") {
		p.next()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return Unary{t.text, x}, nil
	}
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && p.peek().text == "%" {
		p.next()
		x = Unary{"%", x}
	}
	return x, nil
}

func (p *parser) primary() (Node, error) {
	t := p.next()
	switch t.kind {
	case tNum:
		v, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", t.text)
		}
		return Num(v), nil
	case tStr:
		return Str(t.text), nil
	case tErr:
		return Err(t.text), nil
	case tLParen:
		x, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		if p.next().kind != tRParen {
			return nil, fmt.Errorf("missing )")
		}
		return x, nil
	case tPlace:
		if p.resolve == nil {
			return nil, fmt.Errorf("placeholder [%s] outside a workbook", t.text)
		}
		n, err := p.resolve(t.text)
		if err != nil {
			return nil, err
		}
		return p.rangeTail(n)
	case tIdent:
		up := strings.ToUpper(t.text)
		if p.peek().kind == tLParen {
			name := strings.TrimPrefix(strings.TrimPrefix(up, "_XLFN."), "_XLWS.")
			if _, ok := functions[name]; !ok {
				return nil, fmt.Errorf("%s is not a function every app computes the same way; use %s", name, portableHint)
			}
			p.next()
			var args []Node
			if p.peek().kind != tRParen {
				for {
					if p.peek().kind == tComma || p.peek().kind == tRParen {
						args = append(args, nil) // an omitted argument
					} else {
						a, err := p.expr(0)
						if err != nil {
							return nil, err
						}
						args = append(args, a)
					}
					if p.peek().kind == tComma {
						p.next()
						continue
					}
					break
				}
			}
			if p.next().kind != tRParen {
				return nil, fmt.Errorf("missing ) after %s(", name)
			}
			if err := functions[name].arity(len(args)); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			return Call{name, args}, nil
		}
		switch up {
		case "TRUE":
			return Bool(true), nil
		case "FALSE":
			return Bool(false), nil
		}
		ref, err := parseRef(t.text)
		if err != nil {
			return nil, err
		}
		return p.rangeTail(ref)
	}
	return nil, fmt.Errorf("unexpected %q", t.text)
}

// rangeTail reads ":B5" after a reference.
func (p *parser) rangeTail(n Node) (Node, error) {
	from, ok := n.(Ref)
	if !ok || p.peek().kind != tColon {
		return n, nil
	}
	p.next()
	t := p.next()
	if t.kind != tIdent {
		return nil, fmt.Errorf("a range needs a cell after the colon")
	}
	to, err := parseRef(t.text)
	if err != nil {
		return nil, err
	}
	if to.Sheet == "" {
		to.Sheet = from.Sheet
	}
	return Area{Sheet: from.Sheet, From: from, To: to}, nil
}

// parseRef reads A1, $A$1 or 'Sheet'!A1.
func parseRef(s string) (Ref, error) {
	var r Ref
	if strings.HasPrefix(s, "'") {
		i := strings.LastIndex(s, "'!")
		r.Sheet = s[1:i]
		s = s[i+2:]
	}
	i := 0
	if i < len(s) && s[i] == '$' {
		r.AbsCol = true
		i++
	}
	j := i
	for j < len(s) && s[j] >= 'A' && s[j] <= 'Z' || j < len(s) && s[j] >= 'a' && s[j] <= 'z' {
		j++
	}
	if j == i || j-i > 3 {
		return r, fmt.Errorf("%q is not a cell reference or a known name", s)
	}
	r.Col = ColumnNumber(strings.ToUpper(s[i:j]))
	if j < len(s) && s[j] == '$' {
		r.AbsRow = true
		j++
	}
	row, err := strconv.Atoi(s[j:])
	if err != nil || row < 1 {
		return r, fmt.Errorf("%q is not a cell reference or a known name", s)
	}
	r.Row = row
	return r, nil
}

// ColumnNumber turns A into 1 and AA into 27.
func ColumnNumber(letters string) int {
	n := 0
	for _, c := range letters {
		n = n*26 + int(c-'A'+1)
	}
	return n
}

// ColumnName turns 1 into A and 27 into AA.
func ColumnName(n int) string {
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{byte('A' + n%26)}, b...)
		n /= 26
	}
	return string(b)
}

// CellName is A1 for column 1, row 1.
func CellName(col, row int) string { return ColumnName(col) + strconv.Itoa(row) }
