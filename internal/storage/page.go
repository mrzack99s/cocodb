package storage

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"cocodb/internal/types"
)

type PageType uint8

const (
	PageMeta PageType = iota
	PageBTreeInternal
	PageBTreeLeaf
	PageRecord
	PageOverflow
	PageFreeList
	PageVector
	PageText
)

func (t PageType) String() string {
	switch t {
	case PageMeta:
		return "Meta"
	case PageBTreeInternal:
		return "BTreeInternal"
	case PageBTreeLeaf:
		return "BTreeLeaf"
	case PageRecord:
		return "Record"
	case PageOverflow:
		return "Overflow"
	case PageFreeList:
		return "FreeList"
	case PageVector:
		return "Vector"
	case PageText:
		return "Text"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

const (
	PageHeaderSize = 32
	MetaHeaderSize = 116
)

// PageHeader represents the 32-byte header for every non-meta page.
type PageHeader struct {
	ID        types.PageID
	Type      PageType
	Flags     uint16
	LSN       types.LSN
	SlotCount uint16
	FreeStart uint16
	FreeEnd   uint16
	Checksum  uint32
}

// Page represents an in-memory 16 KiB database page buffer with parsed header.
type Page struct {
	Header PageHeader
	Data   []byte
	Dirty  bool
	Pins   int32
}

// NewPage allocates a new blank page buffer with given ID and Type.
func NewPage(id types.PageID, pageType PageType) *Page {
	data := make([]byte, types.DefaultPageSize)
	p := &Page{
		Header: PageHeader{
			ID:        id,
			Type:      pageType,
			SlotCount: 0,
			FreeStart: PageHeaderSize,
			FreeEnd:   uint16(types.DefaultPageSize),
		},
		Data:  data,
		Dirty: true,
	}
	p.WriteHeader()
	return p
}

// WriteHeader writes p.Header into p.Data.
func (p *Page) WriteHeader() {
	binary.BigEndian.PutUint64(p.Data[0:8], uint64(p.Header.ID))
	p.Data[8] = byte(p.Header.Type)
	p.Data[9] = 0 // reserved
	binary.BigEndian.PutUint16(p.Data[10:12], p.Header.Flags)
	binary.BigEndian.PutUint64(p.Data[12:20], uint64(p.Header.LSN))
	binary.BigEndian.PutUint16(p.Data[20:22], p.Header.SlotCount)
	binary.BigEndian.PutUint16(p.Data[22:24], p.Header.FreeStart)
	binary.BigEndian.PutUint16(p.Data[24:26], p.Header.FreeEnd)
	binary.BigEndian.PutUint16(p.Data[26:28], 0) // reserved
	binary.BigEndian.PutUint32(p.Data[28:32], p.Header.Checksum)
}

// ReadHeader parses header fields from p.Data into p.Header.
func (p *Page) ReadHeader() {
	p.Header.ID = types.PageID(binary.BigEndian.Uint64(p.Data[0:8]))
	p.Header.Type = PageType(p.Data[8])
	p.Header.Flags = binary.BigEndian.Uint16(p.Data[10:12])
	p.Header.LSN = types.LSN(binary.BigEndian.Uint64(p.Data[12:20]))
	p.Header.SlotCount = binary.BigEndian.Uint16(p.Data[20:22])
	p.Header.FreeStart = binary.BigEndian.Uint16(p.Data[22:24])
	p.Header.FreeEnd = binary.BigEndian.Uint16(p.Data[24:26])
	p.Header.Checksum = binary.BigEndian.Uint32(p.Data[28:32])
}

// ComputeChecksum calculates CRC32C over p.Data with checksum offset zeroed.
func (p *Page) ComputeChecksum() uint32 {
	saved := binary.BigEndian.Uint32(p.Data[28:32])
	binary.BigEndian.PutUint32(p.Data[28:32], 0)
	c := Checksum(p.Data)
	binary.BigEndian.PutUint32(p.Data[28:32], saved)
	return c
}

// Seal computes and writes the checksum into p.Header and p.Data before disk write.
func (p *Page) Seal() {
	p.WriteHeader()
	binary.BigEndian.PutUint32(p.Data[28:32], 0)
	p.Header.Checksum = Checksum(p.Data)
	binary.BigEndian.PutUint32(p.Data[28:32], p.Header.Checksum)
}

// ValidateChecksum verifies that the computed CRC32C matches stored checksum.
func (p *Page) ValidateChecksum() bool {
	stored := binary.BigEndian.Uint32(p.Data[28:32])
	binary.BigEndian.PutUint32(p.Data[28:32], 0)
	computed := Checksum(p.Data)
	binary.BigEndian.PutUint32(p.Data[28:32], stored)
	return stored == computed
}

// MetaPage represents Page 0 (Meta A) or Page 1 (Meta B).
type MetaPage struct {
	Magic             [8]byte
	FormatVersion     uint32
	PageSize          uint32
	Generation        uint64
	DatabaseID        [16]byte
	CatalogRoot       types.PageID
	RecordDirRoot     types.PageID
	FreeListRoot      types.PageID
	NextPageID        types.PageID
	NextRecordID      types.RecordID
	LastTxnID         types.TxnID
	LastLSN           types.LSN
	LastCheckpointLSN types.LSN
	Flags             uint64
	Checksum          uint32
}

// NewInitialMetaPage creates a freshly initialized MetaPage.
func NewInitialMetaPage(pageSize uint32) *MetaPage {
	if pageSize == 0 {
		pageSize = types.DefaultPageSize
	}
	m := &MetaPage{
		FormatVersion:     types.FormatVersion,
		PageSize:          pageSize,
		Generation:        1,
		CatalogRoot:       types.InvalidPageID,
		RecordDirRoot:     types.InvalidPageID,
		FreeListRoot:      types.InvalidPageID,
		NextPageID:        2, // Pages 0 and 1 are reserved for Meta A and Meta B
		NextRecordID:      1,
		LastTxnID:         0,
		LastLSN:           0,
		LastCheckpointLSN: 0,
	}
	copy(m.Magic[:], []byte(types.MagicHeader))
	_, _ = rand.Read(m.DatabaseID[:])
	return m
}

// Encode serializes MetaPage into a full page-sized byte slice.
func (m *MetaPage) Encode(pageSize uint32) []byte {
	buf := make([]byte, pageSize)
	copy(buf[0:8], m.Magic[:])
	binary.BigEndian.PutUint32(buf[8:12], m.FormatVersion)
	binary.BigEndian.PutUint32(buf[12:16], m.PageSize)
	binary.BigEndian.PutUint64(buf[16:24], m.Generation)
	copy(buf[24:40], m.DatabaseID[:])
	binary.BigEndian.PutUint64(buf[40:48], uint64(m.CatalogRoot))
	binary.BigEndian.PutUint64(buf[48:56], uint64(m.RecordDirRoot))
	binary.BigEndian.PutUint64(buf[56:64], uint64(m.FreeListRoot))
	binary.BigEndian.PutUint64(buf[64:72], uint64(m.NextPageID))
	binary.BigEndian.PutUint64(buf[72:80], uint64(m.NextRecordID))
	binary.BigEndian.PutUint64(buf[80:88], uint64(m.LastTxnID))
	binary.BigEndian.PutUint64(buf[88:96], uint64(m.LastLSN))
	binary.BigEndian.PutUint64(buf[96:104], uint64(m.LastCheckpointLSN))
	binary.BigEndian.PutUint64(buf[104:112], m.Flags)
	binary.BigEndian.PutUint32(buf[112:116], 0) // checksum slot

	m.Checksum = Checksum(buf)
	binary.BigEndian.PutUint32(buf[112:116], m.Checksum)
	return buf
}

// DecodeMetaPage decodes and validates a raw page buffer into a MetaPage.
func DecodeMetaPage(buf []byte) (*MetaPage, error) {
	if len(buf) < MetaHeaderSize {
		return nil, types.ErrInvalidFormat
	}
	var m MetaPage
	copy(m.Magic[:], buf[0:8])
	if string(m.Magic[:]) != types.MagicHeader {
		return nil, fmt.Errorf("%w: invalid magic %q", types.ErrInvalidFormat, string(m.Magic[:]))
	}
	m.FormatVersion = binary.BigEndian.Uint32(buf[8:12])
	if m.FormatVersion != types.FormatVersion {
		return nil, fmt.Errorf("%w: unsupported format version %d", types.ErrInvalidFormat, m.FormatVersion)
	}
	m.PageSize = binary.BigEndian.Uint32(buf[12:16])
	if m.PageSize == 0 || (m.PageSize&(m.PageSize-1)) != 0 {
		return nil, fmt.Errorf("%w: invalid page size %d", types.ErrInvalidFormat, m.PageSize)
	}
	m.Generation = binary.BigEndian.Uint64(buf[16:24])
	copy(m.DatabaseID[:], buf[24:40])
	m.CatalogRoot = types.PageID(binary.BigEndian.Uint64(buf[40:48]))
	m.RecordDirRoot = types.PageID(binary.BigEndian.Uint64(buf[48:56]))
	m.FreeListRoot = types.PageID(binary.BigEndian.Uint64(buf[56:64]))
	m.NextPageID = types.PageID(binary.BigEndian.Uint64(buf[64:72]))
	m.NextRecordID = types.RecordID(binary.BigEndian.Uint64(buf[72:80]))
	m.LastTxnID = types.TxnID(binary.BigEndian.Uint64(buf[80:88]))
	m.LastLSN = types.LSN(binary.BigEndian.Uint64(buf[88:96]))
	m.LastCheckpointLSN = types.LSN(binary.BigEndian.Uint64(buf[96:104]))
	m.Flags = binary.BigEndian.Uint64(buf[104:112])
	storedChecksum := binary.BigEndian.Uint32(buf[112:116])

	// Validate checksum
	saved := binary.BigEndian.Uint32(buf[112:116])
	binary.BigEndian.PutUint32(buf[112:116], 0)
	computedChecksum := Checksum(buf)
	binary.BigEndian.PutUint32(buf[112:116], saved)

	if storedChecksum != computedChecksum {
		return nil, types.ErrChecksumFailed
	}
	m.Checksum = storedChecksum
	return &m, nil
}
