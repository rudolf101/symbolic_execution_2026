package memory

import (
	"fmt"

	"symbolic-execution-course/internal/symbolic"
)

type Memory interface {
	// Allocate выделяет ячейку примитивного типа (доступ через поле 0) или массив int, если tpe == ArrayType
	Allocate(tpe symbolic.ExpressionType) *symbolic.Ref

	AllocateStruct(info *symbolic.TypeInfo) *symbolic.Ref

	AllocateArray(element *symbolic.TypeInfo, length symbolic.SymbolicExpression) *symbolic.Ref

	// NewSymbolicRef создаёт входную ссылку с символьным адресом: она может быть nil и указывать на тот же объект,
	// что и другие входные ссылки, но не на объекты, выделенные во время анализа
	NewSymbolicRef(name string, target *symbolic.TypeInfo) *symbolic.Ref

	AssignField(ref *symbolic.Ref, fieldIdx int, value symbolic.SymbolicExpression)

	GetFieldValue(ref *symbolic.Ref, fieldIdx int) symbolic.SymbolicExpression

	AssignToArray(ref *symbolic.Ref, index int, value symbolic.SymbolicExpression)

	GetFromArray(ref *symbolic.Ref, index int) symbolic.SymbolicExpression

	AssignToArrayAt(ref *symbolic.Ref, index symbolic.SymbolicExpression, value symbolic.SymbolicExpression)

	GetFromArrayAt(ref *symbolic.Ref, index symbolic.SymbolicExpression) symbolic.SymbolicExpression

	Length(ref *symbolic.Ref) symbolic.SymbolicExpression

	// Constraints возвращает ограничения на адреса входных ссылок, которые нужно добавить к условию пути
	Constraints() symbolic.SymbolicExpression

	Clone() Memory
}

// freshBase - начало диапазона адресов объектов, выделенных во время анализа.
// Адреса входных ссылок ограничены значениями ниже него.
const freshBase = int64(1) << 20

// SymbolicMemory реализует модель памяти из лекции: для каждого поля каждого типа структуры хранится
// символьный массив "адрес -> значение поля", для массивов каждого типа - "адрес -> (индекс -> элемент)",
// для длин массивов - "адрес -> длина". Записи и чтения - операции store/select над этими массивами,
// поэтому алиасинг ссылок с символьными адресами разрешается SMT-решателем.
type SymbolicMemory struct {
	regions map[string]symbolic.ArrayExpression
	next    int64
	inputs  []*symbolic.Ref
}

func NewSymbolicMemory() SymbolicMemory {
	return SymbolicMemory{
		regions: make(map[string]symbolic.ArrayExpression),
		next:    freshBase,
	}
}

func (mem *SymbolicMemory) Allocate(tpe symbolic.ExpressionType) *symbolic.Ref {
	if tpe == symbolic.ArrayType {
		return mem.AllocateArray(symbolic.ScalarInfo(symbolic.IntType), symbolic.NewIntConstant(0))
	}
	ref := mem.fresh(symbolic.ScalarInfo(tpe))
	mem.AssignField(ref, 0, zeroValue(ref.Target))
	return ref
}

func (mem *SymbolicMemory) AllocateStruct(info *symbolic.TypeInfo) *symbolic.Ref {
	if info.Kind != symbolic.StructType {
		panic(fmt.Sprintf("%s is not a struct", info.Key()))
	}
	ref := mem.fresh(info)
	for i, field := range info.Fields {
		mem.AssignField(ref, i, mem.zero(field))
	}
	return ref
}

func (mem *SymbolicMemory) AllocateArray(element *symbolic.TypeInfo, length symbolic.SymbolicExpression) *symbolic.Ref {
	info := symbolic.ArrayInfo(element, -1)
	if c, ok := length.(*symbolic.IntConstant); ok {
		info.Length = int(c.Value)
	}
	ref := mem.fresh(info)
	mem.setLength(ref, length)

	key := arrayKey(element)
	contents := symbolic.NewConstArray(contentSort(element), toStorage(element, zeroValue(element)))
	mem.regions[key] = symbolic.NewArrayStore(mem.region(key, arrayRegionSort(element)), ref.Address, contents)

	// Элементы-структуры и массивы фиксированной длины - отдельные объекты, их можно выделить только для известной длины
	if element.Kind == symbolic.StructType || (element.Kind == symbolic.ArrayType && element.Length >= 0) {
		for i := 0; i < info.Length; i++ {
			mem.AssignToArray(ref, i, mem.zero(element))
		}
	}
	return ref
}

