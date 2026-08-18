package storage

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/mrzack99s/cocodb/internal/types"
)

const (
	SlotDirectoryStart = 64 // First 64 bytes reserved for PageHeader and page-specific headers
	SlotSize           = 4  // uint16 Offset + uint16 Length
)

// Slot represents a pointer to a record payload inside a slotted page.
type Slot struct {
	Offset uint16
	Length uint16
}

// SlottedPage wraps a Page and provides slotted record operations.
type SlottedPage struct {
	*Page
}

// WrapSlotted wraps a Page as a SlottedPage.
func WrapSlotted(p *Page) *SlottedPage {
	if p.Header.FreeStart < SlotDirectoryStart {
		p.Header.FreeStart = SlotDirectoryStart
	}
	if p.Header.FreeEnd == 0 {
		p.Header.FreeEnd = uint16(types.DefaultPageSize)
	}
	return &SlottedPage{Page: p}
}

// slotOffset returns the byte offset in Page.Data where slot `idx` metadata starts.
func (sp *SlottedPage) slotOffset(idx uint16) int {
	return int(SlotDirectoryStart + idx*SlotSize)
}

// readSlot reads slot metadata at index `idx`.
func (sp *SlottedPage) readSlot(idx uint16) Slot {
	off := sp.slotOffset(idx)
	return Slot{
		Offset: binary.BigEndian.Uint16(sp.Data[off : off+2]),
		Length: binary.BigEndian.Uint16(sp.Data[off+2 : off+4]),
	}
}

// writeSlot writes slot metadata at index `idx`.
func (sp *SlottedPage) writeSlot(idx uint16, s Slot) {
	off := sp.slotOffset(idx)
	binary.BigEndian.PutUint16(sp.Data[off:off+2], s.Offset)
	binary.BigEndian.PutUint16(sp.Data[off+2:off+4], s.Length)
}

// FreeSpace returns contiguous free space available between slot directory and records.
func (sp *SlottedPage) FreeSpace() uint16 {
	if sp.Header.FreeEnd < sp.Header.FreeStart {
		return 0
	}
	return sp.Header.FreeEnd - sp.Header.FreeStart
}

// SlotCount returns the total number of slots in the directory.
func (sp *SlottedPage) SlotCount() uint16 {
	return sp.Header.SlotCount
}

// Insert adds a new record payload to the slotted page and returns its SlotID.
func (sp *SlottedPage) Insert(payload []byte) (types.SlotID, error) {
	payLen := uint16(len(payload))

	// Find if an unused slot exists
	var targetSlot types.SlotID = types.InvalidSlotID
	for i := uint16(0); i < sp.Header.SlotCount; i++ {
		s := sp.readSlot(i)
		if s.Offset == 0 && s.Length == 0 {
			targetSlot = types.SlotID(i)
			break
		}
	}

	neededSpace := payLen
	if targetSlot == types.InvalidSlotID {
		neededSpace += SlotSize
	}

	if sp.FreeSpace() < neededSpace {
		sp.Compact()
		if sp.FreeSpace() < neededSpace {
			return types.InvalidSlotID, types.ErrPageFull
		}
	}

	newFreeEnd := sp.Header.FreeEnd - payLen
	copy(sp.Data[newFreeEnd:sp.Header.FreeEnd], payload)
	sp.Header.FreeEnd = newFreeEnd

	if targetSlot == types.InvalidSlotID {
		targetSlot = types.SlotID(sp.Header.SlotCount)
		sp.Header.SlotCount++
		sp.Header.FreeStart = SlotDirectoryStart + sp.Header.SlotCount*SlotSize
	}

	sp.writeSlot(uint16(targetSlot), Slot{Offset: newFreeEnd, Length: payLen})
	sp.Dirty = true
	sp.WriteHeader()
	return targetSlot, nil
}

// Get returns the record payload for slotID.
func (sp *SlottedPage) Get(slotID types.SlotID) ([]byte, error) {
	if uint16(slotID) >= sp.Header.SlotCount {
		return nil, types.ErrSlotNotFound
	}
	s := sp.readSlot(uint16(slotID))
	if s.Offset == 0 && s.Length == 0 {
		return nil, types.ErrSlotNotFound
	}
	if int(s.Offset)+int(s.Length) > len(sp.Data) {
		return nil, fmt.Errorf("%w: corrupted slot bounds", types.ErrInvalidPage)
	}
	result := make([]byte, s.Length)
	copy(result, sp.Data[s.Offset:s.Offset+s.Length])
	return result, nil
}

