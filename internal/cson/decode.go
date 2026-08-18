package cson

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/mrzack99s/cocodb/internal/types"
)

// Decode unpacks a full CSON byte slice into a Go map[string]any.
func Decode(data []byte, dict *FieldDictionary) (map[string]any, error) {
	view, err := NewDocumentView(data, dict)
	if err != nil {
		return nil, err
	}
	return view.ToMap(), nil
}

func decodeValue(valType ValueType, valBytes []byte, dict *FieldDictionary) (any, error) {
	switch valType {
	case TypeNull:
		return nil, nil
	case TypeBool:
		if len(valBytes) > 0 && valBytes[0] == 1 {
			return true, nil
		}
		return false, nil
	case TypeInt64:
		if len(valBytes) < 8 {
			return int64(0), types.ErrInvalidFormat
		}
		return int64(binary.BigEndian.Uint64(valBytes)), nil
	case TypeUint64:
		if len(valBytes) < 8 {
			return uint64(0), types.ErrInvalidFormat
		}
		return binary.BigEndian.Uint64(valBytes), nil
	case TypeFloat64:
		if len(valBytes) < 8 {
			return float64(0), types.ErrInvalidFormat
		}
		bits := binary.BigEndian.Uint64(valBytes)
		return math.Float64frombits(bits), nil
	case TypeString:
		return string(valBytes), nil
	case TypeBytes:
		cpy := make([]byte, len(valBytes))
		copy(cpy, valBytes)
		return cpy, nil
	case TypeTime:
		if len(valBytes) < 8 {
			return time.Time{}, types.ErrInvalidFormat
		}
		nano := int64(binary.BigEndian.Uint64(valBytes))
		return time.Unix(0, nano).UTC(), nil
	case TypeVectorFloat32:
		numFloats := len(valBytes) / 4
		vec := make([]float32, numFloats)
		for i := 0; i < numFloats; i++ {
			bits := binary.BigEndian.Uint32(valBytes[i*4 : (i+1)*4])
			vec[i] = math.Float32frombits(bits)
		}
		return vec, nil
	case TypeObject:
		return Decode(valBytes, dict)
	case TypeArray:
		r := bytes.NewReader(valBytes)
		var count uint32
		if err := binary.Read(r, binary.BigEndian, &count); err != nil {
			return nil, err
		}
		arr := make([]any, count)
		for i := uint32(0); i < count; i++ {
			tByte, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			var itemLen uint32
			if err := binary.Read(r, binary.BigEndian, &itemLen); err != nil {
				return nil, err
			}
			iBuf := make([]byte, itemLen)
			if _, err := r.Read(iBuf); err != nil {
				return nil, err
			}
			itemVal, err := decodeValue(ValueType(tByte), iBuf, dict)
			if err != nil {
				return nil, err
			}
			arr[i] = itemVal
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("unknown CSON value type %d", valType)
	}
}
