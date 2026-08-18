package cson

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/mrzack99s/cocodb/internal/types"
)

type fieldLocation struct {
	valType ValueType
	offset  uint32
	length  uint32
}

// DocumentView provides a zero/low-allocation read view into a CSON document.
type DocumentView struct {
	data       []byte
	dict       *FieldDictionary
	fieldCount uint16
}

// NewDocumentView constructs a DocumentView from raw CSON bytes.
func NewDocumentView(data []byte, dict *FieldDictionary) (*DocumentView, error) {
	if len(data) < 8 {
		return nil, types.ErrInvalidFormat
	}

	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != CSONMagic {
		return nil, fmt.Errorf("%w: invalid CSON magic %x", types.ErrInvalidFormat, magic)
	}

	fieldCount := binary.BigEndian.Uint16(data[6:8])
	neededHeader := 8 + int(fieldCount)*FieldEntrySize
	if len(data) < neededHeader {
		return nil, fmt.Errorf("%w: CSON buffer too short for directory", types.ErrInvalidFormat)
	}

	return &DocumentView{
		data:       data,
		dict:       dict,
		fieldCount: fieldCount,
	}, nil
}

func (v *DocumentView) getFieldLoc(name string) (fieldLocation, bool) {
	if v.dict == nil {
		return fieldLocation{}, false
	}
	fID, ok := v.dict.GetID(name)
	if !ok {
		return fieldLocation{}, false
	}

	left := 0
	right := int(v.fieldCount) - 1
	for left <= right {
		mid := left + (right-left)/2
		off := 8 + mid*FieldEntrySize
		id := binary.BigEndian.Uint32(v.data[off : off+4])

		if id == fID {
			return fieldLocation{
				valType: ValueType(v.data[off+4]),
				offset:  binary.BigEndian.Uint32(v.data[off+6 : off+10]),
				length:  binary.BigEndian.Uint32(v.data[off+10 : off+14]),
			}, true
		} else if id < fID {
			left = mid + 1
		} else {
			right = mid - 1
		}
	}
	return fieldLocation{}, false
}

// Reset re-initializes an existing DocumentView with new data and dictionary without heap allocation.
func (v *DocumentView) Reset(data []byte, dict *FieldDictionary) error {
	if len(data) < 8 {
		return fmt.Errorf("%w: CSON buffer too short", types.ErrInvalidFormat)
	}
	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != CSONMagic {
		return fmt.Errorf("%w: invalid CSON magic", types.ErrInvalidFormat)
	}
	fieldCount := binary.BigEndian.Uint16(data[6:8])
	if len(data) < 8+int(fieldCount)*FieldEntrySize {
		return fmt.Errorf("%w: CSON buffer too short for directory", types.ErrInvalidFormat)
	}
	v.data = data
	v.dict = dict
	v.fieldCount = fieldCount
	return nil
}

// RawBytes returns the underlying CSON data.
func (v *DocumentView) RawBytes() []byte {
	return v.data
}

// Has checks if a field is present in the document.
func (v *DocumentView) Has(field string) bool {
	_, ok := v.getFieldLoc(field)
	return ok
}

// StringBytes returns the raw string bytes without string allocation.
func (v *DocumentView) StringBytes(field string) ([]byte, bool) {
	loc, ok := v.getFieldLoc(field)
	if !ok || loc.valType != TypeString {
		return nil, false
	}
	if int(loc.offset+loc.length) > len(v.data) {
		return nil, false
	}
	return v.data[loc.offset : loc.offset+loc.length], true
}

// String reads a string field without full document parsing.
func (v *DocumentView) String(field string) (string, bool) {
	b, ok := v.StringBytes(field)
	if !ok {
		return "", false
	}
	return string(b), true
}

// Int64 reads an int64 field.
func (v *DocumentView) Int64(field string) (int64, bool) {
	loc, ok := v.getFieldLoc(field)
	if !ok {
		return 0, false
	}
	if loc.valType != TypeInt64 && loc.valType != TypeUint64 {
		return 0, false
	}
	if int(loc.offset+loc.length) > len(v.data) || loc.length < 8 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(v.data[loc.offset : loc.offset+8])), true
}

