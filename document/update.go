package document

// Document represents a NoSQL document.
type Document map[string]any

// UpdateOp represents a partial document update operation.
type UpdateOp struct {
	Type  UpdateType
	Field string
	Value any
}

type UpdateType uint8

const (
	OpSet UpdateType = iota + 1
	OpUnset
	OpIncrement
	OpPush
	OpRemove
)

func Set(field string, value any) UpdateOp {
	return UpdateOp{Type: OpSet, Field: field, Value: value}
}

func Unset(field string) UpdateOp {
	return UpdateOp{Type: OpUnset, Field: field}
}

func Increment(field string, delta int64) UpdateOp {
	return UpdateOp{Type: OpIncrement, Field: field, Value: delta}
}

func Push(field string, value any) UpdateOp {
	return UpdateOp{Type: OpPush, Field: field, Value: value}
}

func Remove(field string, value any) UpdateOp {
	return UpdateOp{Type: OpRemove, Field: field, Value: value}
}
