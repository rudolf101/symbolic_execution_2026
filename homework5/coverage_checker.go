package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"symbolic-execution-course/internal"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <path-to-go-file>")
		os.Exit(1)
	}

	sourceFilePath := os.Args[1]

	if _, err := os.Stat(sourceFilePath); os.IsNotExist(err) {
		fmt.Printf("Error: file %s does not exist\n", sourceFilePath)
		os.Exit(1)
	}

	testFilePath := internal.GenerateTestFile(sourceFilePath)

	fmt.Printf("Generated test file: %s\n", testFilePath)

	coverage, err := runTestsWithCoverage(testFilePath)
	if err != nil {
		fmt.Printf("Error running tests: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Code Coverage ===\n%s\n", coverage)
}

func runTestsWithCoverage(testFilePath string) (string, error) {
	testDir := filepath.Dir(testFilePath)

	coverageFile := filepath.Join(os.TempDir(), "coverage.out")
	defer os.Remove(coverageFile)

	// Запускаются только сгенерированные тесты, чтобы другие тесты каталога не влияли на покрытие
	runPattern, err := generatedTestsPattern(testFilePath)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("go", "test", "-v", "-count=1", "-run", runPattern, "-coverprofile="+coverageFile, testDir)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("\n=== Running Tests ===\n")
	fmt.Printf("Command: %s\n", strings.Join(cmd.Args, " "))

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("test execution failed: %w", err)
	}

	coverage, err := analyzeCoverage(coverageFile)
	if err != nil {
		return "", fmt.Errorf("failed to analyze coverage: %w", err)
	}

	return coverage, nil
}

func generatedTestsPattern(testFilePath string) (string, error) {
	code, err := os.ReadFile(testFilePath)
	if err != nil {
		return "", err
	}
	var names []string
	for _, match := range regexp.MustCompile(`(?m)^func (Test\w*)\(`).FindAllStringSubmatch(string(code), -1) {
		names = append(names, match[1])
	}
	return "^(" + strings.Join(names, "|") + ")$", nil
}

func analyzeCoverage(coverageFile string) (string, error) {
	cmd := exec.Command("go", "tool", "cover",
		"-func="+coverageFile)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run go tool cover: %w\nOutput: %s", err, output)
	}

	coverageOutput := string(output)

	return fmt.Sprintf("%s", coverageOutput), nil
}
