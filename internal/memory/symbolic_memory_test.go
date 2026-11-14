package memory

import (
	"testing"

	"github.com/ebukreev/go-z3/z3"
	s "symbolic-execution-course/internal/symbolic"
	"symbolic-execution-course/internal/translator"
)

var (
	intInfo = s.ScalarInfo(s.IntType)
	person  = s.StructInfo("Person", intInfo, intInfo) // Age, ID
	foo     = s.StructInfo("Foo", intInfo)
)

const (
	age = 0
	id  = 1
)

func i(v int64) *s.IntConstant { return s.NewIntConstant(v) }

func eq(l, r s.SymbolicExpression) s.SymbolicExpression { return s.NewBinaryOperation(l, r, s.EQ) }

func ne(l, r s.SymbolicExpression) s.SymbolicExpression { return s.NewBinaryOperation(l, r, s.NE) }

func and(operands ...s.SymbolicExpression) s.SymbolicExpression {
	return s.NewLogicalOperation(operands, s.AND)
}

func not(e s.SymbolicExpression) s.SymbolicExpression {
	return s.NewLogicalOperation([]s.SymbolicExpression{e}, s.NOT)
}

func implies(l, r s.SymbolicExpression) s.SymbolicExpression {
	return s.NewLogicalOperation([]s.SymbolicExpression{l, r}, s.IMPLIES)
}

// satisfiable проверяет выполнимость формулы вместе с ограничениями памяти
func satisfiable(t *testing.T, mem Memory, formula s.SymbolicExpression) bool {
	t.Helper()
	tr := translator.NewZ3Translator()
	value, err := tr.TranslateExpression(and(mem.Constraints(), formula))
	if err != nil {
		t.Fatal(err)
	}
	solver := z3.NewSolver(tr.Context())
	solver.Assert(value.(z3.Bool))
	sat, err := solver.Check()
	if err != nil {
		t.Fatal(err)
	}
	return sat
}

func assertValid(t *testing.T, mem Memory, formula s.SymbolicExpression) {
	t.Helper()
	if satisfiable(t, mem, not(formula)) {
		t.Errorf("expected %s to hold", formula)
	}
}

func assertSat(t *testing.T, mem Memory, formula s.SymbolicExpression, want bool) {
	t.Helper()
	if got := satisfiable(t, mem, formula); got != want {
		t.Errorf("sat(%s) = %v, want %v", formula, got, want)
	}
}

func newMemory() *SymbolicMemory {
	mem := NewSymbolicMemory()
	return &mem
}

func TestStructBasic(t *testing.T) {
	mem := newMemory()
	p := mem.AllocateStruct(person)
	assertValid(t, mem, eq(mem.GetFieldValue(p, age), i(0)))

	mem.AssignField(p, age, i(25))
	mem.AssignField(p, id, i(1001))
	assertValid(t, mem, and(eq(mem.GetFieldValue(p, age), i(25)), eq(mem.GetFieldValue(p, id), i(1001))))
}

func TestAliasing(t *testing.T) {
	mem := newMemory()
	foo1 := mem.NewSymbolicRef("foo1", foo)
	foo2 := mem.NewSymbolicRef("foo2", foo)
	mem.AssignField(foo2, 0, i(5))
	mem.AssignField(foo1, 0, i(2))
	result := mem.GetFieldValue(foo2, 0)

	assertSat(t, mem, eq(result, i(2)), true)
	assertSat(t, mem, eq(result, i(5)), true)
	assertValid(t, mem, implies(eq(foo1, foo2), eq(result, i(2))))
	assertValid(t, mem, implies(ne(foo1, foo2), eq(result, i(5))))
}

func TestAllocatedObjectsDoNotAliasInputs(t *testing.T) {
	mem := newMemory()
	input := mem.NewSymbolicRef("input", foo)
	fresh := mem.AllocateStruct(foo)
	another := mem.AllocateStruct(foo)
	mem.AssignField(input, 0, i(1))
	mem.AssignField(fresh, 0, i(2))
	mem.AssignField(another, 0, i(3))

	assertValid(t, mem, and(ne(input, fresh), ne(fresh, another)))
	assertValid(t, mem, eq(mem.GetFieldValue(input, 0), i(1)))
	assertValid(t, mem, eq(mem.GetFieldValue(fresh, 0), i(2)))
}

