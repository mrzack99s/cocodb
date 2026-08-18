package index

import (
	"encoding/binary"

	"cocodb/internal/btree"
	"cocodb/internal/storage"
	"cocodb/internal/types"
)

// PrimaryIndex maps document _id strings to RecordIDs using a B+Tree.
type PrimaryIndex struct {
	tree  *btree.BTree
	pager storage.Pager
}

func NewPrimaryIndex(pager storage.Pager, root types.PageID) *PrimaryIndex {
	return &PrimaryIndex{
		tree:  btree.NewBTree(pager, root),
		pager: pager,
	}
}

func (pi *PrimaryIndex) Root() types.PageID {
	return pi.tree.Root()
}

func (pi *PrimaryIndex) Put(id string, recID types.RecordID) error {
	k := btree.EncodeString(id)
	v := make([]byte, 8)
	binary.BigEndian.PutUint64(v, uint64(recID))
	return pi.tree.Insert(k, v)
}

func (pi *PrimaryIndex) Get(id string) (types.RecordID, bool, error) {
	k := btree.EncodeString(id)
	v, found, err := pi.tree.Search(k)
	if err != nil || !found || len(v) < 8 {
		return types.InvalidRecordID, false, err
	}
	return types.RecordID(binary.BigEndian.Uint64(v)), true, nil
}

func (pi *PrimaryIndex) Delete(id string) error {
	k := btree.EncodeString(id)
	return pi.tree.Delete(k)
}

func (pi *PrimaryIndex) Tree() *btree.BTree {
	return pi.tree
}
