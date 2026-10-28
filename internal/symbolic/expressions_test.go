package symbolic

import "testing"

func TestStringAndTypes(t *testing.T) {
	x := NewSymbolicVariable("x", IntType)
	y := NewSymbolicVariable("y", IntType)
	sum := NewBinaryOperation(x, y, ADD)
	cond := NewBinaryOperation(sum, NewIntConstant(5), GT)
	both := NewLogicalOperation([]SymbolicExpression{cond, NewLogicalOperation([]SymbolicExpression{NewBoolConstant(false)}, NOT)}, AND)

	cases := []struct {
		expr SymbolicExpression
		str  string
		tpe  ExpressionType
	}{
		{sum, "(x + y)", IntType},
		{cond, "((x + y) > 5)", BoolType},
		{both, "(((x + y) > 5) && !false)", BoolType},
		{NewUnaryOperation(x, BIT_NOT), "^x", IntType},
		{NewConversion(x, Float64Type), "float64(x)", Float64Type},
		{NewTypedIntConstant(-1, Uint64Type), "18446744073709551615", Uint64Type},
		{NewFloatConstant(1.5), "1.5", Float64Type},
		{NewBinaryOperation(NewTypedIntConstant(1, Int8Type), NewTypedIntConstant(2, UintType), SHL), "(1 << 2)", Int8Type},
	}
	for _, c := range cases {
		if c.expr.String() != c.str || c.expr.Type() != c.tpe {
			t.Errorf("got %s: %s, want %s: %s", c.expr, c.expr.Type(), c.str, c.tpe)
		}
	}
}

func TestTypeChecks(t *testing.T) {
	x := NewSymbolicVariable("x", IntType)
	b := NewSymbolicVariable("b", BoolType)
	f := NewSymbolicVariable("f", Float64Type)
	mustPanic := func(name string, build func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected panic", name)
			}
		}()
		build()
	}
	mustPanic("int + bool", func() { NewBinaryOperation(x, b, ADD) })
	mustPanic("bool < bool", func() { NewBinaryOperation(b, b, LT) })
	mustPanic("float % float", func() { NewBinaryOperation(f, f, MOD) })
	mustPanic("int && bool", func() { NewLogicalOperation([]SymbolicExpression{x, b}, AND) })
	mustPanic("! with two operands", func() { NewLogicalOperation([]SymbolicExpression{b, b}, NOT) })
	mustPanic("^float", func() { NewUnaryOperation(f, BIT_NOT) })
}