func TestPathConstraintMutability(t *testing.T) {
	mem := newMemory()
	p := mem.NewSymbolicRef("p", person)
	initial := mem.GetFieldValue(p, age)
	mem.AssignField(p, age, i(18))

	assertSat(t, mem, and(ne(initial, i(18)), ne(mem.GetFieldValue(p, age), i(18))), false)
	assertSat(t, mem, ne(initial, i(18)), true)
}

func TestStructPointerModification(t *testing.T) {
	mem := newMemory()
	p := mem.NewSymbolicRef("p", person)
	oldAge, oldID := mem.GetFieldValue(p, age), mem.GetFieldValue(p, id)
	mem.AssignField(p, age, s.NewBinaryOperation(oldAge, i(10), s.ADD))
	mem.AssignField(p, id, s.NewBinaryOperation(oldID, i(1000), s.ADD))

	assertValid(t, mem, eq(mem.GetFieldValue(p, age), s.NewBinaryOperation(oldAge, i(10), s.ADD)))
	assertValid(t, mem, eq(mem.GetFieldValue(p, id), s.NewBinaryOperation(oldID, i(1000), s.ADD)))
	assertSat(t, mem, eq(mem.GetFieldValue(p, age), i(40)), true)
}

func TestNestedStructs(t *testing.T) {
	address := s.StructInfo("Address", intInfo)
	employee := s.StructInfo("Employee", person, address, s.ScalarInfo(s.Float64Type))

	mem := newMemory()
	emp := mem.AllocateStruct(employee)
	nested := mem.GetFieldValue(emp, 0).(*s.Ref)
	mem.AssignField(nested, age, s.NewBinaryOperation(mem.GetFieldValue(nested, age), i(1), s.ADD))
	mem.AssignField(emp, 2, s.NewBinaryOperation(mem.GetFieldValue(emp, 2), s.NewFloatConstant(1.1), s.MUL))

	reread := mem.GetFieldValue(emp, 0).(*s.Ref)
	assertValid(t, mem, eq(mem.GetFieldValue(reread, age), i(1)))
	assertValid(t, mem, eq(mem.GetFieldValue(emp, 2), s.NewFloatConstant(0)))

	// Вложенные структуры двух входных сотрудников совпадают только при совпадении сотрудников
	e1 := mem.NewSymbolicRef("e1", employee)
	e2 := mem.NewSymbolicRef("e2", employee)
	p1 := mem.GetFieldValue(e1, 0).(*s.Ref)
	mem.AssignField(p1, age, i(99))
	p2 := mem.GetFieldValue(e2, 0).(*s.Ref)
	assertSat(t, mem, eq(mem.GetFieldValue(p2, age), i(99)), true)
	assertValid(t, mem, implies(eq(e1, e2), eq(mem.GetFieldValue(p2, age), i(99))))
}

func TestFixedArray(t *testing.T) {
	mem := newMemory()
	arr := mem.AllocateArray(intInfo, i(5))
	for k := int64(0); k < 5; k++ {
		mem.AssignToArray(arr, int(k), i(k*k))
	}
	assertValid(t, mem, and(eq(mem.GetFromArray(arr, 3), i(9)), eq(mem.Length(arr), i(5))))

	index := s.NewSymbolicVariable("index", s.IntType)
	mem.AssignToArrayAt(arr, index, i(7))
	assertValid(t, mem, eq(mem.GetFromArrayAt(arr, index), i(7)))
	assertSat(t, mem, eq(mem.GetFromArray(arr, 2), i(7)), true)
	assertValid(t, mem, implies(ne(index, i(2)), eq(mem.GetFromArray(arr, 2), i(4))))
}

func TestSliceAliasing(t *testing.T) {
	slice := s.ArrayInfo(intInfo, -1)
	mem := newMemory()
	s1 := mem.NewSymbolicRef("s1", slice)
	s2 := mem.NewSymbolicRef("s2", slice)
	mem.AssignToArray(s1, 0, i(1))
	mem.AssignToArray(s2, 0, i(2))

	assertSat(t, mem, eq(mem.GetFromArray(s1, 0), i(2)), true)
	assertValid(t, mem, implies(ne(s1, s2), eq(mem.GetFromArray(s1, 0), i(1))))
	assertValid(t, mem, implies(eq(s1, s2), eq(mem.Length(s1), mem.Length(s2))))
}