func (mem *SymbolicMemory) NewSymbolicRef(name string, target *symbolic.TypeInfo) *symbolic.Ref {
	ref := symbolic.NewRef(symbolic.NewSymbolicVariable(name, symbolic.AddressType), target)
	mem.inputs = append(mem.inputs, ref)
	return ref
}

func (mem *SymbolicMemory) AssignField(ref *symbolic.Ref, fieldIdx int, value symbolic.SymbolicExpression) {
	field := fieldInfo(ref.Target, fieldIdx)
	key := fieldKey(ref.Target, fieldIdx)
	mem.regions[key] = symbolic.NewArrayStore(mem.region(key, fieldRegionSort(field)), ref.Address, toStorage(field, value))
}

func (mem *SymbolicMemory) GetFieldValue(ref *symbolic.Ref, fieldIdx int) symbolic.SymbolicExpression {
	field := fieldInfo(ref.Target, fieldIdx)
	key := fieldKey(ref.Target, fieldIdx)
	return fromStorage(field, read(mem.region(key, fieldRegionSort(field)), ref.Address))
}

func (mem *SymbolicMemory) AssignToArray(ref *symbolic.Ref, index int, value symbolic.SymbolicExpression) {
	mem.AssignToArrayAt(ref, symbolic.NewIntConstant(int64(index)), value)
}

func (mem *SymbolicMemory) GetFromArray(ref *symbolic.Ref, index int) symbolic.SymbolicExpression {
	return mem.GetFromArrayAt(ref, symbolic.NewIntConstant(int64(index)))
}

func (mem *SymbolicMemory) AssignToArrayAt(ref *symbolic.Ref, index symbolic.SymbolicExpression, value symbolic.SymbolicExpression) {
	element := arrayElement(ref)
	key := arrayKey(element)
	region := mem.region(key, arrayRegionSort(element))
	contents := symbolic.NewArrayStore(read(region, ref.Address).(symbolic.ArrayExpression), index, toStorage(element, value))
	mem.regions[key] = symbolic.NewArrayStore(region, ref.Address, contents)
}

func (mem *SymbolicMemory) GetFromArrayAt(ref *symbolic.Ref, index symbolic.SymbolicExpression) symbolic.SymbolicExpression {
	element := arrayElement(ref)
	contents := read(mem.region(arrayKey(element), arrayRegionSort(element)), ref.Address).(symbolic.ArrayExpression)
	return fromStorage(element, read(contents, index))
}

func (mem *SymbolicMemory) Length(ref *symbolic.Ref) symbolic.SymbolicExpression {
	arrayElement(ref)
	return read(mem.region(lengthKey, lengthSort), ref.Address)
}

