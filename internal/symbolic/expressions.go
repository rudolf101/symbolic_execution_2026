// Package symbolic содержит конкретные реализации символьных выражений
package symbolic

import (
	"fmt"
	"strconv"
	"strings"
)

// SymbolicExpression - базовый интерфейс для всех символьных выражений
type SymbolicExpression interface {
	// Type возвращает тип выражения
	Type() ExpressionType

	// String возвращает строковое представление выражения
	String() string

	// Accept принимает visitor для обхода дерева выражений
	Accept(visitor Visitor) interface{}
}

// SymbolicVariable представляет символьную переменную
type SymbolicVariable struct {
	Name     string
	ExprType ExpressionType
}

// NewSymbolicVariable создаёт новую символьную переменную
func NewSymbolicVariable(name string, exprType ExpressionType) *SymbolicVariable {
	return &SymbolicVariable{
		Name:     name,
		ExprType: exprType,
	}
}

// Type возвращает тип переменной
func (sv *SymbolicVariable) Type() ExpressionType {
	return sv.ExprType
}

// String возвращает строковое представление переменной
func (sv *SymbolicVariable) String() string {
	return sv.Name
}

// Accept реализует Visitor pattern
func (sv *SymbolicVariable) Accept(visitor Visitor) interface{} {
	return visitor.VisitVariable(sv)
}

// IntConstant представляет целочисленную константу.
// Для беззнаковых типов Value хранит битовое представление числа.
type IntConstant struct {
	Value    int64
	ExprType ExpressionType
}

// NewIntConstant создаёт новую целочисленную константу типа int
func NewIntConstant(value int64) *IntConstant {
	return NewTypedIntConstant(value, IntType)
}

// NewTypedIntConstant создаёт целочисленную константу заданного типа
func NewTypedIntConstant(value int64, exprType ExpressionType) *IntConstant {
	if !exprType.IsInteger() {
		panic(fmt.Sprintf("integer constant of non-integer type %s", exprType))
	}
	return &IntConstant{Value: value, ExprType: exprType}
}

// Type возвращает тип константы
func (ic *IntConstant) Type() ExpressionType {
	return ic.ExprType
}

// String возвращает строковое представление константы
func (ic *IntConstant) String() string {
	if ic.ExprType.IsSigned() {
		return strconv.FormatInt(ic.Value, 10)
	}
	return strconv.FormatUint(uint64(ic.Value), 10)
}

// Accept реализует Visitor pattern
func (ic *IntConstant) Accept(visitor Visitor) interface{} {
	return visitor.VisitIntConstant(ic)
}

// FloatConstant представляет константу с плавающей точкой
type FloatConstant struct {
	Value    float64
	ExprType ExpressionType
}

// NewFloatConstant создаёт новую константу типа float64
func NewFloatConstant(value float64) *FloatConstant {
	return NewTypedFloatConstant(value, Float64Type)
}

// NewTypedFloatConstant создаёт константу с плавающей точкой заданного типа
func NewTypedFloatConstant(value float64, exprType ExpressionType) *FloatConstant {
	if !exprType.IsFloat() {
		panic(fmt.Sprintf("float constant of non-float type %s", exprType))
	}
	if exprType == Float32Type {
		value = float64(float32(value))
	}
	return &FloatConstant{Value: value, ExprType: exprType}
}

// Type возвращает тип константы
func (fc *FloatConstant) Type() ExpressionType {
	return fc.ExprType
}

// String возвращает строковое представление константы
func (fc *FloatConstant) String() string {
	return strconv.FormatFloat(fc.Value, 'g', -1, 64)
}

// Accept реализует Visitor pattern
func (fc *FloatConstant) Accept(visitor Visitor) interface{} {
	return visitor.VisitFloatConstant(fc)
}

// BoolConstant представляет булеву константу
type BoolConstant struct {
	Value bool
}

// NewBoolConstant создаёт новую булеву константу
func NewBoolConstant(value bool) *BoolConstant {
	return &BoolConstant{Value: value}
}

// Type возвращает тип константы
func (bc *BoolConstant) Type() ExpressionType {
	return BoolType
}

// String возвращает строковое представление константы
func (bc *BoolConstant) String() string {
	return fmt.Sprintf("%t", bc.Value)
}

