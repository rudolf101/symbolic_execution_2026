// Package translator содержит реализацию транслятора в Z3
package translator

import (
	"fmt"
	"math/big"

	"github.com/ebukreev/go-z3/z3"
	"symbolic-execution-course/internal/symbolic"
)

// Z3Translator транслирует символьные выражения в Z3 формулы.
// Целые числа представляются битовыми векторами соответствующей разрядности,
// числа с плавающей точкой - IEEE 754 числами Z3.
type Z3Translator struct {
	ctx    *z3.Context
	config *z3.Config
	vars   map[string]z3.Value // Кэш переменных
}

// NewZ3Translator создаёт новый экземпляр Z3 транслятора
func NewZ3Translator() *Z3Translator {
	config := &z3.Config{}
	ctx := z3.NewContext(config)

	return &Z3Translator{
		ctx:    ctx,
		config: config,
		vars:   make(map[string]z3.Value),
	}
}

// GetContext возвращает Z3 контекст
func (zt *Z3Translator) GetContext() interface{} {
	return zt.ctx
}

// Context возвращает Z3 контекст без приведения типов
func (zt *Z3Translator) Context() *z3.Context {
	return zt.ctx
}

// Reset сбрасывает состояние транслятора
func (zt *Z3Translator) Reset() {
	zt.vars = make(map[string]z3.Value)
}

// Close освобождает ресурсы
func (zt *Z3Translator) Close() {
	// Z3 контекст закрывается автоматически
}

// TranslateExpression транслирует символьное выражение в Z3
func (zt *Z3Translator) TranslateExpression(expr symbolic.SymbolicExpression) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			if translationError, ok := r.(*TranslationError); ok {
				result, err = nil, translationError
				return
			}
			panic(r)
		}
	}()
	return expr.Accept(zt), nil
}

// VisitVariable транслирует символьную переменную в Z3
func (zt *Z3Translator) VisitVariable(expr *symbolic.SymbolicVariable) interface{} {
	if v, ok := zt.vars[expr.Name]; ok {
		if !v.Sort().AsAST().Equal(zt.sort(expr.ExprType, expr).AsAST()) {
			panic(NewTranslationError(fmt.Sprintf("variable %s is used with different types", expr.Name), expr))
		}
		return v
	}
	v := zt.createZ3Variable(expr.Name, expr.ExprType)
	zt.vars[expr.Name] = v
	return v
}

// VisitIntConstant транслирует целочисленную константу в Z3
func (zt *Z3Translator) VisitIntConstant(expr *symbolic.IntConstant) interface{} {
	width := expr.ExprType.BitWidth()
	value := new(big.Int)
	if expr.ExprType.IsSigned() {
		value.SetInt64(expr.Value)
	} else {
		value.SetUint64(uint64(expr.Value))
	}
	value.Mod(value, new(big.Int).Lsh(big.NewInt(1), uint(width)))
	return zt.ctx.FromBigInt(value, zt.ctx.BVSort(width)).(z3.BV)
}

// VisitFloatConstant транслирует константу с плавающей точкой в Z3
func (zt *Z3Translator) VisitFloatConstant(expr *symbolic.FloatConstant) interface{} {
	return zt.ctx.FromFloat64(expr.Value, zt.sort(expr.ExprType, expr))
}

// VisitBoolConstant транслирует булеву константу в Z3
func (zt *Z3Translator) VisitBoolConstant(expr *symbolic.BoolConstant) interface{} {
	return zt.ctx.FromBool(expr.Value)
}

