package btree

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
)

const (
	LeafHeaderOffset = storage.PageHeaderSize
	LeafPrevOffset   = LeafHeaderOffset     // 8 bytes (offset 32)
	LeafNextOffset   = LeafHeaderOffset + 8 // 8 bytes (offset 40)

	InternalHeaderOffset = storage.PageHeaderSize
	InternalRightOffset  = InternalHeaderOffset // 8 bytes (offset 32)

	MaxKeySize   = 2048
	MaxValueSize = 4096
)

// LeafItem holds a key-value pair extracted from a leaf page.
type LeafItem struct {
	Key   []byte
	Value []byte
}

// LeafNode wraps a PageBTreeLeaf.
type LeafNode struct {
	*storage.Page
}

func WrapLeaf(p *storage.Page) *LeafNode {
	return &LeafNode{Page: p}
}

func (l *LeafNode) PrevLeaf() types.PageID {
	return types.PageID(binary.BigEndian.Uint64(l.Data[LeafPrevOffset : LeafPrevOffset+8]))
}

func (l *LeafNode) SetPrevLeaf(id types.PageID) {
	binary.BigEndian.PutUint64(l.Data[LeafPrevOffset:LeafPrevOffset+8], uint64(id))
	l.Dirty = true
}

func (l *LeafNode) NextLeaf() types.PageID {
	return types.PageID(binary.BigEndian.Uint64(l.Data[LeafNextOffset : LeafNextOffset+8]))
}

func (l *LeafNode) SetNextLeaf(id types.PageID) {
	binary.BigEndian.PutUint64(l.Data[LeafNextOffset:LeafNextOffset+8], uint64(id))
	l.Dirty = true
}

// KeyAt returns the key slice stored in slotID directly from the page buffer without heap allocations.
func (l *LeafNode) KeyAt(slotID types.SlotID) []byte {
	sp := storage.WrapSlotted(l.Page)
	payload, err := sp.GetDirect(slotID)
	if err != nil || len(payload) < 4 {
		return nil
	}
	kLen := binary.BigEndian.Uint16(payload[0:2])
	if int(4+kLen) > len(payload) {
		return nil
	}
	return payload[4 : 4+kLen]
}

// ItemAt returns the LeafItem stored in slotID directly.
func (l *LeafNode) ItemAt(slotID types.SlotID) (LeafItem, bool) {
	sp := storage.WrapSlotted(l.Page)
	payload, err := sp.GetDirect(slotID)
	if err != nil || len(payload) < 4 {
		return LeafItem{}, false
	}
	kLen := binary.BigEndian.Uint16(payload[0:2])
	vLen := binary.BigEndian.Uint16(payload[2:4])
	if int(4+kLen+vLen) > len(payload) {
		return LeafItem{}, false
	}
	return LeafItem{
		Key:   payload[4 : 4+kLen],
		Value: payload[4+kLen : 4+kLen+vLen],
	}, true
}

// Items parses and returns all LeafItems from the leaf node.
// Uses copying reads since items may be passed to SetItems which overwrites the page.
func (l *LeafNode) Items() []LeafItem {
	sp := storage.WrapSlotted(l.Page)
	cnt := sp.SlotCount()
	items := make([]LeafItem, 0, cnt)
	for i := uint16(0); i < cnt; i++ {
		payload, err := sp.Get(types.SlotID(i))
		if err != nil || len(payload) < 4 {
			continue
		}
		kLen := binary.BigEndian.Uint16(payload[0:2])
		vLen := binary.BigEndian.Uint16(payload[2:4])
		if int(4+kLen+vLen) > len(payload) {
			continue
		}
		items = append(items, LeafItem{
			Key:   payload[4 : 4+kLen],
			Value: payload[4+kLen : 4+kLen+vLen],
		})
	}
	return items
}

// SetItems repacks the leaf page with the given sorted items.
func (l *LeafNode) SetItems(items []LeafItem) error {
	prev := l.PrevLeaf()
	next := l.NextLeaf()
	id := l.Header.ID

	for i := range l.Data {
		l.Data[i] = 0
	}
	l.Header = storage.PageHeader{
		ID:        id,
		Type:      storage.PageBTreeLeaf,
		SlotCount: 0,
		FreeStart: storage.SlotDirectoryStart,
		FreeEnd:   uint16(types.DefaultPageSize),
	}
	l.WriteHeader()
	l.SetPrevLeaf(prev)
	l.SetNextLeaf(next)

	sp := storage.WrapSlotted(l.Page)
	for idx, item := range items {
		payload := make([]byte, 4+len(item.Key)+len(item.Value))
		binary.BigEndian.PutUint16(payload[0:2], uint16(len(item.Key)))
		binary.BigEndian.PutUint16(payload[2:4], uint16(len(item.Value)))
		copy(payload[4:], item.Key)
		copy(payload[4+len(item.Key):], item.Value)

		slotID, err := sp.Insert(payload)
		if err != nil {
			return err
		}
		if slotID != types.SlotID(idx) {
			return fmt.Errorf("unexpected slot id assignment in leaf: got %d want %d", slotID, idx)
		}
	}
	l.Dirty = true
	return nil
}

