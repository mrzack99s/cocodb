package query

import (
	"bytes"
	"strings"

	"cocodb/internal/cson"
)

// Expression evaluates a boolean predicate against a DocumentView.
type Expression interface {
	Evaluate(view *cson.DocumentView) bool
}

type eqExpr struct {
	field    string
	val      any
	strBytes []byte
	isStr    bool
	floatVal float64
	isFloat  bool
	intVal   int64
	isInt    bool
	boolVal  bool
	isBool   bool
}

func Eq(field string, val any) Expression {
	expr := &eqExpr{field: field, val: val}
	switch v := val.(type) {
	case string:
		expr.isStr = true
		expr.strBytes = []byte(v)
	case float64:
		expr.isFloat = true
		expr.floatVal = v
	case float32:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int:
		expr.isInt = true
		expr.intVal = int64(v)
	case int64:
		expr.isInt = true
		expr.intVal = v
	case bool:
		expr.isBool = true
		expr.boolVal = v
	}
	return expr
}

func (e *eqExpr) Evaluate(view *cson.DocumentView) bool {
	if e.isStr {
		b, ok := view.StringBytes(e.field)
		return ok && bytes.Equal(b, e.strBytes)
	}
	if e.isFloat {
		f, ok := view.Float64(e.field)
		return ok && f == e.floatVal
	}
	if e.isInt {
		n, ok := view.Int64(e.field)
		return ok && n == e.intVal
	}
	if e.isBool {
		b, ok := view.Bool(e.field)
		return ok && b == e.boolVal
	}
	v, ok := view.Get(e.field)
	if !ok {
		return e.val == nil
	}
	return compareValues(v, e.val) == 0
}

type neExpr struct {
	field string
	val   any
}

func Ne(field string, val any) Expression {
	return &neExpr{field: field, val: val}
}

func (e *neExpr) Evaluate(view *cson.DocumentView) bool {
	v, ok := view.Get(e.field)
	if !ok {
		return e.val != nil
	}
	return compareValues(v, e.val) != 0
}

type gtExpr struct {
	field    string
	val      any
	floatVal float64
	isFloat  bool
}

func Gt(field string, val any) Expression {
	expr := &gtExpr{field: field, val: val}
	switch v := val.(type) {
	case float64:
		expr.isFloat = true
		expr.floatVal = v
	case float32:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int64:
		expr.isFloat = true
		expr.floatVal = float64(v)
	}
	return expr
}

func (e *gtExpr) Evaluate(view *cson.DocumentView) bool {
	if e.isFloat {
		f, ok := view.Float64(e.field)
		return ok && f > e.floatVal
	}
	v, ok := view.Get(e.field)
	if !ok {
		return false
	}
	return compareValues(v, e.val) > 0
}

type gteExpr struct {
	field    string
	val      any
	floatVal float64
	isFloat  bool
}

func Gte(field string, val any) Expression {
	expr := &gteExpr{field: field, val: val}
	switch v := val.(type) {
	case float64:
		expr.isFloat = true
		expr.floatVal = v
	case float32:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int64:
		expr.isFloat = true
		expr.floatVal = float64(v)
	}
	return expr
}

func (e *gteExpr) Evaluate(view *cson.DocumentView) bool {
	if e.isFloat {
		f, ok := view.Float64(e.field)
		return ok && f >= e.floatVal
	}
	v, ok := view.Get(e.field)
	if !ok {
		return false
	}
	return compareValues(v, e.val) >= 0
}

type ltExpr struct {
	field    string
	val      any
	floatVal float64
	isFloat  bool
}

func Lt(field string, val any) Expression {
	expr := &ltExpr{field: field, val: val}
	switch v := val.(type) {
	case float64:
		expr.isFloat = true
		expr.floatVal = v
	case float32:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int64:
		expr.isFloat = true
		expr.floatVal = float64(v)
	}
	return expr
}

func (e *ltExpr) Evaluate(view *cson.DocumentView) bool {
	if e.isFloat {
		f, ok := view.Float64(e.field)
		return ok && f < e.floatVal
	}
	v, ok := view.Get(e.field)
	if !ok {
		return false
	}
	return compareValues(v, e.val) < 0
}

type lteExpr struct {
	field    string
	val      any
	floatVal float64
	isFloat  bool
}