// Float64 reads a float64 field.
func (v *DocumentView) Float64(field string) (float64, bool) {
	loc, ok := v.getFieldLoc(field)
	if !ok {
		return 0, false
	}
	if loc.valType == TypeFloat64 {
		if int(loc.offset+loc.length) > len(v.data) || loc.length < 8 {
			return 0, false
		}
		bits := binary.BigEndian.Uint64(v.data[loc.offset : loc.offset+8])
		return math.Float64frombits(bits), true
	}
	if loc.valType == TypeInt64 {
		if n, ok := v.Int64(field); ok {
			return float64(n), true
		}
	}
	return 0, false
}

// Bool reads a boolean field.
func (v *DocumentView) Bool(field string) (bool, bool) {
	loc, ok := v.getFieldLoc(field)
	if !ok || loc.valType != TypeBool {
		return false, false
	}
	if int(loc.offset+loc.length) > len(v.data) || loc.length < 1 {
		return false, false
	}
	return v.data[loc.offset] == 1, true
}

// Bytes reads raw byte slice field.
func (v *DocumentView) Bytes(field string) ([]byte, bool) {
	loc, ok := v.getFieldLoc(field)
	if !ok || loc.valType != TypeBytes {
		return nil, false
	}
	if int(loc.offset+loc.length) > len(v.data) {
		return nil, false
	}
	res := make([]byte, loc.length)
	copy(res, v.data[loc.offset:loc.offset+loc.length])
	return res, true
}

// Vector reads a float32 vector embedding.
func (v *DocumentView) Vector(field string) ([]float32, bool) {
	loc, ok := v.getFieldLoc(field)
	if !ok || loc.valType != TypeVectorFloat32 {
		return nil, false
	}
	if int(loc.offset+loc.length) > len(v.data) {
		return nil, false
	}
	numFloats := int(loc.length / 4)
	vec := make([]float32, numFloats)
	for i := 0; i < numFloats; i++ {
		bits := binary.BigEndian.Uint32(v.data[loc.offset+uint32(i*4) : loc.offset+uint32((i+1)*4)])
		vec[i] = math.Float32frombits(bits)
	}
	return vec, true
}

// Time reads a timestamp field.
func (v *DocumentView) Time(field string) (time.Time, bool) {
	loc, ok := v.getFieldLoc(field)
	if !ok || loc.valType != TypeTime {
		return time.Time{}, false
	}
	if int(loc.offset+loc.length) > len(v.data) || loc.length < 8 {
		return time.Time{}, false
	}
	nano := int64(binary.BigEndian.Uint64(v.data[loc.offset : loc.offset+8]))
	return time.Unix(0, nano).UTC(), true
}

// Get dynamically extracts and decodes a field value.
func (v *DocumentView) Get(field string) (any, bool) {
	loc, ok := v.getFieldLoc(field)
	if !ok {
		return nil, false
	}
	if int(loc.offset+loc.length) > len(v.data) {
		return nil, false
	}
	valBytes := v.data[loc.offset : loc.offset+loc.length]
	val, err := decodeValue(loc.valType, valBytes, v.dict)
	if err != nil {
		return nil, false
	}
	return val, true
}

// ToMap decodes all fields in the view to a map[string]any.
func (v *DocumentView) ToMap() map[string]any {
	result := make(map[string]any, v.fieldCount)
	for i := 0; i < int(v.fieldCount); i++ {
		off := 8 + i*FieldEntrySize
		fID := binary.BigEndian.Uint32(v.data[off : off+4])
		vType := ValueType(v.data[off+4])
		fOffset := binary.BigEndian.Uint32(v.data[off+6 : off+10])
		fLength := binary.BigEndian.Uint32(v.data[off+10 : off+14])

		fName, ok := v.dict.GetName(fID)
		if !ok {
			fName = fmt.Sprintf("field_%d", fID)
		}
		if int(fOffset+fLength) <= len(v.data) {
			valBytes := v.data[fOffset : fOffset+fLength]
			val, err := decodeValue(vType, valBytes, v.dict)
			if err == nil {
				result[fName] = val
			}
		}
	}
	return result
}