// BinarySearch searches for key in the leaf node with zero heap allocation.
func (l *LeafNode) BinarySearch(key []byte) (int, bool) {
	sp := storage.WrapSlotted(l.Page)
	cnt := int(sp.SlotCount())
	low := 0
	high := cnt - 1
	for low <= high {
		mid := (low + high) / 2
		midKey := l.KeyAt(types.SlotID(mid))
		cmp := bytes.Compare(key, midKey)
		if cmp == 0 {
			return mid, true
		} else if cmp < 0 {
			high = mid - 1
		} else {
			low = mid + 1
		}
	}
	return low, false
}

// InternalItem holds (ChildPageID, Key) where ChildPageID contains keys < Key.
type InternalItem struct {
	Child types.PageID
	Key   []byte
}

// InternalNode wraps a PageBTreeInternal.
type InternalNode struct {
	*storage.Page
}

func WrapInternal(p *storage.Page) *InternalNode {
	return &InternalNode{Page: p}
}

func (in *InternalNode) RightmostChild() types.PageID {
	return types.PageID(binary.BigEndian.Uint64(in.Data[InternalRightOffset : InternalRightOffset+8]))
}

func (in *InternalNode) SetRightmostChild(id types.PageID) {
	binary.BigEndian.PutUint64(in.Data[InternalRightOffset:InternalRightOffset+8], uint64(id))
	in.Dirty = true
}

// ChildAndKeyAt returns child PageID and Key at slotID directly without heap allocations.
func (in *InternalNode) ChildAndKeyAt(slotID types.SlotID) (types.PageID, []byte, bool) {
	sp := storage.WrapSlotted(in.Page)
	payload, err := sp.GetDirect(slotID)
	if err != nil || len(payload) < 10 {
		return types.InvalidPageID, nil, false
	}
	child := types.PageID(binary.BigEndian.Uint64(payload[0:8]))
	kLen := binary.BigEndian.Uint16(payload[8:10])
	if int(10+kLen) > len(payload) {
		return types.InvalidPageID, nil, false
	}
	return child, payload[10 : 10+kLen], true
}

// Items parses and returns all InternalItems from the internal node.
// Uses copying reads since items may be passed to SetItems which overwrites the page.
func (in *InternalNode) Items() []InternalItem {
	sp := storage.WrapSlotted(in.Page)
	cnt := sp.SlotCount()
	items := make([]InternalItem, 0, cnt)
	for i := uint16(0); i < cnt; i++ {
		payload, err := sp.Get(types.SlotID(i))
		if err != nil || len(payload) < 10 {
			continue
		}
		child := types.PageID(binary.BigEndian.Uint64(payload[0:8]))
		kLen := binary.BigEndian.Uint16(payload[8:10])
		if int(10+kLen) > len(payload) {
			continue
		}
		items = append(items, InternalItem{
			Child: child,
			Key:   payload[10 : 10+kLen],
		})
	}
	return items
}

// SetItems repacks the internal node with given items and rightmost child.
func (in *InternalNode) SetItems(items []InternalItem, rightmost types.PageID) error {
	id := in.Header.ID
	for i := range in.Data {
		in.Data[i] = 0
	}
	in.Header = storage.PageHeader{
		ID:        id,
		Type:      storage.PageBTreeInternal,
		SlotCount: 0,
		FreeStart: storage.SlotDirectoryStart,
		FreeEnd:   uint16(types.DefaultPageSize),
	}
	in.WriteHeader()
	in.SetRightmostChild(rightmost)

	sp := storage.WrapSlotted(in.Page)
	for idx, item := range items {
		payload := make([]byte, 10+len(item.Key))
		binary.BigEndian.PutUint64(payload[0:8], uint64(item.Child))
		binary.BigEndian.PutUint16(payload[8:10], uint16(len(item.Key)))
		copy(payload[10:], item.Key)

		slotID, err := sp.Insert(payload)
		if err != nil {
			return err
		}
		if slotID != types.SlotID(idx) {
			return fmt.Errorf("unexpected slot id assignment in internal node: got %d want %d", slotID, idx)
		}
	}
	in.Dirty = true
	return nil
}

// RouteChild finds the child PageID that may contain the target key without heap allocations.
func (in *InternalNode) RouteChild(key []byte) types.PageID {
	sp := storage.WrapSlotted(in.Page)
	cnt := int(sp.SlotCount())
	if cnt == 0 {
		return in.RightmostChild()
	}
	// Binary search for the first slot where key < slotKey
	low, high := 0, cnt
	for low < high {
		mid := (low + high) / 2
		_, midKey, ok := in.ChildAndKeyAt(types.SlotID(mid))
		if !ok {
			return in.RightmostChild()
		}
		if bytes.Compare(key, midKey) < 0 {
			high = mid
		} else {
			low = mid + 1
		}
	}
	// low is the first slot where key < slotKey
	if low < cnt {
		child, _, ok := in.ChildAndKeyAt(types.SlotID(low))
		if ok {
			return child
		}
	}
	return in.RightmostChild()
}
