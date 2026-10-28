// Package symbolic определяет базовые типы символьных выражений
package symbolic

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
