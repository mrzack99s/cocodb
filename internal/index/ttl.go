package index

import (
	"bytes"
	"sync"
	"time"

	"cocodb/internal/btree"
	"cocodb/internal/storage"
	"cocodb/internal/types"
)

// TTLIndex manages time-to-live expirations using an ordered B+Tree.
type TTLIndex struct {
	mu    sync.RWMutex
	tree  *btree.BTree
	pager storage.Pager
}

func NewTTLIndex(pager storage.Pager, root types.PageID) *TTLIndex {
	return &TTLIndex{
		tree:  btree.NewBTree(pager, root),
		pager: pager,
	}
}

func (idx *TTLIndex) Root() types.PageID {
	return idx.tree.Root()
}

func (idx *TTLIndex) makeKey(expireAt time.Time, objectID types.ObjectID, keyOrID []byte) []byte {
	timeKey := btree.EncodeTime(expireAt)
	objKey := btree.EncodeUint64(uint64(objectID))
	return btree.EncodeComposite(timeKey, objKey, keyOrID)
}

// Add schedules an expiration.
func (idx *TTLIndex) Add(expireAt time.Time, objectID types.ObjectID, keyOrID []byte) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	k := idx.makeKey(expireAt, objectID, keyOrID)
	return idx.tree.Insert(k, []byte{1})
}

// Remove cancels an expiration.
func (idx *TTLIndex) Remove(expireAt time.Time, objectID types.ObjectID, keyOrID []byte) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	k := idx.makeKey(expireAt, objectID, keyOrID)
	return idx.tree.Delete(k)
}

// ExpiredItem holds an expired key entry.
type ExpiredItem struct {
	ObjectID types.ObjectID
	KeyOrID  []byte
	IndexKey []byte
}

// FindExpired returns all items that have expired by time `now` up to `limit`.
func (idx *TTLIndex) FindExpired(now time.Time, limit int) ([]ExpiredItem, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.tree.Root() == types.InvalidPageID {
		return nil, nil
	}

	cur := btree.NewCursor(idx.tree)
	defer cur.Close()

	if !cur.First() {
		return nil, cur.Err()
	}

	nowKey := btree.EncodeTime(now)
	var expired []ExpiredItem

	for cur.Valid() && len(expired) < limit {
		k := cur.Key()
		// Compare first 9 bytes (TagTime + 8 bytes)
		if len(k) < 18 {
			if !cur.Next() {
				break
			}
			continue
		}

		timePart := k[:9]
		if bytes.Compare(timePart, nowKey) > 0 {
			break // All subsequent items expire in the future
		}

		objID, err := btree.DecodeUint64(k[9:18])
		if err == nil {
			keyOrID := k[18:]
			expired = append(expired, ExpiredItem{
				ObjectID: types.ObjectID(objID),
				KeyOrID:  keyOrID,
				IndexKey: k,
			})
		}

		if !cur.Next() {
			break
		}
	}

	return expired, cur.Err()
}
