package main

import (
	"fmt"

	"symbolic-execution-course/internal/memory"
	"symbolic-execution-course/internal/symbolic"
)

func main() {
	var mem = memory.NewSymbolicMemory()
	var array = mem.Allocate(symbolic.ArrayType)

	mem.AssignToArray(array, 5, symbolic.NewIntConstant(10))

	var fromArray = mem.GetFromArray(array, 5)
	fmt.Println(fromArray)

	var anotherFromArray = mem.GetFromArray(array, 10)
	fmt.Println(anotherFromArray)

	foo := symbolic.StructInfo("Foo", symbolic.ScalarInfo(symbolic.IntType))
	foo1 := mem.NewSymbolicRef("foo1", foo)
	foo2 := mem.NewSymbolicRef("foo2", foo)
	mem.AssignField(foo2, 0, symbolic.NewIntConstant(5))
	mem.AssignField(foo1, 0, symbolic.NewIntConstant(2))
	fmt.Println(mem.GetFieldValue(foo2, 0))
}