// GetDirect returns a direct slice to the record payload for slotID without copying.
func (sp *SlottedPage) GetDirect(slotID types.SlotID) ([]byte, error) {
	if uint16(slotID) >= sp.Header.SlotCount {
		return nil, types.ErrSlotNotFound
	}
	s := sp.readSlot(uint16(slotID))
	if s.Offset == 0 && s.Length == 0 {
		return nil, types.ErrSlotNotFound
	}
	if int(s.Offset)+int(s.Length) > len(sp.Data) {
		return nil, fmt.Errorf("%w: corrupted slot bounds", types.ErrInvalidPage)
	}
	return sp.Data[s.Offset : s.Offset+s.Length], nil
}

// Update replaces the payload for an existing slotID.
func (sp *SlottedPage) Update(slotID types.SlotID, payload []byte) error {
	if uint16(slotID) >= sp.Header.SlotCount {
		return types.ErrSlotNotFound
	}
	s := sp.readSlot(uint16(slotID))
	if s.Offset == 0 && s.Length == 0 {
		return types.ErrSlotNotFound
	}

	payLen := uint16(len(payload))
	if payLen <= s.Length {
		copy(sp.Data[s.Offset:s.Offset+payLen], payload)
		s.Length = payLen
		sp.writeSlot(uint16(slotID), s)
		sp.Dirty = true
		sp.WriteHeader()
		return nil
	}

	if sp.FreeSpace() < payLen {
		sp.Compact()
		if sp.FreeSpace() < payLen {
			return types.ErrPageFull
		}
	}

	newFreeEnd := sp.Header.FreeEnd - payLen
	copy(sp.Data[newFreeEnd:sp.Header.FreeEnd], payload)
	sp.Header.FreeEnd = newFreeEnd
	sp.writeSlot(uint16(slotID), Slot{Offset: newFreeEnd, Length: payLen})
	sp.Dirty = true
	sp.WriteHeader()
	return nil
}

// Delete marks a slot as unused.
func (sp *SlottedPage) Delete(slotID types.SlotID) error {
	if uint16(slotID) >= sp.Header.SlotCount {
		return types.ErrSlotNotFound
	}
	s := sp.readSlot(uint16(slotID))
	if s.Offset == 0 && s.Length == 0 {
		return types.ErrSlotNotFound
	}
	sp.writeSlot(uint16(slotID), Slot{Offset: 0, Length: 0})

	if uint16(slotID) == sp.Header.SlotCount-1 {
		for sp.Header.SlotCount > 0 {
			last := sp.readSlot(sp.Header.SlotCount - 1)
			if last.Offset == 0 && last.Length == 0 {
				sp.Header.SlotCount--
				sp.Header.FreeStart = SlotDirectoryStart + sp.Header.SlotCount*SlotSize
			} else {
				break
			}
		}
	}
	sp.Dirty = true
	sp.WriteHeader()
	return nil
}

type activeSlot struct {
	id     uint16
	offset uint16
	length uint16
}

// Compact defragments page data by repacking active records towards the bottom of the page.
func (sp *SlottedPage) Compact() {
	var active []activeSlot
	for i := uint16(0); i < sp.Header.SlotCount; i++ {
		s := sp.readSlot(i)
		if s.Offset != 0 && s.Length != 0 {
			active = append(active, activeSlot{
				id:     i,
				offset: s.Offset,
				length: s.Length,
			})
		}
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].offset < active[j].offset
	})

	pageSize := uint16(len(sp.Data))
	temp := make([]byte, pageSize)
	copy(temp[:SlotDirectoryStart+sp.Header.SlotCount*SlotSize], sp.Data[:SlotDirectoryStart+sp.Header.SlotCount*SlotSize])

	curEnd := pageSize
	for i := len(active) - 1; i >= 0; i-- {
		item := active[i]
		curEnd -= item.length
		copy(temp[curEnd:curEnd+item.length], sp.Data[item.offset:item.offset+item.length])
		off := int(SlotDirectoryStart + item.id*SlotSize)
		binary.BigEndian.PutUint16(temp[off:off+2], curEnd)
		binary.BigEndian.PutUint16(temp[off+2:off+4], item.length)
	}

	copy(sp.Data, temp)
	sp.Header.FreeEnd = curEnd
	sp.Header.FreeStart = SlotDirectoryStart + sp.Header.SlotCount*SlotSize
	sp.Dirty = true
	sp.WriteHeader()
}
