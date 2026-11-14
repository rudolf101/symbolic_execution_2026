package internal

import (
	"regexp"
	"strings"
	"testing"
)

func generate(t *testing.T, body string) string {
	t.Helper()
	code, err := GenerateTests("package main\n\n" + body)
	if err != nil {
		t.Fatal(err)
	}
	return string(code)
}

func TestGeneratedCasesCoverAllBranches(t *testing.T) {
	code := generate(t, `
func f(x int) int {
	if x > 10 {
		return 1
	}
	if x < -500 {
		return 2
	}
	return 3
}`)
	if n := strings.Count(code, "t.Run("); n != 3 {
		t.Fatalf("expected 3 cases, got %d:\n%s", n, code)
	}
	for _, want := range []string{"want := 1;", "want := 2;", "want := 3;"} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %q in\n%s", want, code)
		}
	}
	// Предпочитаются небольшие значения параметров
	for _, arg := range regexp.MustCompile(`f\((-?\d+)\)`).FindAllStringSubmatch(code, -1) {
		if len(arg[1]) > 4 {
			t.Errorf("argument %s is not small", arg[1])
		}
	}
}

func TestBreakEdgeIsCovered(t *testing.T) {
	code := generate(t, `
func f(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		if i == 3 {
			break
		}
		s += i
	}
	return s
}`)
	if !strings.Contains(code, "want := 3;") {
		t.Errorf("path through break is not selected:\n%s", code)
	}
}

func TestPanicsAndVoidFunctions(t *testing.T) {
	code := generate(t, `
func f(x int) {
	if x == 7 {
		panic("seven")
	}
}`)
	for _, want := range []string{`strings.Contains(fmt.Sprint(r), "seven")`, "\t\tf(7)\n", `"fmt"`, `"strings"`} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %q in\n%s", want, code)
		}
	}
	if strings.Count(code, "t.Run(") != 2 || !regexp.MustCompile(`\t\tf\(-?\d+\)\n\t}\)`).MatchString(code) {
		t.Errorf("expected a returning and a panicking case:\n%s", code)
	}
}

func TestTypedLiterals(t *testing.T) {
	code := generate(t, `
func f(a uint8, b float32, c bool) float32 {
	if a > 200 && c {
		return b
	}
	return 0
}`)
	if !regexp.MustCompile(`f\(uint8\(\d+\), float32\([-.\de]+\), (true|false)\)`).MatchString(code) {
		t.Errorf("unexpected literals:\n%s", code)
	}
	if !strings.Contains(code, "func sameFloat(") {
		t.Errorf("float results must be compared with sameFloat:\n%s", code)
	}
}

func TestUnsupportedFunctionsAreSkipped(t *testing.T) {
	code := generate(t, `
type point struct{ x int }

func f(p *point) int {
	return p.x
}`)
	if !strings.Contains(code, "// f: тесты не сгенерированы") || strings.Contains(code, "import") {
		t.Errorf("unexpected output:\n%s", code)
	}
}
