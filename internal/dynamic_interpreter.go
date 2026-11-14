package internal

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"strings"

	"github.com/ebukreev/go-z3/z3"
	"golang.org/x/tools/go/ssa"
	"symbolic-execution-course/internal/memory"
	"symbolic-execution-course/internal/symbolic"
)

type ExecutionStatus int

const (
	Running ExecutionStatus = iota
	Returned
	Panicked
)

type Interpreter struct {
	CallStack     []CallStackFrame
	Analyser      *Analyser
	PathCondition symbolic.SymbolicExpression
	Heap          memory.Memory
	Status        ExecutionStatus
	PanicMessage  string
	pathFormula   []z3.Bool
	model         *z3.Model // модель условия пути, позволяет не вызывать решатель для ветвей, которым она удовлетворяет
}

type CallStackFrame struct {
	Function     *ssa.Function
	LocalMemory  map[string]symbolic.SymbolicExpression
	ReturnValue  symbolic.SymbolicExpression
	ReturnValues []symbolic.SymbolicExpression
	Block        *ssa.BasicBlock
	InstrIndex   int
	// BranchVisits - сколько раз путь символьно ветвился в каждом блоке
	BranchVisits map[*ssa.BasicBlock]int
}

func newInterpreter(analyser *Analyser, function *ssa.Function) Interpreter {
	frame := CallStackFrame{
		Function:     function,
		LocalMemory:  make(map[string]symbolic.SymbolicExpression),
		Block:        function.Blocks[0],
		BranchVisits: make(map[*ssa.BasicBlock]int),
	}
	for _, param := range function.Params {
		frame.LocalMemory[localKey(param)] = symbolic.NewSymbolicVariable(param.Name(), expressionType(param.Type()))
	}
	return Interpreter{
		CallStack:     []CallStackFrame{frame},
		Analyser:      analyser,
		PathCondition: symbolic.NewBoolConstant(true),
	}
}

func (interpreter *Interpreter) frame() *CallStackFrame {
	return &interpreter.CallStack[len(interpreter.CallStack)-1]
}

func (interpreter *Interpreter) currentInstruction() ssa.Instruction {
	frame := interpreter.frame()
	return frame.Block.Instrs[frame.InstrIndex]
}

func (interpreter *Interpreter) clone() Interpreter {
	result := *interpreter
	result.CallStack = make([]CallStackFrame, len(interpreter.CallStack))
	for i, frame := range interpreter.CallStack {
		frame.LocalMemory = make(map[string]symbolic.SymbolicExpression, len(frame.LocalMemory))
		for key, value := range interpreter.CallStack[i].LocalMemory {
			frame.LocalMemory[key] = value
		}
		frame.BranchVisits = make(map[*ssa.BasicBlock]int, len(frame.BranchVisits))
		for block, visits := range interpreter.CallStack[i].BranchVisits {
			frame.BranchVisits[block] = visits
		}
		result.CallStack[i] = frame
	}
	return result
}

func (interpreter *Interpreter) interpretDynamically(element ssa.Instruction) []Interpreter {
	switch instr := element.(type) {
	case *ssa.If:
		return interpreter.branch(instr)
	case *ssa.Jump:
		interpreter.enterBlock(instr.Block().Succs[0])
	case *ssa.Return:
		frame := interpreter.frame()
		for _, result := range instr.Results {
			frame.ReturnValues = append(frame.ReturnValues, interpreter.resolveExpression(result))
		}
		if len(frame.ReturnValues) == 1 {
			frame.ReturnValue = frame.ReturnValues[0]
		}
		interpreter.Status = Returned
	case *ssa.Panic:
		interpreter.Status = Panicked
		interpreter.PanicMessage = panicMessage(instr.X)
	case *ssa.BinOp:
		if guard, message := interpreter.runtimeCheck(instr); guard != nil {
			return interpreter.guarded(guard, message, func(state *Interpreter) {
				state.assign(instr, state.binaryOperation(instr))
			})
		}
		interpreter.assign(instr, interpreter.binaryOperation(instr))
	case *ssa.UnOp:
		interpreter.assign(instr, interpreter.unaryOperation(instr))
	case *ssa.Convert:
		operand := interpreter.resolveExpression(instr.X)
		target := expressionType(instr.Type())
		if operand.Type() == target {
			interpreter.assign(instr, operand)
		} else {
			interpreter.assign(instr, symbolic.NewConversion(operand, target))
		}
	case *ssa.ChangeType:
		interpreter.assign(instr, interpreter.resolveExpression(instr.X))
	case *ssa.MakeInterface, *ssa.DebugRef:
		// Интерфейсы поддерживаются только как аргумент panic, значение читается в panicMessage
		interpreter.frame().InstrIndex++
	default:
		panic(fmt.Sprintf("unsupported instruction %T: %s", element, element))
	}
	return []Interpreter{*interpreter}
}