// VisitBinaryOperation транслирует бинарную операцию в Z3
func (zt *Z3Translator) VisitBinaryOperation(expr *symbolic.BinaryOperation) interface{} {
	left := expr.Left.Accept(zt)
	right := expr.Right.Accept(zt)
	t := expr.Left.Type()

	switch {
	case t == symbolic.BoolType:
		l, r := left.(z3.Bool), right.(z3.Bool)
		switch expr.Operator {
		case symbolic.EQ:
			return l.Iff(r)
		case symbolic.NE:
			return l.Xor(r)
		}
	case t.IsFloat():
		l, r := left.(z3.Float), right.(z3.Float)
		switch expr.Operator {
		case symbolic.ADD:
			return l.Add(r)
		case symbolic.SUB:
			return l.Sub(r)
		case symbolic.MUL:
			return l.Mul(r)
		case symbolic.DIV:
			return l.Div(r)
		case symbolic.EQ:
			return l.IEEEEq(r)
		case symbolic.NE:
			return l.IEEEEq(r).Not()
		case symbolic.LT:
			return l.LT(r)
		case symbolic.LE:
			return l.LE(r)
		case symbolic.GT:
			return l.GT(r)
		case symbolic.GE:
			return l.GE(r)
		}
	case t.IsInteger():
		l, r := left.(z3.BV), right.(z3.BV)
		signed := t.IsSigned()
		switch expr.Operator {
		case symbolic.ADD:
			return l.Add(r)
		case symbolic.SUB:
			return l.Sub(r)
		case symbolic.MUL:
			return l.Mul(r)
		case symbolic.DIV:
			if signed {
				return l.SDiv(r)
			}
			return l.UDiv(r)
		case symbolic.MOD:
			if signed {
				return l.SRem(r)
			}
			return l.URem(r)
		case symbolic.BIT_AND:
			return l.And(r)
		case symbolic.BIT_OR:
			return l.Or(r)
		case symbolic.BIT_XOR:
			return l.Xor(r)
		case symbolic.AND_NOT:
			return l.And(r.Not())
		case symbolic.SHL, symbolic.SHR:
			return zt.shift(l, r, t, expr.Right.Type(), expr.Operator)
		case symbolic.EQ:
			return l.Eq(r)
		case symbolic.NE:
			return l.NE(r)
		case symbolic.LT:
			if signed {
				return l.SLT(r)
			}
			return l.ULT(r)
		case symbolic.LE:
			if signed {
				return l.SLE(r)
			}
			return l.ULE(r)
		case symbolic.GT:
			if signed {
				return l.SGT(r)
			}
			return l.UGT(r)
		case symbolic.GE:
			if signed {
				return l.SGE(r)
			}
			return l.UGE(r)
		}
	}
	panic(NewTranslationError(fmt.Sprintf("unsupported operation %s for type %s", expr.Operator, t), expr))
}

// shift реализует сдвиги Go: разрядности операндов могут различаться, счётчик считается беззнаковым
// (проверка счётчика на неотрицательность выполняется интерпретатором).
func (zt *Z3Translator) shift(value, count z3.BV, valueType, countType symbolic.ExpressionType, op symbolic.BinaryOperator) z3.BV {
	width := valueType.BitWidth()
	wide := max(width, countType.BitWidth())
	if extra := wide - width; extra > 0 {
		if op == symbolic.SHR && valueType.IsSigned() {
			value = value.SignExtend(extra)
		} else {
			value = value.ZeroExtend(extra)
		}
	}
	if extra := wide - countType.BitWidth(); extra > 0 {
		count = count.ZeroExtend(extra)
	}

	var result z3.BV
	switch {
	case op == symbolic.SHL:
		result = value.Lsh(count)
	case valueType.IsSigned():
		result = value.SRsh(count)
	default:
		result = value.URsh(count)
	}
	if wide > width {
		result = result.Extract(width-1, 0)
	}
	return result
}

// VisitLogicalOperation транслирует логическую операцию в Z3
func (zt *Z3Translator) VisitLogicalOperation(expr *symbolic.LogicalOperation) interface{} {
	operands := make([]z3.Bool, len(expr.Operands))
	for i, operand := range expr.Operands {
		operands[i] = operand.Accept(zt).(z3.Bool)
	}

	switch expr.Operator {
	case symbolic.AND:
		return operands[0].And(operands[1:]...)
	case symbolic.OR:
		return operands[0].Or(operands[1:]...)
	case symbolic.NOT:
		return operands[0].Not()
	case symbolic.IMPLIES:
		return operands[0].Implies(operands[1])
	}
	panic(NewTranslationError(fmt.Sprintf("unsupported logical operator %s", expr.Operator), expr))
}

