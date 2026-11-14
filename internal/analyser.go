package internal

import (
	"container/heap"
	"fmt"

	"github.com/ebukreev/go-z3/z3"
	"golang.org/x/tools/go/ssa"
	ssabuilder "symbolic-execution-course/internal/ssa"
	"symbolic-execution-course/internal/symbolic"
	"symbolic-execution-course/internal/translator"
)

// Options задают стратегию выбора пути и стратегию остановки анализа:
// анализ завершается, когда очередь состояний пуста или исчерпан глобальный бюджет инструкций MaxSteps.
// Кроме того, путь отбрасывается, если он больше MaxBranchVisits раз символьно ветвится в одном блоке
// (ограничение развёртки циклов с символьным условием; циклы с конкретным условием не ограничиваются).
type Options struct {
	PathSelector    PathSelector
	MaxSteps        int
	MaxBranchVisits int
}

func DefaultOptions() Options {
	return Options{
		PathSelector:    &BfsPathSelector{},
		MaxSteps:        10000,
		MaxBranchVisits: 20,
	}
}

type Analyser struct {
	Package      *ssa.Package
	StatesQueue  PriorityQueue
	PathSelector PathSelector
	Results      []Interpreter
	Z3Translator *translator.Z3Translator

	Options      Options
	Steps        int
	DroppedPaths int
	BlockVisits  map[*ssa.BasicBlock]int
	solver       *z3.Solver
}

func Analyse(source string, functionName string) []Interpreter {
	return AnalyseWith(source, functionName, DefaultOptions()).Results
}

// AnalyseWith запускает анализ с заданными настройками.
// Состояния, не завершившиеся до исчерпания бюджета, остаются в StatesQueue.
func AnalyseWith(source string, functionName string, options Options) *Analyser {
	function, err := ssabuilder.NewBuilder().ParseAndBuildSSA(source, functionName)
	if err != nil {
		panic(err)
	}
	if len(function.Blocks) == 0 {
		panic(fmt.Sprintf("function %s has no body", functionName))
	}

	z3Translator := translator.NewZ3Translator()
	analyser := &Analyser{
		Package:      function.Pkg,
		PathSelector: options.PathSelector,
		Z3Translator: z3Translator,
		Options:      options,
		BlockVisits:  make(map[*ssa.BasicBlock]int),
		solver:       z3.NewSolver(z3Translator.Context()),
	}
	analyser.BlockVisits[function.Blocks[0]]++
	analyser.push(newInterpreter(analyser, function))
	analyser.run()
	return analyser
}

func (analyser *Analyser) run() {
	for analyser.StatesQueue.Len() > 0 && analyser.Steps < analyser.Options.MaxSteps {
		state := heap.Pop(&analyser.StatesQueue).(*Item).value
		analyser.Steps++
		for _, next := range state.interpretDynamically(state.currentInstruction()) {
			if next.Status == Running {
				analyser.push(next)
			} else {
				analyser.Results = append(analyser.Results, next)
			}
		}
	}
}

func (analyser *Analyser) push(state Interpreter) {
	heap.Push(&analyser.StatesQueue, &Item{value: state, priority: analyser.PathSelector.CalculatePriority(state)})
}

func (analyser *Analyser) translate(expr symbolic.SymbolicExpression) z3.Value {
	value, err := analyser.Z3Translator.TranslateExpression(expr)
	if err != nil {
		panic(err)
	}
	return value.(z3.Value)
}

// solve возвращает модель формулы или nil, если формула невыполнима.
// Если решатель не смог дать ответ, путь считается выполнимым, а модель - неизвестной.
func (analyser *Analyser) solve(conjuncts []z3.Bool) (model *z3.Model, sat bool) {
	analyser.solver.Push()
	defer analyser.solver.Pop()
	for _, conjunct := range conjuncts {
		analyser.solver.Assert(conjunct)
	}
	sat, err := analyser.solver.Check()
	if err != nil {
		return nil, true
	}
	if !sat {
		return nil, false
	}
	return analyser.solver.Model(), true
}
