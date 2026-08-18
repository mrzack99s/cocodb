package storage

import (
	"encoding/binary"
	"io"

	"cocodb/internal/types"
)

const (
	OverflowHeaderOffset = PageHeaderSize
	OverflowDataOffset   = OverflowHeaderOffset + 12
	MaxOverflowPayload   = types.DefaultPageSize - OverflowDataOffset
)

// OverflowPage represents an overflow node storing chunks of large payloads.
type OverflowPage struct {
	*Page
}

// WrapOverflow wraps a page as an OverflowPage.
func WrapOverflow(p *Page) *OverflowPage {
	return &OverflowPage{Page: p}
}

// NextPage returns the PageID of the next overflow page in the chain, or InvalidPageID.
func (op *OverflowPage) NextPage() types.PageID {
	return types.PageID(binary.BigEndian.Uint64(op.Data[OverflowHeaderOffset : OverflowHeaderOffset+8]))
}

// SetNextPage sets the pointer to the next overflow page.
func (op *OverflowPage) SetNextPage(next types.PageID) {
	binary.BigEndian.PutUint64(op.Data[OverflowHeaderOffset:OverflowHeaderOffset+8], uint64(next))
	op.Dirty = true
}

// ChunkLength returns the number of valid payload bytes stored in this overflow page.
func (op *OverflowPage) ChunkLength() uint32 {
	return binary.BigEndian.Uint32(op.Data[OverflowHeaderOffset+8 : OverflowHeaderOffset+12])
}

// SetChunkLength sets the valid chunk size.
func (op *OverflowPage) SetChunkLength(l uint32) {
	binary.BigEndian.PutUint32(op.Data[OverflowHeaderOffset+8:OverflowHeaderOffset+12], l)
	op.Dirty = true
}

// Payload returns the data slice of the chunk.
func (op *OverflowPage) Payload() []byte {
	l := op.ChunkLength()
	if l > uint32(MaxOverflowPayload) {
		l = uint32(MaxOverflowPayload)
	}
	return op.Data[OverflowDataOffset : OverflowDataOffset+int(l)]
}

// WriteChunk copies chunk data into the overflow page.
func (op *OverflowPage) WriteChunk(data []byte) int {
	n := len(data)
	if n > int(MaxOverflowPayload) {
		n = int(MaxOverflowPayload)
	}
	copy(op.Data[OverflowDataOffset:OverflowDataOffset+n], data[:n])
	op.SetChunkLength(uint32(n))
	return n
}

// OverflowPointer stores the header placed in a slotted record pointing to overflow pages.
type OverflowPointer struct {
	FirstPage   types.PageID
	TotalLength uint32
}

// Encode serializes the OverflowPointer into 12 bytes.
func (op *OverflowPointer) Encode() []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint64(buf[0:8], uint64(op.FirstPage))
	binary.BigEndian.PutUint32(buf[8:12], op.TotalLength)
	return buf
}

// DecodeOverflowPointer decodes 12 bytes into an OverflowPointer.
func DecodeOverflowPointer(buf []byte) (*OverflowPointer, error) {
	if len(buf) < 12 {
		return nil, io.ErrUnexpectedEOF
	}
	return &OverflowPointer{
		FirstPage:   types.PageID(binary.BigEndian.Uint64(buf[0:8])),
		TotalLength: binary.BigEndian.Uint32(buf[8:12]),
	}, nil
}
