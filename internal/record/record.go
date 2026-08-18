package record

import (
	"encoding/binary"
	"fmt"

	"github.com/mrzack99s/cocodb/internal/types"
)

const (
	RecordHeaderSize = 40

	FlagDeleted  uint16 = 1 << 0
	FlagOverflow uint16 = 1 << 1
)

// Header represents the 40-byte MVCC record header prepended to every record payload.
type Header struct {
	RecordID      types.RecordID
	BeginTxn      types.TxnID
	EndTxn        types.TxnID
	PrevVersion   types.RecordID
	Flags         uint16
	PayloadLength uint32
}

// EncodeTo serializes the Header into the provided buffer.
// The buffer must be at least RecordHeaderSize in length.
func (h *Header) EncodeTo(buf []byte) {
	binary.BigEndian.PutUint64(buf[0:8], uint64(h.RecordID))
	binary.BigEndian.PutUint64(buf[8:16], uint64(h.BeginTxn))
	binary.BigEndian.PutUint64(buf[16:24], uint64(h.EndTxn))
	binary.BigEndian.PutUint64(buf[24:32], uint64(h.PrevVersion))
	binary.BigEndian.PutUint16(buf[32:34], h.Flags)
	binary.BigEndian.PutUint16(buf[34:36], 0) // reserved
	binary.BigEndian.PutUint32(buf[36:40], h.PayloadLength)
}

// Encode serializes the Header into a newly allocated 40-byte buffer.
func (h *Header) Encode() []byte {
	buf := make([]byte, RecordHeaderSize)
	h.EncodeTo(buf)
	return buf
}

// DecodeHeader decodes a 40-byte slice into a Header.
func DecodeHeader(buf []byte) (Header, error) {
	if len(buf) < RecordHeaderSize {
		return Header{}, fmt.Errorf("%w: record buffer too small for header", types.ErrInvalidPage)
	}
	return Header{
		RecordID:      types.RecordID(binary.BigEndian.Uint64(buf[0:8])),
		BeginTxn:      types.TxnID(binary.BigEndian.Uint64(buf[8:16])),
		EndTxn:        types.TxnID(binary.BigEndian.Uint64(buf[16:24])),
		PrevVersion:   types.RecordID(binary.BigEndian.Uint64(buf[24:32])),
		Flags:         binary.BigEndian.Uint16(buf[32:34]),
		PayloadLength: binary.BigEndian.Uint32(buf[36:40]),
	}, nil
}

// IsVisible returns whether this record version is visible to the given snapshot TxnID.
func (h *Header) IsVisible(snapshotTxn types.TxnID, isTxnCommitted func(types.TxnID) bool, activeTxn types.TxnID) bool {
	// Rule 1: Creation must be visible
	if h.BeginTxn > snapshotTxn && h.BeginTxn != activeTxn {
		return false
	}
	if h.BeginTxn != activeTxn && !isTxnCommitted(h.BeginTxn) {
		return false
	}

	// Rule 2: Deletion / superseding must not be visible yet
	if h.EndTxn != 0 {
		if h.EndTxn <= snapshotTxn && (isTxnCommitted(h.EndTxn) || h.EndTxn == activeTxn) {
			// Version was deleted / replaced before our snapshot
			return false
		}
	}

	return true
}

// IsDeleted returns true if this record version was explicitly marked deleted.
func (h *Header) IsDeleted() bool {
	return (h.Flags & FlagDeleted) != 0
}
