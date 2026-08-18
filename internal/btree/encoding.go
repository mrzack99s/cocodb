package btree

import (
	"bytes"
	"encoding/binary"
	"math"
	"time"

	"cocodb/internal/types"
)

// Key type tags for order-preserving serialization
const (
	TagNull byte = iota
	TagFalse
	TagTrue
	TagInt64
	TagUint64
	TagFloat64
	TagTime
	TagString
	TagBytes
	TagUUID
)

// EncodeNull encodes a null value.
func EncodeNull() []byte {
	return []byte{TagNull}
}

// EncodeBool encodes a boolean in order-preserving format (false < true).
func EncodeBool(v bool) []byte {
	if v {
		return []byte{TagTrue}
	}
	return []byte{TagFalse}
}

// EncodeInt64 encodes an int64 such that byte comparison matches numerical order.
// Flips the sign bit (XOR with 0x8000000000000000) so negative numbers are smaller than positive.
func EncodeInt64(v int64) []byte {
	u := uint64(v) ^ 0x8000000000000000
	buf := make([]byte, 9)
	buf[0] = TagInt64
	binary.BigEndian.PutUint64(buf[1:9], u)
	return buf
}

// DecodeInt64 decodes an order-preserving int64.
func DecodeInt64(buf []byte) (int64, error) {
	if len(buf) < 9 || buf[0] != TagInt64 {
		return 0, types.ErrInvalidFormat
	}
	u := binary.BigEndian.Uint64(buf[1:9]) ^ 0x8000000000000000
	return int64(u), nil
}

// EncodeUint64 encodes a uint64 preserving big-endian order.
func EncodeUint64(v uint64) []byte {
	buf := make([]byte, 9)
	buf[0] = TagUint64
	binary.BigEndian.PutUint64(buf[1:9], v)
	return buf
}

// DecodeUint64 decodes a uint64 key.
func DecodeUint64(buf []byte) (uint64, error) {
	if len(buf) < 9 || buf[0] != TagUint64 {
		return 0, types.ErrInvalidFormat
	}
	return binary.BigEndian.Uint64(buf[1:9]), nil
}

// EncodeFloat64 encodes a float64 into order-preserving bytes.
func EncodeFloat64(v float64) []byte {
	bits := math.Float64bits(v)
	if bits&(1<<63) != 0 {
		bits = ^bits
	} else {
		bits ^= 1 << 63
	}
	buf := make([]byte, 9)
	buf[0] = TagFloat64
	binary.BigEndian.PutUint64(buf[1:9], bits)
	return buf
}

// DecodeFloat64 decodes an order-preserving float64.
func DecodeFloat64(buf []byte) (float64, error) {
	if len(buf) < 9 || buf[0] != TagFloat64 {
		return 0, types.ErrInvalidFormat
	}
	bits := binary.BigEndian.Uint64(buf[1:9])
	if bits&(1<<63) != 0 {
		bits ^= 1 << 63
	} else {
		bits = ^bits
	}
	return math.Float64frombits(bits), nil
}

// EncodeString encodes a string with escape bytes for component chaining.
// Bytes 0x00 and 0x01 are escaped so 0x00 0x00 terminates a string field.
func EncodeString(s string) []byte {
	var buf bytes.Buffer
	buf.WriteByte(TagString)
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == 0x00 {
			buf.WriteByte(0x00)
			buf.WriteByte(0xFF)
		} else {
			buf.WriteByte(b)
		}
	}
	buf.WriteByte(0x00)
	buf.WriteByte(0x00) // End of string delimiter
	return buf.Bytes()
}

// EncodeBytes encodes raw byte slice.
func EncodeBytes(b []byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte(TagBytes)
	for i := 0; i < len(b); i++ {
		byteVal := b[i]
		if byteVal == 0x00 {
			buf.WriteByte(0x00)
			buf.WriteByte(0xFF)
		} else {
			buf.WriteByte(byteVal)
		}
	}
	buf.WriteByte(0x00)
	buf.WriteByte(0x00)
	return buf.Bytes()
}

// EncodeTime encodes time.Time into UTC nanosecond int64 order-preserving format.
func EncodeTime(t time.Time) []byte {
	buf := make([]byte, 9)
	buf[0] = TagTime
	u := uint64(t.UTC().UnixNano()) ^ 0x8000000000000000
	binary.BigEndian.PutUint64(buf[1:9], u)
	return buf
}

// EncodeComposite appends components into a single composite key.
func EncodeComposite(components ...[]byte) []byte {
	var buf bytes.Buffer
	for _, c := range components {
		buf.Write(c)
	}
	return buf.Bytes()
}

// AppendRecordID appends a RecordID to a secondary index key to guarantee uniqueness.
func AppendRecordID(key []byte, recID types.RecordID) []byte {
	buf := make([]byte, len(key)+8)
	copy(buf, key)
	binary.BigEndian.PutUint64(buf[len(key):], uint64(recID))
	return buf
}

// ExtractRecordID extracts the trailing RecordID from a secondary index key.
func ExtractRecordID(key []byte) (types.RecordID, []byte) {
	if len(key) < 8 {
		return types.InvalidRecordID, key
	}
	recID := types.RecordID(binary.BigEndian.Uint64(key[len(key)-8:]))
	return recID, key[:len(key)-8]
}