func Lte(field string, val any) Expression {
	expr := &lteExpr{field: field, val: val}
	switch v := val.(type) {
	case float64:
		expr.isFloat = true
		expr.floatVal = v
	case float32:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int:
		expr.isFloat = true
		expr.floatVal = float64(v)
	case int64:
		expr.isFloat = true
		expr.floatVal = float64(v)
	}
	return expr
}

func (e *lteExpr) Evaluate(view *cson.DocumentView) bool {
	if e.isFloat {
		f, ok := view.Float64(e.field)
		return ok && f <= e.floatVal
	}
	v, ok := view.Get(e.field)
	if !ok {
		return false
	}
	return compareValues(v, e.val) <= 0
}

type betweenExpr struct {
	field string
	low   any
	high  any
}

func Between(field string, low, high any) Expression {
	return &betweenExpr{field: field, low: low, high: high}
}

func (e *betweenExpr) Evaluate(view *cson.DocumentView) bool {
	v, ok := view.Get(e.field)
	if !ok {
		return false
	}
	return compareValues(v, e.low) >= 0 && compareValues(v, e.high) <= 0
}

type inExpr struct {
	field string
	vals  []any
}

func In(field string, vals ...any) Expression {
	return &inExpr{field: field, vals: vals}
}

func (e *inExpr) Evaluate(view *cson.DocumentView) bool {
	v, ok := view.Get(e.field)
	if !ok {
		return false
	}
	for _, target := range e.vals {
		if compareValues(v, target) == 0 {
			return true
		}
	}
	return false
}

type andExpr struct {
	exprs []Expression
}

func And(exprs ...Expression) Expression {
	return &andExpr{exprs: exprs}
}

func (e *andExpr) Evaluate(view *cson.DocumentView) bool {
	for _, expr := range e.exprs {
		if !expr.Evaluate(view) {
			return false
		}
	}
	return true
}

type orExpr struct {
	exprs []Expression
}

func Or(exprs ...Expression) Expression {
	return &orExpr{exprs: exprs}
}

func (e *orExpr) Evaluate(view *cson.DocumentView) bool {
	for _, expr := range e.exprs {
		if expr.Evaluate(view) {
			return true
		}
	}
	return false
}

type notExpr struct {
	expr Expression
}

func Not(expr Expression) Expression {
	return &notExpr{expr: expr}
}

func (e *notExpr) Evaluate(view *cson.DocumentView) bool {
	return !e.expr.Evaluate(view)
}

type containsExpr struct {
	field  string
	substr string
}

func Contains(field, substr string) Expression {
	return &containsExpr{field: field, substr: substr}
}

func (e *containsExpr) Evaluate(view *cson.DocumentView) bool {
	s, ok := view.String(e.field)
	if !ok {
		return false
	}
	return strings.Contains(s, e.substr)
}

type startsWithExpr struct {
	field  string
	prefix string
}

func StartsWith(field, prefix string) Expression {
	return &startsWithExpr{field: field, prefix: prefix}
}

func (e *startsWithExpr) Evaluate(view *cson.DocumentView) bool {
	s, ok := view.String(e.field)
	if !ok {
		return false
	}
	return strings.HasPrefix(s, e.prefix)
}

func compareValues(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	switch vA := a.(type) {
	case int64:
		var vB int64
		switch n := b.(type) {
		case int:
			vB = int64(n)
		case int64:
			vB = n
		case float64:
			vB = int64(n)
		}
		if vA < vB {
			return -1
		} else if vA > vB {
			return 1
		}
		return 0
	case int:
		return compareValues(int64(vA), b)
	case float64:
		var vB float64
		switch n := b.(type) {
		case float64:
			vB = n
		case float32:
			vB = float64(n)
		case int:
			vB = float64(n)
		case int64:
			vB = float64(n)
		}
		if vA < vB {
			return -1
		} else if vA > vB {
			return 1
		}
		return 0
	case string:
		if sB, ok := b.(string); ok {
			return strings.Compare(vA, sB)
		}
	case bool:
		if bB, ok := b.(bool); ok {
			if vA == bB {
				return 0
			}
			if !vA && bB {
				return -1
			}
			return 1
		}
	case []byte:
		if bB, ok := b.([]byte); ok {
			return bytes.Compare(vA, bB)
		}
	}
	return 0
}
