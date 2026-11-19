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

// journaledPager is implemented by pagers that can undo a write transaction's page
// changes (storage.DefaultPager).
type journaledPager interface {
	BeginJournal()
	ChangedPages() []*storage.Page
	CommitJournal()
	RollbackJournal()
}

// Checkpoint runs fn while no write transaction is open. A checkpoint flushes cached
// pages and truncates the WAL; doing that in the middle of a write transaction would
// put uncommitted pages in the file and could cut the transaction's records out of the
// log before its commit record is written.
func (tm *TxnManager) Checkpoint(fn func() error) error {
	tm.writeLock.Lock()
	defer tm.writeLock.Unlock()
	return fn()
}

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

	if jp, ok := tm.pager.(journaledPager); ok {
		jp.BeginJournal()
	}

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
		if jp, ok := tm.pager.(journaledPager); ok {
			jp.CommitJournal()
		}
		if tx.isWriterHeld {
			tx.isWriterHeld = false
			tm.writeLock.Unlock()
		}
		tm.readerTable.Deregister(tx.snapshotTxn)
	}()

	// 1. Write PageUpdate records to WAL for every page the transaction changed: the
	// record pages it tracked, plus the B+Tree, catalog, directory and free-list pages
	// the undo journal saw change. Logging only the tracked pages lost every commit made
	// since the last checkpoint when the process died, because the index pages that point
	// at the new records never reached the WAL.
	pages := make(map[types.PageID]*storage.Page, len(tx.dirtyPages))
	for id, page := range tx.dirtyPages {
		pages[id] = page
	}
	jp, journaled := tm.pager.(journaledPager)
	if journaled {
		for _, page := range jp.ChangedPages() {
			pages[page.Header.ID] = page
		}
	}
	var lastLSN types.LSN
	for pageID, page := range pages {
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

	// The meta-page roots that lead to those pages. They are otherwise written only at a
	// checkpoint, so recovery needs them from the log.
	if journaled {
		m := tm.pager.Meta()
		metaRec := wal.NewMetaRecord(tx.txnID, wal.MetaRedo{
			CatalogRoot:   m.CatalogRoot,
			RecordDirRoot: m.RecordDirRoot,
			FreeListRoot:  m.FreeListRoot,
			NextPageID:    m.NextPageID,
			NextRecordID:  m.NextRecordID,
		})
		if _, err := tm.wal.Append(metaRec); err != nil {
			return fmt.Errorf("wal meta record append failed: %w", err)
		}
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

	// Undo every page the transaction changed, including B+Tree, catalog and free-list
	// pages that it did not track. Evicting only the tracked record pages left those
	// changes in the cache, where they hid committed data and later reached the file.
	if jp, ok := tm.pager.(journaledPager); ok {
		jp.RollbackJournal()
		return nil
	}
	for pageID := range tx.dirtyPages {
		tm.pager.Cache().Remove(pageID)
	}

	return nil
}
