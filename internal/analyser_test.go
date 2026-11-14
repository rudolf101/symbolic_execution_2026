package internal

import (
	"sort"
	"strings"
	"testing"
)

func analyseSource(t *testing.T, body string, options Options) *Analyser {
	t.Helper()
	return AnalyseWith("package main\n\n"+body, "f", options)
}

func describe(states []Interpreter) []string {
	result := make([]string, len(states))
	for i, state := range states {
		result[i] = state.String()
	}
	sort.Strings(result)
	return result
}

func expectPaths(t *testing.T, body string, want ...string) {
	t.Helper()
	got := describe(analyseSource(t, body, DefaultOptions()).Results)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("paths:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBranchesAndConstantFolding(t *testing.T) {
	expectPaths(t, `
func f(x int) int {
	a := 2
	b := a * 3
	if b > 5 {
		return x + b
	}
	return 0
}`, "return (x + 6) when true")
}

func TestInfeasibleBranchesArePruned(t *testing.T) {
	expectPaths(t, `
func f(x int) int {
	if x > 10 {
		if x < 5 {
			return 1
		}
		return 2
	}
	return 3
}`,
		"return 2 when ((x > 10) && !(x < 5))",
		"return 3 when !(x > 10)")
}

func TestDivisionByZeroPanics(t *testing.T) {
	expectPaths(t, `
func f(x, y int) int {
	return x / y
}`,
		`panic("integer divide by zero") when !(y != 0)`,
		"return (x / y) when (y != 0)")
	expectPaths(t, `
func f(x int) int {
	return x % 3
}`, "return (x % 3) when true")
}

func TestNegativeShiftPanics(t *testing.T) {
	expectPaths(t, `
func f(x int, s int) int {
	return x << s
}`,
		`panic("negative shift amount") when !(s >= 0)`,
		"return (x << s) when (s >= 0)")
	expectPaths(t, `
func f(x int, s uint) int {
	return x >> s
}`, "return (x >> s) when true")
}

func TestExplicitPanic(t *testing.T) {
	expectPaths(t, `
func f(x int) int {
	if x < 0 {
		panic("negative")
	}
	return x
}`,
		`panic("negative") when (x < 0)`,
		"return x when !(x < 0)")
}

func TestPhiNodesAreEvaluatedSimultaneously(t *testing.T) {
	expectPaths(t, `
func f(a, b int) (int, int) {
	for i := 0; i < 3; i++ {
		a, b = b, a
	}
	return a, b
}`, "return b, a when true")
}

func TestSmallIntegersWrapAround(t *testing.T) {
	expectPaths(t, `
func f(x int8) int8 {
	var y int8 = 127
	y++
	if x > y {
		return x
	}
	return y
}`, "return x when (x > -128)", "return -128 when !(x > -128)")
}

func TestFloats(t *testing.T) {
	expectPaths(t, `
func f(x float64) int {
	if x != x {
		return 0
	}
	return int(x * 0.5)
}`,
		"return 0 when (x != x)",
		"return int((x * 0.5)) when !(x != x)")
}

func TestMultipleReturnValues(t *testing.T) {
	expectPaths(t, `
func f(x int) (int, bool) {
	return -x, !(x > 0)
}`, "return -x, !(x > 0) when true")
}

func TestSymbolicLoopIsBounded(t *testing.T) {
	options := DefaultOptions()
	options.MaxBranchVisits = 5
	analyser := analyseSource(t, `
func f(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i
	}
	return s
}`, options)
	if len(analyser.Results) != 5 || analyser.DroppedPaths != 1 || analyser.StatesQueue.Len() != 0 {
		t.Errorf("results %d, dropped %d, queued %d", len(analyser.Results), analyser.DroppedPaths, analyser.StatesQueue.Len())
	}
}

func TestStepBudgetStopsAnalysis(t *testing.T) {
	options := DefaultOptions()
	options.MaxSteps = 50
	analyser := analyseSource(t, `
func f(n int) int {
	for {
		n++
	}
}`, options)
	if analyser.Steps != 50 || analyser.StatesQueue.Len() != 1 || len(analyser.Results) != 0 {
		t.Errorf("steps %d, queued %d, results %d", analyser.Steps, analyser.StatesQueue.Len(), len(analyser.Results))
	}
}

func TestPathSelectorsFindSamePaths(t *testing.T) {
	body := `
func f(a, b, c int) int {
	r := 0
	if a > b {
		r++
	}
	if b > c {
		r += 2
	}
	if a > c {
		r += 4
	}
	return r
}`
	var expected []string
	for _, selector := range []PathSelector{&BfsPathSelector{}, &DfsPathSelector{}, &RandomPathSelector{}, &CoverageGuidedPathSelector{}} {
		options := DefaultOptions()
		options.PathSelector = selector
		got := describe(analyseSource(t, body, options).Results)
		if len(got) != 6 {
			t.Errorf("%T: found %d paths, want 6", selector, len(got))
		}
		if expected == nil {
			expected = got
		} else if strings.Join(got, "\n") != strings.Join(expected, "\n") {
			t.Errorf("%T found different paths:\n%s", selector, strings.Join(got, "\n"))
		}
	}
}