func TestArrayOfStructs(t *testing.T) {
	mem := newMemory()
	people := mem.AllocateArray(person, i(3))
	for k := 0; k < 3; k++ {
		mem.AssignField(mem.GetFromArray(people, k).(*s.Ref), id, i(int64(k+1)))
	}
	second := mem.GetFromArray(people, 1).(*s.Ref)
	mem.AssignField(second, age, i(30))
	mem.AssignField(second, age, s.NewBinaryOperation(mem.GetFieldValue(second, age), i(5), s.ADD))

	first := mem.GetFromArray(people, 0).(*s.Ref)
	assertValid(t, mem, ne(first, second))
	assertValid(t, mem, eq(mem.GetFieldValue(mem.GetFromArray(people, 1).(*s.Ref), age), i(35)))
	assertValid(t, mem, and(eq(mem.GetFieldValue(first, age), i(0)), eq(mem.GetFieldValue(first, id), i(1))))
}

func TestPointerFields(t *testing.T) {
	node := s.StructInfo("Node", intInfo)
	node.Fields = append(node.Fields, s.PointerInfo(node)) // val, next

	mem := newMemory()
	n1 := mem.AllocateStruct(node)
	n2 := mem.AllocateStruct(node)
	assertValid(t, mem, eq(mem.GetFieldValue(n1, 1), s.NilRef(node)))

	mem.AssignField(n1, 1, n2)
	mem.AssignField(mem.GetFieldValue(n1, 1).(*s.Ref), 0, i(3))
	assertValid(t, mem, eq(mem.GetFieldValue(n2, 0), i(3)))

	// Входной список может быть циклическим: in.next может совпадать с in
	in := mem.NewSymbolicRef("in", node)
	mem.AssignField(in, 0, i(1))
	next := mem.GetFieldValue(in, 1).(*s.Ref)
	mem.AssignField(next, 0, i(2))
	assertSat(t, mem, eq(mem.GetFieldValue(in, 0), i(2)), true)
	assertSat(t, mem, eq(mem.GetFieldValue(in, 0), i(1)), true)
	assertValid(t, mem, implies(ne(in, next), eq(mem.GetFieldValue(in, 0), i(1))))
}

func TestScalarCellsAndDefaultArray(t *testing.T) {
	mem := newMemory()
	cell := mem.Allocate(s.BoolType)
	assertValid(t, mem, eq(mem.GetFieldValue(cell, 0), s.NewBoolConstant(false)))
	mem.AssignField(cell, 0, s.NewBoolConstant(true))
	assertValid(t, mem, mem.GetFieldValue(cell, 0))

	array := mem.Allocate(s.ArrayType)
	mem.AssignToArray(array, 5, i(10))
	assertValid(t, mem, and(eq(mem.GetFromArray(array, 5), i(10)), eq(mem.GetFromArray(array, 10), i(0))))
}

func TestCloneIsIndependent(t *testing.T) {
	mem := newMemory()
	p := mem.AllocateStruct(person)
	mem.AssignField(p, age, i(1))

	clone := mem.Clone()
	clone.AssignField(p, age, i(2))
	q := clone.AllocateStruct(person)

	assertValid(t, mem, eq(mem.GetFieldValue(p, age), i(1)))
	assertValid(t, clone, and(eq(clone.GetFieldValue(p, age), i(2)), ne(p, q)))
}

func TestTypeErrors(t *testing.T) {
	mem := newMemory()
	p := mem.AllocateStruct(person)
	mustPanic := func(name string, action func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected panic", name)
			}
		}()
		action()
	}
	mustPanic("wrong value type", func() { mem.AssignField(p, age, s.NewBoolConstant(true)) })
	mustPanic("missing field", func() { mem.GetFieldValue(p, 2) })
	mustPanic("struct as array", func() { mem.GetFromArray(p, 0) })
	mustPanic("int as reference", func() { mem.AssignField(mem.AllocateStruct(s.StructInfo("Box", s.PointerInfo(person))), 0, i(1)) })
}