// Accept реализует Visitor pattern
func (bc *BoolConstant) Accept(visitor Visitor) interface{} {
	return visitor.VisitBoolConstant(bc)
}

// BinaryOperation представляет бинарную операцию
type BinaryOperation struct {
	Left     SymbolicExpression
	Right    SymbolicExpression
	Operator BinaryOperator
}

// NewBinaryOperation создаёт новую бинарную операцию
func NewBinaryOperation(left, right SymbolicExpression, op BinaryOperator) *BinaryOperation {
	lt, rt := left.Type(), right.Type()
	mismatch := func() {
		panic(fmt.Sprintf("invalid operand types for %s: %s and %s", op, lt, rt))
	}
	switch {
	case op == ADD || op == SUB || op == MUL || op == DIV:
		if lt != rt || !lt.IsNumeric() {
			mismatch()
		}
	case op == MOD || op.IsBitwise():
		if lt != rt || !lt.IsInteger() {
			mismatch()
		}
	case op.IsShift():
		if !lt.IsInteger() || !rt.IsInteger() {
			mismatch()
		}
	case op == EQ || op == NE:
		if lt != rt || !(lt.IsNumeric() || lt == BoolType) {
			mismatch()
		}
	case op.IsComparison():
		if lt != rt || !lt.IsNumeric() {
			mismatch()
		}
	default:
		panic(fmt.Sprintf("unknown binary operator %d", op))
	}
	return &BinaryOperation{Left: left, Right: right, Operator: op}
}

// Type возвращает результирующий тип операции
func (bo *BinaryOperation) Type() ExpressionType {
	if bo.Operator.IsComparison() {
		return BoolType
	}
	return bo.Left.Type()
}

// String возвращает строковое представление операции
func (bo *BinaryOperation) String() string {
	return fmt.Sprintf("(%s %s %s)", bo.Left, bo.Operator, bo.Right)
}

// Accept реализует Visitor pattern
func (bo *BinaryOperation) Accept(visitor Visitor) interface{} {
	return visitor.VisitBinaryOperation(bo)
}

// LogicalOperation представляет логическую операцию
type LogicalOperation struct {
	Operands []SymbolicExpression
	Operator LogicalOperator
}

// NewLogicalOperation создаёт новую логическую операцию
func NewLogicalOperation(operands []SymbolicExpression, op LogicalOperator) *LogicalOperation {
	for _, operand := range operands {
		if operand.Type() != BoolType {
			panic(fmt.Sprintf("logical operand %s has type %s", operand, operand.Type()))
		}
	}
	switch op {
	case AND, OR:
		if len(operands) == 0 {
			panic(fmt.Sprintf("%s requires at least one operand", op))
		}
	case NOT:
		if len(operands) != 1 {
			panic("! requires exactly one operand")
		}
	case IMPLIES:
		if len(operands) != 2 {
			panic("=> requires exactly two operands")
		}
	default:
		panic(fmt.Sprintf("unknown logical operator %d", op))
	}
	return &LogicalOperation{Operands: operands, Operator: op}
}

// Type возвращает тип логической операции (всегда bool)
func (lo *LogicalOperation) Type() ExpressionType {
	return BoolType
}

// String возвращает строковое представление логической операции
func (lo *LogicalOperation) String() string {
	if lo.Operator == NOT {
		return "!" + lo.Operands[0].String()
	}
	parts := make([]string, len(lo.Operands))
	for i, operand := range lo.Operands {
		parts[i] = operand.String()
	}
	return "(" + strings.Join(parts, " "+lo.Operator.String()+" ") + ")"
}

// Accept реализует Visitor pattern
func (lo *LogicalOperation) Accept(visitor Visitor) interface{} {
	return visitor.VisitLogicalOperation(lo)
}

// Операторы для бинарных выражений
type BinaryOperator int

const (
	// Арифметические операторы
	ADD BinaryOperator = iota
	SUB
	MUL
	DIV
	MOD

	// Побитовые операторы
	BIT_AND
	BIT_OR
	BIT_XOR
	AND_NOT
	SHL
	SHR

	// Операторы сравнения
	EQ // равно
	NE // не равно
	LT // меньше
	LE // меньше или равно
	GT // больше
	GE // больше или равно
)

