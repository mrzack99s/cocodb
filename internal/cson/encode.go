package cson

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"
)

var (
	boolTrue  = []byte{1}
	boolFalse = []byte{0}
)

type rawField struct {
	fieldID  uint32
	valType  ValueType
	valBytes []byte
	scalar   [8]byte
}

// Encode serializes a map[string]any document into CSON binary format.
func Encode(doc map[string]any, dict *FieldDictionary) ([]byte, error) {
	if dict == nil {
		dict = NewFieldDictionary()
	}

	docLen := len(doc)
	var rawFields []rawField
	if docLen <= 16 {
		var stackFields [16]rawField
		rawFields = stackFields[:0]
	} else {
		rawFields = make([]rawField, 0, docLen)
	}

	for k, val := range doc {
		fieldID := dict.GetOrAssignID(k)
		var sBuf [8]byte
		valType, valBytes, err := encodeValueFast(val, dict, &sBuf)
		if err != nil {
			return nil, fmt.Errorf("encoding field %q: %w", k, err)
		}

		rf := rawField{
			fieldID: fieldID,
			valType: valType,
		}
		if len(valBytes) > 0 && len(valBytes) <= 8 && &valBytes[0] == &sBuf[0] {
			rf.scalar = sBuf
			rf.valBytes = rf.scalar[:len(valBytes)]
		} else {
			rf.valBytes = valBytes
		}

		rawFields = append(rawFields, rf)
	}

	// Sort fields by fieldID so DocumentView binary search is valid and fast
	sort.Slice(rawFields, func(i, j int) bool {
		return rawFields[i].fieldID < rawFields[j].fieldID
	})

	totalPayloadLen := 0
	for _, f := range rawFields {
		totalPayloadLen += len(f.valBytes)
	}

	headerSize := 8 + len(rawFields)*FieldEntrySize
	result := make([]byte, headerSize+totalPayloadLen)

	// Write Header
	binary.BigEndian.PutUint32(result[0:4], CSONMagic)
	binary.BigEndian.PutUint16(result[4:6], CSONVersion)
	binary.BigEndian.PutUint16(result[6:8], uint16(len(rawFields)))

	// Write Field Directory & Payload in sorted order
	currOffset := uint32(headerSize)
	for i, f := range rawFields {
		off := 8 + i*FieldEntrySize
		binary.BigEndian.PutUint32(result[off:off+4], f.fieldID)
		result[off+4] = byte(f.valType)
		result[off+5] = 0 // reserved
		binary.BigEndian.PutUint32(result[off+6:off+10], currOffset)
		binary.BigEndian.PutUint32(result[off+10:off+14], uint32(len(f.valBytes)))

		if len(f.valBytes) > 0 {
			copy(result[currOffset:], f.valBytes)
			currOffset += uint32(len(f.valBytes))
		}
	}

	return result, nil
}

func encodeValueFast(v any, dict *FieldDictionary, scalar *[8]byte) (ValueType, []byte, error) {
	if v == nil {
		return TypeNull, nil, nil
	}

	switch val := v.(type) {
	case bool:
		if val {
			return TypeBool, boolTrue, nil
		}
		return TypeBool, boolFalse, nil

	case int:
		binary.BigEndian.PutUint64(scalar[:], uint64(val))
		return TypeInt64, scalar[:8], nil
	case int64:
		binary.BigEndian.PutUint64(scalar[:], uint64(val))
		return TypeInt64, scalar[:8], nil
	case int32:
		binary.BigEndian.PutUint64(scalar[:], uint64(int64(val)))
		return TypeInt64, scalar[:8], nil

	case uint:
		binary.BigEndian.PutUint64(scalar[:], uint64(val))
		return TypeUint64, scalar[:8], nil
	case uint64:
		binary.BigEndian.PutUint64(scalar[:], val)
		return TypeUint64, scalar[:8], nil
	case uint32:
		binary.BigEndian.PutUint64(scalar[:], uint64(val))
		return TypeUint64, scalar[:8], nil

	case float64:
		binary.BigEndian.PutUint64(scalar[:], math.Float64bits(val))
		return TypeFloat64, scalar[:8], nil
	case float32:
		binary.BigEndian.PutUint64(scalar[:], math.Float64bits(float64(val)))
		return TypeFloat64, scalar[:8], nil

	case string:
		return TypeString, []byte(val), nil
	case []byte:
		return TypeBytes, val, nil

	case time.Time:
		binary.BigEndian.PutUint64(scalar[:], uint64(val.UTC().UnixNano()))
		return TypeTime, scalar[:8], nil

	case []float32:
		buf := make([]byte, len(val)*4)
		for i, f := range val {
			binary.BigEndian.PutUint32(buf[i*4:(i+1)*4], math.Float32bits(f))
		}
		return TypeVectorFloat32, buf, nil

	case map[string]any:
		subBytes, err := Encode(val, dict)
		if err != nil {
			return TypeNull, nil, err
		}
		return TypeObject, subBytes, nil

	case []any:
		var buf bytes.Buffer
		var s [8]byte
		binary.BigEndian.PutUint32(s[:4], uint32(len(val)))
		buf.Write(s[:4])

		for _, item := range val {
			var elemScalar [8]byte
			iType, iBytes, err := encodeValueFast(item, dict, &elemScalar)
			if err != nil {
				return TypeNull, nil, err
			}
			buf.WriteByte(byte(iType))
			binary.BigEndian.PutUint32(s[:4], uint32(len(iBytes)))
			buf.Write(s[:4])
			buf.Write(iBytes)
		}
		return TypeArray, buf.Bytes(), nil

	default:
		return TypeNull, nil, fmt.Errorf("unsupported type %T in CSON", v)
	}
}