func (interpreter *Interpreter) resolveExpression(value ssa.Value) symbolic.SymbolicExpression {
	switch v := value.(type) {
	case *ssa.Const:
		return constantExpression(v)
	default:
		if expr, ok := interpreter.frame().LocalMemory[localKey(value)]; ok {
			return expr
		}
		panic(fmt.Sprintf("unresolved value %s (%T)", value.Name(), value))
	}
}

func (interpreter *Interpreter) assign(value ssa.Value, expr symbolic.SymbolicExpression) {
	frame := interpreter.frame()
	frame.LocalMemory[localKey(value)] = interpreter.Analyser.fold(expr)
	frame.InstrIndex++
}

func (interpreter *Interpreter) enterBlock(target *ssa.BasicBlock) {
	frame := interpreter.frame()
	predIndex := -1
	for i, pred := range target.Preds {
		if pred == frame.Block {
			predIndex = i
			break
		}
	}

	// Все phi блока вычисляются одновременно относительно значений на входе
	values := make(map[string]symbolic.SymbolicExpression)
	index := 0
	for ; index < len(target.Instrs); index++ {
		phi, ok := target.Instrs[index].(*ssa.Phi)
		if !ok {
			break
		}
		values[localKey(phi)] = interpreter.resolveExpression(phi.Edges[predIndex])
	}
	for key, value := range values {
		frame.LocalMemory[key] = value
	}
	frame.Block = target
	frame.InstrIndex = index
	interpreter.Analyser.BlockVisits[target]++
}

func (interpreter *Interpreter) branch(instr *ssa.If) []Interpreter {
	condition := interpreter.resolveExpression(instr.Cond)
	thenBlock, elseBlock := instr.Block().Succs[0], instr.Block().Succs[1]
	if c, ok := condition.(*symbolic.BoolConstant); ok {
		if c.Value {
			interpreter.enterBlock(thenBlock)
		} else {
			interpreter.enterBlock(elseBlock)
		}
		return []Interpreter{*interpreter}
	}

	branchVisits := interpreter.frame().BranchVisits
	branchVisits[instr.Block()]++
	if branchVisits[instr.Block()] > interpreter.Analyser.Options.MaxBranchVisits {
		interpreter.Analyser.DroppedPaths++
		return nil
	}

	var states []Interpreter
	for _, alternative := range []struct {
		guard symbolic.SymbolicExpression
		block *ssa.BasicBlock
	}{
		{condition, thenBlock},
		{negate(condition), elseBlock},
	} {
		state := interpreter.clone()
		if state.assume(alternative.guard) {
			state.enterBlock(alternative.block)
			states = append(states, state)
		}
	}
	return states
}

// guarded разделяет исполнение на путь, где guard нарушен (паника), и путь, где он выполнен.
func (interpreter *Interpreter) guarded(guard symbolic.SymbolicExpression, message string, proceed func(*Interpreter)) []Interpreter {
	var states []Interpreter
	failed := interpreter.clone()
	if failed.assume(negate(guard)) {
		failed.Status = Panicked
		failed.PanicMessage = message
		states = append(states, failed)
	}
	if interpreter.assume(guard) {
		proceed(interpreter)
		states = append(states, *interpreter)
	}
	return states
}

// assume добавляет условие к условию пути и сообщает, остался ли путь выполнимым.
func (interpreter *Interpreter) assume(condition symbolic.SymbolicExpression) bool {
	if c, ok := condition.(*symbolic.BoolConstant); ok {
		return c.Value
	}
	if c, ok := interpreter.PathCondition.(*symbolic.BoolConstant); ok && c.Value {
		interpreter.PathCondition = condition
	} else if and, ok := interpreter.PathCondition.(*symbolic.LogicalOperation); ok && and.Operator == symbolic.AND {
		operands := append(append([]symbolic.SymbolicExpression{}, and.Operands...), condition)
		interpreter.PathCondition = symbolic.NewLogicalOperation(operands, symbolic.AND)
	} else {
		interpreter.PathCondition = symbolic.NewLogicalOperation([]symbolic.SymbolicExpression{interpreter.PathCondition, condition}, symbolic.AND)
	}
	formula := interpreter.Analyser.translate(condition).(z3.Bool)
	// полное выражение среза не даёт клонам дописывать в общий массив
	interpreter.pathFormula = append(interpreter.pathFormula[:len(interpreter.pathFormula):len(interpreter.pathFormula)], formula)
	if interpreter.model != nil {
		if holds, ok := interpreter.model.Eval(formula, true).(z3.Bool).AsBool(); ok && holds {
			return true
		}
	}
	model, sat := interpreter.Analyser.solve(interpreter.pathFormula)
	interpreter.model = model
	return sat
}

