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

func TestRefsAndArrays(t *testing.T) {
	person := StructInfo("Person", ScalarInfo(IntType))
	p := NewRef(NewSymbolicVariable("p", AddressType), person)
	sort := &ArraySort{Index: AddressType, Element: IntType}
	heap := NewArrayVariable("heap", sort)
	store := NewArrayStore(heap, p.Address, NewIntConstant(1))
	read := NewArraySelect(store, p.Address)
	nested := &ArraySort{Index: AddressType, Element: ArrayType, Inner: &ArraySort{Index: IntType, Element: BoolType}}

	cases := []struct {
		expr SymbolicExpression
		str  string
		tpe  ExpressionType
	}{
		{p, "&Person@p", RefType},
		{NilRef(person), "nil", RefType},
		{NewBinaryOperation(p, NilRef(person), EQ), "(&Person@p == nil)", BoolType},
		{read, "heap{p <- 1}[p]", IntType},
		{NewArraySelect(NewArrayVariable("arrays", nested), p.Address), "arrays[p]", ArrayType},
	}
	for _, c := range cases {
		if c.expr.String() != c.str || c.expr.Type() != c.tpe {
			t.Errorf("got %s: %s, want %s: %s", c.expr, c.expr.Type(), c.str, c.tpe)
		}
	}
	if ArrayInfo(PointerInfo(person), -1).Key() != "[]*Person" || ArrayInfo(person, 3).Key() != "[3]Person" {
		t.Error("unexpected type keys")
	}

	mustPanic := func(name string, build func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected panic", name)
			}
		}()
		build()
	}
	mustPanic("int address", func() { NewRef(NewIntConstant(1), person) })
	mustPanic("wrong index", func() { NewArraySelect(heap, NewIntConstant(0)) })
	mustPanic("wrong element", func() { NewArrayStore(heap, p.Address, NewBoolConstant(true)) })
	mustPanic("ref < ref", func() { NewBinaryOperation(p, p, LT) })
}
