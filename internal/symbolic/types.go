// Package symbolic определяет базовые типы символьных выражений
package symbolic

import "fmt"

// ExpressionType представляет тип символьного выражения
type ExpressionType int

const (
	IntType ExpressionType = iota
	BoolType
	ArrayType
	Int8Type
	Int16Type
	Int32Type
	Int64Type
	UintType
	Uint8Type
	Uint16Type
	Uint32Type
	Uint64Type
	Float32Type
	Float64Type
	RefType
	StructType
)

// String возвращает строковое представление типа
func (et ExpressionType) String() string {
	switch et {
	case IntType:
		return "int"
	case BoolType:
		return "bool"
	case ArrayType:
		return "array"
	case Int8Type:
		return "int8"
	case Int16Type:
		return "int16"
	case Int32Type:
		return "int32"
	case Int64Type:
		return "int64"
	case UintType:
		return "uint"
	case Uint8Type:
		return "uint8"
	case Uint16Type:
		return "uint16"
	case Uint32Type:
		return "uint32"
	case Uint64Type:
		return "uint64"
	case Float32Type:
		return "float32"
	case Float64Type:
		return "float64"
	case RefType:
		return "ref"
	case StructType:
		return "struct"
	default:
		return "unknown"
	}
}

// IsInteger сообщает, является ли тип целочисленным
func (et ExpressionType) IsInteger() bool {
	return et.BitWidth() > 0
}

// IsFloat сообщает, является ли тип числом с плавающей точкой
func (et ExpressionType) IsFloat() bool {
	return et == Float32Type || et == Float64Type
}

// IsNumeric сообщает, является ли тип числовым
func (et ExpressionType) IsNumeric() bool {
	return et.IsInteger() || et.IsFloat()
}

// IsSigned сообщает, является ли целочисленный тип знаковым
func (et ExpressionType) IsSigned() bool {
	switch et {
	case IntType, Int8Type, Int16Type, Int32Type, Int64Type:
		return true
	default:
		return false
	}
}

// BitWidth возвращает разрядность целочисленного типа (0 для остальных типов)
func (et ExpressionType) BitWidth() int {
	switch et {
	case Int8Type, Uint8Type:
		return 8
	case Int16Type, Uint16Type:
		return 16
	case Int32Type, Uint32Type:
		return 32
	case IntType, Int64Type, UintType, Uint64Type:
		return 64
	default:
		return 0
	}
}

// AddressType - тип адресов объектов в символьной памяти
const AddressType = Uint64Type

// TypeInfo описывает тип объекта в памяти: скаляр, структуру, массив или указатель.
// Поля-структуры и поля-массивы хранятся как ссылки на отдельные объекты.
type TypeInfo struct {
	Kind    ExpressionType
	Name    string      // имя структуры
	Fields  []*TypeInfo // поля структуры
	Element *TypeInfo   // элемент массива
	Length  int         // длина массива фиксированного размера, -1 для срезов
	Target  *TypeInfo   // тип, на который указывает указатель
}

// ScalarInfo описывает значение примитивного типа
func ScalarInfo(t ExpressionType) *TypeInfo {
	if !t.IsNumeric() && t != BoolType {
		panic(fmt.Sprintf("%s is not a scalar type", t))
	}
	return &TypeInfo{Kind: t}
}

// StructInfo описывает структуру с заданными полями
func StructInfo(name string, fields ...*TypeInfo) *TypeInfo {
	return &TypeInfo{Kind: StructType, Name: name, Fields: fields}
}

// ArrayInfo описывает массив длины length (-1 для среза)
func ArrayInfo(element *TypeInfo, length int) *TypeInfo {
	return &TypeInfo{Kind: ArrayType, Element: element, Length: length}
}

// PointerInfo описывает указатель на target
func PointerInfo(target *TypeInfo) *TypeInfo {
	return &TypeInfo{Kind: RefType, Target: target}
}

// Key возвращает имя типа; типы с одинаковым именем делят области памяти
func (info *TypeInfo) Key() string {
	switch info.Kind {
	case StructType:
		return info.Name
	case ArrayType:
		if info.Length >= 0 {
			return fmt.Sprintf("[%d]%s", info.Length, info.Element.Key())
		}
		return "[]" + info.Element.Key()
	case RefType:
		return "*" + info.Target.Key()
	default:
		return info.Kind.String()
	}
}

// StorageType возвращает тип, которым значение такого типа хранится в памяти
func (info *TypeInfo) StorageType() ExpressionType {
	if info.Kind == RefType || info.Kind == StructType || info.Kind == ArrayType {
		return AddressType
	}
	return info.Kind
}

// ReferenceTarget возвращает тип объекта, ссылка на который хранится в значении такого типа
func (info *TypeInfo) ReferenceTarget() *TypeInfo {
	if info.Kind == RefType {
		return info.Target
	}
	if info.Kind == StructType || info.Kind == ArrayType {
		return info
	}
	return nil
}

// ArraySort описывает сорт символьного массива: отображение Index -> Element.
// Если Element == ArrayType, сорт элемента задаётся Inner.
type ArraySort struct {
	Index   ExpressionType
	Element ExpressionType
	Inner   *ArraySort
}

// Equal сравнивает сорта структурно
func (s *ArraySort) Equal(other *ArraySort) bool {
	if s == nil || other == nil {
		return s == other
	}
	return s.Index == other.Index && s.Element == other.Element && s.Inner.Equal(other.Inner)
}

func (s *ArraySort) String() string {
	element := s.Element.String()
	if s.Inner != nil {
		element = s.Inner.String()
	}
	return fmt.Sprintf("(%s -> %s)", s.Index, element)
}