var binaryOperators = map[token.Token]symbolic.BinaryOperator{
	token.ADD:     symbolic.ADD,
	token.SUB:     symbolic.SUB,
	token.MUL:     symbolic.MUL,
	token.QUO:     symbolic.DIV,
	token.REM:     symbolic.MOD,
	token.AND:     symbolic.BIT_AND,
	token.OR:      symbolic.BIT_OR,
	token.XOR:     symbolic.BIT_XOR,
	token.AND_NOT: symbolic.AND_NOT,
	token.SHL:     symbolic.SHL,
	token.SHR:     symbolic.SHR,
	token.EQL:     symbolic.EQ,
	token.NEQ:     symbolic.NE,
	token.LSS:     symbolic.LT,
	token.LEQ:     symbolic.LE,
	token.GTR:     symbolic.GT,
	token.GEQ:     symbolic.GE,
}

func (interpreter *Interpreter) binaryOperation(instr *ssa.BinOp) symbolic.SymbolicExpression {
	op, ok := binaryOperators[instr.Op]
	if !ok {
		panic(fmt.Sprintf("unsupported binary operator %s", instr.Op))
	}
	return symbolic.NewBinaryOperation(interpreter.resolveExpression(instr.X), interpreter.resolveExpression(instr.Y), op)
}

// runtimeCheck возвращает условие, при нарушении которого операция вызывает панику в Go.
func (interpreter *Interpreter) runtimeCheck(instr *ssa.BinOp) (symbolic.SymbolicExpression, string) {
	right := interpreter.resolveExpression(instr.Y)
	t := right.Type()
	if !t.IsInteger() {
		return nil, ""
	}
	zero := symbolic.NewTypedIntConstant(0, t)
	switch {
	case instr.Op == token.QUO || instr.Op == token.REM:
		return interpreter.Analyser.fold(symbolic.NewBinaryOperation(right, zero, symbolic.NE)), "integer divide by zero"
	case (instr.Op == token.SHL || instr.Op == token.SHR) && t.IsSigned():
		return interpreter.Analyser.fold(symbolic.NewBinaryOperation(right, zero, symbolic.GE)), "negative shift amount"
	}
	return nil, ""
}

func (interpreter *Interpreter) unaryOperation(instr *ssa.UnOp) symbolic.SymbolicExpression {
	operand := interpreter.resolveExpression(instr.X)
	switch instr.Op {
	case token.SUB:
		return symbolic.NewUnaryOperation(operand, symbolic.NEG)
	case token.XOR:
		return symbolic.NewUnaryOperation(operand, symbolic.BIT_NOT)
	case token.NOT:
		return negate(operand)
	}
	panic(fmt.Sprintf("unsupported unary operator %s", instr.Op))
}

func negate(expr symbolic.SymbolicExpression) symbolic.SymbolicExpression {
	switch e := expr.(type) {
	case *symbolic.BoolConstant:
		return symbolic.NewBoolConstant(!e.Value)
	case *symbolic.LogicalOperation:
		if e.Operator == symbolic.NOT {
			return e.Operands[0]
		}
	}
	return symbolic.NewLogicalOperation([]symbolic.SymbolicExpression{expr}, symbolic.NOT)
}

func constantExpression(c *ssa.Const) symbolic.SymbolicExpression {
	t := expressionType(c.Type())
	switch {
	case c.Value == nil:
		panic(fmt.Sprintf("unsupported constant %s", c))
	case t == symbolic.BoolType:
		return symbolic.NewBoolConstant(constant.BoolVal(c.Value))
	case t.IsFloat():
		value, _ := constant.Float64Val(constant.ToFloat(c.Value))
		return symbolic.NewTypedFloatConstant(value, t)
	case t.IsSigned():
		value, _ := constant.Int64Val(c.Value)
		return symbolic.NewTypedIntConstant(value, t)
	default:
		value, _ := constant.Uint64Val(c.Value)
		return symbolic.NewTypedIntConstant(int64(value), t)
	}
}

