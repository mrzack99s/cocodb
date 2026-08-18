package btree

import (
	"bytes"

	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
)

// Cursor provides bidirectional range scanning over a B+Tree.
type Cursor struct {
	tree     *BTree
	currPage *storage.Page
	currLeaf *LeafNode
	items    []LeafItem
	idx      int
	valid    bool
	err      error
}

// NewCursor creates a new Cursor for the given BTree.
func NewCursor(tree *BTree) *Cursor {
	return &Cursor{
		tree:  tree,
		idx:   -1,
		valid: false,
	}
}

// Valid returns true if the cursor points to a valid item.
func (c *Cursor) Valid() bool {
	return c.valid && c.err == nil
}

// Err returns any error encountered during cursor movement.
func (c *Cursor) Err() error {
	return c.err
}

// Key returns a copy of the current key.
func (c *Cursor) Key() []byte {
	if !c.Valid() || c.idx < 0 || c.idx >= len(c.items) {
		return nil
	}
	k := make([]byte, len(c.items[c.idx].Key))
	copy(k, c.items[c.idx].Key)
	return k
}

// Value returns a copy of the current value.
func (c *Cursor) Value() []byte {
	if !c.Valid() || c.idx < 0 || c.idx >= len(c.items) {
		return nil
	}
	v := make([]byte, len(c.items[c.idx].Value))
	copy(v, c.items[c.idx].Value)
	return v
}

// First positions the cursor at the very first item in the B+Tree.
func (c *Cursor) First() bool {
	c.release()
	if c.tree == nil || c.tree.Root() == types.InvalidPageID {
		c.valid = false
		return false
	}
	leftmost, err := c.tree.findLeftmostLeaf()
	if err != nil {
		c.err = err
		c.valid = false
		return false
	}
	c.currPage = leftmost
	c.currLeaf = WrapLeaf(leftmost)
	c.items = c.currLeaf.Items()
	if len(c.items) == 0 {
		c.valid = false
		return false
	}
	c.idx = 0
	c.valid = true
	return true
}

// Last positions the cursor at the very last item in the B+Tree.
func (c *Cursor) Last() bool {
	c.release()
	if c.tree == nil || c.tree.Root() == types.InvalidPageID {
		c.valid = false
		return false
	}
	rightmost, err := c.tree.findRightmostLeaf()
	if err != nil {
		c.err = err
		c.valid = false
		return false
	}
	c.currPage = rightmost
	c.currLeaf = WrapLeaf(rightmost)
	c.items = c.currLeaf.Items()
	if len(c.items) == 0 {
		c.valid = false
		return false
	}
	c.idx = len(c.items) - 1
	c.valid = true
	return true
}

// Seek positions the cursor at the first item whose Key >= key.
func (c *Cursor) Seek(key []byte) bool {
	c.release()
	if c.tree == nil || c.tree.Root() == types.InvalidPageID {
		c.valid = false
		return false
	}
	leafPage, err := c.tree.findLeaf(key)
	if err != nil {
		c.err = err
		c.valid = false
		return false
	}
	c.currPage = leafPage
	c.currLeaf = WrapLeaf(leafPage)
	c.items = c.currLeaf.Items()

	idx, _ := c.currLeaf.BinarySearch(key)
	if idx < len(c.items) {
		c.idx = idx
		c.valid = true
		return true
	}

	// Key is greater than all items in this leaf, check next leaf
	if c.currLeaf.NextLeaf() != types.InvalidPageID {
		nextPage, err := c.tree.pager.Get(c.currLeaf.NextLeaf())
		if err != nil {
			c.err = err
			c.valid = false
			return false
		}
		c.currPage = nextPage
		c.currLeaf = WrapLeaf(nextPage)
		c.items = c.currLeaf.Items()
		if len(c.items) > 0 {
			c.idx = 0
			c.valid = true
			return true
		}
	}

	c.valid = false
	return false
}

// Next advances the cursor to the next item.
func (c *Cursor) Next() bool {
	if !c.valid {
		return false
	}
	c.idx++
	if c.idx < len(c.items) {
		return true
	}

	// Move to next leaf
	nextID := c.currLeaf.NextLeaf()
	if nextID == types.InvalidPageID {
		c.valid = false
		return false
	}

	nextPage, err := c.tree.pager.Get(nextID)
	if err != nil {
		c.err = err
		c.valid = false
		return false
	}
	c.currPage = nextPage
	c.currLeaf = WrapLeaf(nextPage)
	c.items = c.currLeaf.Items()
	if len(c.items) == 0 {
		c.valid = false
		return false
	}
	c.idx = 0
	return true
}

// Prev moves the cursor to the previous item.
func (c *Cursor) Prev() bool {
	if !c.valid {
		return false
	}
	c.idx--
	if c.idx >= 0 {
		return true
	}

	// Move to prev leaf
	prevID := c.currLeaf.PrevLeaf()
	if prevID == types.InvalidPageID {
		c.valid = false
		return false
	}

	prevPage, err := c.tree.pager.Get(prevID)
	if err != nil {
		c.err = err
		c.valid = false
		return false
	}
	c.currPage = prevPage
	c.currLeaf = WrapLeaf(prevPage)
	c.items = c.currLeaf.Items()
	if len(c.items) == 0 {
		c.valid = false
		return false
	}
	c.idx = len(c.items) - 1
	return true
}

func (c *Cursor) release() {
	c.currPage = nil
	c.currLeaf = nil
	c.items = nil
	c.idx = -1
	c.valid = false
	c.err = nil
}

// Close closes the cursor.
func (c *Cursor) Close() error {
	c.release()
	return nil
}

// PrefixScan iterates over all keys matching prefix.
func (c *Cursor) PrefixScan(prefix []byte, fn func(k, v []byte) bool) error {
	if !c.Seek(prefix) {
		return c.Err()
	}
	for c.Valid() {
		k := c.Key()
		if !bytes.HasPrefix(k, prefix) {
			break
		}
		if !fn(k, c.Value()) {
			break
		}
		if !c.Next() {
			break
		}
	}
	return c.Err()
}

// RangeScan iterates over keys in [start, end).
func (c *Cursor) RangeScan(start, end []byte, fn func(k, v []byte) bool) error {
	if start != nil {
		if !c.Seek(start) {
			return c.Err()
		}
	} else {
		if !c.First() {
			return c.Err()
		}
	}

	for c.Valid() {
		k := c.Key()
		if end != nil && bytes.Compare(k, end) >= 0 {
			break
		}
		if !fn(k, c.Value()) {
			break
		}
		if !c.Next() {
			break
		}
	}
	return c.Err()
}
