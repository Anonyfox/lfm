package compile

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
)

// Config numbers reach templates as json.Number, which keeps their exact source
// text for printing ("9.90" stays "9.90"). Its kind is string, so the builtin
// comparisons would order 10 before 6. These replacements compare two numbers
// by value, exactly, and keep the builtin behavior for everything else.

// number returns v as an exact rational when it is a JSON or Go number.
func number(v any) (*big.Rat, bool) {
	switch n := v.(type) {
	case json.Number:
		r, ok := new(big.Rat).SetString(string(n))
		return r, ok
	case nil, bool, string:
		return nil, false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return new(big.Rat).SetInt64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return new(big.Rat).SetFrac(new(big.Int).SetUint64(rv.Uint()), big.NewInt(1)), true
	case reflect.Float32, reflect.Float64:
		r := new(big.Rat)
		if r.SetFloat64(rv.Float()) == nil {
			return nil, false
		}
		return r, true
	}
	return nil, false
}

// compare orders a and b: numbers by value, strings and string kinds by
// bytes. ok is false when they cannot be ordered.
func compare(a, b any) (order int, ok bool) {
	if x, isNum := number(a); isNum {
		if y, isNum := number(b); isNum {
			return x.Cmp(y), true
		}
	}
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	if av.IsValid() && bv.IsValid() && av.Kind() == reflect.String && bv.Kind() == reflect.String {
		switch x, y := av.String(), bv.String(); {
		case x < y:
			return -1, true
		case x > y:
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func equal(a, b any) (bool, error) {
	if order, ok := compare(a, b); ok {
		return order == 0, nil
	}
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	if !av.IsValid() || !bv.IsValid() {
		return !av.IsValid() && !bv.IsValid(), nil
	}
	if av.Type() != bv.Type() {
		if av.Kind() == bv.Kind() && av.Kind() == reflect.Bool {
			return av.Bool() == bv.Bool(), nil
		}
		return false, fmt.Errorf("incompatible types for comparison: %T and %T", a, b)
	}
	if !av.Comparable() {
		return false, fmt.Errorf("uncomparable type %T", a)
	}
	return av.Equal(bv), nil
}

func order(a, b any) (int, error) {
	if o, ok := compare(a, b); ok {
		return o, nil
	}
	return 0, fmt.Errorf("incompatible types for comparison: %T and %T", a, b)
}

// comparisons are the template functions that replace the builtins.
var comparisons = map[string]any{
	// eq reports whether a equals any of the others, like the builtin.
	"eq": func(a any, others ...any) (bool, error) {
		if len(others) == 0 {
			return false, fmt.Errorf("missing argument for comparison")
		}
		for _, b := range others {
			if same, err := equal(a, b); err != nil || same {
				return same, err
			}
		}
		return false, nil
	},
	"ne": func(a, b any) (bool, error) { same, err := equal(a, b); return !same, err },
	"lt": func(a, b any) (bool, error) { o, err := order(a, b); return o < 0, err },
	"le": func(a, b any) (bool, error) { o, err := order(a, b); return o <= 0, err },
	"gt": func(a, b any) (bool, error) { o, err := order(a, b); return o > 0, err },
	"ge": func(a, b any) (bool, error) { o, err := order(a, b); return o >= 0, err },
}