func expressionType(t types.Type) symbolic.ExpressionType {
	if basic, ok := t.Underlying().(*types.Basic); ok {
		switch basic.Kind() {
		case types.Bool, types.UntypedBool:
			return symbolic.BoolType
		case types.Int, types.UntypedInt:
			return symbolic.IntType
		case types.Int8:
			return symbolic.Int8Type
		case types.Int16:
			return symbolic.Int16Type
		case types.Int32, types.UntypedRune:
			return symbolic.Int32Type
		case types.Int64:
			return symbolic.Int64Type
		case types.Uint, types.Uintptr:
			return symbolic.UintType
		case types.Uint8:
			return symbolic.Uint8Type
		case types.Uint16:
			return symbolic.Uint16Type
		case types.Uint32:
			return symbolic.Uint32Type
		case types.Uint64:
			return symbolic.Uint64Type
		case types.Float32:
			return symbolic.Float32Type
		case types.Float64, types.UntypedFloat:
			return symbolic.Float64Type
		}
	}
	panic(fmt.Sprintf("unsupported type %s", t))
}

func localKey(value ssa.Value) string {
	if _, ok := value.(*ssa.Parameter); ok {
		return value.Name()
	}
	return "%" + value.Name()
}

func panicMessage(value ssa.Value) string {
	if iface, ok := value.(*ssa.MakeInterface); ok {
		value = iface.X
	}
	if c, ok := value.(*ssa.Const); ok && c.Value != nil {
		if c.Value.Kind() == constant.String {
			return constant.StringVal(c.Value)
		}
		return c.Value.ExactString()
	}
	return "panic"
}

func (interpreter Interpreter) String() string {
	var result string
	switch interpreter.Status {
	case Returned:
		values := make([]string, len(interpreter.frame().ReturnValues))
		for i, value := range interpreter.frame().ReturnValues {
			values[i] = value.String()
		}
		result = "return " + strings.Join(values, ", ")
	case Panicked:
		result = fmt.Sprintf("panic(%q)", interpreter.PanicMessage)
	default:
		result = "running at " + interpreter.currentInstruction().String()
	}
	return fmt.Sprintf("%s when %s", result, interpreter.PathCondition)
}

// fold заменяет выражение над константами его значением
func (analyser *Analyser) fold(expr symbolic.SymbolicExpression) symbolic.SymbolicExpression {
	if !hasConstantOperands(expr) {
		return expr
	}
	simplified := analyser.Z3Translator.Context().Simplify(analyser.translate(expr), nil)
	t := expr.Type()
	switch value := simplified.(type) {
	case z3.Bool:
		if b, ok := value.AsBool(); ok {
			return symbolic.NewBoolConstant(b)
		}
	case z3.BV:
		if t.IsSigned() {
			if n, ok := value.AsBigSigned(); ok {
				return symbolic.NewTypedIntConstant(n.Int64(), t)
			}
		} else if n, ok := value.AsBigUnsigned(); ok {
			return symbolic.NewTypedIntConstant(int64(n.Uint64()), t)
		}
	case z3.Float:
		if f, ok := value.AsBigFloat(); ok {
			if f == nil {
				return symbolic.NewTypedFloatConstant(math.NaN(), t)
			}
			v, _ := f.Float64()
			return symbolic.NewTypedFloatConstant(v, t)
		}
	}
	return expr
}

func hasConstantOperands(expr symbolic.SymbolicExpression) bool {
	var operands []symbolic.SymbolicExpression
	switch e := expr.(type) {
	case *symbolic.BinaryOperation:
		operands = []symbolic.SymbolicExpression{e.Left, e.Right}
	case *symbolic.UnaryOperation:
		operands = []symbolic.SymbolicExpression{e.Operand}
	case *symbolic.Conversion:
		operands = []symbolic.SymbolicExpression{e.Operand}
	case *symbolic.LogicalOperation:
		operands = e.Operands
	default:
		return false
	}
	for _, operand := range operands {
		switch operand.(type) {
		case *symbolic.IntConstant, *symbolic.FloatConstant, *symbolic.BoolConstant:
		default:
			return false
		}
	}
	return true
}
