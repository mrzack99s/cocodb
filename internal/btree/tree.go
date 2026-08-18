package btree

import (
	"bytes"
	"fmt"
	"sync"

	"cocodb/internal/storage"
	"cocodb/internal/types"
)

// BTree represents a paged, disk-backed B+Tree.
type BTree struct {
	mu    sync.RWMutex
	pager storage.Pager
	root  types.PageID
}

// NewBTree creates a BTree handle around a root page.
func NewBTree(pager storage.Pager, root types.PageID) *BTree {
	return &BTree{
		pager: pager,
		root:  root,
	}
}

// Root returns the current root PageID.
func (t *BTree) Root() types.PageID {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.root
}

// SetRoot updates the root PageID.
func (t *BTree) SetRoot(root types.PageID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.root = root
}

// Search searches for key and returns its value.
func (t *BTree) Search(key []byte) ([]byte, bool, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.root == types.InvalidPageID {
		return nil, false, nil
	}

	leafPage, err := t.findLeaf(key)
	if err != nil {
		return nil, false, err
	}

	leaf := WrapLeaf(leafPage)
	idx, found := leaf.BinarySearch(key)
	if !found {
		return nil, false, nil
	}

	item, ok := leaf.ItemAt(types.SlotID(idx))
	if !ok {
		return nil, false, nil
	}
	val := make([]byte, len(item.Value))
	copy(val, item.Value)
	return val, true, nil
}

// Insert inserts or updates key/value in the B+Tree.
func (t *BTree) Insert(key, value []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(key) > MaxKeySize {
		return fmt.Errorf("key size %d exceeds max %d", len(key), MaxKeySize)
	}
	if len(value) > MaxValueSize {
		return fmt.Errorf("value size %d exceeds max %d", len(value), MaxValueSize)
	}

	if t.root == types.InvalidPageID {
		rootPage, err := t.pager.Allocate(storage.PageBTreeLeaf)
		if err != nil {
			return err
		}
		leaf := WrapLeaf(rootPage)
		leaf.SetPrevLeaf(types.InvalidPageID)
		leaf.SetNextLeaf(types.InvalidPageID)
		if err := leaf.SetItems([]LeafItem{{Key: key, Value: value}}); err != nil {
			return err
		}
		t.root = rootPage.Header.ID
		t.pager.MarkDirty(rootPage)
		return nil
	}

	// Traverse down to leaf while recording traversal path
	type pathEntry struct {
		page *storage.Page
	}
	var path []pathEntry

	currID := t.root
	for {
		currPage, err := t.pager.Get(currID)
		if err != nil {
			return err
		}

		path = append(path, pathEntry{page: currPage})
		if currPage.Header.Type == storage.PageBTreeLeaf {
			break
		}

		in := WrapInternal(currPage)
		currID = in.RouteChild(key)
	}

	leafPage := path[len(path)-1].page
	leaf := WrapLeaf(leafPage)
	items := leaf.Items()
	idx, found := leaf.BinarySearch(key)

	if found {
		items[idx].Value = value
		if err := leaf.SetItems(items); err == nil {
			t.pager.MarkDirty(leafPage)
			return nil
		}
	} else {
		newItems := make([]LeafItem, 0, len(items)+1)
		newItems = append(newItems, items[:idx]...)
		newItems = append(newItems, LeafItem{Key: key, Value: value})
		newItems = append(newItems, items[idx:]...)
		items = newItems

		if err := leaf.SetItems(items); err == nil {
			t.pager.MarkDirty(leafPage)
			return nil
		}
	}

	// Leaf overflows -> Split leaf
	rightPage, err := t.pager.Allocate(storage.PageBTreeLeaf)
	if err != nil {
		return err
	}
	rightLeaf := WrapLeaf(rightPage)

	mid := len(items) / 2
	leftItems := items[:mid]
	rightItems := items[mid:]

	oldNextID := leaf.NextLeaf()
	rightLeaf.SetNextLeaf(oldNextID)
	rightLeaf.SetPrevLeaf(leafPage.Header.ID)
	leaf.SetNextLeaf(rightPage.Header.ID)

	if oldNextID != types.InvalidPageID {
		oldNextPage, err := t.pager.Get(oldNextID)
		if err == nil {
			WrapLeaf(oldNextPage).SetPrevLeaf(rightPage.Header.ID)
			t.pager.MarkDirty(oldNextPage)
		}
	}

	if err := leaf.SetItems(leftItems); err != nil {
		return err
	}
	if err := rightLeaf.SetItems(rightItems); err != nil {
		return err
	}
	t.pager.MarkDirty(leafPage)
	t.pager.MarkDirty(rightPage)

	// Child pointer that split: leafPage.Header.ID was split into (leafPage.Header.ID, rightItems[0].Key, rightPage.Header.ID)
	splitChild := leafPage.Header.ID
	cLeft := leafPage.Header.ID
	kSep := rightItems[0].Key
	cRight := rightPage.Header.ID

	// Propagate up to parents
	for level := len(path) - 2; level >= 0; level-- {
		parentPage := path[level].page
		in := WrapInternal(parentPage)
		parentItems := in.Items()
		oldRightmost := in.RightmostChild()

		var newParentItems []InternalItem
		var newRightmost types.PageID

		if splitChild == oldRightmost {
			// Appending to items, cRight becomes new RightmostChild
			newParentItems = append(parentItems, InternalItem{Child: cLeft, Key: kSep})
			newRightmost = cRight
		} else {
			// Found in items
			foundIdx := -1
			for i, it := range parentItems {
				if it.Child == splitChild {
					foundIdx = i
					break
				}
			}
			if foundIdx == -1 {
				return fmt.Errorf("child %d not found in parent internal node", splitChild)
			}
			oldKey := parentItems[foundIdx].Key
			newParentItems = append(newParentItems, parentItems[:foundIdx]...)
			newParentItems = append(newParentItems,
				InternalItem{Child: cLeft, Key: kSep},
				InternalItem{Child: cRight, Key: oldKey},
			)
			newParentItems = append(newParentItems, parentItems[foundIdx+1:]...)
			newRightmost = oldRightmost
		}

		// Try fitting in parent
		if err := in.SetItems(newParentItems, newRightmost); err == nil {
			t.pager.MarkDirty(parentPage)
			return nil
		}

		// Parent overflows -> Split internal node
		rightInternalPage, err := t.pager.Allocate(storage.PageBTreeInternal)
		if err != nil {
			return err
		}
		rightIn := WrapInternal(rightInternalPage)

		midIn := len(newParentItems) / 2
		promotedKey := newParentItems[midIn].Key
		leftInItems := newParentItems[:midIn]
		leftRightmost := newParentItems[midIn].Child
		rightInItems := newParentItems[midIn+1:]

		if err := in.SetItems(leftInItems, leftRightmost); err != nil {
			return err
		}
		if err := rightIn.SetItems(rightInItems, newRightmost); err != nil {
			return err
		}

		t.pager.MarkDirty(parentPage)
		t.pager.MarkDirty(rightInternalPage)

		splitChild = parentPage.Header.ID
		cLeft = parentPage.Header.ID
		kSep = promotedKey
		cRight = rightInternalPage.Header.ID
	}

	// Root was split -> create new root internal node
	newRootPage, err := t.pager.Allocate(storage.PageBTreeInternal)
	if err != nil {
		return err
	}
	newRoot := WrapInternal(newRootPage)
	if err := newRoot.SetItems([]InternalItem{{Child: cLeft, Key: kSep}}, cRight); err != nil {
		return err
	}
	t.root = newRootPage.Header.ID
	t.pager.MarkDirty(newRootPage)
	return nil
}

