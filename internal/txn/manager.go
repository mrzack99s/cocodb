package txn

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
	"github.com/mrzack99s/cocodb/internal/wal"
)

var (
	ErrTxnReadOnly = errors.New("coco/txn: transaction is read-only")
	ErrTxnClosed   = errors.New("coco/txn: transaction has already completed")
)

type SyncMode int

const (
	SyncFull SyncMode = iota
	SyncNormal
	SyncOff
)

// TxnManager coordinates transactions, MVCC visibility, and single-writer serialization.
type TxnManager struct {
	mu            sync.RWMutex
	writeLock     sync.Mutex
	pager         storage.Pager
	wal           *wal.WAL
	readerTable   *ReaderTable
	lastTxnID     atomic.Uint64
	committedTxns map[types.TxnID]bool
	syncMode      SyncMode
}

// NewTxnManager creates a new transaction manager.
func NewTxnManager(pager storage.Pager, walManager *wal.WAL, syncMode SyncMode) *TxnManager {
	meta := pager.Meta()
	tm := &TxnManager{
		pager:         pager,
		wal:           walManager,
		readerTable:   NewReaderTable(),
		committedTxns: make(map[types.TxnID]bool),
		syncMode:      syncMode,
	}
	tm.lastTxnID.Store(uint64(meta.LastTxnID))

	// Link WAL flush callback into Pager for WAL safety rule
	pager.SetWALFlusher(func(lsn types.LSN) error {
		return walManager.Flush(lsn)
	})

	return tm
}

// ReaderTable returns the reader table.
func (tm *TxnManager) ReaderTable() *ReaderTable {
	return tm.readerTable
}

// LastCommittedTxnID returns the highest committed TxnID.
func (tm *TxnManager) LastCommittedTxnID() types.TxnID {
	return types.TxnID(tm.lastTxnID.Load())
}

// LastTxnID returns the latest assigned or committed TxnID.
func (tm *TxnManager) LastTxnID() types.TxnID {
	return tm.LastCommittedTxnID()
}

// IsCommitted checks whether a given TxnID has committed.
func (tm *TxnManager) IsCommitted(txID types.TxnID) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.committedTxns[txID] || txID <= tm.pager.Meta().LastTxnID
}

// Begin starts a new read-only or read-write transaction.
func (tm *TxnManager) Begin(readOnly bool) (*Transaction, error) {
	if readOnly {
		snapshot := types.TxnID(tm.lastTxnID.Load())

		tm.readerTable.Register(snapshot)
		return &Transaction{
			manager:     tm,
			readOnly:    true,
			snapshotTxn: snapshot,
			txnID:       snapshot,
			open:        true,
		}, nil
	}

	// Read-Write transaction requires acquiring writer lock
	tm.writeLock.Lock()

	newTxnID := types.TxnID(tm.lastTxnID.Add(1))
	snapshot := newTxnID - 1

	// Append TxnBegin to WAL
	_, _ = tm.wal.Append(&wal.Record{
		Header: wal.RecordHeader{
			Type:  wal.RecordTxnBegin,
			TxnID: newTxnID,
		},
	})

	tm.readerTable.Register(snapshot)

	return &Transaction{
		manager:      tm,
		readOnly:     false,
		snapshotTxn:  snapshot,
		txnID:        newTxnID,
		dirtyPages:   make(map[types.PageID]*storage.Page),
		savepoints:   make(map[string]SavepointState),
		isWriterHeld: true,
		open:         true,
	}, nil
}

// Commit commits the transaction.
func (tm *TxnManager) Commit(tx *Transaction) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if !tx.open {
		return ErrTxnClosed
	}
	tx.open = false

	if tx.readOnly {
		tm.readerTable.Deregister(tx.snapshotTxn)
		return nil
	}

	defer func() {
		if tx.isWriterHeld {
			tx.isWriterHeld = false
			tm.writeLock.Unlock()
		}
		tm.readerTable.Deregister(tx.snapshotTxn)
	}()

	// 1. Write PageUpdate records to WAL for each touched page
	var lastLSN types.LSN
	for pageID, page := range tx.dirtyPages {
		page.Seal()
		rec := wal.NewPageUpdateRecord(0, tx.txnID, pageID, page.Data)
		lsn, err := tm.wal.Append(rec)
		if err != nil {
			return fmt.Errorf("wal append page update failed: %w", err)
		}
		page.Header.LSN = lsn
		lastLSN = lsn
		tm.pager.MarkDirty(page)
	}

	// 2. Append TxnCommit record
	commitRec := wal.NewTxnCommitRecord(0, tx.txnID)
	commitLSN, err := tm.wal.Append(commitRec)
	if err != nil {
		return fmt.Errorf("wal commit record append failed: %w", err)
	}
	lastLSN = commitLSN

	// 3. Apply SyncMode
	switch tm.syncMode {
	case SyncFull:
		if err := tm.wal.Flush(lastLSN); err != nil {
			return err
		}
	case SyncNormal:
		if err := tm.wal.Sync(); err != nil {
			return err
		}
	case SyncOff:
		// Do not fsync
	}

	tm.mu.Lock()
	tm.committedTxns[tx.txnID] = true
	tm.pager.Meta().LastTxnID = tx.txnID
	tm.pager.Meta().LastLSN = lastLSN

	oldestReader := tm.readerTable.OldestActiveSnapshot(types.TxnID(tm.lastTxnID.Load()))
	for id := range tm.committedTxns {
		if id <= oldestReader {
			delete(tm.committedTxns, id)
		}
	}
	tm.mu.Unlock()

	return nil
}

// Rollback rolls back the transaction, discarding uncommitted mutations.
func (tm *TxnManager) Rollback(tx *Transaction) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if !tx.open {
		return nil
	}
	tx.open = false

	if tx.readOnly {
		tm.readerTable.Deregister(tx.snapshotTxn)
		return nil
	}

	defer func() {
		if tx.isWriterHeld {
			tx.isWriterHeld = false
			tm.writeLock.Unlock()
		}
		tm.readerTable.Deregister(tx.snapshotTxn)
	}()

	// Append TxnAbort to WAL
	abortRec := wal.NewTxnAbortRecord(0, tx.txnID)
	_, _ = tm.wal.Append(abortRec)

	// Invalidate / evict modified pages from cache so dirty uncommitted mutations vanish
	for pageID := range tx.dirtyPages {
		tm.pager.Cache().Remove(pageID)
	}

	return nil
}
