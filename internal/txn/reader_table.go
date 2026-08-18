package txn

import (
	"sync"

	"github.com/mrzack99s/cocodb/internal/types"
)

// ReaderTable tracks all currently active read snapshots.
type ReaderTable struct {
	mu      sync.RWMutex
	readers map[types.TxnID]int
}

func NewReaderTable() *ReaderTable {
	return &ReaderTable{
		readers: make(map[types.TxnID]int),
	}
}

// Register adds an active snapshot reader.
func (rt *ReaderTable) Register(snapshotTxn types.TxnID) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.readers[snapshotTxn]++
}

// Deregister removes an active snapshot reader.
func (rt *ReaderTable) Deregister(snapshotTxn types.TxnID) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if cnt, ok := rt.readers[snapshotTxn]; ok {
		if cnt <= 1 {
			delete(rt.readers, snapshotTxn)
		} else {
			rt.readers[snapshotTxn] = cnt - 1
		}
	}
}

// OldestActiveSnapshot returns the lowest active snapshot TxnID, or fallback if none.
func (rt *ReaderTable) OldestActiveSnapshot(fallback types.TxnID) types.TxnID {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if len(rt.readers) == 0 {
		return fallback
	}

	var oldest types.TxnID = types.InvalidTxnID
	for txID := range rt.readers {
		if oldest == types.InvalidTxnID || txID < oldest {
			oldest = txID
		}
	}
	return oldest
}
