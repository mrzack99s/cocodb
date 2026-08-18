package wal

import (
	"cocodb/internal/storage"
	"cocodb/internal/types"
)

// RecoveryResult stores statistics from a crash recovery run.
type RecoveryResult struct {
	RecordsScanned int
	TxnsCommitted  int
	PagesRedone    int
	LastLSN        types.LSN
}

// Recover scans the WAL, identifies committed transactions, redoes page changes, and checkpoints.
func Recover(walManager *WAL, pager storage.Pager) (*RecoveryResult, error) {
	records, err := walManager.ReadAll()
	if err != nil {
		return nil, err
	}

	result := &RecoveryResult{
		RecordsScanned: len(records),
	}

	if len(records) == 0 {
		return result, nil
	}

	// Phase 1: Identify all committed transactions
	committedTxns := make(map[types.TxnID]bool)
	var maxLSN types.LSN

	for _, rec := range records {
		if rec.Header.LSN > maxLSN {
			maxLSN = rec.Header.LSN
		}
		if rec.Header.Type == RecordTxnCommit {
			committedTxns[rec.Header.TxnID] = true
		}
	}
	result.TxnsCommitted = len(committedTxns)
	result.LastLSN = maxLSN

	// Phase 2: Redo committed updates in LSN order
	for _, rec := range records {
		if rec.Header.Type == RecordPageUpdate {
			if committedTxns[rec.Header.TxnID] {
				if len(rec.PageData) == int(types.DefaultPageSize) {
					// Write raw page directly to backend
					offset := int64(rec.PageID) * int64(types.DefaultPageSize)
					if _, err := pager.Backend().WriteAt(rec.PageData, offset); err == nil {
						if rec.PageID >= pager.Meta().NextPageID {
							pager.Meta().NextPageID = rec.PageID + 1
						}
						// Refresh cache entry
						pager.Cache().Remove(rec.PageID)
						page, err := pager.Get(rec.PageID)
						if err == nil {
							pager.MarkDirty(page)
							result.PagesRedone++
						}
					}
				}
			}
		}
	}

	// Phase 3: Flush redone pages and checkpoint
	if err := pager.FlushAll(); err != nil {
		return nil, err
	}

	meta := pager.Meta()
	meta.LastLSN = maxLSN
	meta.LastCheckpointLSN = maxLSN
	if err := pager.SaveMeta(); err != nil {
		return nil, err
	}

	// Truncate WAL after successful recovery
	if err := walManager.Truncate(); err != nil {
		return nil, err
	}

	return result, nil
}
