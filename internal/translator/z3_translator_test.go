package translator

import (
	"math"
	"testing"

	"github.com/ebukreev/go-z3/z3"
	s "symbolic-execution-course/internal/symbolic"
)

func check(t *testing.T, tr *Z3Translator, expr s.SymbolicExpression) bool {
	t.Helper()
	value, err := tr.TranslateExpression(expr)
	if err != nil {
		t.Fatalf("translate %s: %v", expr, err)
	}
	solver := z3.NewSolver(tr.Context())
	solver.Assert(value.(z3.Bool))
	sat, err := solver.Check()
	if err != nil {
		t.Fatalf("check %s: %v", expr, err)
	}
	return sat
}

func assertValid(t *testing.T, expr s.SymbolicExpression) {
	t.Helper()
	if check(t, NewZ3Translator(), s.NewLogicalOperation([]s.SymbolicExpression{expr}, s.NOT)) {
		t.Errorf("expected %s to be valid", expr)
	}
}

func assertSat(t *testing.T, expr s.SymbolicExpression, want bool) {
	t.Helper()
	if got := check(t, NewZ3Translator(), expr); got != want {
		t.Errorf("sat(%s) = %v, want %v", expr, got, want)
	}
}

func bin(l, r s.SymbolicExpression, op s.BinaryOperator) *s.BinaryOperation {
	return s.NewBinaryOperation(l, r, op)
}

func eq(l, r s.SymbolicExpression) *s.BinaryOperation { return bin(l, r, s.EQ) }

func i(v int64) *s.IntConstant { return s.NewIntConstant(v) }

func TestIntegerArithmeticFollowsGo(t *testing.T) {
	assertValid(t, eq(bin(i(-7), i(2), s.DIV), i(-7/2)))
	assertValid(t, eq(bin(i(-7), i(2), s.MOD), i(-7%2)))
	assertValid(t, eq(bin(i(math.MaxInt64), i(1), s.ADD), i(math.MinInt64)))
	assertValid(t, eq(bin(i(6), i(3), s.AND_NOT), i(6&^3)))
	assertValid(t, eq(bin(i(-8), i(1), s.SHR), i(-4)))
	assertValid(t, eq(bin(i(1), i(70), s.SHL), i(0)))
	assertValid(t, eq(s.NewUnaryOperation(i(5), s.BIT_NOT), i(^5)))

	u8 := func(v int64) *s.IntConstant { return s.NewTypedIntConstant(v, s.Uint8Type) }
	assertValid(t, eq(bin(u8(250), u8(10), s.ADD), u8(4)))
	assertValid(t, bin(u8(200), u8(100), s.GT))
	assertValid(t, eq(bin(u8(200), s.NewTypedIntConstant(1, s.Uint64Type), s.SHR), u8(100)))
	assertValid(t, eq(s.NewConversion(s.NewTypedIntConstant(-1, s.Int8Type), s.Uint16Type), s.NewTypedIntConstant(0xffff, s.Uint16Type)))
	assertValid(t, eq(s.NewConversion(i(300), s.Uint8Type), u8(44)))
}

func TestSymbolicIntegers(t *testing.T) {
	x := s.NewSymbolicVariable("x", s.IntType)
	assertSat(t, bin(bin(x, i(1), s.ADD), x, s.LT), true)
	assertSat(t, s.NewLogicalOperation([]s.SymbolicExpression{bin(x, i(10), s.GT), bin(x, i(5), s.LT)}, s.AND), false)
	assertValid(t, s.NewLogicalOperation([]s.SymbolicExpression{bin(x, i(10), s.GT), bin(x, i(5), s.GT)}, s.IMPLIES))
}

func TestFloats(t *testing.T) {
	nan := s.NewFloatConstant(math.NaN())
	assertValid(t, bin(nan, nan, s.NE))
	assertValid(t, eq(s.NewFloatConstant(0), s.NewUnaryOperation(s.NewFloatConstant(0), s.NEG)))
	assertValid(t, eq(bin(s.NewFloatConstant(0.1), s.NewFloatConstant(0.2), s.ADD), s.NewFloatConstant(0.30000000000000004)))
	assertValid(t, eq(s.NewConversion(s.NewFloatConstant(-2.9), s.IntType), i(-2)))
	assertValid(t, eq(s.NewConversion(i(3), s.Float64Type), s.NewFloatConstant(3)))
	assertValid(t, eq(s.NewConversion(s.NewFloatConstant(0.1), s.Float32Type), s.NewTypedFloatConstant(0.1, s.Float32Type)))

	a := s.NewSymbolicVariable("a", s.Float64Type)
	inRange := s.NewLogicalOperation([]s.SymbolicExpression{
		bin(a, s.NewFloatConstant(10.1), s.GT),
		bin(a, s.NewFloatConstant(11.125), s.LT),
	}, s.AND)
	assertSat(t, inRange, true)
	assertSat(t, bin(a, a, s.NE), true)
}

func TestBooleans(t *testing.T) {
	a := s.NewSymbolicVariable("a", s.BoolType)
	b := s.NewSymbolicVariable("b", s.BoolType)
	assertValid(t, eq(bin(a, b, s.NE), s.NewLogicalOperation([]s.SymbolicExpression{eq(a, b)}, s.NOT)))
	assertSat(t, s.NewLogicalOperation([]s.SymbolicExpression{a, s.NewLogicalOperation([]s.SymbolicExpression{a}, s.NOT)}, s.AND), false)
}

func TestTranslationErrors(t *testing.T) {
	tr := NewZ3Translator()
	if _, err := tr.TranslateExpression(s.NewSymbolicVariable("x", s.IntType)); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.TranslateExpression(s.NewSymbolicVariable("x", s.BoolType)); err == nil {
		t.Error("expected error for variable reused with another type")
	}
	if _, err := tr.TranslateExpression(s.NewSymbolicVariable("arr", s.ArrayType)); err == nil {
		t.Error("expected error for unsupported type")
	}
}
