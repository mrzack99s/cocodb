package catalog

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"cocodb/internal/types"
)

type ObjectType uint8

const (
	ObjectBucket ObjectType = iota + 1
	ObjectCollection
	ObjectIndex
	ObjectVectorIndex
	ObjectTextIndex
	ObjectQueue
)

func (t ObjectType) String() string {
	switch t {
	case ObjectBucket:
		return "Bucket"
	case ObjectCollection:
		return "Collection"
	case ObjectIndex:
		return "Index"
	case ObjectVectorIndex:
		return "VectorIndex"
	case ObjectTextIndex:
		return "TextIndex"
	case ObjectQueue:
		return "Queue"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

// Object represents a schema catalog entry.
type Object struct {
	ID        types.ObjectID
	Type      ObjectType
	Name      string
	Root      types.PageID
	Flags     uint64
	ExtraData []byte // e.g. Index definition, schema rules
}

// Encode serializes the Object into bytes for storage in the catalog B+Tree.
func (o *Object) Encode() []byte {
	var buf bytes.Buffer
	buf.WriteByte(byte(o.Type))
	_ = binary.Write(&buf, binary.BigEndian, uint64(o.ID))
	_ = binary.Write(&buf, binary.BigEndian, uint64(o.Root))
	_ = binary.Write(&buf, binary.BigEndian, o.Flags)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(o.Name)))
	buf.WriteString(o.Name)
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(o.ExtraData)))
	buf.Write(o.ExtraData)
	return buf.Bytes()
}

// DecodeObject decodes bytes into an Object.
func DecodeObject(data []byte) (*Object, error) {
	if len(data) < 25 {
		return nil, types.ErrInvalidFormat
	}
	r := bytes.NewReader(data)
	typeByte, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	var id, root, flags uint64
	var nameLen uint16
	_ = binary.Read(r, binary.BigEndian, &id)
	_ = binary.Read(r, binary.BigEndian, &root)
	_ = binary.Read(r, binary.BigEndian, &flags)
	_ = binary.Read(r, binary.BigEndian, &nameLen)

	nameBytes := make([]byte, nameLen)
	if _, err := r.Read(nameBytes); err != nil {
		return nil, err
	}

	var extraLen uint32
	_ = binary.Read(r, binary.BigEndian, &extraLen)
	extra := make([]byte, extraLen)
	if extraLen > 0 {
		_, _ = r.Read(extra)
	}

	return &Object{
		ID:        types.ObjectID(id),
		Type:      ObjectType(typeByte),
		Name:      string(nameBytes),
		Root:      types.PageID(root),
		Flags:     flags,
		ExtraData: extra,
	}, nil
}