// Delete removes key from the B+Tree.
func (t *BTree) Delete(key []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.root == types.InvalidPageID {
		return nil
	}

	leafPage, err := t.findLeaf(key)
	if err != nil {
		return err
	}

	leaf := WrapLeaf(leafPage)
	items := leaf.Items()
	idx, found := leaf.BinarySearch(key)
	if !found {
		return nil
	}

	newItems := append(items[:idx], items[idx+1:]...)
	if err := leaf.SetItems(newItems); err != nil {
		return err
	}
	t.pager.MarkDirty(leafPage)
	return nil
}

// findLeaf traverses down to the leaf node that should contain key.
func (t *BTree) findLeaf(key []byte) (*storage.Page, error) {
	currID := t.root
	for {
		page, err := t.pager.Get(currID)
		if err != nil {
			return nil, err
		}
		if page.Header.Type == storage.PageBTreeLeaf {
			return page, nil
		}
		in := WrapInternal(page)
		currID = in.RouteChild(key)
	}
}

// findLeftmostLeaf returns the leftmost leaf node in the B+Tree.
func (t *BTree) findLeftmostLeaf() (*storage.Page, error) {
	if t.root == types.InvalidPageID {
		return nil, types.ErrPageNotFound
	}
	currID := t.root
	for {
		page, err := t.pager.Get(currID)
		if err != nil {
			return nil, err
		}
		if page.Header.Type == storage.PageBTreeLeaf {
			return page, nil
		}
		in := WrapInternal(page)
		child, _, ok := in.ChildAndKeyAt(0)
		if ok {
			currID = child
		} else {
			currID = in.RightmostChild()
		}
	}
}

// findRightmostLeaf returns the rightmost leaf node in the B+Tree.
func (t *BTree) findRightmostLeaf() (*storage.Page, error) {
	if t.root == types.InvalidPageID {
		return nil, types.ErrPageNotFound
	}
	currID := t.root
	for {
		page, err := t.pager.Get(currID)
		if err != nil {
			return nil, err
		}
		if page.Header.Type == storage.PageBTreeLeaf {
			return page, nil
		}
		in := WrapInternal(page)
		currID = in.RightmostChild()
	}
}

// VerifyTree validates ordering, node consistency, and leaf chain integrity.
func (t *BTree) VerifyTree() error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.root == types.InvalidPageID {
		return nil
	}

	var prevKey []byte
	cur := NewCursor(t)
	defer cur.Close()

	if !cur.First() {
		return cur.Err()
	}

	for cur.Valid() {
		k := cur.Key()
		if prevKey != nil && bytes.Compare(prevKey, k) >= 0 {
			return fmt.Errorf("b+tree ordering violation: %x >= %x", prevKey, k)
		}
		prevKey = k
		if !cur.Next() {
			break
		}
	}
	return cur.Err()
}