// String возвращает строковое представление оператора
func (op BinaryOperator) String() string {
	switch op {
	case ADD:
		return "+"
	case SUB:
		return "-"
	case MUL:
		return "*"
	case DIV:
		return "/"
	case MOD:
		return "%"
	case BIT_AND:
		return "&"
	case BIT_OR:
		return "|"
	case BIT_XOR:
		return "^"
	case AND_NOT:
		return "&^"
	case SHL:
		return "<<"
	case SHR:
		return ">>"
	case EQ:
		return "=="
	case NE:
		return "!="
	case LT:
		return "<"
	case LE:
		return "<="
	case GT:
		return ">"
	case GE:
		return ">="
	default:
		return "unknown"
	}
}

// IsComparison сообщает, возвращает ли оператор bool
func (op BinaryOperator) IsComparison() bool {
	return op >= EQ && op <= GE
}

// IsBitwise сообщает, является ли оператор побитовой операцией над двумя операндами одного типа
func (op BinaryOperator) IsBitwise() bool {
	return op >= BIT_AND && op <= AND_NOT
}

// IsShift сообщает, является ли оператор сдвигом
func (op BinaryOperator) IsShift() bool {
	return op == SHL || op == SHR
}

// Логические операторы
type LogicalOperator int

const (
	AND LogicalOperator = iota
	OR
	NOT
	IMPLIES
)

// String возвращает строковое представление логического оператора
func (op LogicalOperator) String() string {
	switch op {
	case AND:
		return "&&"
	case OR:
		return "||"
	case NOT:
		return "!"
	case IMPLIES:
		return "=>"
	default:
		return "unknown"
	}
}

type Ref struct {
	// TODO: Выбрать и написать внутреннее представление символьной ссылки
}

func (ref *Ref) Type() ExpressionType {
	panic("не реализовано")
}

func (ref *Ref) String() string {
	panic("не реализовано")
}

func (ref *Ref) Accept(visitor Visitor) interface{} {
	panic("не реализовано")
}

// UnaryOperator - арифметические унарные операторы (логическое отрицание - LogicalOperation с NOT)
type UnaryOperator int

const (
	NEG     UnaryOperator = iota // -x
	BIT_NOT                      // ^x
)

// String возвращает строковое представление оператора
func (op UnaryOperator) String() string {
	switch op {
	case NEG:
		return "-"
	case BIT_NOT:
		return "^"
	default:
		return "unknown"
	}
}

// UnaryOperation представляет унарную операцию
type UnaryOperation struct {
	Operand  SymbolicExpression
	Operator UnaryOperator
}

// NewUnaryOperation создаёт новую унарную операцию
func NewUnaryOperation(operand SymbolicExpression, op UnaryOperator) *UnaryOperation {
	t := operand.Type()
	if (op == NEG && !t.IsNumeric()) || (op == BIT_NOT && !t.IsInteger()) {
		panic(fmt.Sprintf("invalid operand type for unary %s: %s", op, t))
	}
	return &UnaryOperation{Operand: operand, Operator: op}
}

// Type возвращает тип операции
func (uo *UnaryOperation) Type() ExpressionType {
	return uo.Operand.Type()
}

// String возвращает строковое представление операции
func (uo *UnaryOperation) String() string {
	return uo.Operator.String() + uo.Operand.String()
}

// Accept реализует Visitor pattern
func (uo *UnaryOperation) Accept(visitor Visitor) interface{} {
	return visitor.VisitUnaryOperation(uo)
}

// Conversion представляет преобразование числового значения к другому типу
type Conversion struct {
	Operand    SymbolicExpression
	TargetType ExpressionType
}

// NewConversion создаёт новое преобразование типа
func NewConversion(operand SymbolicExpression, target ExpressionType) *Conversion {
	if !operand.Type().IsNumeric() || !target.IsNumeric() {
		panic(fmt.Sprintf("unsupported conversion from %s to %s", operand.Type(), target))
	}
	return &Conversion{Operand: operand, TargetType: target}
}

// Type возвращает целевой тип
func (c *Conversion) Type() ExpressionType {
	return c.TargetType
}

// String возвращает строковое представление преобразования
func (c *Conversion) String() string {
	return fmt.Sprintf("%s(%s)", c.TargetType, c.Operand)
}

// Accept реализует Visitor pattern
func (c *Conversion) Accept(visitor Visitor) interface{} {
	return visitor.VisitConversion(c)
}