func (mem *SymbolicMemory) Constraints() symbolic.SymbolicExpression {
	if len(mem.inputs) == 0 {
		return symbolic.NewBoolConstant(true)
	}
	bound := symbolic.NewTypedIntConstant(freshBase, symbolic.AddressType)
	conditions := make([]symbolic.SymbolicExpression, len(mem.inputs))
	for i, input := range mem.inputs {
		conditions[i] = symbolic.NewBinaryOperation(input.Address, bound, symbolic.LT)
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	return symbolic.NewLogicalOperation(conditions, symbolic.AND)
}

func (mem *SymbolicMemory) Clone() Memory {
	clone := &SymbolicMemory{
		regions: make(map[string]symbolic.ArrayExpression, len(mem.regions)),
		next:    mem.next,
		inputs:  append([]*symbolic.Ref{}, mem.inputs...),
	}
	for key, region := range mem.regions {
		clone.regions[key] = region
	}
	return clone
}

func (mem *SymbolicMemory) fresh(target *symbolic.TypeInfo) *symbolic.Ref {
	mem.next++
	return symbolic.NewRef(symbolic.NewTypedIntConstant(mem.next, symbolic.AddressType), target)
}

// region возвращает текущее состояние области памяти; изначально её содержимое произвольно (входная память)
func (mem *SymbolicMemory) region(key string, sort *symbolic.ArraySort) symbolic.ArrayExpression {
	if region, ok := mem.regions[key]; ok {
		return region
	}
	return symbolic.NewArrayVariable("heap:"+key, sort)
}

func (mem *SymbolicMemory) setLength(ref *symbolic.Ref, length symbolic.SymbolicExpression) {
	mem.regions[lengthKey] = symbolic.NewArrayStore(mem.region(lengthKey, lengthSort), ref.Address, length)
}

// zero возвращает нулевое значение типа; вложенные структуры и массивы фиксированной длины выделяются как отдельные объекты
func (mem *SymbolicMemory) zero(info *symbolic.TypeInfo) symbolic.SymbolicExpression {
	switch {
	case info.Kind == symbolic.StructType:
		return mem.AllocateStruct(info)
	case info.Kind == symbolic.ArrayType && info.Length >= 0:
		return mem.AllocateArray(info.Element, symbolic.NewIntConstant(int64(info.Length)))
	}
	return zeroValue(info)
}

// read строит чтение из массива, пропуская записи по заведомо другим конкретным индексам
func read(array symbolic.ArrayExpression, index symbolic.SymbolicExpression) symbolic.SymbolicExpression {
	key, concrete := index.(*symbolic.IntConstant)
	for concrete {
		switch a := array.(type) {
		case *symbolic.ArrayStore:
			storeKey, ok := a.Index.(*symbolic.IntConstant)
			if !ok {
				return symbolic.NewArraySelect(array, index)
			}
			if storeKey.Value == key.Value {
				return a.Value
			}
			array = a.Array
		case *symbolic.ConstArray:
			return a.Value
		default:
			concrete = false
		}
	}
	return symbolic.NewArraySelect(array, index)
}

const lengthKey = "len"

var lengthSort = &symbolic.ArraySort{Index: symbolic.AddressType, Element: symbolic.IntType}

func fieldKey(target *symbolic.TypeInfo, fieldIdx int) string {
	return fmt.Sprintf("%s.%d", target.Key(), fieldIdx)
}

func arrayKey(element *symbolic.TypeInfo) string {
	return "[]" + element.Key()
}

func fieldRegionSort(field *symbolic.TypeInfo) *symbolic.ArraySort {
	return &symbolic.ArraySort{Index: symbolic.AddressType, Element: field.StorageType()}
}

func contentSort(element *symbolic.TypeInfo) *symbolic.ArraySort {
	return &symbolic.ArraySort{Index: symbolic.IntType, Element: element.StorageType()}
}

func arrayRegionSort(element *symbolic.TypeInfo) *symbolic.ArraySort {
	return &symbolic.ArraySort{Index: symbolic.AddressType, Element: symbolic.ArrayType, Inner: contentSort(element)}
}

func fieldInfo(target *symbolic.TypeInfo, fieldIdx int) *symbolic.TypeInfo {
	switch {
	case target.Kind == symbolic.StructType && fieldIdx >= 0 && fieldIdx < len(target.Fields):
		return target.Fields[fieldIdx]
	case target.Kind != symbolic.StructType && target.Kind != symbolic.ArrayType && fieldIdx == 0:
		return target
	}
	panic(fmt.Sprintf("%s has no field %d", target.Key(), fieldIdx))
}

func arrayElement(ref *symbolic.Ref) *symbolic.TypeInfo {
	if ref.Target.Kind != symbolic.ArrayType {
		panic(fmt.Sprintf("%s is not an array", ref.Target.Key()))
	}
	return ref.Target.Element
}

func toStorage(info *symbolic.TypeInfo, value symbolic.SymbolicExpression) symbolic.SymbolicExpression {
	if info.ReferenceTarget() != nil {
		ref, ok := value.(*symbolic.Ref)
		if !ok {
			panic(fmt.Sprintf("expected reference to %s, got %s", info.ReferenceTarget().Key(), value))
		}
		return ref.Address
	}
	if value.Type() != info.Kind {
		panic(fmt.Sprintf("expected %s, got %s of type %s", info.Kind, value, value.Type()))
	}
	return value
}

func fromStorage(info *symbolic.TypeInfo, value symbolic.SymbolicExpression) symbolic.SymbolicExpression {
	if target := info.ReferenceTarget(); target != nil {
		return symbolic.NewRef(value, target)
	}
	return value
}

func zeroValue(info *symbolic.TypeInfo) symbolic.SymbolicExpression {
	switch {
	case info.ReferenceTarget() != nil:
		return symbolic.NilRef(info.ReferenceTarget())
	case info.Kind == symbolic.BoolType:
		return symbolic.NewBoolConstant(false)
	case info.Kind.IsFloat():
		return symbolic.NewTypedFloatConstant(0, info.Kind)
	default:
		return symbolic.NewTypedIntConstant(0, info.Kind)
	}
}
