package record

import (
	"encoding/binary"

	"github.com/mrzack99s/cocodb/internal/btree"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
)

// Location represents the physical page and slot of a record.
type Location struct {
	Page types.PageID
	Slot types.SlotID
}

// Directory manages the RecordID -> Location index backed by a B+Tree.
type Directory struct {
	tree  *btree.BTree
	pager storage.Pager
}

// NewDirectory creates or opens the Record Directory.
func NewDirectory(pager storage.Pager, root types.PageID) *Directory {
	return &Directory{
		tree:  btree.NewBTree(pager, root),
		pager: pager,
	}
}

// Root returns the root PageID of the directory B+Tree.
func (d *Directory) Root() types.PageID {
	return d.tree.Root()
}

// Put records or updates the physical location for recordID.
func (d *Directory) Put(id types.RecordID, loc Location) error {
	var kBuf [9]byte
	kBuf[0] = btree.TagUint64
	binary.BigEndian.PutUint64(kBuf[1:9], uint64(id))

	var valBuf [10]byte
	binary.BigEndian.PutUint64(valBuf[0:8], uint64(loc.Page))
	binary.BigEndian.PutUint16(valBuf[8:10], uint16(loc.Slot))

	if err := d.tree.Insert(kBuf[:], valBuf[:]); err != nil {
		return err
	}
	d.pager.Meta().RecordDirRoot = d.tree.Root()
	return nil
}

// Get finds the physical location for recordID.
func (d *Directory) Get(id types.RecordID) (Location, bool, error) {
	var kBuf [9]byte
	kBuf[0] = btree.TagUint64
	binary.BigEndian.PutUint64(kBuf[1:9], uint64(id))
	val, found, err := d.tree.Search(kBuf[:])
	if err != nil || !found || len(val) < 10 {
		return Location{}, false, err
	}
	loc := Location{
		Page: types.PageID(binary.BigEndian.Uint64(val[0:8])),
		Slot: types.SlotID(binary.BigEndian.Uint16(val[8:10])),
	}
	return loc, true, nil
}

// Delete removes the mapping for recordID.
func (d *Directory) Delete(id types.RecordID) error {
	var kBuf [9]byte
	kBuf[0] = btree.TagUint64
	binary.BigEndian.PutUint64(kBuf[1:9], uint64(id))
	if err := d.tree.Delete(kBuf[:]); err != nil {
		return err
	}
	d.pager.Meta().RecordDirRoot = d.tree.Root()
	return nil
}
