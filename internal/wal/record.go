package wal

import (
	"encoding/binary"
	"fmt"
	"io"

	"cocodb/internal/storage"
	"cocodb/internal/types"
)

const (
	WALMagic   uint32 = 0x434F434F // "COCO"
	WALVersion uint16 = 1

	WALRecordHeaderSize = 31 // 4 + 2 + 1 + 4 + 8 + 8 + 4
)

type RecordType uint8

const (
	RecordTxnBegin RecordType = iota + 1
	RecordPageUpdate
	RecordPageAlloc
	RecordPageFree
	RecordTxnCommit
	RecordTxnAbort
	RecordCheckpoint
)

func (r RecordType) String() string {
	switch r {
	case RecordTxnBegin:
		return "TxnBegin"
	case RecordPageUpdate:
		return "PageUpdate"
	case RecordPageAlloc:
		return "PageAlloc"
	case RecordPageFree:
		return "PageFree"
	case RecordTxnCommit:
		return "TxnCommit"
	case RecordTxnAbort:
		return "TxnAbort"
	case RecordCheckpoint:
		return "Checkpoint"
	default:
		return fmt.Sprintf("Unknown(%d)", r)
	}
}

// RecordHeader represents the 31-byte header for each WAL entry.
type RecordHeader struct {
	Magic   uint32
	Version uint16
	Type    RecordType
	Length  uint32 // Payload length
	LSN     types.LSN
	TxnID   types.TxnID
	CRC     uint32 // CRC32C of payload
}

// Record represents a single WAL log entry.
type Record struct {
	Header  RecordHeader
	Payload []byte

	// Decoded helper fields for RecordPageUpdate / RecordPageAlloc / RecordPageFree
	PageID   types.PageID
	PageType storage.PageType
	PageData []byte
}

// Encode serializes a Record into a byte slice.
func (r *Record) Encode() []byte {
	r.Header.Magic = WALMagic
	r.Header.Version = WALVersion
	r.Header.Length = uint32(len(r.Payload))
	r.Header.CRC = storage.Checksum(r.Payload)

	buf := make([]byte, WALRecordHeaderSize+len(r.Payload))
	binary.BigEndian.PutUint32(buf[0:4], r.Header.Magic)
	binary.BigEndian.PutUint16(buf[4:6], r.Header.Version)
	buf[6] = byte(r.Header.Type)
	binary.BigEndian.PutUint32(buf[7:11], r.Header.Length)
	binary.BigEndian.PutUint64(buf[11:19], uint64(r.Header.LSN))
	binary.BigEndian.PutUint64(buf[19:27], uint64(r.Header.TxnID))
	binary.BigEndian.PutUint32(buf[27:31], r.Header.CRC)

	copy(buf[31:], r.Payload)
	return buf
}

// DecodeRecord decodes a single WAL record from a reader.
func DecodeRecord(reader io.Reader) (*Record, error) {
	headerBuf := make([]byte, WALRecordHeaderSize)
	if _, err := io.ReadFull(reader, headerBuf); err != nil {
		return nil, err
	}

	magic := binary.BigEndian.Uint32(headerBuf[0:4])
	if magic != WALMagic {
		return nil, fmt.Errorf("%w: invalid WAL magic %x", types.ErrInvalidFormat, magic)
	}
	version := binary.BigEndian.Uint16(headerBuf[4:6])
	recType := RecordType(headerBuf[6])
	length := binary.BigEndian.Uint32(headerBuf[7:11])
	lsn := types.LSN(binary.BigEndian.Uint64(headerBuf[11:19]))
	txnID := types.TxnID(binary.BigEndian.Uint64(headerBuf[19:27]))
	crc := binary.BigEndian.Uint32(headerBuf[27:31])

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}

	computedCRC := storage.Checksum(payload)
	if crc != computedCRC {
		return nil, types.ErrChecksumFailed
	}

	rec := &Record{
		Header: RecordHeader{
			Magic:   magic,
			Version: version,
			Type:    recType,
			Length:  length,
			LSN:     lsn,
			TxnID:   txnID,
			CRC:     crc,
		},
		Payload: payload,
	}

	// Unpack specific record payloads
	switch recType {
	case RecordPageUpdate:
		if len(payload) >= 8 {
			rec.PageID = types.PageID(binary.BigEndian.Uint64(payload[0:8]))
			rec.PageData = payload[8:]
		}
	case RecordPageAlloc:
		if len(payload) >= 9 {
			rec.PageID = types.PageID(binary.BigEndian.Uint64(payload[0:8]))
			rec.PageType = storage.PageType(payload[8])
		}
	case RecordPageFree:
		if len(payload) >= 8 {
			rec.PageID = types.PageID(binary.BigEndian.Uint64(payload[0:8]))
		}
	}

	return rec, nil
}

// NewPageUpdateRecord creates a WAL record for full-page redo.
func NewPageUpdateRecord(lsn types.LSN, txnID types.TxnID, pageID types.PageID, pageData []byte) *Record {
	payload := make([]byte, 8+len(pageData))
	binary.BigEndian.PutUint64(payload[0:8], uint64(pageID))
	copy(payload[8:], pageData)

	return &Record{
		Header: RecordHeader{
			Type:  RecordPageUpdate,
			LSN:   lsn,
			TxnID: txnID,
		},
		Payload:  payload,
		PageID:   pageID,
		PageData: pageData,
	}
}

// NewTxnCommitRecord creates a WAL commit record.
func NewTxnCommitRecord(lsn types.LSN, txnID types.TxnID) *Record {
	return &Record{
		Header: RecordHeader{
			Type:  RecordTxnCommit,
			LSN:   lsn,
			TxnID: txnID,
		},
	}
}

// NewTxnAbortRecord creates a WAL abort record.
func NewTxnAbortRecord(lsn types.LSN, txnID types.TxnID) *Record {
	return &Record{
		Header: RecordHeader{
			Type:  RecordTxnAbort,
			LSN:   lsn,
			TxnID: txnID,
		},
	}
}
