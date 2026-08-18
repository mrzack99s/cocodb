package txn

import (
	"fmt"
	"sync"

	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
)

// SavepointState captures modified page copies at a savepoint.
type SavepointState struct {
	Name       string
	PageCopies map[types.PageID][]byte
}

// Transaction represents an active database transaction.
type Transaction struct {
	mu           sync.Mutex
	manager      *TxnManager
	readOnly     bool
	snapshotTxn  types.TxnID
	txnID        types.TxnID
	dirtyPages   map[types.PageID]*storage.Page
	savepoints   map[string]SavepointState
	isWriterHeld bool
	open         bool
}

// ID returns the transaction ID.
func (tx *Transaction) ID() types.TxnID {
	return tx.txnID
}

// SnapshotID returns the snapshot TxnID used for visibility checks.
func (tx *Transaction) SnapshotID() types.TxnID {
	return tx.snapshotTxn
}

// ReadOnly returns whether this transaction is read-only.
func (tx *Transaction) ReadOnly() bool {
	return tx.readOnly
}

// Commit commits the transaction.
func (tx *Transaction) Commit() error {
	return tx.manager.Commit(tx)
}

// Rollback rolls back the transaction.
func (tx *Transaction) Rollback() error {
	return tx.manager.Rollback(tx)
}

// TrackDirty registers a modified page in this transaction.
func (tx *Transaction) TrackDirty(p *storage.Page) error {
	if tx.readOnly {
		return ErrTxnReadOnly
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.open {
		return ErrTxnClosed
	}
	tx.dirtyPages[p.Header.ID] = p
	return nil
}

// Savepoint establishes a named savepoint.
func (tx *Transaction) Savepoint(name string) error {
	if tx.readOnly {
		return ErrTxnReadOnly
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.open {
		return ErrTxnClosed
	}

	copies := make(map[types.PageID][]byte, len(tx.dirtyPages))
	for id, p := range tx.dirtyPages {
		c := make([]byte, len(p.Data))
		copy(c, p.Data)
		copies[id] = c
	}
	tx.savepoints[name] = SavepointState{
		Name:       name,
		PageCopies: copies,
	}
	return nil
}

// RollbackTo rolls back all mutations made since the named savepoint.
func (tx *Transaction) RollbackTo(name string) error {
	if tx.readOnly {
		return ErrTxnReadOnly
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if !tx.open {
		return ErrTxnClosed
	}

	sp, ok := tx.savepoints[name]
	if !ok {
		return fmt.Errorf("savepoint %q not found", name)
	}

	// Restore pages to snapshot state or remove new dirty pages
	for id, p := range tx.dirtyPages {
		if savedData, found := sp.PageCopies[id]; found {
			copy(p.Data, savedData)
			p.ReadHeader()
		} else {
			// Page was modified/allocated after savepoint: remove from dirty
			delete(tx.dirtyPages, id)
			tx.manager.pager.Cache().Remove(id)
		}
	}
	return nil
}
