package types

import "errors"

// Persistent identifier types (explicit width, no native int)
type PageID uint64
type SlotID uint16
type RecordID uint64
type TxnID uint64
type LSN uint64
type ObjectID uint64
type FieldID uint32

const (
	DefaultPageSize uint32 = 16384 // 16 KiB default
	FormatVersion   uint32 = 1
	MagicHeader            = "COCODB01"

	InvalidPageID   PageID   = 0xFFFFFFFFFFFFFFFF
	InvalidRecordID RecordID = 0xFFFFFFFFFFFFFFFF
	InvalidSlotID   SlotID   = 0xFFFF
	InvalidTxnID    TxnID    = 0
	InvalidLSN      LSN      = 0
	InvalidObjectID ObjectID = 0
	InvalidFieldID  FieldID  = 0

	MaxTxnID TxnID = 0xFFFFFFFFFFFFFFFF
)

// Common internal errors
var (
	ErrPageNotFound   = errors.New("coco/storage: page not found")
	ErrPageFull       = errors.New("coco/storage: slotted page has insufficient space")
	ErrSlotNotFound   = errors.New("coco/storage: slot not found or deleted")
	ErrInvalidPage    = errors.New("coco/storage: invalid page type or corruption")
	ErrChecksumFailed = errors.New("coco/storage: page checksum mismatch")
	ErrCorruptMeta    = errors.New("coco/storage: corrupted meta pages")
	ErrInvalidFormat  = errors.New("coco/storage: invalid database format")
)
