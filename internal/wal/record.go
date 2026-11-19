package wal

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/mrzack99s/cocodb/internal/crypto"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
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
	RecordMeta // structural meta-page fields as of a commit, see MetaRedo
)

// MetaRedo holds the meta-page fields a commit may change. The pages they point to are
// logged as RecordPageUpdate in the same transaction, so recovery needs both to find the
// committed data.
type MetaRedo struct {
	CatalogRoot   types.PageID
	RecordDirRoot types.PageID
	FreeListRoot  types.PageID
	NextPageID    types.PageID
	NextRecordID  types.RecordID
}

const metaRedoSize = 5 * 8

// NewMetaRecord creates a WAL record carrying the meta-page fields of a commit.
func NewMetaRecord(txnID types.TxnID, m MetaRedo) *Record {
	payload := make([]byte, metaRedoSize)
	binary.BigEndian.PutUint64(payload[0:8], uint64(m.CatalogRoot))
	binary.BigEndian.PutUint64(payload[8:16], uint64(m.RecordDirRoot))
	binary.BigEndian.PutUint64(payload[16:24], uint64(m.FreeListRoot))
	binary.BigEndian.PutUint64(payload[24:32], uint64(m.NextPageID))
	binary.BigEndian.PutUint64(payload[32:40], uint64(m.NextRecordID))
	return &Record{
		Header:  RecordHeader{Type: RecordMeta, TxnID: txnID},
		Payload: payload,
		Meta:    &m,
	}
}

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
	case RecordMeta:
		return "Meta"
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

	// Decoded RecordMeta payload
	Meta *MetaRedo
}

// Encode serializes a Record into a byte slice, encrypting the payload if encKey is provided.
func (r *Record) Encode(encKeys ...[]byte) []byte {
	r.Header.Magic = WALMagic
	r.Header.Version = WALVersion

	payloadToWrite := r.Payload
	if len(encKeys) > 0 && len(encKeys[0]) == 32 {
		encrypted, err := crypto.EncryptWAL(r.Payload, encKeys[0], uint64(r.Header.LSN))
		if err == nil {
			payloadToWrite = encrypted
		}
	}

	r.Header.Length = uint32(len(payloadToWrite))
	r.Header.CRC = storage.Checksum(payloadToWrite)

	buf := make([]byte, WALRecordHeaderSize+len(payloadToWrite))
	binary.BigEndian.PutUint32(buf[0:4], r.Header.Magic)
	binary.BigEndian.PutUint16(buf[4:6], r.Header.Version)
	buf[6] = byte(r.Header.Type)
	binary.BigEndian.PutUint32(buf[7:11], r.Header.Length)
	binary.BigEndian.PutUint64(buf[11:19], uint64(r.Header.LSN))
	binary.BigEndian.PutUint64(buf[19:27], uint64(r.Header.TxnID))
	binary.BigEndian.PutUint32(buf[27:31], r.Header.CRC)

	copy(buf[31:], payloadToWrite)
	return buf
}

// DecodeRecord decodes a single WAL record from a reader, decrypting payload if key is supplied.
func DecodeRecord(reader io.Reader, encKeys ...[]byte) (*Record, error) {
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

	rawPayload := make([]byte, length)
	if _, err := io.ReadFull(reader, rawPayload); err != nil {
		return nil, err
	}

	computedCRC := storage.Checksum(rawPayload)
	if crc != computedCRC {
		return nil, types.ErrChecksumFailed
	}

	payload := rawPayload
	if len(encKeys) > 0 && len(encKeys[0]) == 32 {
		decrypted, err := crypto.DecryptWAL(rawPayload, encKeys[0], uint64(lsn))
		if err != nil {
			return nil, fmt.Errorf("%w: decrypt wal record lsn %d: %v", crypto.ErrDecryptFailed, lsn, err)
		}
		payload = decrypted
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
	case RecordMeta:
		if len(payload) >= metaRedoSize {
			rec.Meta = &MetaRedo{
				CatalogRoot:   types.PageID(binary.BigEndian.Uint64(payload[0:8])),
				RecordDirRoot: types.PageID(binary.BigEndian.Uint64(payload[8:16])),
				FreeListRoot:  types.PageID(binary.BigEndian.Uint64(payload[16:24])),
				NextPageID:    types.PageID(binary.BigEndian.Uint64(payload[24:32])),
				NextRecordID:  types.RecordID(binary.BigEndian.Uint64(payload[32:40])),
			}
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
