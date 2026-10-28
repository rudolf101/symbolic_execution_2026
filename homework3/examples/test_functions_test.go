package main

import (
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ebukreev/go-z3/z3"
	"symbolic-execution-course/internal"
	"symbolic-execution-course/internal/symbolic"
)

var functions = map[string]any{
	"test1":                 test1,
	"test2":                 test2,
	"testArithmetic":        testArithmetic,
	"testUnary":             testUnary,
	"testComparisons":       testComparisons,
	"testLogicalOps":        testLogicalOps,
	"testWhileLoop":         testWhileLoop,
	"testForLoop":           testForLoop,
	"testInfiniteLoopBreak": testInfiniteLoopBreak,
	"testLoopWithConcreteBoundAndSymbolicBranching": testLoopWithConcreteBoundAndSymbolicBranching,
	"testLoopWithSymbolicBoundAndSymbolicBranching": testLoopWithSymbolicBoundAndSymbolicBranching,
	"testComplexConditions":                         testComplexConditions,
	"testNestedIf":                                  testNestedIf,
	"testBitwise":                                   testBitwise,
	"testCombined":                                  testCombined,
	"testEdgeCases":                                 testEdgeCases,
	"testMultipleReturns":                           testMultipleReturns,
	"testSimpleSum":                                 testSimpleSum,
}

// Количество путей для функций без циклов с символьной границей
var expectedPaths = map[string]int{
	"test1":                 2,
	"test2":                 3,
	"testArithmetic":        2,
	"testUnary":             2,
	"testComparisons":       4,
	"testLogicalOps":        3,
	"testComplexConditions": 6,
	"testNestedIf":          4,
	"testBitwise":           2,
	"testEdgeCases":         3,
	"testMultipleReturns":   3,
	"testSimpleSum":         3,
	"testLoopWithConcreteBoundAndSymbolicBranching": 2,
}

func TestEveryPathMatchesConcreteExecution(t *testing.T) {
	source, err := os.ReadFile("test_functions.go")
	if err != nil {
		t.Fatal(err)
	}
	for name, function := range functions {
		t.Run(name, func(t *testing.T) {
			analyser := internal.AnalyseWith(string(source), name, internal.DefaultOptions())
			if len(analyser.Results) == 0 {
				t.Fatal("no paths found")
			}
			if want, ok := expectedPaths[name]; ok {
				if len(analyser.Results) != want || analyser.StatesQueue.Len() != 0 {
					t.Errorf("found %d paths (%d unfinished), want %d", len(analyser.Results), analyser.StatesQueue.Len(), want)
				}
			}
			for _, state := range analyser.Results {
				checkPath(t, analyser, reflect.ValueOf(function), state)
			}
		})
	}
}

func checkPath(t *testing.T, analyser *internal.Analyser, function reflect.Value, state internal.Interpreter) {
	t.Helper()
	tr := analyser.Z3Translator
	solver := z3.NewSolver(tr.Context())
	solver.Assert(translate(t, tr, state.PathCondition).(z3.Bool))
	if sat, err := solver.Check(); !sat || err != nil {
		t.Fatalf("infeasible path %s: %v", state, err)
	}
	model := solver.Model()

	signature := function.Type()
	ssaFunction := state.CallStack[0].Function
	args := make([]reflect.Value, signature.NumIn())
	for i := range args {
		param := ssaFunction.Params[i]
		variable := symbolic.NewSymbolicVariable(param.Name(), state.CallStack[0].LocalMemory[param.Name()].Type())
		args[i] = toGo(t, model.Eval(translate(t, tr, variable), true), signature.In(i))
	}

	results, panicked := call(function, args)
	switch state.Status {
	case internal.Panicked:
		if panicked == nil || !strings.Contains(fmt.Sprint(panicked), state.PanicMessage) {
			t.Errorf("%s on %v: concrete execution gave %v, panic %v", state, args, results, panicked)
		}
	case internal.Returned:
		if panicked != nil {
			t.Fatalf("%s on %v: concrete execution panicked with %v", state, args, panicked)
		}
		expected := toGo(t, model.Eval(translate(t, tr, state.CallStack[0].ReturnValue), true), signature.Out(0))
		if !same(expected, results[0]) {
			t.Errorf("%s on %v: symbolic result %v, concrete %v", state, args, expected, results[0])
		}
	default:
		t.Errorf("unexpected status of finished state %s", state)
	}
}

func call(function reflect.Value, args []reflect.Value) (results []reflect.Value, panicked any) {
	defer func() { panicked = recover() }()
	return function.Call(args), nil
}

func translate(t *testing.T, tr interface {
	TranslateExpression(symbolic.SymbolicExpression) (interface{}, error)
}, expr symbolic.SymbolicExpression) z3.Value {
	t.Helper()
	value, err := tr.TranslateExpression(expr)
	if err != nil {
		t.Fatal(err)
	}
	return value.(z3.Value)
}

func toGo(t *testing.T, value z3.Value, goType reflect.Type) reflect.Value {
	t.Helper()
	result := reflect.New(goType).Elem()
	switch v := value.(type) {
	case z3.Bool:
		b, ok := v.AsBool()
		if !ok {
			t.Fatalf("not a literal: %s", v)
		}
		result.SetBool(b)
	case z3.BV:
		n, ok := v.AsBigSigned()
		if !ok {
			t.Fatalf("not a literal: %s", v)
		}
		result.SetInt(n.Int64())
	case z3.Float:
		f, ok := v.AsBigFloat()
		if !ok {
			t.Fatalf("not a literal: %s", v)
		}
		if f == nil {
			result.SetFloat(math.NaN())
		} else {
			x, _ := f.Float64()
			result.SetFloat(x)
		}
	default:
		t.Fatalf("unexpected value %T", value)
	}
	return result
}

func same(a, b reflect.Value) bool {
	if a.Kind() == reflect.Float64 {
		return math.Float64bits(a.Float()) == math.Float64bits(b.Float()) || (math.IsNaN(a.Float()) && math.IsNaN(b.Float()))
	}
	return a.Interface() == b.Interface()
}
