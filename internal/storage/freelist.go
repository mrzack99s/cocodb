package storage

import (
	"encoding/binary"
	"fmt"

	"github.com/mrzack99s/cocodb/internal/types"
)

const (
	FreeListPageHeaderOffset = PageHeaderSize
	MaxFreeListEntries       = (types.DefaultPageSize - PageHeaderSize - 16) / 8 // ~2040 IDs
)

// FreeListPage manages a single PageFreeList node storing a batch of freed PageIDs.
type FreeListPage struct {
	*Page
}

// WrapFreeList wraps a page as a FreeListPage.
func WrapFreeList(p *Page) *FreeListPage {
	return &FreeListPage{Page: p}
}

// NextPage returns the pointer to the next FreeListPage in the chain.
func (fl *FreeListPage) NextPage() types.PageID {
	return types.PageID(binary.BigEndian.Uint64(fl.Data[FreeListPageHeaderOffset : FreeListPageHeaderOffset+8]))
}

// SetNextPage sets the pointer to the next FreeListPage.
func (fl *FreeListPage) SetNextPage(next types.PageID) {
	binary.BigEndian.PutUint64(fl.Data[FreeListPageHeaderOffset:FreeListPageHeaderOffset+8], uint64(next))
	fl.Dirty = true
}

// Count returns the number of freed PageIDs stored in this page.
func (fl *FreeListPage) Count() uint32 {
	return binary.BigEndian.Uint32(fl.Data[FreeListPageHeaderOffset+8 : FreeListPageHeaderOffset+12])
}

// SetCount sets the number of stored freed PageIDs.
func (fl *FreeListPage) SetCount(c uint32) {
	binary.BigEndian.PutUint32(fl.Data[FreeListPageHeaderOffset+8:FreeListPageHeaderOffset+12], c)
	fl.Dirty = true
}

// Push adds a freed PageID into this page.
func (fl *FreeListPage) Push(id types.PageID) error {
	cnt := fl.Count()
	if cnt >= uint32(MaxFreeListEntries) {
		return fmt.Errorf("freelist page full")
	}
	off := FreeListPageHeaderOffset + 16 + int(cnt)*8
	binary.BigEndian.PutUint64(fl.Data[off:off+8], uint64(id))
	fl.SetCount(cnt + 1)
	return nil
}

// Pop removes and returns the last freed PageID from this page.
func (fl *FreeListPage) Pop() (types.PageID, error) {
	cnt := fl.Count()
	if cnt == 0 {
		return types.InvalidPageID, fmt.Errorf("freelist page empty")
	}
	cnt--
	off := FreeListPageHeaderOffset + 16 + int(cnt)*8
	id := types.PageID(binary.BigEndian.Uint64(fl.Data[off : off+8]))
	fl.SetCount(cnt)
	return id, nil
}