// VisitUnaryOperation транслирует унарную операцию в Z3
func (zt *Z3Translator) VisitUnaryOperation(expr *symbolic.UnaryOperation) interface{} {
	operand := expr.Operand.Accept(zt)
	switch value := operand.(type) {
	case z3.BV:
		if expr.Operator == symbolic.NEG {
			return value.Neg()
		}
		return value.Not()
	case z3.Float:
		if expr.Operator == symbolic.NEG {
			return value.Neg()
		}
	}
	panic(NewTranslationError(fmt.Sprintf("unsupported unary operator %s", expr.Operator), expr))
}

// VisitConversion транслирует преобразование числовых типов по правилам Go
func (zt *Z3Translator) VisitConversion(expr *symbolic.Conversion) interface{} {
	from, to := expr.Operand.Type(), expr.TargetType
	operand := expr.Operand.Accept(zt)

	switch {
	case from.IsInteger() && to.IsInteger():
		value := operand.(z3.BV)
		fromWidth, toWidth := from.BitWidth(), to.BitWidth()
		switch {
		case toWidth < fromWidth:
			return value.Extract(toWidth-1, 0)
		case toWidth > fromWidth && from.IsSigned():
			return value.SignExtend(toWidth - fromWidth)
		case toWidth > fromWidth:
			return value.ZeroExtend(toWidth - fromWidth)
		}
		return value
	case from.IsInteger() && to.IsFloat():
		if from.IsSigned() {
			return operand.(z3.BV).SToFloat(zt.sort(to, expr))
		}
		return operand.(z3.BV).UToFloat(zt.sort(to, expr))
	case from.IsFloat() && to.IsInteger():
		// Go отбрасывает дробную часть при преобразовании в целое
		previous := zt.ctx.SetRoundingMode(z3.RoundToZero)
		defer zt.ctx.SetRoundingMode(previous)
		if to.IsSigned() {
			return operand.(z3.Float).ToSBV(to.BitWidth())
		}
		return operand.(z3.Float).ToUBV(to.BitWidth())
	case from.IsFloat() && to.IsFloat():
		return operand.(z3.Float).ToFloat(zt.sort(to, expr))
	}
	panic(NewTranslationError(fmt.Sprintf("unsupported conversion from %s to %s", from, to), expr))
}

// Вспомогательные методы

func (zt *Z3Translator) sort(t symbolic.ExpressionType, expr symbolic.SymbolicExpression) z3.Sort {
	switch {
	case t == symbolic.BoolType:
		return zt.ctx.BoolSort()
	case t == symbolic.Float32Type:
		return zt.ctx.FloatSort(8, 24)
	case t == symbolic.Float64Type:
		return zt.ctx.FloatSort(11, 53)
	case t.IsInteger():
		return zt.ctx.BVSort(t.BitWidth())
	}
	panic(NewTranslationError(fmt.Sprintf("unsupported type %s", t), expr))
}

// createZ3Variable создаёт Z3 переменную соответствующего типа
func (zt *Z3Translator) createZ3Variable(name string, exprType symbolic.ExpressionType) z3.Value {
	return zt.ctx.Const(name, zt.sort(exprType, symbolic.NewSymbolicVariable(name, exprType)))
}

// castToZ3Type приводит значение к нужному Z3 типу
func (zt *Z3Translator) castToZ3Type(value interface{}, targetType symbolic.ExpressionType) (z3.Value, error) {
	var ok bool
	switch {
	case targetType == symbolic.BoolType:
		_, ok = value.(z3.Bool)
	case targetType.IsInteger():
		_, ok = value.(z3.BV)
	case targetType.IsFloat():
		_, ok = value.(z3.Float)
	}
	if !ok {
		return nil, fmt.Errorf("cannot cast %T to %s", value, targetType)
	}
	return value.(z3.Value), nil
}
