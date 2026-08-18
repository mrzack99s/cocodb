package kv

type BatchOpType uint8

const (
	BatchPut BatchOpType = iota + 1
	BatchDelete
)

type BatchOp struct {
	Type  BatchOpType
	Key   []byte
	Value []byte
}

// Batch represents an atomic batch of KV mutations.
type Batch struct {
	ops []BatchOp
}

func NewBatch() *Batch {
	return &Batch{
		ops: make([]BatchOp, 0),
	}
}

func (b *Batch) Put(key, value []byte) *Batch {
	k := make([]byte, len(key))
	copy(k, key)
	v := make([]byte, len(value))
	copy(v, value)
	b.ops = append(b.ops, BatchOp{Type: BatchPut, Key: k, Value: v})
	return b
}

func (b *Batch) Delete(key []byte) *Batch {
	k := make([]byte, len(key))
	copy(k, key)
	b.ops = append(b.ops, BatchOp{Type: BatchDelete, Key: k})
	return b
}

func (b *Batch) Ops() []BatchOp {
	return b.ops
}
