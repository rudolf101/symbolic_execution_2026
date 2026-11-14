// Package ssa предоставляет функции для построения SSA представления
package ssa

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Builder отвечает за построение SSA из исходного кода Go
type Builder struct {
	fset *token.FileSet
}

// NewBuilder создаёт новый экземпляр Builder
func NewBuilder() *Builder {
	return &Builder{
		fset: token.NewFileSet(),
	}
}

// BuildPackage парсит исходный код Go и строит SSA представление всего пакета
func (b *Builder) BuildPackage(source string) (*ssa.Package, error) {
	file, err := parser.ParseFile(b.fset, "main.go", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	info := &types.Info{
		Types:        make(map[ast.Expr]types.TypeAndValue),
		Defs:         make(map[*ast.Ident]types.Object),
		Uses:         make(map[*ast.Ident]types.Object),
		Implicits:    make(map[ast.Node]types.Object),
		Instances:    make(map[*ast.Ident]types.Instance),
		Scopes:       make(map[ast.Node]*types.Scope),
		Selections:   make(map[*ast.SelectorExpr]*types.Selection),
		FileVersions: make(map[*ast.File]string),
	}
	pkg := types.NewPackage(file.Name.Name, file.Name.Name)
	config := &types.Config{Importer: importer.Default()}
	if err := types.NewChecker(config, b.fset, pkg, info).Files([]*ast.File{file}); err != nil {
		return nil, fmt.Errorf("type check: %w", err)
	}

	program := ssa.NewProgram(b.fset, ssa.SanityCheckFunctions)
	ssaPkg := program.CreatePackage(pkg, []*ast.File{file}, info, false)
	ssaPkg.Build()
	return ssaPkg, nil
}

// ParseAndBuildSSA парсит исходный код Go и создаёт SSA представление
// Возвращает SSA программу и функцию по имени
func (b *Builder) ParseAndBuildSSA(source string, funcName string) (*ssa.Function, error) {
	pkg, err := b.BuildPackage(source)
	if err != nil {
		return nil, err
	}

	function := pkg.Func(funcName)
	if function == nil {
		return nil, fmt.Errorf("function %q not found", funcName)
	}
	return function, nil
}
